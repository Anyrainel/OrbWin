package localnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"slices"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

// RelayManager borrows the common ledger: it must close before its owner releases
// that ledger. It never owns rdd, workload containers, networks or trust stores.
// Calls serialize under mu, are bounded, and are cancelled by Close. Failed or
// unknown mutations retain immutable receipts and prevent another Start.
type RelayManager struct {
	mu        sync.Mutex
	cli       *client.Client
	ledger    *NamespaceLedger
	image     ToolsImage
	current   *relayRecord
	pending   *relayRecord // actual producer retains ambiguous create/failed acceptance
	closeErr  error
	portProbe func(ServicePorts) error
	closed    bool
	ctx       context.Context
	cancel    context.CancelFunc
}

func OpenRelayManager(ctx context.Context, cli *client.Client, ledger *NamespaceLedger, image ToolsImage) (*RelayManager, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if cli == nil || ledger == nil {
		return nil, errors.New("exclusive local-services owner and daemon client required")
	}
	receipt, err := ledger.committedImageReceipt(ctx, image.AssetSHA)
	if err != nil {
		return nil, err
	}
	if receipt.ID == "" || receipt.ID != image.ID || receipt.Token != image.Token {
		return nil, errors.New("relay image differs from committed import receipt")
	}
	if err = ledger.verifyDaemon(ctx, cli); err != nil {
		return nil, err
	}
	if err = verifyToolsImage(ctx, cli, image); err != nil {
		return nil, err
	}
	manager := &RelayManager{cli: cli, ledger: ledger, image: image, portProbe: probeRelayHostPorts}
	manager.ctx, manager.cancel = context.WithCancel(context.Background())
	if _, err = manager.Recover(ctx); err != nil {
		manager.cancel()
		return nil, err
	}
	return manager, nil
}

func (manager *RelayManager) operation(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	stop := context.AfterFunc(manager.ctx, cancel)
	return ctx, func() { stop(); cancel() }
}

// Recovery never resumes a stale gateway identity. It removes only exact proven
// receipts; a missing unknown create remains blocked against late acceptance.
func (manager *RelayManager) Recover(parent context.Context) (int, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed || manager.current != nil {
		return 0, errors.New("relay recovery requires an open idle controller")
	}
	ctx, done := manager.operation(parent)
	defer done()
	if err := manager.ledger.verifyDaemon(ctx, manager.cli); err != nil {
		return 0, err
	}
	records, err := manager.ledger.relayRecords()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, record := range records {
		if err = manager.remove(ctx, record); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
func (manager *RelayManager) remove(ctx context.Context, record relayRecord) error {
	if err := manager.ledger.verifyDaemon(ctx, manager.cli); err != nil {
		return err
	}
	lookup := record.ID
	if lookup == "" {
		lookup = record.Intent.name()
	}
	result, err := manager.cli.ContainerInspect(ctx, lookup, client.ContainerInspectOptions{})
	if cerrdefs.IsNotFound(err) {
		if record.ID == "" {
			return errors.New("relay create acceptance remains unknown; retain its receipt")
		}
		return manager.ledger.settleRelay(record)
	}
	if err != nil {
		return err
	}
	id := record.ID
	if id == "" {
		id = result.Container.ID
	}
	if err = verifyRelayObject(record.Intent, id, result.Container, false); err != nil {
		return err
	}
	if record.ID == "" {
		if err = manager.ledger.CreatedRelay(record.Intent, id); err != nil {
			return err
		}
		record.ID = id
	}
	// Daemon identity is revalidated after inspection and before any removal.
	if err = manager.ledger.verifyDaemon(ctx, manager.cli); err != nil {
		return err
	}
	if _, err = manager.cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true}); err != nil && !cerrdefs.IsNotFound(err) {
		return err
	}
	if result.Container.State.Running {
		if err = awaitRelayPublicationRelease(ctx, record.Intent.servicePorts(), manager.portProbe); err != nil {
			return err
		}
	}
	return manager.ledger.settleRelay(record)
}

// Docker deletion acknowledgement may precede the owned Windows publication
// watcher's listener release. Observe availability, never evict a listener or
// replay creation. On timeout the known-ID receipt remains recoverable.
func awaitRelayPublicationRelease(parent context.Context, ports ServicePorts, probe func(ServicePorts) error) error {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	for {
		err := probe(ports)
		if err == nil {
			return nil
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("relay removed but its previous publication is not yet available: %w", errors.Join(ctx.Err(), err))
		case <-timer.C:
		}
	}
}

func (manager *RelayManager) Start(parent context.Context, descriptor GatewayDescriptor, networks []RelayNetwork) (string, error) {
	return manager.StartWithPorts(parent, descriptor, networks, DefaultServicePorts())
}
func (manager *RelayManager) StartWithPorts(parent context.Context, descriptor GatewayDescriptor, networks []RelayNetwork, ports ServicePorts) (string, error) {
	if err := ports.validate(); err != nil {
		return "", err
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return "", errors.New("relay controller is closed")
	}
	ctx, done := manager.operation(parent)
	defer done()
	if err := manager.ledger.verifyDaemon(ctx, manager.cli); err != nil {
		return "", err
	}
	bridge, err := manager.cli.NetworkInspect(ctx, "bridge", client.NetworkInspectOptions{})
	if err != nil {
		return "", err
	}
	descriptor.PublicRoot = slices.Clone(descriptor.PublicRoot)
	networks = slices.Clone(networks)
	for i := range networks {
		networks[i].Aliases = slices.Clone(networks[i].Aliases)
	}
	intent := RelayIntent{Session: descriptor.Session, ImageID: manager.image.ID, AssetSHA: manager.image.AssetSHA, BridgeID: bridge.Network.ID, Gateway: descriptor, Networks: networks, Ports: ports}
	options, err := RelayConfiguration(intent)
	if err != nil {
		return "", err
	}
	if !time.Now().Before(descriptor.ExpiresAt) {
		return "", errors.New("relay gateway identity expired")
	}
	if manager.current != nil {
		if !reflect.DeepEqual(manager.current.Intent, intent) {
			return "", errors.New("retire the current relay before changing gateway or selected networks")
		}
		if err = VerifyRelay(ctx, manager.cli, intent, manager.current.ID, true); err != nil {
			return "", err
		}
		return manager.current.ID, nil
	}
	records, err := manager.ledger.relayRecords()
	if err != nil {
		return "", err
	}
	if len(records) != 0 {
		return "", errors.New("pending relay receipts require exact recovery before setup")
	}
	if err = verifyToolsImage(ctx, manager.cli, manager.image); err != nil {
		return "", err
	}
	if err = VerifyRelayNetworks(ctx, manager.cli, intent); err != nil {
		return "", err
	}
	if err = manager.portProbe(ports); err != nil {
		return "", err
	}
	if err = manager.ledger.BeginRelay(intent); err != nil {
		return "", err
	}
	manager.pending = &relayRecord{Version: 1, Daemon: manager.ledger.daemon, Intent: intent}
	created, err := manager.cli.ContainerCreate(ctx, options)
	if err != nil {
		return "", fmt.Errorf("relay creation was not acknowledged; retain its receipt: %w", err)
	}
	record := relayRecord{Version: 1, Daemon: manager.ledger.daemon, Intent: intent, ID: created.ID}
	manager.pending = &record
	if err = manager.ledger.CreatedRelay(intent, created.ID); err != nil {
		return "", err
	}
	manager.current = &record
	// Every path after accepted creation either returns a verified relay or
	// attempts exact cleanup with a fresh bounded context and retains failures.
	fail := func(cause error) (string, error) {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cleanErr := manager.remove(cleanup, record)
		if cleanErr == nil {
			manager.current = nil
			manager.pending = nil
		}
		return "", errors.Join(cause, cleanErr)
	}
	if err = VerifyRelayNetworks(ctx, manager.cli, intent); err != nil {
		return fail(err)
	}
	if _, err = manager.cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return fail(err)
	}
	if err = VerifyRelay(ctx, manager.cli, intent, created.ID, true); err != nil {
		return fail(err)
	}
	if err = runRelayReady(ctx, manager.cli, record); err != nil {
		return fail(err)
	}
	manager.pending = nil
	return created.ID, nil
}

var ErrRelayPortUnavailable = errors.New("local service ports are already in use")

// This preflight avoids committing an unknown-create receipt for an already
// observable conflict. Docker's publication adapter still owns the definitive
// reservation and rejects a process winning the subsequent bind race.
func probeRelayHostPorts(ports ServicePorts) error {
	if err := ports.validate(); err != nil {
		return err
	}
	var listeners []net.Listener
	defer func() {
		for _, listener := range listeners {
			listener.Close()
		}
	}()
	for _, address := range []string{fmt.Sprintf("127.0.0.1:%d", ports.HTTP), fmt.Sprintf("127.0.0.1:%d", ports.HTTPS)} {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			return fmt.Errorf("%w (%s): %v", ErrRelayPortUnavailable, address, err)
		}
		listeners = append(listeners, listener)
	}
	return nil
}
func (manager *RelayManager) Renew(parent context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed || manager.current == nil {
		return errors.New("no active owned relay")
	}
	ctx, done := manager.operation(parent)
	defer done()
	if err := manager.ledger.verifyDaemon(ctx, manager.cli); err != nil {
		return err
	}
	if err := VerifyRelay(ctx, manager.cli, manager.current.Intent, manager.current.ID, true); err != nil {
		return err
	}
	return runNamespaceCommand(ctx, manager.cli, manager.current.ID, "renew", manager.current.Intent.Session, "renewed\n")
}

// Stop permits an explicit later Start with a fresh gateway session. Close also
// latches shutdown and interrupts outstanding work; neither releases the ledger.
func (manager *RelayManager) Stop(parent context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return errors.New("relay controller is closed")
	}
	ctx, done := manager.operation(parent)
	defer done()
	if err := manager.ledger.verifyDaemon(ctx, manager.cli); err != nil {
		return err
	}
	if manager.current == nil {
		records, err := manager.ledger.relayRecords()
		if err != nil {
			return err
		}
		for _, record := range records {
			if err = manager.remove(ctx, record); err != nil {
				return err
			}
		}
		return nil
	}
	if err := manager.remove(ctx, *manager.current); err != nil {
		return err
	}
	manager.current = nil
	return nil
}
func (manager *RelayManager) Close(parent context.Context) (resultErr error) {
	manager.cancel()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return manager.closeErr
	}
	manager.closed = true
	defer func() { manager.closeErr = resultErr }()
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	if err := manager.ledger.verifyDaemon(ctx, manager.cli); err != nil {
		return err
	}
	records, err := manager.ledger.relayRecords()
	if err != nil {
		return err
	}
	if manager.pending != nil {
		// Unknown creates cannot become successful Close because a later lookup
		// is absent or metadata was damaged. Retain this actual issuing owner.
		return errors.New("relay mutation remains unconfirmed; actual issuer and acknowledged scope retained")
	}
	for _, record := range records {
		if err = manager.remove(ctx, record); err != nil {
			return err
		}
	}
	manager.current = nil
	manager.pending = nil
	return nil
}

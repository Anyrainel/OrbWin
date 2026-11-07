package localnet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

type NamespaceHandle struct {
	Intent NamespaceIntent
	ID     string
}
type PendingNamespace struct {
	Session     string `json:"session"`
	ContainerID string `json:"containerId"`
	StartedAt   string `json:"startedAt"`
	HelperID    string `json:"helperId,omitempty"`
	State       string `json:"state"`
}

// Retained uncertainty is part of service status, never a hidden success or an
// instruction to discard a missing-by-name intent. Active handles are excluded.
func (manager *NamespaceManager) Pending() ([]PendingNamespace, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return nil, errors.New("local-services namespace manager is closed")
	}
	records, err := manager.ledger.records()
	if err != nil {
		return nil, err
	}
	items := make([]PendingNamespace, 0)
	for _, record := range records {
		if active, ok := manager.active[namespaceKey(record.Intent)]; ok && active.ID == record.ID {
			continue
		}
		state := "cleanup-pending"
		if record.ID == "" {
			state = "create-acceptance-unknown"
		}
		items = append(items, PendingNamespace{Session: record.Intent.Session, ContainerID: record.Intent.ContainerID, StartedAt: record.Intent.StartedAt, HelperID: record.ID, State: state})
	}
	return items, nil
}

// Owns only service namespace helpers, never rdd or workload lifecycle. Methods
// serialize create/recovery/removal under mu; renew/dial setup run in parallel.
// The route controller retires connections before Release/Close. The image ID
// must come from the verified import receipt.
// On process loss the guest lease expires; the next exclusive owner can recover
// recorded helpers. Unknown creates are never retried with the same name.
type NamespaceManager struct {
	poisonMu             sync.Mutex
	poisoned             map[string]error
	mu                   sync.Mutex
	cli                  *client.Client
	ledger               *NamespaceLedger
	image, asset, daemon string
	active               map[string]NamespaceHandle
	pending              map[string]namespaceRecord // producer-issued native mutations not positively settled
	closeErr             error
	datagramSlots        map[string]int // mu: pending, active and uncertain UDP execs per exact helper ID.
	closed               bool
	ctx                  context.Context
	cancel               context.CancelFunc
	renewing             sync.WaitGroup
	borrowedOwner        bool
}

// Plain-path fixture constructor. The production SDK transfers its opaque
// private ledger through OpenNamespaceManagerWithLedger instead.
func OpenNamespaceManager(ctx context.Context, cli *client.Client, directory, daemon, imageID, assetSHA string) (*NamespaceManager, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if cli == nil || !fullID(assetSHA) || !strings.HasPrefix(imageID, "sha256:") || !fullID(strings.TrimPrefix(imageID, "sha256:")) {
		return nil, errors.New("verified service asset and owned daemon client required")
	}
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return nil, err
	}
	if daemon == "" || info.Info.ID != daemon || info.Info.OSType != "linux" {
		return nil, errors.New("local-services endpoint does not match the registered owned Linux daemon")
	}
	image, err := cli.ImageInspect(ctx, imageID)
	if err != nil {
		return nil, err
	}
	object := image.InspectResponse
	if object.ID != imageID || object.Os != "linux" || object.Architecture != "amd64" || object.Config == nil || object.Config.Labels["dev.orbwin.component"] != "local-services-tools" || object.Config.Labels["dev.orbwin.source-sha256"] != assetSHA {
		return nil, errors.New("service tools image differs from the verified import")
	}
	ledger, err := OpenNamespaceLedger(directory, daemon)
	if err != nil {
		return nil, err
	}
	manager, err := newNamespaceManager(ctx, cli, ledger, daemon, imageID, assetSHA)
	if err != nil {
		ledger.Close()
		return nil, err
	}
	return manager, nil
}

// The permanent process can acquire one owner before authority/image setup and
// transfer it here, avoiding an ownership gap or a second competing lock. On
// success the manager closes the ledger; on failure the caller retains it for
// diagnostics/recovery. The image must match a committed import receipt.
func OpenNamespaceManagerWithLedger(ctx context.Context, cli *client.Client, ledger *NamespaceLedger, image ToolsImage) (*NamespaceManager, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if ledger == nil {
		return nil, errors.New("exclusive local-services owner required")
	}
	daemon := ledger.daemon
	receipt, err := ledger.committedImageReceipt(ctx, image.AssetSHA)
	if err != nil {
		return nil, err
	}
	if receipt.ID == "" || receipt.ID != image.ID || receipt.Token != image.Token {
		return nil, errors.New("service image differs from its committed import receipt")
	}
	if err := ledger.verifyDaemon(ctx, cli); err != nil {
		return nil, err
	}
	if err := verifyToolsImage(ctx, cli, image); err != nil {
		return nil, err
	}
	return newNamespaceManager(ctx, cli, ledger, daemon, image.ID, image.AssetSHA)
}

// The permanent process retains the exclusive lease across host generations.
// This constructor borrows it; Close still joins and cleans this manager's work
// but leaves settings/authority ownership to the caller. Do not close that owner
// until every borrowed generation has finished.
func OpenNamespaceManagerBorrowingLedger(ctx context.Context, cli *client.Client, ledger *NamespaceLedger, image ToolsImage) (*NamespaceManager, error) {
	manager, err := OpenNamespaceManagerWithLedger(ctx, cli, ledger, image)
	if err != nil {
		return nil, err
	}
	manager.borrowedOwner = true
	return manager, nil
}
func newNamespaceManager(ctx context.Context, cli *client.Client, ledger *NamespaceLedger, daemon, imageID, assetSHA string) (*NamespaceManager, error) {
	manager := &NamespaceManager{cli: cli, ledger: ledger, image: imageID, asset: assetSHA, daemon: daemon, active: make(map[string]NamespaceHandle)}
	manager.ctx, manager.cancel = context.WithCancel(context.Background())
	if _, err := manager.Recover(ctx); err != nil {
		manager.cancel()
		return nil, err
	}
	return manager, nil
}
func namespaceKey(intent NamespaceIntent) string {
	key := intent.ContainerID + "\x00" + intent.StartedAt
	if intent.LANLeaseSHA != "" {
		key += "\x00lan/" + intent.LANLeaseSHA
	}
	return key
}
func (manager *NamespaceManager) Target(ctx context.Context, id string) (string, error) {
	manager.mu.Lock()
	if manager.closed || !fullID(id) {
		manager.mu.Unlock()
		return "", errors.New("open controller and full target ID required")
	}
	manager.renewing.Add(1)
	manager.mu.Unlock()
	defer manager.renewing.Done()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(manager.ctx, cancel)
	defer stop()
	if err := manager.verifyDaemon(ctx); err != nil {
		return "", err
	}
	result, err := manager.cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return "", err
	}
	if result.Container.State == nil {
		return "", errors.New("service target state unavailable")
	}
	started := result.Container.State.StartedAt
	stamp, err := time.Parse(time.RFC3339Nano, started)
	if err != nil || stamp.IsZero() {
		return "", errors.New("service target incarnation unavailable")
	}
	if err := validateServiceTarget(result.Container, id, started); err != nil {
		return "", err
	}
	return started, nil
}
func (manager *NamespaceManager) verifyDaemon(ctx context.Context) error {
	info, err := manager.cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return err
	}
	if info.Info.ID != manager.daemon || info.Info.OSType != "linux" {
		return errors.New("local-services daemon identity changed; no helper mutation performed")
	}
	return nil
}

// Recover requires an idle manager with the exclusive process lease. Missing
// unknown creates remain pending: a timed-out daemon request may still finish.
func (manager *NamespaceManager) Recover(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed || len(manager.active) != 0 {
		return 0, errors.New("namespace recovery requires an open idle controller")
	}
	if err := manager.verifyDaemon(ctx); err != nil {
		return 0, err
	}
	records, err := manager.ledger.records()
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if record.ID == "" {
			result, err := manager.cli.ContainerInspect(ctx, record.Intent.name(), client.ContainerInspectOptions{})
			if cerrdefs.IsNotFound(err) {
				continue
			}
			if err != nil {
				return removed, err
			}
			id := result.Container.ID
			if err = VerifyNamespaceOwnership(ctx, manager.cli, record.Intent, id); err != nil {
				return removed, err
			}
			if err = manager.ledger.Created(record.Intent, id); err != nil {
				return removed, err
			}
			record.ID = id
		}
		if err = manager.cleanup(ctx, record); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
func (manager *NamespaceManager) Acquire(ctx context.Context, containerID, startedAt string) (NamespaceHandle, error) {
	return manager.acquire(ctx, containerID, startedAt, "")
}
func (manager *NamespaceManager) AcquireLAN(ctx context.Context, containerID, startedAt, leaseSHA string) (NamespaceHandle, error) {
	if !fullID(leaseSHA) {
		return NamespaceHandle{}, errors.New("exact native LAN lease digest required")
	}
	return manager.acquire(ctx, containerID, startedAt, leaseSHA)
}
func (manager *NamespaceManager) acquire(ctx context.Context, containerID, startedAt, leaseSHA string) (NamespaceHandle, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return NamespaceHandle{}, errors.New("local-services namespace manager is closed")
	}
	if err := manager.verifyDaemon(ctx); err != nil {
		return NamespaceHandle{}, err
	}
	var session [16]byte
	if _, err := rand.Read(session[:]); err != nil {
		return NamespaceHandle{}, err
	}
	intent := NamespaceIntent{Session: hex.EncodeToString(session[:]), ImageID: manager.image, AssetSHA: manager.asset, ContainerID: containerID, StartedAt: startedAt, LANLeaseSHA: leaseSHA}
	if err := intent.validate(); err != nil {
		return NamespaceHandle{}, err
	}
	if existing, ok := manager.active[namespaceKey(intent)]; ok {
		if err := manager.streamFailure(existing.ID); err != nil {
			return NamespaceHandle{}, err
		}
		if err := VerifyServiceTarget(ctx, manager.cli, existing.Intent); err != nil {
			return NamespaceHandle{}, err
		}
		if err := VerifyNamespace(ctx, manager.cli, existing.Intent, existing.ID); err != nil {
			return NamespaceHandle{}, err
		}
		return existing, nil
	}
	purposeCount := 0
	for _, active := range manager.active {
		if (active.Intent.LANLeaseSHA == "") == (leaseSHA == "") {
			purposeCount++
		}
	}
	if len(manager.active) >= 32 || purposeCount >= 16 {
		return NamespaceHandle{}, errors.New("owned namespace helper budget reached (sixteen HTTP and sixteen LAN maximum)")
	}
	if err := VerifyServiceTarget(ctx, manager.cli, intent); err != nil {
		return NamespaceHandle{}, err
	}
	options, err := NamespaceConfiguration(intent)
	if err != nil {
		return NamespaceHandle{}, err
	}
	if err = manager.ledger.Begin(intent); err != nil {
		return NamespaceHandle{}, err
	}
	if manager.pending == nil {
		manager.pending = map[string]namespaceRecord{}
	}
	manager.pending[intent.Session] = namespaceRecord{Version: 1, Daemon: manager.daemon, Intent: intent}
	created, err := manager.cli.ContainerCreate(ctx, options)
	if err != nil {
		return NamespaceHandle{}, fmt.Errorf("service helper create uncertain/refused; ownership intent retained: %w", err)
	}
	record := namespaceRecord{Version: 1, Daemon: manager.daemon, Intent: intent, ID: created.ID}
	manager.pending[intent.Session] = record
	if err = manager.ledger.Created(intent, created.ID); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// No accepted-ID flush: keep the record even after exact local cleanup.
		cleanupErr := manager.removeExact(cleanupCtx, intent, created.ID)
		if cleanupErr == nil {
			delete(manager.pending, intent.Session)
		}
		return NamespaceHandle{}, errors.Join(err, cleanupErr)
	}
	_, err = manager.cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{})
	if err == nil {
		err = VerifyServiceTarget(ctx, manager.cli, intent)
	}
	if err == nil {
		err = VerifyNamespace(ctx, manager.cli, intent, created.ID)
	}
	if err == nil {
		err = runNamespaceCommand(ctx, manager.cli, created.ID, "ready-netns", intent.Session, "renewed\n")
	}
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cleanupErr := manager.cleanup(cleanupCtx, record)
		if cleanupErr == nil {
			delete(manager.pending, intent.Session)
		}
		return NamespaceHandle{}, errors.Join(err, cleanupErr)
	}
	handle := NamespaceHandle{Intent: intent, ID: created.ID}
	manager.active[namespaceKey(intent)] = handle
	delete(manager.pending, intent.Session)
	manager.poisonMu.Lock()
	if manager.poisoned == nil {
		manager.poisoned = make(map[string]error)
	}
	manager.poisoned[handle.ID] = nil
	manager.poisonMu.Unlock()
	return handle, nil
}
func (manager *NamespaceManager) Renew(ctx context.Context, handle NamespaceHandle) error {
	manager.mu.Lock()
	if manager.closed || manager.active[namespaceKey(handle.Intent)] != handle {
		manager.mu.Unlock()
		return errors.New("service helper is not owned by this active controller")
	}
	manager.renewing.Add(1)
	manager.mu.Unlock()
	defer manager.renewing.Done()
	if err := manager.streamFailure(handle.ID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(manager.ctx, cancel)
	defer stop()
	if err := manager.verifyDaemon(ctx); err != nil {
		return err
	}
	if err := VerifyServiceTarget(ctx, manager.cli, handle.Intent); err != nil {
		return err
	}
	if err := VerifyNamespace(ctx, manager.cli, handle.Intent, handle.ID); err != nil {
		return err
	}
	return runNamespaceCommand(ctx, manager.cli, handle.ID, "renew-netns", handle.Intent.Session, "renewed\n")
}
func (manager *NamespaceManager) Release(ctx context.Context, handle NamespaceHandle) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed || manager.active[namespaceKey(handle.Intent)] != handle {
		return errors.New("service helper is not owned by this active controller")
	}
	err := manager.cleanup(ctx, namespaceRecord{Version: 1, Daemon: manager.daemon, Intent: handle.Intent, ID: handle.ID})
	if err == nil {
		delete(manager.active, namespaceKey(handle.Intent))
		manager.poisonMu.Lock()
		delete(manager.poisoned, handle.ID)
		manager.poisonMu.Unlock()
	}
	return err
}
func (manager *NamespaceManager) cleanup(ctx context.Context, record namespaceRecord) error {
	if err := manager.removeExact(ctx, record.Intent, record.ID); err != nil {
		return err
	}
	return manager.ledger.settled(record)
}
func (manager *NamespaceManager) removeExact(ctx context.Context, intent NamespaceIntent, id string) error {
	if err := manager.verifyDaemon(ctx); err != nil {
		return err
	}
	if err := VerifyNamespaceOwnership(ctx, manager.cli, intent, id); err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		return err
	}
	_, err := manager.cli.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true})
	if cerrdefs.IsNotFound(err) {
		return nil
	}
	if !cerrdefs.IsConflict(err) {
		return err
	}
	// AutoRemove may already own deletion. Never turn a conflict into a search,
	// another force target or a successful settlement without exact-ID absence.
	for {
		_, err = manager.cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Dial setup shares controller close cancellation; returned connections belong
// to the registry, which closes them before namespace-manager shutdown.
func (manager *NamespaceManager) Dial(ctx context.Context, route ServiceRoute) (net.Conn, error) {
	manager.mu.Lock()
	handle, exists := manager.active[namespaceRouteKey(route)]
	if manager.closed || !exists || handle.ID != route.HelperID || route.Port == 0 {
		manager.mu.Unlock()
		return nil, errors.New("route has no active owned namespace helper")
	}
	manager.renewing.Add(1)
	manager.mu.Unlock()
	defer manager.renewing.Done()
	if err := manager.streamFailure(handle.ID); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(manager.ctx, cancel)
	defer stop()
	if err := manager.verifyDaemon(ctx); err != nil {
		return nil, err
	}
	cleanup := func(ctx context.Context, token string) error {
		err := manager.verifyDaemon(ctx)
		if err == nil {
			err = VerifyNamespaceOwnership(ctx, manager.cli, handle.Intent, handle.ID)
			if cerrdefs.IsNotFound(err) {
				return nil
			}
		}
		if err == nil {
			err = revokeNamespaceStream(ctx, manager.cli, handle.ID, token)
		}
		if err != nil {
			manager.poisonMu.Lock()
			if _, tracked := manager.poisoned[handle.ID]; tracked {
				manager.poisoned[handle.ID] = fmt.Errorf("namespace stream revocation unconfirmed; helper recovery required: %w", err)
			}
			manager.poisonMu.Unlock()
		}
		return err
	}
	connection, err := dialVerifiedNamespace(ctx, manager.cli, handle.Intent, handle.ID, route.Port, cleanup)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil || manager.ctx.Err() != nil {
		connection.Close()
		return nil, errors.New("local-services dial cancelled during controller shutdown")
	}
	return connection, nil
}
func (manager *NamespaceManager) Close(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return manager.closeErr
	}
	manager.closed = true
	manager.cancel()
	var failures []error
	if len(manager.pending) != 0 {
		failures = append(failures, errors.New("namespace helper mutation remains unconfirmed; same controller and intent retained"))
	}
	for key, handle := range manager.active {
		if err := manager.cleanup(ctx, namespaceRecord{Version: 1, Daemon: manager.daemon, Intent: handle.Intent, ID: handle.ID}); err != nil {
			failures = append(failures, err)
		} else {
			delete(manager.active, key)
			manager.poisonMu.Lock()
			delete(manager.poisoned, handle.ID)
			manager.poisonMu.Unlock()
		}
	}
	manager.renewing.Wait()
	if !manager.borrowedOwner {
		// Legacy plaintext constructors are fixture-only and can model process
		// loss by releasing that file lease; native managed owners retain their
		// actual ledger when helper retirement is unconfirmed.
		if len(failures) == 0 || manager.ledger.stateStore == nil {
			failures = append(failures, manager.ledger.Close())
		}
	}
	manager.closeErr = errors.Join(failures...)
	return manager.closeErr
}
func (manager *NamespaceManager) streamFailure(id string) error {
	manager.poisonMu.Lock()
	defer manager.poisonMu.Unlock()
	return manager.poisoned[id]
}

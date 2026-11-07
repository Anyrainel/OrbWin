//go:build windows

package localnet

import (
	"context"
	"errors"
	"github.com/moby/moby/client"
	"orbwin.local/session/internal/sdkstate"
	"testing"
	"time"
)

func TestColdRuntimeMissingNativeLeafRefusesBeforeRootOrActorUse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// The inert transition has no Root/native backing. The production
	// constructor must reject absent/zero native leaf support before touching
	// it; neither an actor nor a native Root/cohort grant is fabricated here.
	for _, leaf := range []*sdkstate.LANObservationFactory{nil, {}} {
		h, err := NewColdServiceRuntime(ctx, &sdkstate.ColdTransition{}, RuntimeOptions{}, leaf)
		if h != nil || !errors.Is(err, sdkstate.ErrRecoveryRequired) {
			t.Fatal("missing real leaf holder reached actor/root construction", err)
		}
	}
}

type coldRuntimeOriginFake struct {
	*stateOriginFake
	nativeCloses int
}

func (f *coldRuntimeOriginFake) Close() error { f.nativeCloses++; return nil }

func inertColdRuntimeHolder(t *testing.T) (*ColdServiceRuntime, *coldRuntimeOriginFake) {
	t.Helper()
	return inertColdRuntimeHolderWithFactory(t, nil)
}

func inertColdRuntimeHolderWithFactory(t *testing.T, factory func() (WSLTrustClient, error)) (*ColdServiceRuntime, *coldRuntimeOriginFake) {
	t.Helper()
	origin := &coldRuntimeOriginFake{stateOriginFake: &stateOriginFake{records: map[string][]byte{}}}
	store := &coldManagedServiceState{serviceStateStore: origin}
	ledger := &NamespaceLedger{directory: managedStateLabel, daemon: "owned-daemon", stateStore: store}
	cli, err := client.New(client.WithHost("http://127.0.0.1:1"), client.WithAPIVersion("1.56"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	asset, sha := testToolsAsset()
	options := RuntimeOptions{Client: cli, Ledger: ledger, Installation: "00000000-0000-4000-8000-000000000001", ToolsAsset: asset, ToolsSHA256: sha, WSLTrustFactory: factory}
	guest := newColdGuestIssuer(options)
	options.WSLTrustFactory = guest.factory
	runtime, err := NewServiceRuntime(options)
	if err != nil {
		t.Fatal(err)
	}
	// Only the native state adapter is modeled; the actual actor and its full
	// existing inert shutdown sequence run. No engine/listener/trust route runs.
	h := &ColdServiceRuntime{runtime: runtime, store: store, guest: guest, served: true}
	h.nativeOrigin = h
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = runtime.Close(ctx)
	})
	return h, origin
}

func TestColdRuntimeHolderJoinsActualActorAndRetainsNativeRoot(t *testing.T) {
	h, origin := inertColdRuntimeHolder(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Join(ctx); err != nil {
		t.Fatal(err)
	}
	if !h.joined || h.unknown || !h.store.closed || origin.nativeCloses != 0 {
		t.Fatal("actor retirement released native root or missed logical ledger join")
	}
	if h.Join(ctx) != nil {
		t.Fatal("known joined owner changed result")
	}
	if h.VerifyJoined(ctx) != nil {
		t.Fatal("actual completed actor proof unavailable")
	}
	h.runtime.opts.Ledger.mu.Lock()
	h.runtime.opts.Ledger.stateStore = origin
	h.runtime.opts.Ledger.mu.Unlock()
	if h.VerifyJoined(ctx) == nil {
		t.Fatal("foreign ledger backing borrowed completed actor proof")
	}
	h.runtime.opts.Ledger.mu.Lock()
	h.runtime.opts.Ledger.stateStore = h.store
	h.runtime.opts.Ledger.mu.Unlock()
	if h.store.Begin(ctx) == nil {
		t.Fatal("joined ledger admitted new records")
	}
}

func TestColdRuntimeCopiedRealHolderCannotForwardItsConstructorOrigin(t *testing.T) {
	h, _ := inertColdRuntimeHolder(t)
	copy := &ColdServiceRuntime{nativeOrigin: h, root: h.root, transition: h.transition, runtime: h.runtime, store: h.store, ledger: h.ledger, leaves: h.leaves, guest: h.guest}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if copy.RootBacking() != nil || copy.LeafFactoryBacking() != nil || copy.VerifyJoined(ctx) == nil || copy.Join(ctx) == nil {
		t.Fatal("copied holder reused original constructor origin")
	}
	select {
	case <-h.runtime.done:
		t.Fatal("foreign wrapper closed actual actor")
	default:
	}
	if h.Join(ctx) != nil {
		t.Fatal("original actual actor cannot join")
	}
}

func TestColdRuntimeHolderUnknownLedgerFinishRetainsEveryOwner(t *testing.T) {
	h, origin := inertColdRuntimeHolder(t)
	h.runtime.opts.Ledger.mu.Lock()
	origin.finishErr = errors.New("unknown record close")
	h.runtime.opts.Ledger.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if h.Join(ctx) == nil || h.joined || !h.unknown || origin.nativeCloses != 0 {
		t.Fatal("unknown actor cleanup admitted native release")
	}
	h.runtime.opts.Ledger.mu.Lock()
	origin.finishErr = nil
	h.runtime.opts.Ledger.mu.Unlock()
	if h.Join(ctx) == nil || origin.nativeCloses != 0 {
		t.Fatal("unknown actor join replayed")
	}
}

func TestColdRuntimeHolderUnknownGuestCannotAdmitRetirementAfterActorJoin(t *testing.T) {
	calls := 0
	h, origin := inertColdRuntimeHolderWithFactory(t, func() (WSLTrustClient, error) {
		calls++
		return nil, errors.New("custom issuer effect unknown")
	})
	if _, err := h.runtime.opts.WSLTrustFactory(); err == nil || calls != 1 {
		t.Fatal("custom issuer did not enter the unknown boundary")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if h.Join(ctx) == nil || h.joined || !h.unknown || origin.nativeCloses != 0 {
		t.Fatal("actor join admitted unknown guest retirement")
	}
	select {
	case <-h.runtime.done:
	default:
		t.Fatal("guest refusal prevented the actual actor from joining")
	}
	if !h.store.closed || h.VerifyJoined(ctx) == nil || h.Join(ctx) == nil || calls != 1 {
		t.Fatal("unknown guest owner replayed or borrowed actor completion")
	}
}

func TestColdRuntimeHolderCloseInitiatesActorShutdownAndWaitsForIPCJoin(t *testing.T) {
	h, _ := inertColdRuntimeHolder(t)
	listener := &coldIPCJoinedListener{accepted: make(chan struct{}), closed: make(chan struct{})}
	h.serveDone, h.serving = make(chan struct{}), true
	loopReturned, releaseJoin := make(chan struct{}), make(chan struct{})
	go func() {
		_ = serveServiceIPCListener(context.Background(), h.runtime, listener)
		close(loopReturned)
		<-releaseJoin // Model a late native serving-loop completion acknowledgement.
		h.mu.Lock()
		h.serving = false
		close(h.serveDone)
		h.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	joined := make(chan error, 1)
	go func() { joined <- h.Join(ctx) }()
	select {
	case <-loopReturned:
	case <-ctx.Done():
		t.Fatal("actual runtime Close did not stop the shared IPC loop")
	}
	select {
	case <-joined:
		t.Fatal("holder ignored the late IPC join")
	default:
	}
	if h.VerifyJoined(ctx) == nil {
		t.Fatal("pending IPC join borrowed actor done proof")
	}
	close(releaseJoin)
	select {
	case err := <-joined:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("joined IPC did not release holder")
	}
}

//go:build windows

package localnet

import (
	"context"
	"encoding/hex"
	"orbwin.local/session/internal/sdkself"
	"orbwin.local/session/internal/sdkstate"
	"sync"
	"time"
)

// ColdServiceRuntime retains the actual runtime, its cold ledger and native
// transition as one ownership chain. It is created only from CandidateRoot;
// ordinary runtimes and caller-provided ledgers cannot be attached afterward.
type ColdServiceRuntime struct {
	nativeOrigin                              *ColdServiceRuntime // immutable actual-constructor self pointer
	mu                                        sync.Mutex
	transition                                *sdkstate.ColdTransition
	root                                      *sdkstate.Root
	runtime                                   *ServiceRuntime
	store                                     *coldManagedServiceState
	ledger                                    *NamespaceLedger
	construction                              coldRuntimeConstruction
	unstarted                                 *sdkstate.LANLeafUnstarted
	ipc                                       *sdkstate.ColdIPCLease
	serveDone                                 chan struct{}
	guest                                     *coldGuestIssuer
	leaves                                    *sdkstate.LANObservationFactory
	leafSeal                                  *sdkstate.LANLeafSeal
	serving, served, closing, joined, unknown bool
}

// The actor's existing joined cleanup must close its logical ledger before it
// reports completion. The cold transition keeps the actual native root alive
// until that proof and final commit, avoiding a runtime/root close dependency
// cycle. All record operations still use the original managed root/guard.
type coldManagedServiceState struct {
	serviceStateStore
	closed bool // protected by the containing ledger mutex
}

func (s *coldManagedServiceState) Begin(ctx context.Context) error {
	if s.closed {
		return sdkstate.ErrRecoveryRequired
	}
	return s.serviceStateStore.Begin(ctx)
}

func (s *coldManagedServiceState) Check() error {
	if s.closed {
		return sdkstate.ErrRecoveryRequired
	}
	return s.serviceStateStore.Check()
}

func (s *coldManagedServiceState) Close() error {
	if s.closed {
		return nil
	}
	if err := s.Finish(); err != nil {
		return err
	}
	s.closed = true
	return nil
}

func NewColdServiceRuntime(ctx context.Context, transition *sdkstate.ColdTransition, options RuntimeOptions, leaves *sdkstate.LANObservationFactory) (*ColdServiceRuntime, error) {
	if transition == nil || options.Ledger != nil || options.coldLANLeaf != nil {
		return nil, sdkstate.ErrState
	}
	// Native support and actual holder exist BEFORE registration/actor start,
	// never discovered missing after SDK Ready. Missing issuer remains a known
	// pre-start refusal; generic native queries are not a cold fallback.
	if leaves == nil || leaves.CheckNativeAdoption(ctx) != nil {
		return nil, sdkstate.ErrRecoveryRequired
	}
	leafIdentity, leafErr := leaves.SDKBacking().Identity()
	if leafErr != nil || options.SelfIdentity == nil || !sdkself.SameIdentity(leafIdentity, *options.SelfIdentity) {
		return nil, sdkstate.ErrRecoveryRequired
	}
	root, receipt, err := transition.CandidateRoot(ctx)
	if err != nil || leaves.RootBacking() != root || leaves.TransitionBacking() != transition || options.Installation != receipt.Scope.Installation || options.BinarySHA256 != hex.EncodeToString(receipt.Scope.SDKImage[:]) || options.SelfIdentity == nil || !options.SelfIdentity.Valid() || options.SelfIdentity.ImageSHA256 != options.BinarySHA256 {
		return nil, sdkstate.ErrRecoveryRequired
	}
	store := &coldManagedServiceState{serviceStateStore: &managedServiceState{root: root}}
	ledger := &NamespaceLedger{directory: managedStateLabel, daemon: receipt.Scope.Daemon, stateStore: store}
	guest := newColdGuestIssuer(options)
	options.WSLTrustFactory = guest.factory
	h := &ColdServiceRuntime{transition: transition, root: root, store: store, ledger: ledger, guest: guest, leaves: leaves, unknown: true}
	h.nativeOrigin = h
	if transition.RegisterRuntime(ctx, h) != nil {
		return h, sdkstate.ErrRecoveryRequired
	}
	options.Ledger = ledger
	options.coldLANLeaf = leaves
	constructed, err := h.construction.construct(options)
	if err == nil && constructed == nil {
		err = sdkstate.ErrRecoveryRequired
	} // no callee no-start receipt for an impossible nil/nil return
	h.mu.Lock()
	h.runtime = constructed
	if err == nil {
		h.unknown = false
	}
	h.mu.Unlock()
	if err != nil {
		// Actual normal constructor completion is necessary; nil alone is never
		// no-start authority. Partial logical/native state must finish positively.
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = h.PrepareUnstarted(cleanup)
		cancel()
		return h, err
	} // Retain the transition/root on partial construction.
	return h, nil
}

func (h *ColdServiceRuntime) ServeIPC(ctx context.Context) error {
	if h == nil || h.nativeOrigin != h || ctx == nil {
		return sdkstate.ErrState
	}
	h.mu.Lock()
	if h.unknown || h.runtime == nil || h.serving || h.served || h.closing {
		h.mu.Unlock()
		return sdkstate.ErrRecoveryRequired
	}
	// Never hold the upper holder mutex while entering the lower transition:
	// final Close holds the lower root mutex while reading VerifyJoined above.
	h.serving, h.served, h.unknown = true, true, true
	h.mu.Unlock()
	admission, cancel := context.WithTimeout(ctx, 5*time.Second)
	lease, err := h.transition.IPCLease(admission)
	cancel()
	h.mu.Lock()
	if err != nil {
		h.serving = false
		h.mu.Unlock()
		return err
	}
	h.ipc, h.serving, h.served = lease, true, true
	h.unknown = false
	h.serveDone = make(chan struct{})
	runtime := h.runtime
	h.mu.Unlock()
	err = ServeColdServiceIPC(ctx, runtime, lease)
	h.mu.Lock()
	h.serving = false
	close(h.serveDone)
	h.mu.Unlock()
	return err
}

func (h *ColdServiceRuntime) RootBacking() *sdkstate.Root {
	if h == nil || h.nativeOrigin != h {
		return nil
	}
	return h.root
}

func (h *ColdServiceRuntime) LeafFactoryBacking() *sdkstate.LANObservationFactory {
	if h == nil || h.nativeOrigin != h {
		return nil
	}
	return h.leaves
}

func (h *ColdServiceRuntime) PrepareUnstarted(ctx context.Context) error {
	if h == nil || h.nativeOrigin != h || ctx == nil || ctx.Err() != nil || !h.construction.failedBeforeActor() {
		return sdkstate.ErrRecoveryRequired
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return sdkstate.ErrState
	}
	h.mu.Lock()
	if h.runtime != nil || h.leaves == nil || h.ledger == nil || h.store == nil || h.guest == nil || h.served || h.serving {
		h.mu.Unlock()
		return sdkstate.ErrRecoveryRequired
	}
	leaves, ledger, guest := h.leaves, h.ledger, h.guest
	h.mu.Unlock()
	seal, err := leaves.SealUnstartedNative()
	if err != nil || guest.seal(ctx) != nil || guest.verifyNoGuestIssued() != nil || ledger.Close() != nil {
		return sdkstate.ErrRecoveryRequired
	}
	h.mu.Lock()
	h.unstarted = seal
	h.mu.Unlock()
	return nil // no Root/controller release and never a normal joined-runtime grant
}

func (h *ColdServiceRuntime) VerifyUnstarted(ctx context.Context) error {
	if h == nil || h.nativeOrigin != h || ctx == nil || ctx.Err() != nil {
		return sdkstate.ErrState
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.construction.failedBeforeActor() || h.runtime != nil || h.unstarted == nil || h.leaves == nil || h.unstarted.Backing(h.leaves) != nil || h.ledger == nil || h.store == nil || h.guest == nil || h.serving || h.served {
		return sdkstate.ErrRecoveryRequired
	}
	if h.guest.verifyNoGuestIssued() != nil {
		return sdkstate.ErrRecoveryRequired
	}
	h.ledger.mu.Lock()
	defer h.ledger.mu.Unlock()
	if !h.ledger.closed || h.ledger.closeErr != nil || !h.store.closed || h.ledger.stateStore != h.store {
		return sdkstate.ErrRecoveryRequired
	}
	return nil
}

func (h *ColdServiceRuntime) VerifyJoined(ctx context.Context) error {
	if h == nil || h.nativeOrigin != h || ctx == nil || ctx.Err() != nil {
		return sdkstate.ErrState
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.unknown || !h.joined || h.runtime == nil || h.store == nil || h.serving {
		return sdkstate.ErrRecoveryRequired
	}
	if h.guest == nil || h.guest.verifyNoGuestIssued() != nil {
		return sdkstate.ErrRecoveryRequired
	}
	if h.leaves != nil && (h.leafSeal == nil || h.leafSeal.Backing(h.leaves) != nil) {
		return sdkstate.ErrRecoveryRequired
	}
	runtime := h.runtime
	select {
	case <-runtime.done:
	default:
		return sdkstate.ErrRecoveryRequired
	}
	runtime.opts.Ledger.mu.Lock()
	defer runtime.opts.Ledger.mu.Unlock()
	if runtime.result != nil || runtime.generation != nil || !runtime.opts.Ledger.closed || !h.store.closed || runtime.opts.Ledger.stateStore != h.store {
		return sdkstate.ErrRecoveryRequired
	}
	return nil
}

// Join consumes the actual runtime's existing Close result. That actor joins
// LAN queries/work, generation HTTPS/relay cleanup, generation shutdown and its
// logical ledger before closing done. Cancellation/unknown cleanup retains this
// holder and the native transition; done alone never grants successful join.
func (h *ColdServiceRuntime) Join(ctx context.Context) error {
	if h == nil || h.nativeOrigin != h || ctx == nil {
		return sdkstate.ErrState
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return sdkstate.ErrState
	}
	h.mu.Lock()
	if h.unknown || h.runtime == nil || h.closing {
		h.mu.Unlock()
		return sdkstate.ErrRecoveryRequired
	}
	if h.joined {
		h.mu.Unlock()
		return nil
	}
	h.closing, h.unknown = true, true
	runtime := h.runtime
	serveDone := h.serveDone
	h.mu.Unlock()
	if h.leaves != nil && h.leaves.SealAdmissions() != nil {
		return sdkstate.ErrRecoveryRequired
	}
	if h.guest == nil || h.guest.seal(ctx) != nil {
		return sdkstate.ErrRecoveryRequired
	}
	if err := runtime.Close(ctx); err != nil {
		return err
	}
	// Close may initiate shutdown while the IPC server is still accepting.
	// The actual actor's done channel stops that same loop; wait for its joined
	// return before acknowledging the holder. No replacement listener is opened.
	if serveDone != nil {
		select {
		case <-serveDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// Positive actor completion synchronizes result/generation and store state.
	select {
	case <-runtime.done:
	default:
		return sdkstate.ErrRecoveryRequired
	}
	runtime.opts.Ledger.mu.Lock()
	closed := runtime.opts.Ledger.closed && h.store.closed && runtime.opts.Ledger.stateStore == h.store
	runtime.opts.Ledger.mu.Unlock()
	if !closed || runtime.result != nil || runtime.generation != nil {
		return sdkstate.ErrRecoveryRequired
	}
	// Do not let the upper control owner acknowledge retirement admission or
	// release its client/self leases while guest ownership remains unknown.
	// Actor completion alone cannot retire a dispatched Linux operation.
	if h.guest.verifyNoGuestIssued() != nil {
		return sdkstate.ErrRecoveryRequired
	}
	var leafSeal *sdkstate.LANLeafSeal
	if h.leaves != nil {
		var err error
		leafSeal, err = h.leaves.Join(ctx)
		if err != nil || leafSeal.Backing(h.leaves) != nil {
			return sdkstate.ErrRecoveryRequired
		}
	}
	h.mu.Lock()
	h.leafSeal = leafSeal
	h.joined, h.unknown, h.closing = true, false, false
	h.mu.Unlock()
	return nil
}

// Close is the same cold runtime owner's final shutdown path. The lower layer
// independently checks the registered actual holder, committed final grant and
// original joined pipe, then positively closes native resources with the old
// controller last. No extra orchestrator or caller-provided retirement flag.
func (h *ColdServiceRuntime) Close(ctx context.Context) error {
	if err := h.Join(ctx); err != nil {
		return err
	}
	return h.transition.Close()
}

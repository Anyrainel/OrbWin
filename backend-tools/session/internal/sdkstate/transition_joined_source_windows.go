//go:build windows

package sdkstate

import (
	"context"
	"orbwin.local/session/internal/sdkself"
	"reflect"
)

// The immutable compiled Go type is checked only alongside the actual native
// Root/master and constructor self-pointer checked by its backing methods.
// This prevents forwarding adapters across the intentional import-cycle seam;
// a type name, callback return or marker alone is never a native source grant.
func nativeColdRuntimeOrigin(p ColdRuntimeParticipant) bool {
	v := reflect.ValueOf(p)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() {
		return false
	}
	t := v.Type().Elem()
	return t.PkgPath() == "orbwin.local/session/localnet" && t.Name() == "ColdServiceRuntime"
}

// A JOINED-source participant, not OS end-of-birth/engine/guest retirement or
// final Root release. It has no callback/DTO constructor and no exported fields.
type ColdJoinedSource struct {
	transition *ColdTransition
	root       *Root
	control    *ColdControl
	backend    *nativeColdBackend
	master     *nativeColdLANMaster
	runtime    ColdRuntimeParticipant
	factory    *LANObservationFactory
	closedSelf *sdkself.JoinedLeaseClose
}

func (t *ColdTransition) JoinedSource(ctx context.Context, control *ColdControl, closedSelf *sdkself.JoinedLeaseClose) (*ColdJoinedSource, error) {
	if t == nil || !bounded(ctx) || control == nil || closedSelf == nil {
		return nil, ErrState
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.root == nil || t.root.s == nil {
		return nil, ErrRecoveryRequired
	}
	t.root.s.mu.Lock()
	defer t.root.s.mu.Unlock()
	b, ok := t.native.(*nativeColdBackend)
	if !ok || b == nil || b.leafMaster == nil {
		return nil, ErrRecoveryRequired
	}
	if b.joinedSource != nil {
		r := b.joinedSource
		if r.control != control || r.closedSelf != closedSelf || r.checkLocked(ctx, t) != nil {
			return nil, ErrRecoveryRequired
		}
		return r, nil
	}
	r := &ColdJoinedSource{transition: t, root: t.root, control: control, backend: b, master: b.leafMaster, runtime: t.runtime, factory: t.leafFactory, closedSelf: closedSelf}
	if r.checkLocked(ctx, t) != nil || ctx.Err() != nil {
		return nil, ErrRecoveryRequired
	}
	b.joinedSource = r
	return r, nil
}

func (r *ColdJoinedSource) Check(ctx context.Context, t *ColdTransition, control *ColdControl) error {
	if r == nil || t == nil || !bounded(ctx) || r.transition != t || r.control != control {
		return ErrRecoveryRequired
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.root == nil || t.root.s == nil {
		return ErrRecoveryRequired
	}
	t.root.s.mu.Lock()
	defer t.root.s.mu.Unlock()
	if r.backend == nil || r.backend.joinedSource != r {
		return ErrRecoveryRequired
	}
	return r.checkLocked(ctx, t)
}

// Actual transition + Root mutexes held. This distinct JOINED check uses the
// authority's still-retained native self lease. Execution adoption correctly
// refuses m.self after its extra entry lease closed; no allowClosed exception.
func (r *ColdJoinedSource) checkLocked(ctx context.Context, t *ColdTransition) error {
	if r == nil || t == nil || !bounded(ctx) || r.transition != t || r.root == nil || r.root.s == nil || r.root != t.root || r.backend == nil || r.master == nil || r.control == nil || r.closedSelf == nil || t.closed || t.unknown || !t.committed || t.runtime == nil || !nativeColdRuntimeOrigin(t.runtime) || !sameColdRuntimeParticipant(t.runtime, r.runtime) || t.leafFactory != r.factory {
		return ErrRecoveryRequired
	}
	b := r.backend
	m := r.master
	a := b.authority
	rb, ok := r.root.s.b.(*transitionBackend)
	if !ok || rb.cold != b || t.native != b || b.leafMaster != m || b.rootCapability != r.root || !sameColdRuntimeParticipant(b.runtime, r.runtime) || a == nil || !a.finalConsumed || r.root.s.closed || r.root.s.poison || r.root.s.active != nil || b.closed || b.unknown || !b.finished || b.check(ctx) != nil {
		return ErrRecoveryRequired
	}
	if m.transition != t || m.root != r.root || m.control != r.control || m.factory != r.factory || m.unknown || m.closed || m.closeAttempted || m.self == nil || m.ownQuery == 0 || m.returnedRoot == 0 || r.control.authority != a || r.control.io != a.controlIO || !a.controlTaken || sameTransitionNativeObject(m.ownQuery, m.returnedRoot) != nil {
		return ErrRecoveryRequired
	}
	closed, err := r.closedSelf.Backing(m.self)
	retained, heldErr := a.self.Identity()
	if err != nil || heldErr != nil || !sdkself.SameIdentity(closed, retained) {
		return ErrRecoveryRequired
	}
	a.supervisorMu.RLock()
	same := a.pendingSupervisor == m.supervisor && a.logicalSupervisor == m.supervisor && m.supervisor != nil && m.supervisor.transition == t && m.supervisor.provisional == r.control
	a.supervisorMu.RUnlock()
	if !same || a.recheckPendingSupervisor() != nil || r.runtime.RootBacking() != r.root || r.runtime.LeafFactoryBacking() != r.factory || r.runtime.VerifyJoined(ctx) != nil {
		return ErrRecoveryRequired
	}
	if m.joinedSourceBacking(r.factory) != nil || ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	return nil
}

// Read-only actual factory/master mailbox check, independently used at issuance
// and publication. The exact factory's Join proof is checked by actual holder.
func (m *nativeColdLANMaster) joinedSourceBacking(f *LANObservationFactory) error {
	if m == nil || f == nil || m.factory != f || f.identity != f || m.pump == nil || m.pump.native != m || m.pump.factory != f {
		return ErrRecoveryRequired
	}
	m.leafMu.Lock()
	retained := m.retained != nil
	m.leafMu.Unlock()
	m.pump.mu.Lock()
	pending := m.pump.pending != nil || m.pump.unknown
	m.pump.mu.Unlock()
	f.mu.Lock()
	sealed := f.sealed && !f.unknown && len(f.active) == 0 && f.adoption == m && f.cohort == m && f.root == m.root && f.transition == m.transition && f.lease == m.self
	f.mu.Unlock()
	if retained || pending || !sealed {
		return ErrRecoveryRequired
	}
	return nil
}

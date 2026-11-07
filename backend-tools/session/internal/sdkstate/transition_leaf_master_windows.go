//go:build windows

package sdkstate

import (
	"context"
	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/sdkself"
	"reflect"
	"sync"
	"unsafe"
)

// Authority-layer actual native master, never supplied by a public callback.
// Native root-process custody has a constructor below. Source issuance also
// requires this actual master's private same-pump per-leaf owner; it cannot
// qualify the still-missing whole SDK/engine/guest retirement families.
type nativeColdLANMaster struct {
	mu                                    sync.Mutex // one-use physical closure only; no control/Root callbacks
	transition                            *ColdTransition
	root                                  *Root
	control                               *ColdControl
	self                                  *sdkself.Lease
	supervisor                            *transitionPendingSupervisor
	factory                               *LANObservationFactory
	ownQuery, returnedRoot                windows.Handle
	unknown                               bool
	closePrepared, closeAttempted, closed bool
	closeErr                              error
	unknownCloses                         []windows.Handle
	unstartedReleased                     bool
	pump                                  *coldLANLeafPump
	leafMu                                sync.Mutex
	retained                              *nativeColdLeafRetention
}

func (m *nativeColdLANMaster) checkLocked(ctx context.Context, self *sdkself.Lease) error {
	if m == nil || m.unknown || m.closed || m.closeAttempted || !bounded(ctx) || self == nil || m.self != self || m.transition == nil || m.root == nil || m.control == nil || m.control.authority == nil || m.control.io == nil || m.ownQuery == 0 || m.returnedRoot == 0 {
		return ErrColdLeaf
	}
	t := m.transition
	b, ok := t.native.(*nativeColdBackend)
	if !ok || b == nil || b.leafMaster != m || b.authority != m.control.authority || t.root != m.root || m.root.s == nil || t.closed || t.unknown || m.root.s.closed || m.root.s.poison {
		return ErrColdLeaf
	}
	rb, ok := m.root.s.b.(*transitionBackend)
	if !ok || rb.cold != b || b.check(ctx) != nil {
		return ErrColdLeaf
	}
	a := b.authority
	a.supervisorMu.RLock()
	same := a.pendingSupervisor == m.supervisor && m.supervisor != nil && m.supervisor.transition == t && m.supervisor.provisional == m.control
	a.supervisorMu.RUnlock()
	if !same || m.control.io != a.controlIO || a.recheckPendingSupervisor() != nil || sameTransitionNativeObject(m.ownQuery, m.returnedRoot) != nil {
		return ErrColdLeaf
	}
	actual, err := self.Identity()
	original, originalErr := a.self.Identity()
	if err != nil || originalErr != nil || !sdkself.SameIdentity(actual, original) {
		return ErrColdLeaf
	}
	// Read-only native scope checks may coexist with an admitted data guard;
	// ordinary SDK state transactions are not a leaf-ownership failure.
	if m.pump == nil || m.pump.native != m {
		return ErrColdLeaf
	}
	m.pump.mu.Lock()
	unknown := m.pump.unknown
	m.pump.mu.Unlock()
	if unknown {
		return ErrColdLeaf
	}
	return nil
}
func (m *nativeColdLANMaster) checkLANLeafAdoption(ctx context.Context, self *sdkself.Lease) error {
	if m == nil || m.root == nil || m.root.s == nil {
		return ErrColdLeaf
	}
	m.root.s.mu.Lock()
	defer m.root.s.mu.Unlock()
	return m.checkLocked(ctx, self)
}
func (m *nativeColdLANMaster) SDKBacking() *sdkself.Lease {
	if m == nil {
		return nil
	}
	return m.self
}
func (m *nativeColdLANMaster) CheckNativeSupport(ctx context.Context) error {
	return m.checkLANLeafAdoption(ctx, m.SDKBacking())
}
func (m *nativeColdLANMaster) RetainBeforeResume(ctx context.Context, leaf *LANObservationLeaf) error {
	if m == nil || m.pump == nil {
		return ErrColdLeaf
	}
	return m.pump.request(ctx, leaf, "birth")
}
func (m *nativeColdLANMaster) RecordStopped(ctx context.Context, leaf *LANObservationLeaf) error {
	if m == nil || m.pump == nil {
		return ErrColdLeaf
	}
	return m.pump.request(ctx, leaf, "stopped")
}

// Actual authenticated native custody, separate from factory issuance. No job
// handle is ever loaned to SDK. Both duplicate outputs are owned before error;
// unsupported/partial/unknown retains the actual Root/master graph.
func (t *ColdTransition) AdoptNativeLANMaster(ctx context.Context, control *ColdControl, self *sdkself.Lease, record ColdLeafMasterRecord) error {
	if t == nil || !bounded(ctx) || control == nil || self == nil || record.Process == 0 || record.Process == uint64(^uintptr(0)) || uint64(uintptr(record.Process)) != record.Process {
		return ErrState
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	b, ok := t.native.(*nativeColdBackend)
	if !ok || b == nil || b.authority == nil || b.leafMaster != nil || t.root == nil || t.root.s == nil || t.closed || t.unknown || t.committed || t.runtime != nil {
		return ErrRecoveryRequired
	}
	t.root.s.mu.Lock()
	defer t.root.s.mu.Unlock()
	rb, ok := t.root.s.b.(*transitionBackend)
	a := b.authority
	if !ok || rb.cold != b || t.root.s.closed || t.root.s.poison || t.root.s.active != nil || !b.finished || b.unknown || control.authority != a || control.io == nil || control.io != a.controlIO || !a.controlTaken || b.check(ctx) != nil {
		return ErrRecoveryRequired
	}
	a.supervisorMu.RLock()
	supervisor := a.pendingSupervisor
	valid := supervisor != nil && supervisor.provisional == control && supervisor.transition == t && a.logicalSupervisor == nil
	a.supervisorMu.RUnlock()
	if !valid || a.recheckPendingSupervisor() != nil {
		return ErrRecoveryRequired
	}
	identity, err := self.Identity()
	original, originalErr := a.self.Identity()
	if err != nil || originalErr != nil || !sdkself.SameIdentity(identity, original) {
		return ErrRecoveryRequired
	}
	m := &nativeColdLANMaster{transition: t, root: t.root, control: control, self: self, supervisor: supervisor, unknown: true}
	b.leafMaster = m // actual Root backing BEFORE first duplicate/native output
	rights := uint32(windows.PROCESS_QUERY_INFORMATION | windows.SYNCHRONIZE)
	if windows.DuplicateHandle(windows.CurrentProcess(), windows.CurrentProcess(), windows.CurrentProcess(), &m.ownQuery, rights, false, 0) != nil || m.ownQuery == 0 {
		return ErrRecoveryRequired
	}
	if windows.DuplicateHandle(supervisor.process, windows.Handle(record.Process), windows.CurrentProcess(), &m.returnedRoot, rights, false, 0) != nil || m.returnedRoot == 0 {
		return ErrRecoveryRequired
	}
	if sameTransitionNativeObject(m.ownQuery, m.returnedRoot) != nil || transitionLeafProcessRights(m.returnedRoot) != nil || a.recheckPendingSupervisor() != nil || ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	m.unknown = false
	m.pump = &coldLANLeafPump{native: m}
	return nil // custody only; the separately bound actual factory is not Ready
}

func sameTransitionNativeObject(one, two windows.Handle) error {
	if one == 0 || two == 0 || one == windows.InvalidHandle || two == windows.InvalidHandle {
		return ErrState
	}
	compare := windows.NewLazySystemDLL("kernelbase.dll").NewProc("CompareObjectHandles")
	if compare.Find() != nil {
		return ErrState
	}
	same, _, _ := compare.Call(uintptr(one), uintptr(two))
	if same == 0 {
		return ErrState
	}
	return nil
}
func transitionLeafProcessRights(handle windows.Handle) error {
	var facts [14]uint32
	var size uint32
	status, _, _ := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryObject").Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&facts)), unsafe.Sizeof(facts), uintptr(unsafe.Pointer(&size)))
	required := uint32(windows.PROCESS_QUERY_INFORMATION | windows.SYNCHRONIZE)
	if status != 0 || size != uint32(unsafe.Sizeof(facts)) || facts[1]&required != required || facts[1]&^(required|windows.PROCESS_QUERY_LIMITED_INFORMATION) != 0 {
		return ErrState
	}
	return nil
}

// Called under the actual Root close mutex after registered actor/source/IPC
// joins, before Root closure. This is read-only except its private phase receipt.
func (m *nativeColdLANMaster) prepareOwnerRelease() error {
	if m == nil {
		return ErrRecoveryRequired
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.unknown || m.closed || m.closeAttempted || m.factory == nil || m.root == nil || m.transition == nil || m.factory.RootBacking() != m.root || m.factory.TransitionBacking() != m.transition || m.ownQuery == 0 || m.returnedRoot == 0 {
		return ErrRecoveryRequired
	}
	if m.closePrepared {
		return nil
	}
	f := m.factory
	m.leafMu.Lock()
	retained := m.retained != nil
	m.leafMu.Unlock()
	if retained || m.pump == nil {
		return ErrRecoveryRequired
	}
	m.pump.mu.Lock()
	pending := m.pump.pending != nil || m.pump.unknown
	m.pump.mu.Unlock()
	if pending {
		return ErrRecoveryRequired
	}
	f.mu.Lock()
	joined := f.sealed && !f.unknown && len(f.active) == 0 && f.adoption == m && f.cohort == m && f.lease == m.self
	f.mu.Unlock()
	if !joined || sameTransitionNativeObject(m.ownQuery, m.returnedRoot) != nil {
		return ErrRecoveryRequired
	}
	m.closePrepared = true
	return nil
}

func (m *nativeColdLANMaster) closeKnown() error {
	if m == nil {
		return ErrRecoveryRequired
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closeAttempted {
		return m.closeErr
	}
	if !m.closePrepared || m.unknown || m.closed || m.factory == nil || m.root == nil || m.transition == nil || m.ownQuery == 0 || m.returnedRoot == 0 {
		return ErrRecoveryRequired
	}
	return m.closeQueriesLocked()
}

// One actual checked-close implementation for normal joined closure and known
// unstarted abort. Caller holds m.mu and has admitted one of those actual phases.
func (m *nativeColdLANMaster) closeQueriesLocked() error {
	if m.closeAttempted {
		return m.closeErr
	}
	if m.ownQuery == 0 || m.returnedRoot == 0 {
		return ErrRecoveryRequired
	}
	m.closeAttempted, m.closeErr, m.unknown = true, ErrRecoveryRequired, true
	for _, slot := range []*windows.Handle{&m.returnedRoot, &m.ownQuery} {
		if *slot != 0 {
			h := *slot
			*slot = 0
			if windows.CloseHandle(h) != nil {
				m.unknownCloses = append(m.unknownCloses, h)
				return ErrRecoveryRequired
			}
		}
	}
	m.closed, m.unknown, m.closeErr = true, false, nil
	return nil
}

// Releases ONLY known master-query copies before actor start and before any
// actual leaf birth. An already registered holder additionally needs its actual
// failed-callee/partial-owner completion proof. It does not close Root/controller, claim SDK
// process retirement, restore predecessor state, or permit a new adoption.
// Unknown/partial adoption keeps its actual recovery graph instead.
func (t *ColdTransition) ReleaseUnstartedLANMaster(ctx context.Context, control *ColdControl) error {
	if t == nil || !bounded(ctx) || control == nil {
		return ErrState
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	b, ok := t.native.(*nativeColdBackend)
	if !ok || b == nil || b.leafMaster == nil || b.authority == nil || t.root == nil || t.root.s == nil || t.closed || t.unknown {
		return ErrRecoveryRequired
	}
	m := b.leafMaster
	if m.transition != t || m.root != t.root || m.control != control || control.authority != b.authority || control.io != b.authority.controlIO {
		return ErrRecoveryRequired
	}
	var unstarted ColdRuntimeUnstarted
	if t.runtime != nil || b.runtime != nil {
		candidate, ok := t.runtime.(ColdRuntimeUnstarted)
		if !ok || !sameColdRuntimeParticipant(t.runtime, b.runtime) || candidate.RootBacking() != t.root || candidate.LeafFactoryBacking() != m.factory || candidate.PrepareUnstarted(ctx) != nil {
			return ErrRecoveryRequired
		}
		unstarted = candidate
	}
	// Seal/check outside Root.mu: the native factory's admission recheck takes
	// factory.mu -> Root.mu; never introduce the reverse-order wait.
	var noBirth *LANLeafUnstarted
	if m.factory != nil {
		var err error
		noBirth, err = m.factory.SealUnstartedNative()
		if err != nil {
			return ErrRecoveryRequired
		}
	}
	t.root.s.mu.Lock()
	defer t.root.s.mu.Unlock()
	rb, ok := t.root.s.b.(*transitionBackend)
	if !ok || rb.cold != b || t.root.s.closed || t.root.s.poison || t.root.s.active != nil || t.committed || b.ipc != nil || m.factory != nil && (noBirth == nil || noBirth.factory != m.factory) {
		return ErrRecoveryRequired
	}
	if unstarted != nil && (!sameColdRuntimeParticipant(unstarted, t.runtime) || !sameColdRuntimeParticipant(t.runtime, b.runtime) || unstarted.VerifyUnstarted(ctx) != nil) {
		return ErrRecoveryRequired
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.unstartedReleased && m.closed && m.closeAttempted {
		return m.closeErr
	}
	if m.unknown || m.closeAttempted || m.closed || m.ownQuery == 0 || m.returnedRoot == 0 || ctx.Err() != nil || sameTransitionNativeObject(m.ownQuery, m.returnedRoot) != nil {
		return ErrRecoveryRequired
	}
	if err := m.closeQueriesLocked(); err != nil {
		return err
	}
	m.unstartedReleased = true
	return nil
}

func sameColdRuntimeParticipant(one, two ColdRuntimeParticipant) bool {
	a, b := reflect.ValueOf(one), reflect.ValueOf(two)
	return a.IsValid() && b.IsValid() && a.Kind() == reflect.Pointer && b.Kind() == reflect.Pointer && !a.IsNil() && !b.IsNil() && a.Type() == b.Type() && a.Pointer() == b.Pointer()
}

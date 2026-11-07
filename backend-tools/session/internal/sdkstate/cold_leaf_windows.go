//go:build windows

package sdkstate

import (
	"context"
	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/sdkself"
)

// The ONLY native factory constructor. Neither a self lease nor a callback
// can supply the actual Root/control/master relationship checked here.
func (t *ColdTransition) NewColdLANObservationFactory(ctx context.Context, control *ColdControl, self *sdkself.Lease) (*LANObservationFactory, error) {
	if t == nil || !bounded(ctx) || control == nil || self == nil {
		return nil, ErrColdLeaf
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	b, ok := t.native.(*nativeColdBackend)
	if !ok || b == nil || b.authority == nil || t.root == nil || t.root.s == nil || t.closed || t.unknown {
		return nil, ErrColdLeaf
	}
	t.root.s.mu.Lock()
	defer t.root.s.mu.Unlock()
	a := b.authority
	rootBackend, ok := t.root.s.b.(*transitionBackend)
	if !ok || rootBackend.cold != b || b.r == nil || !b.finished || b.unknown || !t.receipt.valid() || t.receipt.Scope != a.grant.Scope || t.root.s.active != nil {
		return nil, ErrColdLeaf
	}
	if t.root.s.closed || t.root.s.poison || control.authority != a || control.io == nil || control.io != a.controlIO || !a.controlTaken || b.leafMaster == nil || b.leafMaster.transition != t || b.leafMaster.root != t.root || b.leafMaster.control != control || b.leafMaster.self != self || b.leafMaster.factory != nil {
		return nil, ErrColdLeaf
	}
	m := b.leafMaster
	if m.checkLocked(ctx, self) != nil {
		return nil, ErrColdLeaf
	}
	f := &LANObservationFactory{root: t.root, transition: t, lease: self, adoption: m, cohort: m, active: map[*LANObservationLeaf]struct{}{}, changed: make(chan struct{})}
	f.identity = f
	m.factory = f
	m.pump.factory = f
	f.start = func(ctx context.Context, leaf *LANObservationLeaf) (lanLeafNative, error) {
		return startNativeColdLANLeaf(ctx, leaf)
	}
	coldLeafCustody.Lock()
	coldLeafCustody.owners[f] = struct{}{}
	coldLeafCustody.Unlock()
	return f, nil
}

// The physical SDKself resource owner supplies no Root/cohort/source grant.
// This private adapter retains that owner plus the actual authority-layer leaf.
type coldLANPhysicalAdapter struct {
	leaf     *LANObservationLeaf
	physical *sdkself.NativeLANLeaf
}

func startNativeColdLANLeaf(ctx context.Context, leaf *LANObservationLeaf) (lanLeafNative, error) {
	if leaf == nil || leaf.factory == nil || leaf.factory.adoption == nil || leaf.factory.adoption.checkLANLeafAdoption(ctx, leaf.factory.lease) != nil {
		return nil, knownLANLeafNoBirth(leaf, ErrColdLeaf)
	}
	f := leaf.factory
	f.mu.Lock()
	if f.nativeAttempts == ^uint64(0) {
		f.unknown = true
		f.mu.Unlock()
		return nil, ErrColdLeaf
	}
	f.nativeAttempts++ // BEFORE every possible physical creation attempt
	f.mu.Unlock()
	physical, err := sdkself.StartNativeLANObservation(ctx, leaf.factory.lease, leaf.ticket)
	var native *coldLANPhysicalAdapter
	if physical != nil {
		native = &coldLANPhysicalAdapter{leaf, physical}
		leaf.setNative(native)
	}
	if err != nil {
		if physical == nil && sdkself.IsNativeLANNoBirth(err, leaf.ticket) {
			f.mu.Lock()
			f.nativeNoBirth++
			f.mu.Unlock() // exact opaque physical proof, not caller error/flag
			return nil, knownLANLeafNoBirth(leaf, err)
		}
		if native != nil {
			return native, ErrColdLeaf
		}
		return nil, ErrColdLeaf
	}
	if native == nil {
		return nil, ErrColdLeaf
	}
	return native, nil
}
func (n *coldLANPhysicalAdapter) resume(ctx context.Context) error {
	if n == nil || n.physical == nil {
		return ErrColdLeaf
	}
	// The actual native master must check the exact retained leaf, not merely
	// a callback success or a borrowed valid factory, immediately before Resume.
	if ctx != nil && ctx.Err() == nil {
		m, ok := n.leaf.factory.adoption.(*nativeColdLANMaster)
		if !ok || m.verifyRetained(n.leaf, ctx) != nil {
			return ErrColdLeaf
		}
	}
	err := n.physical.Resume(ctx)
	if sdkself.IsNativeLANNoResume(err, n.leaf.ticket) {
		return &lanLeafNoResume{n.leaf, err}
	}
	return err
}
func (n *coldLANPhysicalAdapter) neverResumed() bool {
	return n != nil && n.physical != nil && n.physical.NeverResumed()
}
func (n *coldLANPhysicalAdapter) collect(ctx context.Context) ([]byte, error, error) {
	return n.physical.Collect(ctx)
}
func (n *coldLANPhysicalAdapter) closeKnown(ctx context.Context) error {
	return n.physical.CloseKnown(ctx)
}
func (l *LANObservationLeaf) NativeCohortProcess() (windows.Handle, uint32, uint64, error) {
	if l == nil {
		return 0, 0, 0, ErrColdLeaf
	}
	n, ok := l.nativeOwner().(*coldLANPhysicalAdapter)
	if !ok || n == nil || n.physical == nil {
		return 0, 0, 0, ErrColdLeaf
	}
	return n.physical.NativeCohortProcess()
}

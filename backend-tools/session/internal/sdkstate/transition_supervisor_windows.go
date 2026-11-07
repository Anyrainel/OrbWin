//go:build windows

package sdkstate

import (
	"context"
	"golang.org/x/sys/windows"
)

// A pending observation only: the actual final-slot sender selected this live
// child of the original launch owner. This capture alone does NOT accept the
// SDK job/control graph or release TEMP custody. The separate actual controller
// offer/roundtrip/retention exchange precedes logical supervisor acceptance.
type transitionPendingSupervisor struct {
	process     windows.Handle
	token       windows.Token // close owner is authority.tokens
	expected    transitionProcess
	provisional *ColdControl
	transition  *ColdTransition
}

func (a *nativeColdAuthority) holdPendingSupervisor(ctx context.Context, final transitionFinalRecord) error {
	// consumeFinal holds a.mu. Runtime checks do not take that mutex, so use a
	// separate publication lock; no reader may see a partially captured holder.
	// Order is authority.mu -> supervisorMu, never the reverse.
	a.supervisorMu.Lock()
	defer a.supervisorMu.Unlock()
	if !bounded(ctx) || a.pendingSupervisor == nil || a.pendingSupervisor.provisional == nil || a.parentImage == 0 || a.recheckParentImage() != nil {
		return ErrRecoveryRequired
	}
	p := transitionProcess{final.ServingPID, final.ServingCreation, final.ServingSession, final.ServingImage}
	if p.PID == 0 || p.PID == a.grant.Parent.PID || p.Creation <= a.grant.Parent.Creation || p.Session != a.session || p.Image != a.grant.Parent.Image {
		return ErrRecoveryRequired
	}
	// Final publication can only match the actual target captured from TEMP's
	// one-use native PI handoff BEFORE candidate Resume. This record cannot open
	// a different process by PID or replace the retained provisional object.
	if a.pendingSupervisor.expected != p || a.recheckPendingSupervisorLocked() != nil || ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	return nil
}

func (a *nativeColdAuthority) recheckPendingSupervisor() error {
	a.supervisorMu.RLock()
	defer a.supervisorMu.RUnlock()
	return a.recheckPendingSupervisorLocked()
}

func (a *nativeColdAuthority) recheckPendingSupervisorLocked() error {
	s := a.pendingSupervisor
	if s == nil {
		return nil // No provisional target has been admitted yet.
	}
	if s.provisional == nil || s.provisional.authority != a || s.provisional.io != a.controlIO || s.transition == nil || s.process == 0 || s.token == 0 || s.expected.Image != a.grant.Parent.Image || ordinaryTransitionToken(s.token, a.sid, a.session) != nil || transitionProcessMatch(s.process, s.expected, false) != nil || transitionMappedImage(s.process, a.parentImage) != nil || transitionSupervisorOutsideJobs(s.process) != nil {
		return ErrRecoveryRequired
	}
	return nil
}

// The kernel parent and pipe origin remain TEMP forever. After acceptance only
// the actual candidate is the live supervisor; the retained original code chain
// remains immutable provenance, while post-exit class44 on TEMP is unavailable.
func (a *nativeColdAuthority) recheckSupervisorOrigin() error {
	a.supervisorMu.RLock()
	defer a.supervisorMu.RUnlock()
	if a.logicalSupervisor == nil {
		return a.recheckParentImage()
	}
	if a.logicalSupervisor != a.pendingSupervisor || a.recheckPendingSupervisorLocked() != nil || a.parentImage == 0 || len(a.parentCode) < 5 {
		return ErrRecoveryRequired
	}
	for _, entry := range a.parentCode {
		if entry.recheck() != nil {
			return ErrRecoveryRequired
		}
	}
	return nil
}

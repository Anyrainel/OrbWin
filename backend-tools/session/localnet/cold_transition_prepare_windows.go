//go:build windows

package localnet

import (
	"context"
	"orbwin.local/session/internal/sdkself"
	"orbwin.local/session/internal/sdkstate"
	"sync"
	"time"
)

// ColdServicePreparation is the synchronous cold-entry ownership chain. It
// creates no actor or second service loop. The original inherited native owner,
// original SDK pipe listener, semantic signer and every partial transition stay
// reachable if any preparation or runtime-construction step is unknown.
type ColdServicePreparation struct {
	mu         sync.Mutex
	retirement *sdkstate.ColdRetirement
	transition *sdkstate.ColdTransition
	signing    *ColdSigningValidator
	runtime    *ColdServiceRuntime
	unknown    bool
	offer      *sdkstate.ColdSupervisorOffer
	control    *sdkstate.ColdControl
}

// ConsumeColdService accepts only the lower layer's actual inherited native
// grant. It performs the complete existing inventory/seal/import and signer
// validation before exposing a receipt to the temporary upgrader. A receipt is
// data, not runtime-ready/job acceptance. No ordinary Open or CLI dispatch is
// changed here; the native SDK launcher/peer still has to call this entry.
func ConsumeColdService(ctx context.Context, raw uintptr) (*ColdServicePreparation, error) {
	if !coldPreparationContext(ctx) || raw == 0 || raw == ^uintptr(0) {
		return nil, sdkstate.ErrState
	}
	p := &ColdServicePreparation{unknown: true, signing: NewColdSigningValidator()}
	var err error
	p.retirement, err = sdkstate.ConsumeColdRetirement(ctx, raw)
	if err != nil || p.retirement == nil {
		return p, sdkstate.ErrRecoveryRequired
	}
	p.transition, err = sdkstate.BeginColdTransition(ctx, p.retirement, p.signing)
	if err != nil || p.transition == nil {
		return p, sdkstate.ErrRecoveryRequired
	}
	if _, _, err = p.signing.CandidateAuthority(ctx, p.transition); err != nil {
		return p, sdkstate.ErrRecoveryRequired
	}
	p.unknown = false
	return p, nil
}

func coldPreparationContext(ctx context.Context) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	_, bounded := ctx.Deadline()
	return bounded
}

func (p *ColdServicePreparation) NativeControl(ctx context.Context) (*sdkstate.ColdControl, error) {
	if p == nil || !coldPreparationContext(ctx) {
		return nil, sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.retirement == nil || p.transition == nil {
		return nil, sdkstate.ErrRecoveryRequired
	}
	control, err := p.retirement.NativeControl(ctx)
	if err == nil {
		p.control = control
	}
	return control, err
}

func (p *ColdServicePreparation) Inventory(ctx context.Context) (sdkstate.TransitionReceipt, error) {
	if p == nil || !coldPreparationContext(ctx) {
		return sdkstate.TransitionReceipt{}, sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.retirement == nil || p.transition == nil || p.signing == nil {
		return sdkstate.TransitionReceipt{}, sdkstate.ErrRecoveryRequired
	}
	if _, _, err := p.signing.CandidateAuthority(ctx, p.transition); err != nil {
		return sdkstate.TransitionReceipt{}, sdkstate.ErrRecoveryRequired
	}
	_, receipt, err := p.transition.CandidateRoot(ctx)
	return receipt, err
}

func (p *ColdServicePreparation) ProvisionSupervisor(ctx context.Context, control *sdkstate.ColdControl, record sdkstate.ColdSupervisorRecord) error {
	if p == nil || !coldPreparationContext(ctx) {
		return sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.transition == nil || p.retirement == nil || p.runtime != nil {
		return sdkstate.ErrRecoveryRequired
	}
	if err := p.transition.ProvisionNativeSupervisor(ctx, control, record); err != nil {
		p.unknown = true
		return err
	}
	return nil
}

func (p *ColdServicePreparation) AdoptLeafMaster(ctx context.Context, control *sdkstate.ColdControl, self *sdkself.Lease, record sdkstate.ColdLeafMasterRecord) (*sdkstate.LANObservationFactory, error) {
	if p == nil || !coldPreparationContext(ctx) {
		return nil, sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.transition == nil || p.control != control || p.runtime != nil {
		return nil, sdkstate.ErrRecoveryRequired
	}
	p.unknown = true // retain the actual graph before any partial native operation
	if err := p.transition.AdoptNativeLANMaster(ctx, control, self, record); err != nil {
		return nil, err
	}
	factory, err := p.transition.NewColdLANObservationFactory(ctx, control, self)
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = p.transition.ReleaseUnstartedLANMaster(cleanup, control)
		cancel()
		return nil, err
	}
	p.unknown = false
	return factory, nil
}

func (p *ColdServicePreparation) NextLeafOffer(ctx context.Context, control *sdkstate.ColdControl) (*sdkstate.ColdLeafOfferRecord, error) {
	if p == nil || !coldPreparationContext(ctx) {
		return nil, sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.transition == nil || p.control != control {
		return nil, sdkstate.ErrRecoveryRequired
	}
	return p.transition.NextNativeLeafOffer(ctx, control)
}

func (p *ColdServicePreparation) AcknowledgeLeaf(ctx context.Context, control *sdkstate.ColdControl, record sdkstate.ColdLeafAckRecord) error {
	if p == nil || !coldPreparationContext(ctx) {
		return sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.transition == nil || p.control != control {
		return sdkstate.ErrRecoveryRequired
	}
	if err := p.transition.AcknowledgeNativeLeaf(ctx, control, record); err != nil {
		p.unknown = true
		return err
	}
	return nil
}

// Actual preparation/control binding; no callback or caller holder selects the
// registered runtime. The lower constructor rechecks its native JOINED scope.
func (p *ColdServicePreparation) JoinedRuntimeSource(ctx context.Context, control *sdkstate.ColdControl, closed *sdkself.JoinedLeaseClose) (*sdkstate.ColdJoinedSource, error) {
	if p == nil || !coldPreparationContext(ctx) {
		return nil, sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.transition == nil || p.control != control || p.runtime == nil {
		return nil, sdkstate.ErrRecoveryRequired
	}
	return p.transition.JoinedSource(ctx, control, closed)
}

func (p *ColdServicePreparation) CheckJoinedRuntimeSource(ctx context.Context, control *sdkstate.ColdControl, source *sdkstate.ColdJoinedSource) error {
	if p == nil || !coldPreparationContext(ctx) {
		return sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.transition == nil || p.control != control || p.runtime == nil || source == nil {
		return sdkstate.ErrRecoveryRequired
	}
	return source.Check(ctx, p.transition, control)
}

// StartRuntime delegates once to the existing actual runtime holder. It does
// not replace its Close/Join/IPC loop. Native options must still come from the
// actual SDK entry owner; caller bytes or inventory digests cannot supply code,
// publication or guest authority. Invalid options after admission retain p.
func (p *ColdServicePreparation) StartRuntime(ctx context.Context, options RuntimeOptions, leaves *sdkstate.LANObservationFactory) (*ColdServiceRuntime, error) {
	if p == nil || !coldPreparationContext(ctx) {
		return nil, sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.runtime != nil || p.retirement == nil || p.transition == nil || p.signing == nil || options.Ledger != nil {
		return nil, sdkstate.ErrRecoveryRequired
	}
	if _, _, err := p.signing.CandidateAuthority(ctx, p.transition); err != nil {
		return nil, sdkstate.ErrRecoveryRequired
	}
	p.unknown = true
	var err error
	p.runtime, err = NewColdServiceRuntime(ctx, p.transition, options, leaves)
	if err != nil || p.runtime == nil {
		// A known pre-start failure may settle ONLY master query copies. Lower
		// code proves actual callee/no-birth/partial-owner completion; this never
		// clears p.unknown or closes Root/controller/SDK scope on an error path.
		if p.control != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = p.transition.ReleaseUnstartedLANMaster(cleanup, p.control)
			cancel()
		}
		return p.runtime, sdkstate.ErrRecoveryRequired
	}
	p.unknown = false
	return p.runtime, nil
}

// For the actual native factory-construction failure caller before StartRuntime.
// It remains callable on an unknown preparation because the lower operation is
// independently phase-closed and cannot release a controller or replay adoption.
func (p *ColdServicePreparation) ReleaseUnstartedMasterQueries(ctx context.Context) error {
	if p == nil || !coldPreparationContext(ctx) {
		return sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.transition == nil || p.control == nil {
		return sdkstate.ErrRecoveryRequired
	}
	return p.transition.ReleaseUnstartedLANMaster(ctx, p.control)
}

// Commit uses the same original lower native final slot. Its successful return
// still cannot switch supervisor or close the temporary owner's SDK job copy.
func (p *ColdServicePreparation) Commit(ctx context.Context) error {
	if p == nil || !coldPreparationContext(ctx) {
		return sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.runtime == nil || p.transition == nil {
		return sdkstate.ErrRecoveryRequired
	}
	// Return known pre-admission busy to the sole control owner immediately.
	// Waiting here could deadlock an active guard whose leaf needs that same
	// owner's custody ACK. Admitted/unknown operations never return ErrBusy.
	return p.transition.Commit(ctx)
}

func (p *ColdServicePreparation) OfferSupervisor(ctx context.Context) ([4]uint64, error) {
	var empty [4]uint64
	if p == nil || !coldPreparationContext(ctx) {
		return empty, sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.runtime == nil || p.transition == nil || p.offer != nil {
		return empty, sdkstate.ErrRecoveryRequired
	}
	p.unknown = true
	var err error
	p.offer, err = p.transition.OfferNativeSupervisor(ctx)
	if err == sdkstate.ErrBusy && p.offer == nil {
		// The callee returns this exact sentinel only before allocating an offer
		// or issuing any native duplicate. Leave the pump available for leaves.
		p.unknown = false
		return empty, sdkstate.ErrBusy
	}
	if err != nil || p.offer == nil {
		return empty, sdkstate.ErrRecoveryRequired
	}
	handles, err := p.offer.NativeHandles()
	if err != nil {
		return empty, sdkstate.ErrRecoveryRequired
	}
	p.unknown = false
	return handles, nil
}

func (p *ColdServicePreparation) AcceptSupervisor(ctx context.Context) error {
	if p == nil || !coldPreparationContext(ctx) {
		return sdkstate.ErrState
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unknown || p.offer == nil {
		return sdkstate.ErrRecoveryRequired
	}
	p.unknown = true
	if p.offer.AcceptSupervisor(ctx) != nil {
		return sdkstate.ErrRecoveryRequired
	}
	p.unknown = false
	return nil
}

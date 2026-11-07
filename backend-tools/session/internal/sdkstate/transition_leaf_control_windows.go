//go:build windows

package sdkstate

import "context"

// Only the actual owner of the original authenticated control may service this
// mailbox. These methods perform no transport read/write or spawn another pump.
func (t *ColdTransition) NextNativeLeafOffer(ctx context.Context, control *ColdControl) (*ColdLeafOfferRecord, error) {
	if t == nil || !bounded(ctx) || control == nil {
		return nil, ErrState
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	b, ok := t.native.(*nativeColdBackend)
	if !ok || b == nil || b.authority == nil || t.closed || t.unknown || control.authority != b.authority || control.io != b.authority.controlIO {
		return nil, ErrRecoveryRequired
	}
	if b.leafMaster == nil {
		return nil, nil
	} // no source/factory exists yet
	m := b.leafMaster
	if m.control != control || m.transition != t || m.pump == nil {
		return nil, ErrRecoveryRequired
	}
	return m.pump.next(ctx)
}

func (t *ColdTransition) AcknowledgeNativeLeaf(ctx context.Context, control *ColdControl, record ColdLeafAckRecord) error {
	if t == nil || !bounded(ctx) || control == nil || !record.Valid() {
		return ErrState
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	b, ok := t.native.(*nativeColdBackend)
	if !ok || b == nil || b.authority == nil || b.leafMaster == nil || t.closed || t.unknown {
		return ErrRecoveryRequired
	}
	m := b.leafMaster
	if m.control != control || m.transition != t || control.authority != b.authority || control.io != b.authority.controlIO || m.pump == nil {
		return ErrRecoveryRequired
	}
	return m.pump.ack(ctx, record)
}

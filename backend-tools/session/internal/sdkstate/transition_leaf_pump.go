package sdkstate

import (
	"context"
	"sync"
)

// Closed private control DATA, never native ownership by itself. Each sender
// and receiver still binds the actual leaf/master and native kernel objects.
type ColdLeafOfferRecord struct {
	Offer, Leaf uint64
	Kind        string
	Process     uint64
	PID         uint32
	Creation    uint64
}
type ColdLeafAckRecord struct {
	Offer, Leaf uint64
	Kind        string
	Process     uint64
}

func (a ColdLeafAckRecord) Valid() bool {
	if a.Offer == 0 || a.Leaf == 0 {
		return false
	}
	switch a.Kind {
	case "closed":
		return a.Process == 0
	case "birth", "stopped":
		return a.Process != 0 && a.Process != uint64(^uintptr(0)) && uint64(uintptr(a.Process)) == a.Process
	default:
		return false
	}
}

type coldLANPumpNative interface {
	validateLeafOperation(context.Context, *LANObservationLeaf, string) error
	describeLeafOperation(context.Context, *coldLANLeafOperation) (ColdLeafOfferRecord, error)
	acknowledgeLeafOperation(context.Context, *coldLANLeafOperation, ColdLeafAckRecord) error
}
type coldLANLeafOperation struct {
	owner                                     context.Context
	leaf                                      *LANObservationLeaf
	kind                                      string
	serial                                    uint64
	offered, ackAttempted, completed, unknown bool
	done                                      chan error // exactly one completion; caller cancellation never discards owner
}

// This is a mailbox of the existing authenticated control owner, not another
// transport worker. It never performs I/O and never holds mu across native/Root
// checks. The actual native master is the only production constructor/caller.
type coldLANLeafPump struct {
	mu      sync.Mutex
	factory *LANObservationFactory
	native  coldLANPumpNative
	serial  uint64
	pending *coldLANLeafOperation
	unknown bool
}

func (s *coldLANLeafPump) request(ctx context.Context, leaf *LANObservationLeaf, kind string) error {
	if s == nil || ctx == nil || ctx.Err() != nil || leaf == nil || leaf.factory == nil || leaf.factory != s.factory || s.native == nil {
		return ErrColdLeaf
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return ErrColdLeaf
	}
	switch kind {
	case "birth", "stopped", "closed":
	default:
		return ErrColdLeaf
	}
	f := leaf.factory
	f.mu.Lock()
	_, claimed := f.active[leaf]
	valid := f.identity == f && claimed && leaf.id != 0
	f.mu.Unlock()
	if !valid || s.native.validateLeafOperation(ctx, leaf, kind) != nil {
		return ErrColdLeaf
	}
	s.mu.Lock()
	if s.unknown || s.pending != nil || s.serial == ^uint64(0) {
		s.mu.Unlock()
		return ErrColdLeaf
	}
	s.serial++
	op := &coldLANLeafOperation{owner: ctx, leaf: leaf, kind: kind, serial: s.serial, done: make(chan error, 1)}
	s.pending = op // actual leaf stays reachable before any transport publication
	s.mu.Unlock()
	select {
	case err := <-op.done:
		return err
	case <-ctx.Done():
		s.mu.Lock()
		completed := op.completed && !op.unknown
		s.mu.Unlock()
		if completed {
			return nil
		} // actual positive native ACK already won the race
		s.fail(op)
		return ErrColdLeaf
	}
}

func (s *coldLANLeafPump) fail(op *coldLANLeafOperation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unknown = true
	if op != nil {
		op.unknown = true
		select {
		case op.done <- ErrColdLeaf:
		default:
		}
	}
	// No deletion of a pending/unknown native leaf or automatic offer retry.
}

func (s *coldLANLeafPump) next(ctx context.Context) (*ColdLeafOfferRecord, error) {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return nil, ErrColdLeaf
	}
	s.mu.Lock()
	if s.unknown {
		s.mu.Unlock()
		return nil, ErrColdLeaf
	}
	op := s.pending
	if op == nil {
		s.mu.Unlock()
		return nil, nil
	}
	if op.unknown || op.offered || op.ackAttempted {
		s.mu.Unlock()
		s.fail(op)
		return nil, ErrColdLeaf
	}
	op.offered = true // one actual publication; no poll/replay of a lost offer
	s.mu.Unlock()
	record, err := s.native.describeLeafOperation(ctx, op)
	if err != nil {
		s.fail(op)
		return nil, ErrColdLeaf
	}
	s.mu.Lock()
	valid := !s.unknown && !op.unknown && s.pending == op
	s.mu.Unlock()
	if !valid {
		return nil, ErrColdLeaf
	}
	record.Offer, record.Leaf, record.Kind = op.serial, op.leaf.id, op.kind
	return &record, nil
}

func (s *coldLANLeafPump) ack(ctx context.Context, ack ColdLeafAckRecord) error {
	if s == nil || ctx == nil || ctx.Err() != nil {
		return ErrColdLeaf
	}
	s.mu.Lock()
	op := s.pending
	if s.unknown || op == nil || op.unknown || !op.offered || op.ackAttempted || ack.Offer != op.serial || ack.Leaf != op.leaf.id || ack.Kind != op.kind {
		s.mu.Unlock()
		s.fail(op)
		return ErrColdLeaf
	}
	op.ackAttempted = true // BEFORE native duplicate/validation; never replay unknown
	s.mu.Unlock()
	if s.native.acknowledgeLeafOperation(ctx, op, ack) != nil {
		s.fail(op)
		return ErrColdLeaf
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unknown || op.unknown || s.pending != op {
		return ErrColdLeaf
	}
	if op.owner.Err() != nil {
		s.unknown, op.unknown = true, true
		select {
		case op.done <- ErrColdLeaf:
		default:
		}
		return ErrColdLeaf
	}
	op.completed = true
	s.pending = nil
	op.done <- nil
	return nil
}

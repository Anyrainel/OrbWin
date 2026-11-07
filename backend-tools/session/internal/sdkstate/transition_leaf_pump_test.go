package sdkstate

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// Actual mailbox coordinator with native adapter edges only. These tests do
// not construct native roots/masters, execute leaves, or issue cohort grants.
type coldPumpNativeFake struct {
	describeErr, ackErr error
	acks                atomic.Int32
	ackHook             func()
}

func (*coldPumpNativeFake) validateLeafOperation(context.Context, *LANObservationLeaf, string) error {
	return nil
}
func (n *coldPumpNativeFake) describeLeafOperation(context.Context, *coldLANLeafOperation) (ColdLeafOfferRecord, error) {
	return ColdLeafOfferRecord{Process: 711, PID: 12, Creation: 24}, n.describeErr
}
func (n *coldPumpNativeFake) acknowledgeLeafOperation(context.Context, *coldLANLeafOperation, ColdLeafAckRecord) error {
	n.acks.Add(1)
	if n.ackHook != nil {
		n.ackHook()
	}
	return n.ackErr
}

func TestColdLeafPumpDeadlineDuringNativeAckCannotBecomeLateResumeAdmission(t *testing.T) {
	s, leaf, native := coldPumpFixture()
	owner, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.request(owner, leaf, "birth") }()
	awaitColdPumpPending(t, s)
	frame, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	record, err := s.next(frame)
	if err != nil {
		t.Fatal(err)
	}
	native.ackHook = cancel // native output may exist, but owner interval expired
	if s.ack(frame, ColdLeafAckRecord{Offer: record.Offer, Leaf: record.Leaf, Kind: record.Kind, Process: 812}) == nil {
		t.Fatal("late native result admitted Resume")
	}
	if err := <-done; err == nil {
		t.Fatal("expired native owner reported positive custody")
	}
	s.mu.Lock()
	unknown, pending := s.unknown, s.pending
	s.mu.Unlock()
	if !unknown || pending == nil {
		t.Fatal("late native result dropped actual unknown graph")
	}
}

func coldPumpFixture() (*coldLANLeafPump, *LANObservationLeaf, *coldPumpNativeFake) {
	f, _, _ := newColdLeafCoordinatorFake()
	leaf := &LANObservationLeaf{factory: f, id: 1}
	f.active[leaf] = struct{}{}
	n := &coldPumpNativeFake{}
	return &coldLANLeafPump{factory: f, native: n}, leaf, n
}
func awaitColdPumpPending(t *testing.T, s *coldLANLeafPump) {
	t.Helper()
	until := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		pending := s.pending != nil
		s.mu.Unlock()
		if pending {
			return
		}
		if time.Now().After(until) {
			t.Fatal("actual pending leaf not held")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestColdLeafPumpPublishesActualPendingClaimOnceAndPositiveAckWinsCancellation(t *testing.T) {
	s, leaf, native := coldPumpFixture()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.request(ctx, leaf, "birth") }()
	awaitColdPumpPending(t, s)
	record, err := s.next(ctx)
	if err != nil || record == nil || record.Offer != 1 || record.Leaf != leaf.id || record.Kind != "birth" {
		t.Fatal("actual claim lost during publication", record, err)
	}
	if err := s.ack(ctx, ColdLeafAckRecord{Offer: record.Offer, Leaf: record.Leaf, Kind: record.Kind, Process: 812}); err != nil {
		t.Fatal(err)
	}
	cancel() // completion is recorded under the SAME owner lock before this race
	if err := <-done; err != nil || native.acks.Load() != 1 {
		t.Fatal("completed native acceptance became false unknown", err)
	}
	s.mu.Lock()
	pending, unknown := s.pending, s.unknown
	s.mu.Unlock()
	if pending != nil || unknown {
		t.Fatal("positive actual mailbox did not settle")
	}
}

func TestColdLeafPumpUnknownRetainsActualClaimAndNeverReplays(t *testing.T) {
	for _, mode := range []string{"describe", "ack", "cancel", "wrong-ack", "duplicate-offer"} {
		t.Run(mode, func(t *testing.T) {
			s, leaf, native := coldPumpFixture()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- s.request(ctx, leaf, "birth") }()
			awaitColdPumpPending(t, s)
			if mode == "describe" {
				native.describeErr = errors.New(mode)
			}
			record, err := s.next(ctx)
			if mode == "describe" {
				if err == nil {
					t.Fatal("native description error ignored")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				ack := ColdLeafAckRecord{Offer: record.Offer, Leaf: record.Leaf, Kind: record.Kind, Process: 812}
				switch mode {
				case "ack":
					native.ackErr = errors.New(mode)
					if s.ack(ctx, ack) == nil {
						t.Fatal("native ACK failure accepted")
					}
				case "wrong-ack":
					ack.Leaf++
					if s.ack(ctx, ack) == nil {
						t.Fatal("foreign leaf ACK accepted")
					}
				case "cancel":
					cancel()
				case "duplicate-offer":
					if _, err := s.next(ctx); err == nil {
						t.Fatal("lost offer replayed")
					}
				}
			}
			if err := <-done; err == nil {
				t.Fatal("unknown actual custody became success")
			}
			s.mu.Lock()
			pending, unknown := s.pending, s.unknown
			s.mu.Unlock()
			if pending == nil || pending.leaf != leaf || !unknown {
				t.Fatal("actual unknown leaf graph dropped")
			}
			owner, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			before := native.acks.Load()
			if _, err := s.next(owner); err == nil {
				t.Fatal("unknown offer republished")
			}
			if s.ack(owner, ColdLeafAckRecord{Offer: 1, Leaf: 1, Kind: "birth", Process: 812}) == nil || native.acks.Load() != before {
				t.Fatal("unknown native acceptance retried")
			}
		})
	}
}

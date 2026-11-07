package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func awaitColdDrain(t *testing.T, drain *coldServiceDrain) (bool, error) {
	t.Helper()
	until := time.Now().Add(time.Second)
	for {
		joined, err := drain.poll()
		if joined || err != nil {
			return joined, err
		}
		if time.Now().After(until) {
			t.Fatal("actual drain coordinator did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestColdDrainPromptSealKeepsSamePumpAvailableDuringActualPendingJoin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	drain := &coldServiceDrain{}
	entered, retained := make(chan struct{}), make(chan struct{})
	var seals, joins atomic.Int32
	if err := drain.begin(ctx, func() error { seals.Add(1); return nil }, func(owner context.Context) error {
		joins.Add(1)
		close(entered)
		<-retained // the SAME control pump must still deliver pending custody
		return owner.Err()
	}); err != nil {
		t.Fatal(err)
	}
	<-entered
	if joined, err := drain.poll(); joined || err != nil || drain.requireJoined() == nil {
		t.Fatal("pending owner counted as actual join", joined, err)
	}
	cancel() // cancellation of the completed frame must not cancel native owner
	close(retained)
	if joined, err := awaitColdDrain(t, drain); !joined || err != nil || drain.requireJoined() != nil {
		t.Fatal("actual owner did not finish after same-pump acceptance", err)
	}
	next, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if drain.begin(next, func() error { seals.Add(1); return nil }, func(context.Context) error { joins.Add(1); return nil }) == nil || seals.Load() != 1 || joins.Load() != 1 {
		t.Fatal("completed actual operation was replayed")
	}
}

func TestColdDrainErrorsAndPanicCannotCacheNilJoinOrReplay(t *testing.T) {
	for _, mode := range []string{"seal-error", "seal-panic", "join-error", "join-panic"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			drain := &coldServiceDrain{}
			var joins atomic.Int32
			err := drain.begin(ctx, func() error {
				if mode == "seal-panic" {
					panic("seal")
				}
				if mode == "seal-error" {
					return errors.New(mode)
				}
				return nil
			}, func(context.Context) error {
				joins.Add(1)
				if mode == "join-panic" {
					panic("join")
				}
				return errors.New(mode)
			})
			if mode == "seal-error" || mode == "seal-panic" {
				if err == nil || joins.Load() != 0 {
					t.Fatal("failed seal entered join")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if joined, err := awaitColdDrain(t, drain); joined || err == nil || drain.requireJoined() == nil {
				t.Fatal("unwind cached false completed join")
			}
			before := joins.Load()
			if drain.begin(ctx, func() error { return nil }, func(context.Context) error { joins.Add(1); return nil }) == nil || joins.Load() != before {
				t.Fatal("unknown operation replayed")
			}
		})
	}
}

func TestColdDrainCancelledBeforeAdmissionHasNoEffectsAndMayStartLater(t *testing.T) {
	drain := &coldServiceDrain{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	if drain.begin(ctx, func() error { calls++; return nil }, func(context.Context) error { calls++; return nil }) == nil || calls != 0 || drain.started {
		t.Fatal("known pre-admission cancellation started or poisoned drain")
	}
	owner, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := drain.begin(owner, func() error { return nil }, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if joined, err := awaitColdDrain(t, drain); !joined || err != nil {
		t.Fatal(err)
	}
}

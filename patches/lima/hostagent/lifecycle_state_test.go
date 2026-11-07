// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package hostagent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestRoutineSealJoinsRegisteredInitialization(t *testing.T) {
	var g routineGroup
	entered := make(chan struct{})
	release := make(chan struct{})
	var stack cleanupStack
	if !g.start(func() { close(entered); <-release; stack.add(func() error { return nil }) }) {
		t.Fatal("admission")
	}
	<-entered
	g.seal()
	if g.start(func() { t.Error("late birth") }) {
		t.Fatal("sealed birth")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if g.join(cancelled) == nil {
		t.Fatal("unjoined initialization accepted")
	}
	close(release)
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if g.join(ctx) != nil || stack.close() != nil {
		t.Fatal("original initializer/callback not joined")
	}
}
func TestCleanupLIFOOutsideLockAndStopsUnknown(t *testing.T) {
	for _, panicClose := range []bool{false, true} {
		var s cleanupStack
		var order []int
		s.add(func() error { order = append(order, 1); return nil })
		s.add(func() error {
			order = append(order, 2)
			if panicClose {
				panic("controlled")
			}
			return errors.New("controlled")
		})
		s.add(func() error { order = append(order, 3); s.mu.Lock(); s.mu.Unlock(); return nil })
		if s.close() == nil || s.close() == nil || !reflect.DeepEqual(order, []int{3, 2}) {
			t.Fatal("suffix/retry", order)
		}
	}
}
func TestLateCleanupRegistrationCannotBecomePositive(t *testing.T) {
	var s cleanupStack
	effects := 0
	s.add(func() error { effects++; return nil })
	s.add(func() error {
		if s.add(func() error { effects++; return nil }) {
			t.Fatal("late admitted")
		}
		return nil
	})
	if s.close() == nil || s.close() == nil || effects != 0 {
		t.Fatal("late suffix/positive", effects)
	}
}
func TestReadinessRetryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(waitRequirementRetry(ctx, time.Hour), context.Canceled) {
		t.Fatal("sleep ignored cancellation")
	}
}

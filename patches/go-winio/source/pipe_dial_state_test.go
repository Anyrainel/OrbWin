package winio

import (
	"errors"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
)

func TestOriginalDialBusyOnlyCompletedNoOutput(t *testing.T) {
	invalid := ^uintptr(0)
	for _, v := range []struct {
		raw  uintptr
		err  error
		busy bool
	}{{invalid, syscall.Errno(231), true}, {0, syscall.Errno(231), false}, {7, syscall.Errno(231), false}, {invalid, syscall.Errno(997), false}, {invalid, nil, false}, {invalid, errors.New("pipe busy"), false}} {
		if originalDialBusy(v.raw, invalid, v.err) != v.busy {
			t.Fatalf("raw=%x error=%v", v.raw, v.err)
		}
	}
}
func TestOriginalDialRawAdmission(t *testing.T) {
	invalid := ^uintptr(0)
	for _, h := range []uintptr{0, invalid, invalid - 1, invalid - 2} {
		if originalDialHandle(h, invalid) {
			t.Fatalf("pseudo/empty %x", h)
		}
	}
	if !originalDialHandle(7, invalid) || !originalDialBOOL(1) || originalDialBOOL(uintptr(1)<<32) {
		t.Fatal("native width admission")
	}
	if !originalDialAssociation(7, 7) || originalDialAssociation(8, 7) || originalDialAssociation(0, 0) {
		t.Fatal("association identity")
	}
}
func TestOriginalDialCancellationCuts(t *testing.T) {
	// Pre-effect cancellation and late cancellation after an exact settled open
	// may settle once. An entered but unreturned call never admits cleanup.
	for _, after := range []bool{false, true} {
		s := originalDialPhase{}
		if after {
			s.enter()
			s.complete()
		}
		if !s.beginClose() {
			t.Fatal("settled rejection")
		}
		s.closedReply(nil)
		if !s.closed || s.closeErr != nil || s.beginClose() {
			t.Fatal("replay")
		}
	}
	s := originalDialPhase{}
	s.enter()
	if s.beginClose() {
		t.Fatal("pending cleanup")
	}
	if s.enter() || !s.unknown {
		t.Fatal("unknown retry")
	}
}
func TestOriginalDialNoSuffixAfterEachUnknownCut(t *testing.T) {
	for cut := 0; cut < 4; cut++ {
		s := originalDialPhase{}
		effects := 0
		for phase := 0; phase < 4; phase++ {
			if !s.enter() {
				break
			}
			effects++
			if phase == cut {
				s.unknown = true
				break
			}
			s.complete()
		}
		if effects != cut+1 || s.beginClose() {
			t.Fatalf("suffix at %d", cut)
		}
	}
}
func TestOriginalDialCloseFailureSticky(t *testing.T) {
	failure := errors.New("checked original close failed")
	s := originalDialPhase{}
	s.enter()
	s.complete()
	if !s.beginClose() {
		t.Fatal("admission")
	}
	s.closedReply(failure)
	if s.closeErr != failure || !s.unknown || s.closed || s.beginClose() {
		t.Fatal("lost failure")
	}
	s.closedReply(nil)
	if s.closed || s.closeErr != failure {
		t.Fatal("failure reset")
	}
	s = originalDialPhase{handedOff: true}
	if s.beginClose() {
		t.Fatal("second closer after successful facade")
	}
}
func TestOriginalSingletonConsumedOnceNeverDefaults(t *testing.T) {
	for _, panicAt := range []int{0, 1, 2, 3} {
		var once sync.Once
		s := originalIOInitState{}
		var calls atomic.Int32
		var joins sync.WaitGroup
		init := func() {
			defer func() {
				if recover() != nil {
					s.unknown = true
				}
			}()
			calls.Add(1)
			s.begin()
			if panicAt == 0 {
				panic("call")
			}
			if !s.publish(7, ^uintptr(0)) {
				t.Error("publish")
			}
			if panicAt == 1 {
				panic("returned")
			}
			s.processor = true
			if panicAt == 2 {
				panic("processor")
			}
			s.ready = true
			s.err = nil
		}
		for i := 0; i < 32; i++ {
			joins.Add(1)
			go func() {
				defer joins.Done()
				once.Do(init)
				if (s.result() == nil) != (panicAt == 3) {
					t.Error("consumed Once gave incorrect cached result")
				}
			}()
		}
		joins.Wait()
		if calls.Load() != 1 {
			t.Fatal("second init")
		}
	}
}
func TestOriginalSingletonRejectsNullPseudoAndUnentered(t *testing.T) {
	for _, raw := range []uintptr{0, ^uintptr(0), ^uintptr(0) - 1} {
		s := originalIOInitState{}
		s.begin()
		if s.publish(raw, ^uintptr(0)) || s.result() == nil {
			t.Fatal("bad port")
		}
	}
	s := originalIOInitState{}
	if s.result() == nil || s.publish(7, ^uintptr(0)) {
		t.Fatal("unentered init")
	}
}

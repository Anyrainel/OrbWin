package landatagram

import (
	"testing"
	"time"
)

type policyTimer struct {
	f       func()
	stopped bool
	delay   time.Duration
}

func (t *policyTimer) Stop() bool { was := !t.stopped; t.stopped = true; return was }
func policyFixture() (*ProcessWatchdog, *[]*policyTimer, *int) {
	now := time.Unix(10, 0)
	timers := []*policyTimer{}
	exits := new(int)
	w := newProcessWatchdog(func() time.Time { return now }, func(d time.Duration, f func()) watchdogTimer {
		t := &policyTimer{f: f, delay: d}
		timers = append(timers, t)
		return t
	}, func() { *exits++ })
	return w, &timers, exits
}
func TestWatchdogOnlyActiveIOAndStaleResetCannotTerminate(t *testing.T) {
	w, timers, exits := policyFixture()
	_ = w.SetWriteDeadline(time.Unix(11, 0))
	if len(*timers) != 0 {
		t.Fatal("idle write armed")
	}
	end, e := w.BeginWrite()
	if e != nil {
		t.Fatal(e)
	}
	old := (*timers)[0]
	_ = w.SetWriteDeadline(time.Unix(12, 0))
	latest := (*timers)[1]
	old.f()
	if *exits != 0 {
		t.Fatal("stale reset killed")
	}
	end()
	latest.f()
	if *exits != 0 {
		t.Fatal("completed IO killed")
	}
	_, _ = w.BeginWrite()
	(*timers)[2].f()
	if *exits != 1 {
		t.Fatal("expiry lost")
	}
	if e = w.SetReadDeadline(time.Time{}); e == nil {
		t.Fatal("terminal revived")
	}
	old.f()
	if *exits != 1 {
		t.Fatal("multiple termination")
	}
}
func TestWatchdogCloseRetainsFinalGuardUntilActualJoin(t *testing.T) {
	w, timers, exits := policyFixture()
	end, _ := w.BeginRead()
	w.StartClose()
	closeArm := (*timers)[1]
	end()
	if closeArm.stopped || closeArm.delay != 2*time.Second {
		t.Fatal("Close return cancelled final guard")
	}
	closeArm.f()
	if *exits != 1 {
		t.Fatal("blocked pump not terminated")
	}
	w2, timers2, exits2 := policyFixture()
	_, _ = w2.BeginRead()
	w2.StartClose()
	w2.Joined()
	for _, timer := range *timers2 {
		timer.f()
	}
	if *exits2 != 0 {
		t.Fatal("joined process killed")
	}
}

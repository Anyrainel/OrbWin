package lanhelper

import (
	"testing"
	"time"
)

func TestLANHelperWatchdogNeverEquatesDeadlineOrLostParentWithJoinedProvider(t *testing.T) {
	now := time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name            string
		deadline, pulse time.Time
		held, exit      bool
	}{
		{"healthy", now.Add(time.Second), now.Add(-time.Second), true, false},
		{"parent-lost", now.Add(time.Second), now, false, true},
		{"heartbeat-expired", now.Add(time.Minute), now.Add(-6 * time.Second), true, true},
		{"deadline", now, now, true, true},
		{"future-pulse", now.Add(time.Minute), now.Add(time.Second), true, true},
		{"missing-pulse", now.Add(time.Minute), time.Time{}, true, true},
	} {
		if got := watchdogMustExit(now, test.deadline, test.pulse, test.held); got != test.exit {
			t.Fatal(test.name, got)
		}
	}
}

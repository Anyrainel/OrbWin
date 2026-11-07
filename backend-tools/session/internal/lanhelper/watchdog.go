package lanhelper

import "time"

// This pure decision is NOT native parent/pipe evidence. The one-shot helper
// supplies held native liveness and its own local pulse receipt time.
func watchdogMustExit(now, deadline, lastPulse time.Time, parentHeld bool) bool {
	return !parentHeld || !now.Before(deadline) || lastPulse.IsZero() || now.Before(lastPulse) || now.Sub(lastPulse) > 5*time.Second
}

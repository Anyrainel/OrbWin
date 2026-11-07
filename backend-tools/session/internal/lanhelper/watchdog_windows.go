//go:build windows

package lanhelper

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// ONLY the fixed early read-only SDK child role may use this cap. It must never
// wrap the long-lived SDK server. Parent death cannot leave a blocked MI child.
func StartReadOnlyObservationCap() func() bool {
	timer := time.AfterFunc(10*time.Second, func() { os.Exit(125) })
	return timer.Stop
}

// Called only by the elevated one-shot entrypoint. This process terminates
// ITSELF: an ordinary manager need not have terminate rights over an alternate
// administrator process. A broker/provider may complete late after this exit;
// the previously flushed protected dispatch intent remains unknown/no replay.
// The caller must stop/join this observer before closing the native parent.
func startNativeProcessCap(deadline time.Time) func() {
	timer := time.AfterFunc(time.Until(deadline), func() { os.Exit(125) })
	return func() { timer.Stop() }
}
func startNativeWatchdog(ctx context.Context, manager *NativeManager, lastPulse *atomic.Pointer[time.Time], deadline time.Time) func() {
	stop, joined := make(chan struct{}), make(chan struct{})
	var once sync.Once
	go func() {
		defer close(joined)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				os.Exit(125)
			case now := <-ticker.C:
				pulse := lastPulse.Load()
				var stamp time.Time
				if pulse != nil {
					stamp = *pulse
				}
				if watchdogMustExit(now, deadline, stamp, manager.checkPipe(manager.pipe) == nil) {
					os.Exit(125)
				}
			}
		}
	}()
	return func() { once.Do(func() { close(stop); <-joined }) }
}

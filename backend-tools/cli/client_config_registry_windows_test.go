package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise the actual acquisition coordinator with local retention state and
// data-only owner placeholders. No native API, handle, lease or grant is used.
func TestNativeConfigRegistryReentrantFailureAndPanicRetainWithoutDeadlock(t *testing.T) {
	for _, mode := range []string{"error", "panic", "fenced-success"} {
		t.Run(mode, func(t *testing.T) {
			r := &nativeConfigRegistry{}
			n := &nativeClientConfig{}
			done := make(chan struct{})
			var result error
			var panicSeen bool
			go func() {
				defer close(done)
				defer func() { panicSeen = recover() != nil }()
				_, result = r.acquire(n, func() (*clientConfigGuard, error) {
					if mode == "panic" {
						// No earlier fence: the coordinator's panic path must
						// establish both poison and registry failure itself.
						panic("synthetic post-effect uncertainty")
					}
					// This is the same reentry as a native publication error's
					// successorFence. The reservation mutex must be released.
					r.fence()
					switch mode {
					case "fenced-success":
						return &clientConfigGuard{native: n}, nil
					default:
						return nil, errClientConfigRecovery
					}
				})
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("acquisition held registry mutex across reentrant failure")
			}
			if panicSeen != (mode == "panic") || (!panicSeen && !errors.Is(result, errClientConfigRecovery)) {
				t.Fatal("uncertainty became a successful acquisition")
			}
			r.Lock()
			retained := r.failed && len(r.roots) == 1 && r.roots[0] == n && n.poison
			r.Unlock()
			if !retained {
				t.Fatal("failure lost the strongly retained owner or admission fence")
			}
			called := false
			if _, err := r.acquire(&nativeClientConfig{}, func() (*clientConfigGuard, error) {
				called = true
				return nil, nil
			}); err == nil || called {
				t.Fatal("uncertainty admitted another acquisition")
			}
		})
	}
}

func TestNativeConfigRegistryBudgetReservationDoesNotSerializeNativeWork(t *testing.T) {
	r := &nativeConfigRegistry{}
	entered := make(chan struct{}, 16)
	release := make(chan struct{})
	var count atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n := &nativeClientConfig{}
			_, _ = r.acquire(n, func() (*clientConfigGuard, error) {
				count.Add(1)
				entered <- struct{}{}
				<-release
				return &clientConfigGuard{native: n}, nil
			})
		}()
	}
	for i := 0; i < 16; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("registry serialized the owned work or failed to reserve its bound")
		}
	}
	r.Lock()
	if len(r.roots) != 16 || r.failed {
		r.Unlock()
		close(release)
		t.Fatal("retention capacity was not reserved atomically")
	}
	r.Unlock()
	close(release)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bounded owned work did not return")
	}
	if count.Load() != 16 {
		t.Fatal("retention capacity admitted too many callbacks")
	}
}

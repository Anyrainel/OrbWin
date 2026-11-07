package localnet

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// These lifecycle fixtures own only channels and counters. The actual actor's
// terminal path runs, with an already-cancelled context preventing inventory,
// native storage, listener or engine calls. Unexpected interface calls panic.
type runtimeLANShutdownHelper struct {
	LANAdministrativeHelper
	started chan struct{}
	joined  chan struct{}
	start   sync.Once
	release sync.Once
	calls   atomic.Int32
	err     error
}

func (h *runtimeLANShutdownHelper) Close(ctx context.Context) error {
	h.calls.Add(1)
	h.start.Do(func() { close(h.started) })
	select {
	case <-h.joined:
		return h.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (h *runtimeLANShutdownHelper) join() { h.release.Do(func() { close(h.joined) }) }

type runtimeLANShutdownStore struct {
	serviceStateStore
	calls atomic.Int32
	err   error
}

func (s *runtimeLANShutdownStore) Close() error { s.calls.Add(1); return s.err }

func runtimeLANShutdownFixture(t *testing.T, helper *runtimeLANShutdownHelper, store *runtimeLANShutdownStore, generation *serviceGeneration) (*ServiceRuntime, chan struct{}, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ledger := &NamespaceLedger{stateStore: store}
	r := &ServiceRuntime{
		opts:          RuntimeOptions{Ledger: ledger, LAN: &LANAdapters{Helper: helper}},
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
		cleanupResult: make(chan struct{}),
		generation:    generation,
	}
	returned := make(chan struct{})
	start := func() { go func() { r.loop(); close(returned) }() }
	t.Cleanup(func() {
		// Only these exact inert objects may use test-only channel settlement.
		// Production has no reset/release path for an uncertain native owner.
		if r.opts.Ledger != ledger || ledger.stateStore != store || ledger.lease != nil || r.opts.LAN == nil || r.opts.LAN.Helper != helper {
			t.Error("test cleanup refused a runtime with non-fixture owners")
			return
		}
		helper.join()
		if generation != nil {
			if host, ok := generation.shutdown.host.(*delayedGenerationOwner); ok {
				select {
				case <-host.joined:
				default:
					close(host.joined)
				}
			} else if _, ok := generation.shutdown.host.(*terminalGenerationOwner); !ok {
				t.Error("test cleanup refused a generation with non-fixture owners")
				return
			}
			if generation.shutdown.lan != nil || generation.shutdown.backend != nil {
				t.Error("test cleanup refused unexpected generation owners")
				return
			}
		}
		select {
		case <-r.cleanupResult:
		case <-time.After(time.Second):
			t.Error("inert cleanup did not return")
			return
		}
		select {
		case <-r.done:
		default:
			close(r.done) // Settle only the exact inert failed fixture, after assertions.
		}
		select {
		case <-returned:
		case <-time.After(time.Second):
			t.Error("inert actor retention goroutine did not settle")
		}
	})
	return r, returned, start
}

func newRuntimeLANShutdownHelper() *runtimeLANShutdownHelper {
	return &runtimeLANShutdownHelper{started: make(chan struct{}), joined: make(chan struct{})}
}

func TestRuntimeLANShutdownDrainsWorkAndGenerationBeforeHelperAndLedger(t *testing.T) {
	helper := newRuntimeLANShutdownHelper()
	store := &runtimeLANShutdownStore{}
	host := &delayedGenerationOwner{joined: make(chan struct{})}
	generation := &serviceGeneration{shutdown: newGenerationShutdown(host, nil, nil)}
	r, _, start := runtimeLANShutdownFixture(t, helper, store, generation)
	r.lanQueries.Add(1)
	r.lanWork.Add(1)
	var queryDone, workDone sync.Once
	releaseQuery := func() { queryDone.Do(r.lanQueries.Done) }
	releaseWork := func() { workDone.Do(r.lanWork.Done) }
	t.Cleanup(releaseQuery)
	t.Cleanup(releaseWork)
	start()
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if !errors.Is(r.Close(short), context.DeadlineExceeded) || host.closes.Load() != 0 || helper.calls.Load() != 0 || store.calls.Load() != 0 {
		t.Fatal("pending observation lost its runtime/helper/ledger ownership")
	}
	releaseQuery()
	short2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if !errors.Is(r.Close(short2), context.DeadlineExceeded) || host.closes.Load() != 0 {
		t.Fatal("pending operation worker was not drained before generation shutdown")
	}
	releaseWork()
	deadline := time.After(time.Second)
	for host.closes.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("generation shutdown did not begin")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if helper.calls.Load() != 0 || store.calls.Load() != 0 {
		t.Fatal("administrative helper or ledger closed before generation join")
	}
	close(host.joined)
	select {
	case <-helper.started:
	case <-time.After(time.Second):
		t.Fatal("runtime never closed its administrative helper")
	}
	short3, cancel3 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel3()
	if !errors.Is(r.Close(short3), context.DeadlineExceeded) || store.calls.Load() != 0 || helper.calls.Load() != 1 {
		t.Fatal("helper close was retried or ledger released before its positive result")
	}
	helper.join()
	bounded, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := r.Close(bounded); err != nil {
		t.Fatal(err)
	}
	if err := r.Wait(bounded); err != nil || helper.calls.Load() != 1 || host.closes.Load() != 1 || store.calls.Load() != 1 {
		t.Fatal("positive shutdown did not settle each original owner exactly once", err)
	}
	snapshot, err := r.Snapshot()
	if err != nil || snapshot.Phase != "stopped" {
		t.Fatal("positive resource closure was not published", snapshot, err)
	}
}

func TestRuntimeLANShutdownFailureReturnsErrorButRetainsPositiveWaitBarrier(t *testing.T) {
	for _, failure := range []string{"helper", "ledger", "generation"} {
		t.Run(failure, func(t *testing.T) {
			cause := errors.New(failure + " native outcome unconfirmed")
			helper := newRuntimeLANShutdownHelper()
			helper.join()
			store := &runtimeLANShutdownStore{}
			var generation *serviceGeneration
			switch failure {
			case "helper":
				helper.err = cause
			case "ledger":
				store.err = cause
			case "generation":
				generation = &serviceGeneration{shutdown: newGenerationShutdown(&terminalGenerationOwner{failure: cause}, nil, nil)}
			}
			r, returned, start := runtimeLANShutdownFixture(t, helper, store, generation)
			start()
			bounded, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := r.Close(bounded); !errors.Is(err, cause) {
				t.Fatal("Close did not return the original cleanup error promptly", err)
			}
			snapshot, err := r.Snapshot()
			if err != nil || snapshot.Phase != "cleanup-pending" || !strings.Contains(snapshot.Error, cause.Error()) {
				t.Fatal("unknown cleanup was published as stopped or lost its cause", snapshot, err)
			}
			select {
			case <-returned:
				t.Fatal("original actor stopped retaining its uncertain runtime graph")
			default:
			}
			short, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer stop()
			if !errors.Is(r.Wait(short), context.DeadlineExceeded) {
				t.Fatal("positive Wait could release the ordinary caller's SDK state root")
			}
			if !errors.Is(r.Close(bounded), cause) {
				t.Fatal("repeat Close erased the retained failure")
			}
			wantHelper, wantStore := int32(1), int32(0)
			if failure == "generation" {
				wantHelper = 0
			}
			if failure == "ledger" {
				wantStore = 1
			}
			if helper.calls.Load() != wantHelper || store.calls.Load() != wantStore {
				t.Fatal("shutdown retried cleanup or advanced past an uncertain owner")
			}
			r.opts.Ledger.mu.Lock()
			closed := r.opts.Ledger.closed
			r.opts.Ledger.mu.Unlock()
			if failure != "ledger" && closed {
				t.Fatal("helper/generation failure released the ledger")
			}
			if failure == "generation" && r.generation != generation {
				t.Fatal("generation failure lost its exact original owner")
			}
		})
	}
}

func TestRuntimeLANShutdownGenerationCloseLeavesSharedAdministrativeHelperOpen(t *testing.T) {
	helper := newRuntimeLANShutdownHelper()
	controller := newLANController("inert-installation", nil, &LANAdapters{Helper: helper})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := controller.Close(ctx); err != nil || helper.calls.Load() != 0 {
		t.Fatal("replaceable generation closed the runtime's shared administrative helper", err)
	}
}

func TestRuntimeLANShutdownCloseAcceptsExistingPositiveBarrier(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	joined := make(chan struct{})
	close(joined)
	// Existing inert holders may expose an already-positive barrier without
	// constructing a cleanup worker. Even a cancelled caller observes that join.
	r := &ServiceRuntime{cancel: cancel, done: joined}
	if err := r.Close(ctx); err != nil {
		t.Fatal("an existing positive join became a cleanup-result timeout", err)
	}
}

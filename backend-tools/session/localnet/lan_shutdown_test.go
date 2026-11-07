package localnet

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type delayedGenerationOwner struct {
	joined chan struct{}
	closes atomic.Int32
}

func (o *delayedGenerationOwner) Close(context.Context) error { o.closes.Add(1); return nil }
func (o *delayedGenerationOwner) Wait(ctx context.Context) error {
	select {
	case <-o.joined:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func TestGenerationShutdownDeadlineRetainsOwnerUntilLateChildJoin(t *testing.T) {
	lan := &delayedGenerationOwner{joined: make(chan struct{})}
	host := &delayedGenerationOwner{joined: make(chan struct{})}
	backend := &delayedGenerationOwner{joined: make(chan struct{})}
	s := newGenerationShutdown(host, lan, backend)
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if !errors.Is(s.Close(short), context.DeadlineExceeded) || host.closes.Load() != 0 {
		t.Fatal("host/ledger release preceded LAN positive join")
	}
	close(lan.joined)
	deadline := time.After(time.Second)
	for host.closes.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("host Close was never initiated after LAN join")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	short2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if s.Close(short2) == nil || host.closes.Load() != 1 || lan.closes.Load() != 1 {
		t.Fatal("deadline retried shutdown or reported false stopped")
	}
	close(host.joined)
	short3, cancel3 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel3()
	if s.Wait(short3) == nil {
		t.Fatal("host completion lost common backend/ledger ownership")
	}
	close(backend.joined)
	bounded, cancel4 := context.WithTimeout(context.Background(), time.Second)
	defer cancel4()
	if s.Wait(bounded) != nil || s.Close(bounded) != nil || host.closes.Load() != 1 || lan.closes.Load() != 1 {
		t.Fatal("same shutdown owner did not complete exactly once")
	}
}

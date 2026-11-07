package localnet

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLANHTTPViewClosePreservesLANAndNewHTTPGeneration(t *testing.T) {
	base := &namespaceReferenceFake{}
	shared := newSharedNamespaces(base, nil, nil)
	lanOwner := strings.Repeat("1", 64)
	handle, err := shared.borrow(context.Background(), lanOwner, strings.Repeat("a", 64), "started")
	if err != nil {
		t.Fatal(err)
	}
	first, err := newHTTPNamespaceView(shared)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newHTTPNamespaceView(shared)
	if err != nil {
		t.Fatal(err)
	}
	if first.owner == second.owner {
		t.Fatal("HTTP generations share a reference identity")
	}
	for _, view := range []*httpNamespaceView{first, second} {
		if got, e := view.Acquire(context.Background(), handle.Intent.ContainerID, handle.Intent.StartedAt); e != nil || got != handle {
			t.Fatal("HTTP view did not share exact existing helper", e)
		}
	}
	if err = first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = second.Renew(context.Background(), handle); err != nil {
		t.Fatal("old HTTP cleanup removed new owner", err)
	}
	if err = shared.renewOwner(context.Background(), lanOwner, handle); err != nil {
		t.Fatal("HTTP cleanup removed LAN owner", err)
	}
	if _, err = first.Acquire(context.Background(), handle.Intent.ContainerID, handle.Intent.StartedAt); err == nil {
		t.Fatal("closed HTTP view reacquired helper")
	}
	if err = second.Close(context.Background()); err != nil || base.releases.Load() != 0 || base.closes.Load() != 0 || base.acquires.Load() != 1 {
		t.Fatal("HTTP views changed physical LAN backend", err)
	}
	if err = shared.release(context.Background(), lanOwner, handle); err != nil || base.releases.Load() != 1 {
		t.Fatal("LAN final reference did not release once", err)
	}
	if err = shared.Close(context.Background()); err != nil || base.closes.Load() != 1 {
		t.Fatal("common backend did not close once", err)
	}
}
func TestLANHTTPViewCloseJoinsLateAcquireWithoutAbandoningSharedOwner(t *testing.T) {
	base := &namespaceReferenceFake{acquireEntered: make(chan struct{}), acquireReturn: make(chan struct{})}
	shared := newSharedNamespaces(base, nil, nil)
	view, err := newHTTPNamespaceView(shared)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan error, 1)
	go func() {
		_, err := view.Acquire(context.Background(), strings.Repeat("a", 64), "started")
		acquired <- err
	}()
	<-base.acquireEntered
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if view.Close(short) == nil || base.releases.Load() != 0 || base.closes.Load() != 0 {
		t.Fatal("late HTTP acquire abandoned or shared backend closed")
	}
	cancel()
	close(base.acquireReturn)
	if err = <-acquired; err != nil {
		t.Fatal(err)
	}
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err = view.Wait(ctx); err != nil || base.releases.Load() != 1 || base.closes.Load() != 0 {
		t.Fatal("late acquire did not join exact HTTP release", err)
	}
	if err = shared.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

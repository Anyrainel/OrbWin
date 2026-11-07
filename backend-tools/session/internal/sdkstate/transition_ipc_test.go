package sdkstate

import (
	"context"
	"errors"
	"net"
	"testing"
)

type sdkColdIPCListener struct {
	closes int
	err    error
}

func (l *sdkColdIPCListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (l *sdkColdIPCListener) Addr() net.Addr            { return nil }
func (l *sdkColdIPCListener) Close() error              { l.closes++; return l.err }

func TestSDKColdIPCReusesOriginalListenerAndJoinsOnce(t *testing.T) {
	original := &sdkColdIPCListener{}
	s := &coldIPCState{listener: &coldIPCListener{Listener: original}}
	lease := &ColdIPCLease{s}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	run := func(listener net.Listener) error {
		called = true
		if listener.(*coldIPCListener).Listener != original {
			t.Fatal("replaced native listener")
		}
		if listener.Close() != nil || listener.Close() != nil {
			t.Fatal("close")
		}
		return context.Canceled
	}
	if lease.Serve(ctx, run) == nil || called || s.claimed {
		t.Fatal("cancelled preflight consumed listener")
	}
	if !errors.Is(lease.Serve(context.Background(), run), context.Canceled) || !s.joined || s.unknown || s.serving || original.closes != 1 {
		t.Fatal("original listener did not close positively once")
	}
	called = false
	if lease.Serve(context.Background(), run) == nil || called || original.closes != 1 {
		t.Fatal("late handoff rebound/reused listener")
	}
}

func TestSDKColdIPCUnknownOriginalCloseNeverAdmitsRelease(t *testing.T) {
	original := &sdkColdIPCListener{err: errors.New("lost native close reply")}
	s := &coldIPCState{listener: &coldIPCListener{Listener: original}}
	lease := &ColdIPCLease{s}
	if lease.Serve(context.Background(), func(net.Listener) error { return nil }) == nil || !s.unknown || s.joined {
		t.Fatal("unknown close admitted release")
	}
	original.err = nil
	if lease.Serve(context.Background(), func(net.Listener) error { t.Fatal("unknown listener retried"); return nil }) == nil || original.closes != 1 {
		t.Fatal("unknown native close retried")
	}
}

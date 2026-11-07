//go:build windows

package localnet

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

type coldIPCJoinedListener struct {
	mu         sync.Mutex
	connection net.Conn
	accepted   chan struct{}
	closed     chan struct{}
	once       sync.Once
}

func (l *coldIPCJoinedListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	c := l.connection
	l.connection = nil
	l.mu.Unlock()
	if c != nil {
		close(l.accepted)
		return c, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}
func (l *coldIPCJoinedListener) Close() error   { l.once.Do(func() { close(l.closed) }); return nil }
func (l *coldIPCJoinedListener) Addr() net.Addr { return nil }

func TestColdServiceIPCSharedLoopCancellationJoinsAcceptedWorker(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	listener := &coldIPCJoinedListener{connection: server, accepted: make(chan struct{}), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &ServiceRuntime{done: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- serveServiceIPCListener(ctx, runtime, listener) }()
	select {
	case <-listener.accepted:
	case <-time.After(time.Second):
		t.Fatal("shared server did not use original listener")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server returned without prompt cancellation/worker join")
	}
	if _, err := client.Write([]byte{0}); err == nil {
		t.Fatal("accepted connection survived joined return")
	}
}

func TestColdServiceIPCSharedLoopInvalidRuntimeRetiresOwnedListener(t *testing.T) {
	listener := &coldIPCJoinedListener{accepted: make(chan struct{}), closed: make(chan struct{})}
	if serveServiceIPCListener(context.Background(), nil, listener) == nil {
		t.Fatal("absent runtime admitted")
	}
	select {
	case <-listener.closed:
	default:
		t.Fatal("invalid serving request leaked its original listener")
	}
}

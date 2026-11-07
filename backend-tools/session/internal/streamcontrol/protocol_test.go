package streamcontrol

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T, limit int) (*Server, func() net.Conn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := newServer(listener, limit)
	done := make(chan error, 1)
	go func() { done <- server.Serve() }()
	t.Cleanup(func() {
		server.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(time.Second):
			t.Error("control accept loop did not stop")
		}
	})
	return server, func() net.Conn {
		connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		return connection
	}
}
func nonce(index int) Token { token, _ := ParseToken(fmt.Sprintf("%032x", index)); return token }
func TestRevocationWaitsForRegisteredResourcesAndRefusesLateStarts(t *testing.T) {
	server, dial := fixture(t, HistoryLimit)
	connection := dial()
	released := make(chan struct{})
	terminate := make(chan struct{})
	lease, err := Register(context.Background(), connection, nonce(1), func() { <-terminate; close(released); connection.Close() })
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- Revoke(context.Background(), dial(), nonce(1)) }()
	select {
	case err := <-done:
		t.Fatalf("revocation acknowledged before resources were closed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(terminate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("completed revocation was not acknowledged")
	}
	select {
	case <-released:
	default:
		t.Fatal("revocation preceded resource teardown")
	}
	_ = lease.Close()
	if err := Revoke(context.Background(), dial(), nonce(2)); err != nil {
		t.Fatal(err)
	}
	if late, err := Register(context.Background(), dial(), nonce(2), func() { t.Error("late nonce was admitted") }); err == nil {
		late.Close()
		t.Fatal("revoked delayed start was admitted")
	}
	if again, err := Register(context.Background(), dial(), nonce(1), func() {}); err == nil {
		again.Close()
		t.Fatal("completed nonce was replayed")
	}
	server.mu.Lock()
	active, seen := len(server.active), len(server.seen)
	server.mu.Unlock()
	if active != 0 || seen != 2 {
		t.Fatalf("incorrect retained ownership: active=%d seen=%d", active, seen)
	}
}
func TestNormalCompletionAndRepeatedCancellationRemainBounded(t *testing.T) {
	server, dial := fixture(t, HistoryLimit)
	for index := range 100 {
		connection := dial()
		token := nonce(index + 1)
		lease, err := Register(context.Background(), connection, token, func() { connection.Close() })
		if err != nil {
			t.Fatal(err)
		}
		if index%2 == 0 {
			if err := lease.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if err := Revoke(context.Background(), dial(), token); err != nil {
			t.Fatal(err)
		}
		_ = lease.Close()
		if err := Revoke(context.Background(), dial(), token); err != nil {
			t.Fatal("idempotent revocation failed", err)
		}
	}
	server.mu.Lock()
	active, seen := len(server.active), len(server.seen)
	server.mu.Unlock()
	if active != 0 || seen != 100 {
		t.Fatalf("stream ownership accumulated: %d/%d", active, seen)
	}
}
func TestHistoryExhaustionFailsClosedAndReportsUnhealthy(t *testing.T) {
	_, dial := fixture(t, 3)
	for index := range 3 {
		if err := Revoke(context.Background(), dial(), nonce(index+1)); err != nil {
			t.Fatal(err)
		}
	}
	if err := Healthy(context.Background(), dial()); err == nil {
		t.Fatal("exhausted history renewed a healthy helper")
	}
	if err := Revoke(context.Background(), dial(), nonce(4)); err == nil {
		t.Fatal("unrecorded nonce was acknowledged at capacity")
	}
	if lease, err := Register(context.Background(), dial(), nonce(4), func() {}); err == nil {
		lease.Close()
		t.Fatal("history exhaustion admitted an untracked stream")
	}
	if err := Revoke(context.Background(), dial(), nonce(1)); err != nil {
		t.Fatal("known revoked nonce stopped being idempotent at capacity")
	}
}
func TestControlLossInvokesOnlyItsOwnedTerminationCallback(t *testing.T) {
	server, dial := fixture(t, HistoryLimit)
	connection := dial()
	var terminated atomic.Int32
	done := make(chan struct{})
	lease, err := Register(context.Background(), connection, nonce(1), func() { terminated.Add(1); connection.Close(); close(done) })
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("control loss did not terminate the owned dial")
	}
	_ = lease.Close()
	if terminated.Load() != 1 {
		t.Fatal("termination callback ran more than once")
	}
}
func TestCancelledRegistrationCannotAdmitTargetIO(t *testing.T) {
	_, dial := fixture(t, HistoryLimit)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if lease, err := Register(ctx, dial(), nonce(1), func() { t.Error("cancelled registration watcher started") }); err == nil {
		lease.Close()
		t.Fatal("cancelled setup returned a usable lease")
	}
}

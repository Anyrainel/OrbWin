package localnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"orbwin.local/session/internal/sdkself"
	"strings"
	"sync"
	"testing"
	"time"
)

type sdkPeerFixture struct {
	identity                              sdkself.Identity
	calls, closed                         int
	failBefore, changeAfter, closeFailure bool
}

func (p *sdkPeerFixture) Identity() (sdkself.Identity, error) {
	p.calls++
	if p.failBefore {
		return sdkself.Identity{}, errors.New("private native detail")
	}
	i := p.identity
	if p.changeAfter && p.calls > 1 {
		i.PID++
	}
	return i, nil
}
func (p *sdkPeerFixture) Close() error {
	p.closed++
	if p.closeFailure {
		return errors.New("close unknown")
	}
	return nil
}

type sdkConnFixture struct {
	read           *bytes.Reader
	writes, closed int
	deadlineError  bool
}

type delayedSDKCloseFixture struct {
	*sdkConnFixture
	once             sync.Once
	entered, release chan struct{}
}

func (c *delayedSDKCloseFixture) Close() error {
	c.once.Do(func() { close(c.entered); <-c.release })
	return nil
}

func TestSDKPeerLeaseRetainedUntilCancellationCloseActuallyJoins(t *testing.T) {
	base, peer := sdkPipeFixture(t, nil)
	connection := &delayedSDKCloseFixture{sdkConnFixture: base, entered: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() {
		_, err := callAuthenticatedService(ctx, connection, ServiceRequest{Version: 1, Operation: "snapshot"}, []byte("{}"), func(context.Context, net.Conn) (servicePeer, error) { return peer, nil })
		done <- err
	}()
	select {
	case <-connection.entered:
	case <-time.After(time.Second):
		t.Fatal("cancellation close did not start")
	}
	if peer.closed != 0 {
		t.Fatal("peer lease released before cancellation close joined")
	}
	select {
	case <-done:
		t.Fatal("operation returned before cancellation close joined")
	default:
	}
	close(connection.release)
	select {
	case err := <-done:
		if err == nil || peer.closed != 1 || base.writes != 0 {
			t.Fatal("cancellation dropped peer ownership or dispatched request", err)
		}
	case <-time.After(time.Second):
		t.Fatal("positive cancellation close did not release ownership")
	}
}

func (c *sdkConnFixture) Read(b []byte) (int, error)  { return c.read.Read(b) }
func (c *sdkConnFixture) Write(b []byte) (int, error) { c.writes++; return len(b), nil }
func (c *sdkConnFixture) Close() error                { c.closed++; return nil }
func (c *sdkConnFixture) LocalAddr() net.Addr         { return nil }
func (c *sdkConnFixture) RemoteAddr() net.Addr        { return nil }
func (c *sdkConnFixture) SetDeadline(time.Time) error {
	if c.deadlineError {
		return errors.New("deadline")
	}
	return nil
}
func (c *sdkConnFixture) SetReadDeadline(time.Time) error  { return nil }
func (c *sdkConnFixture) SetWriteDeadline(time.Time) error { return nil }
func sdkPipeFixture(t *testing.T, change func(*ServiceResponse)) (*sdkConnFixture, *sdkPeerFixture) {
	t.Helper()
	identity := sdkself.Identity{Schema: 1, PID: 123, CreationTime: "123456789", Session: 1, ImageSHA256: strings.Repeat("a", 64), BundleSHA256: strings.Repeat("b", 64), LeaseHeld: true}
	response := ServiceResponse{Version: 1, State: &RuntimeSnapshot{SelfIdentity: &identity}}
	if change != nil {
		change(&response)
	}
	data, _ := json.Marshal(response)
	var frame bytes.Buffer
	if writeServiceFrame(&frame, data, ipcResponseBudget) != nil {
		t.Fatal("fixture frame")
	}
	return &sdkConnFixture{read: bytes.NewReader(frame.Bytes())}, &sdkPeerFixture{identity: identity}
}
func TestSDKSelfPeerAuthenticationPrecedesEveryRequestWrite(t *testing.T) {
	for _, mode := range []string{"auth", "partial-auth", "nil-peer", "identity", "invalid-identity", "canceled", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			connection, peer := sdkPipeFixture(t, nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			if mode == "identity" {
				peer.failBefore = true
			}
			if mode == "invalid-identity" {
				peer.identity.LeaseHeld = false
			}
			if mode == "deadline" {
				connection.deadlineError = true
			}
			_, err := callAuthenticatedService(ctx, connection, ServiceRequest{Version: 1, Operation: "snapshot"}, []byte(`{"version":1,"operation":"snapshot"}`), func(context.Context, net.Conn) (servicePeer, error) {
				if mode == "auth" {
					return nil, errors.New("private detail")
				}
				if mode == "partial-auth" {
					return peer, errors.New("partial authentication")
				}
				if mode == "nil-peer" {
					return nil, nil
				}
				return peer, nil
			})
			if err == nil || connection.writes != 0 || connection.closed != 1 {
				t.Fatal("unauthenticated request dispatched or leaked")
			}
			if mode == "partial-auth" && peer.closed != 1 {
				t.Fatal("partial peer lease leaked")
			}
		})
	}
}
func TestSDKSelfReplyMustMatchSameRetainedNativePeer(t *testing.T) {
	for _, mode := range []string{"changed-state", "missing-state-identity", "changed-peer", "unknown-close", "success"} {
		t.Run(mode, func(t *testing.T) {
			connection, peer := sdkPipeFixture(t, func(response *ServiceResponse) {
				if mode == "changed-state" {
					copy := *response.State.SelfIdentity
					copy.BundleSHA256 = strings.Repeat("c", 64)
					response.State.SelfIdentity = &copy
				}
				if mode == "missing-state-identity" {
					response.State.SelfIdentity = nil
				}
			})
			peer.changeAfter = mode == "changed-peer"
			peer.closeFailure = mode == "unknown-close"
			response, err := callAuthenticatedService(context.Background(), connection, ServiceRequest{Version: 1, Operation: "snapshot"}, []byte(`{"version":1,"operation":"snapshot"}`), func(context.Context, net.Conn) (servicePeer, error) { return peer, nil })
			if mode == "success" {
				if err != nil || response.State == nil || peer.calls != 2 {
					t.Fatal(err)
				}
			} else if err == nil || response.State != nil {
				t.Fatal("mismatched/unknown peer response accepted")
			}
			if peer.closed != 1 || connection.closed != 1 {
				t.Fatal("peer/pipe retirement not joined")
			}
		})
	}
}

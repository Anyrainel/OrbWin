//go:build windows && amd64

package lanhelper

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
)

type fakeAdminChild struct {
	wait            func(context.Context) error
	releaseErr      error
	waits, releases atomic.Int32
}

func (child *fakeAdminChild) Wait(ctx context.Context) error {
	child.waits.Add(1)
	if child.wait != nil {
		return child.wait(ctx)
	}
	return nil
}
func (child *fakeAdminChild) release() error {
	child.releases.Add(1)
	return child.releaseErr
}

// Every test owner is exclusively fake: no native manager, authority, process,
// image, pipe or constructor is created. Only these exact test slots may reset.
func fakeAdminOwner(t *testing.T) (*AdministrativeClient, *adminNativeSession) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	client := &AdministrativeClient{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	client.nativeOrigin = client
	if !claimAdministrativeClient(client) {
		t.Fatal("fake administrative owner slot unexpectedly occupied")
	}
	s := &adminNativeSession{client: client, ctx: ctx, cancel: cancel, opened: make(chan struct{}), done: make(chan struct{})}
	client.current = s
	client.sessions.Add(1)
	t.Cleanup(func() {
		cancel()
		if client.manager != nil || client.authority != nil || s.launcher != nil || s.helper != nil {
			t.Fatal("test cannot reset a native administrative owner")
		}
		for _, child := range s.children {
			if _, ok := child.(*fakeAdminChild); !ok {
				t.Fatal("test cannot reset native child custody")
			}
		}
		releaseAdministrativeClient(client)
	})
	return client, s
}

func TestLANAdminChildWaitFailureDoesNotReleaseOrReportSuccessfulClose(t *testing.T) {
	client, s := fakeAdminOwner(t)
	failure := errors.New("fake native wait failure")
	child := &fakeAdminChild{wait: func(context.Context) error { return failure }}
	s.children = []adminChildLifetime{child}
	close(s.opened)
	if err := s.finish(); !errors.Is(err, failure) {
		t.Fatal("wait failure lost", err)
	}
	if child.releases.Load() != 0 || client.current != s || len(s.diagnostics) != 1 {
		t.Fatal("unknown child exit released or dropped its owner")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for range 2 {
		if err := client.Close(ctx); !errors.Is(err, failure) {
			t.Fatal("client close invented success", err)
		}
		if !errors.Is(s.finish(), failure) {
			t.Fatal("repeated settlement lost its terminal error")
		}
	}
	if child.waits.Load() != 1 || child.releases.Load() != 0 {
		t.Fatal("failed native wait was retried or released")
	}
	if _, err := NewAdministrativeClient(strings.Repeat("1", 32), strings.Repeat("2", 64)); err == nil {
		t.Fatal("unknown fake owner admitted a native successor constructor")
	}
}

func TestLANAdminChildReleaseFailureIsRetainedWithoutRetry(t *testing.T) {
	client, s := fakeAdminOwner(t)
	failure := errors.New("fake native release failure")
	child := &fakeAdminChild{releaseErr: failure}
	other := &fakeAdminChild{}
	s.children = []adminChildLifetime{child, other}
	close(s.opened)
	if !errors.Is(s.finish(), failure) || child.waits.Load() != 1 || child.releases.Load() != 1 || other.releases.Load() != 1 {
		t.Fatal("independent children were not settled exactly once")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !errors.Is(client.Close(ctx), failure) || client.current != s || len(s.diagnostics) != 1 {
		t.Fatal("failed checked release dropped the retained session")
	}
	_ = s.finish()
	_ = client.Close(ctx)
	if child.releases.Load() != 1 {
		t.Fatal("uncertain native release was retried")
	}
	administrativeClientOwner.Lock()
	retained := administrativeClientOwner.client == client
	administrativeClientOwner.Unlock()
	if !retained {
		t.Fatal("unknown client lost its bounded strong custody")
	}
}

func TestLANAdminCloseTimeoutKeepsLateOpenAndChildUntilPositiveJoin(t *testing.T) {
	client, s := fakeAdminOwner(t)
	waiting, exited := make(chan struct{}), make(chan struct{})
	child := &fakeAdminChild{wait: func(context.Context) error { close(waiting); <-exited; return nil }}
	finished := make(chan error, 1)
	go func() { finished <- s.finish() }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(client.Close(ctx), context.Canceled) {
		t.Fatal("cancelled close claimed joined ownership")
	}
	if child.releases.Load() != 0 {
		t.Fatal("late child released before its open result")
	}
	s.mu.Lock()
	s.children = append(s.children, child)
	s.mu.Unlock()
	close(s.opened)
	<-waiting
	select {
	case <-client.done:
		t.Fatal("client completed before positive child wait")
	default:
	}
	close(exited)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	joined, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := client.Close(joined); err != nil || child.releases.Load() != 1 || client.current != nil {
		t.Fatal("positive late join failed", err)
	}
	administrativeClientOwner.Lock()
	released := administrativeClientOwner.client == nil
	administrativeClientOwner.Unlock()
	if !released {
		t.Fatal("successful close retained the single admission slot")
	}
}

func TestLANAdminAcknowledgedMutationStillReturnsChildCleanupFailure(t *testing.T) {
	for _, purpose := range []string{"activate", "remove", "settle-remove", "cleanup-remove", "cleanup-settle"} {
		t.Run(purpose, func(t *testing.T) {
			client, s := fakeAdminOwner(t)
			failure := errors.New("fake acknowledged child release failure")
			s.children = []adminChildLifetime{&fakeAdminChild{releaseErr: failure}}
			sha := strings.Repeat("a", 64)
			s.prepared = Prepared{LeaseSHA: sha}
			s.record.Purpose, s.record.LeaseSHA256 = purpose, sha
			left, right := net.Pipe()
			defer right.Close()
			s.control = left
			s.ownConnection(left)
			close(s.opened)
			server := make(chan error, 1)
			go func() {
				defer right.Close()
				_ = right.SetDeadline(time.Now().Add(time.Second))
				frame, err := ReadAdmin(right)
				if err == nil {
					reply := AdminFrame{Version: 2, Sequence: 1, RequestSequence: frame.Sequence, Kind: "removed", Removal: &lanlease.RemovalAcknowledgement{LeaseSHA: sha}}
					if purpose == "activate" {
						reply.Kind, reply.Removal, reply.Rule = "committed", nil, &lanlease.RuleAcknowledgement{LeaseSHA: sha}
					}
					err = WriteAdmin(right, reply)
				}
				server <- err
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var err error
			if purpose == "activate" {
				_, err = client.CommitNamespace(ctx, s.prepared, lanfirewall.CallerIdentity{PID: 1}, lanlease.Observation{Scope: lanfirewall.Scope{ClientNetworks: []string{}}}, sha)
			} else {
				_, err = client.finishRemoval(ctx, s.prepared, purpose)
			}
			if !errors.Is(err, failure) {
				t.Fatal("acknowledged mutation hid failed child cleanup", err)
			}
			if err := <-server; err != nil {
				t.Fatal("synthetic protocol exchange failed", err)
			}
			if !errors.Is(client.Close(ctx), failure) {
				t.Fatal("client close lost the mutation cleanup error")
			}
		})
	}
}

// Fake channels/process absence only. No Windows pipe/process/API is invoked.
func TestLANAdminCancellationRetainsLateOpenUntilJoined(t *testing.T) {
	client := &AdministrativeClient{}
	ctx, cancel := context.WithCancel(context.Background())
	s := &adminNativeSession{client: client, ctx: ctx, cancel: cancel, opened: make(chan struct{}), done: make(chan struct{})}
	client.current = s
	client.sessions.Add(1)
	left, right := net.Pipe()
	defer right.Close()
	s.requestClose()
	finished := make(chan struct{})
	go func() { s.finish(); close(finished) }()
	select {
	case <-finished:
		t.Fatal("late open was treated as joined")
	case <-time.After(10 * time.Millisecond):
	}
	s.ownConnection(left)
	_ = right.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := right.Read(make([]byte, 1)); err == nil {
		t.Fatal("late authentication channel remained open")
	}
	close(s.opened)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("positive empty-child join failed")
	}
	client.sessions.Wait()
	if client.current != nil {
		t.Fatal("joined session not retired")
	}
	s.finish() // Once-only WG release; cannot underflow or revive authority.
}

func TestLANLaunchGreetingCannotCarryAuthority(t *testing.T) {
	for _, kind := range []string{"helper-ready", "received"} {
		var buffer bytes.Buffer
		if err := writeLaunch(&buffer, launchFrame{Version: 1, Kind: kind}); err != nil {
			t.Fatal(err)
		}
		frame, err := readLaunch(&buffer)
		if err != nil || frame.Kind != kind {
			t.Fatal(err)
		}
	}
	if (launchFrame{Version: 1, Kind: "helper-ready", SourceHandle: "1"}).validate() == nil {
		t.Fatal("greeting acquired unrelated native handle")
	}
}

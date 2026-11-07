package localnet

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type namespaceReferenceFake struct {
	acquires, releases, closes, renews, dials atomic.Int32
	acquireErr, releaseErr                    error
	acquireEntered, acquireReturn             chan struct{}
	dialFactory                               func() (net.Conn, error)
}

func (f *namespaceReferenceFake) Target(context.Context, string) (string, error) {
	return "started", nil
}
func (f *namespaceReferenceFake) Acquire(_ context.Context, id, started string) (NamespaceHandle, error) {
	f.acquires.Add(1)
	if f.acquireEntered != nil {
		close(f.acquireEntered)
		<-f.acquireReturn
	}
	return NamespaceHandle{ID: strings.Repeat("b", 64), Intent: NamespaceIntent{ContainerID: id, StartedAt: started}}, f.acquireErr
}
func (f *namespaceReferenceFake) AcquireLAN(ctx context.Context, id, started, leaseSHA string) (NamespaceHandle, error) {
	h, err := f.Acquire(ctx, id, started)
	h.Intent.LANLeaseSHA = leaseSHA
	h.ID = leaseSHA
	return h, err
}
func (f *namespaceReferenceFake) Release(context.Context, NamespaceHandle) error {
	f.releases.Add(1)
	return f.releaseErr
}
func (f *namespaceReferenceFake) Renew(context.Context, NamespaceHandle) error {
	f.renews.Add(1)
	return nil
}
func (f *namespaceReferenceFake) Dial(context.Context, ServiceRoute) (net.Conn, error) {
	f.dials.Add(1)
	if f.dialFactory != nil {
		return f.dialFactory()
	}
	return nil, errors.New("no real transport in reference tests")
}

type referenceMemoryConn struct{ closeErr error }

func (*referenceMemoryConn) Read([]byte) (int, error)         { return 0, errors.New("no real IO") }
func (*referenceMemoryConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *referenceMemoryConn) Close() error                   { return c.closeErr }
func (*referenceMemoryConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (*referenceMemoryConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (*referenceMemoryConn) SetDeadline(time.Time) error      { return nil }
func (*referenceMemoryConn) SetReadDeadline(time.Time) error  { return nil }
func (*referenceMemoryConn) SetWriteDeadline(time.Time) error { return nil }
func (*referenceMemoryConn) CloseWrite() error                { return nil }

func TestLANHelperQuotaSharedAcrossLeasesAndUnknownRevocationRetainsSlot(t *testing.T) {
	f := &namespaceReferenceFake{dialFactory: func() (net.Conn, error) { return &referenceMemoryConn{}, nil }}
	s := newSharedNamespaces(f, nil, nil)
	a, b := strings.Repeat("c", 64), strings.Repeat("d", 64)
	h := referenceBorrow(t, s, a)
	referenceBorrow(t, s, b)
	referenceBorrow(t, s, httpNamespaceOwner)
	r := ServiceRoute{ContainerID: h.Intent.ContainerID, StartedAt: h.Intent.StartedAt, HelperID: h.ID, Port: 80}
	connections := make([]net.Conn, 0, maxLANNamespaceStreams)
	for i := 0; i < maxLANNamespaceStreams; i++ {
		owner := a
		if i%2 == 1 {
			owner = b
		}
		conn, e := s.dialOwner(context.Background(), owner, r)
		if e != nil {
			t.Fatal(e)
		}
		connections = append(connections, conn)
	}
	if _, e := s.dialOwner(context.Background(), b, r); e == nil || f.dials.Load() != maxLANNamespaceStreams {
		t.Fatal("per-lease quota allowed helper exhaustion or dispatched full-capacity exec")
	}
	// Existing HTTP admission stays independent; no secretly changed HTTP limit.
	http, e := s.Dial(context.Background(), r)
	if e != nil {
		t.Fatal(e)
	}
	http.Close()
	if s.release(context.Background(), a, h) == nil {
		t.Fatal("owner released live stream references")
	}
	if connections[0].Close() != nil {
		t.Fatal("close")
	}
	replacement, e := s.dialOwner(context.Background(), a, r)
	if e != nil {
		t.Fatal("positive revocation did not free a slot")
	}
	replacement.Close()
	// A failed close retains its slot, poisons all leases sharing this helper,
	// and is never retried as a new deletion or silently reusable capacity.
	bad := connections[1].(*lanReferenceConn)
	bad.Conn.(*referenceMemoryConn).closeErr = errors.New("revocation acknowledgement lost")
	if bad.Close() == nil {
		t.Fatal("unknown revocation became success")
	}
	before := f.dials.Load()
	if _, e = s.dialOwner(context.Background(), a, r); e == nil || f.dials.Load() != before {
		t.Fatal("poisoned helper admitted fresh exec")
	}
	for _, c := range connections {
		_ = c.Close()
	}
	if s.release(context.Background(), a, h) != nil || s.Release(context.Background(), h) != nil {
		t.Fatal("nonlast closed references could not relinquish local ownership")
	}
	if s.release(context.Background(), b, h) == nil || f.releases.Load() != 0 {
		t.Fatal("unknown stream caused delete fallback")
	}
}
func (f *namespaceReferenceFake) Close(context.Context) error { f.closes.Add(1); return nil }
func referenceBorrow(t *testing.T, s *sharedNamespaces, owner string) NamespaceHandle {
	t.Helper()
	h, e := s.borrow(context.Background(), owner, strings.Repeat("a", 64), "started")
	if e != nil {
		t.Fatal(e)
	}
	return h
}
func TestLANAndHTTPShareOneNamespaceUntilEitherLastOwner(t *testing.T) {
	for _, first := range []string{httpNamespaceOwner, strings.Repeat("c", 64)} {
		t.Run(first, func(t *testing.T) {
			f := &namespaceReferenceFake{}
			s := newSharedNamespaces(f, nil, nil)
			lan := strings.Repeat("c", 64)
			h := referenceBorrow(t, s, httpNamespaceOwner)
			if got := referenceBorrow(t, s, lan); got != h {
				t.Fatal("second physical namespace")
			}
			// Re-reading one owner's reference does not invent additional ownership.
			if referenceBorrow(t, s, lan) != h || f.acquires.Load() != 1 {
				t.Fatal("non-idempotent borrow")
			}
			second := lan
			if first == lan {
				second = httpNamespaceOwner
			}
			if s.release(context.Background(), first, h) != nil || f.releases.Load() != 0 {
				t.Fatal("first owner destroyed shared namespace")
			}
			if s.renewOwner(context.Background(), second, h) != nil {
				t.Fatal("remaining owner lost namespace")
			}
			if s.release(context.Background(), second, h) != nil || f.releases.Load() != 1 {
				t.Fatal("last owner did not release once")
			}
			if s.Close(context.Background()) != nil || f.closes.Load() != 1 {
				t.Fatal("close")
			}
		})
	}
}
func TestPartialAcquireDoesNotInventReferenceOrDelete(t *testing.T) {
	f := &namespaceReferenceFake{acquireErr: errors.New("unknown create")}
	s := newSharedNamespaces(f, nil, nil)
	if _, e := s.Acquire(context.Background(), strings.Repeat("a", 64), "started"); e == nil {
		t.Fatal("partial acquire became ownership")
	}
	if len(s.refs) != 0 || f.releases.Load() != 0 {
		t.Fatal("invented cleanup of unknown create")
	}
	if s.Close(context.Background()) != nil || f.closes.Load() != 1 {
		t.Fatal("existing backend retains unknown-create ledger")
	}
}
func TestUnknownReleaseCannotBeReborrowedRenewedDeletedOrCloseOwner(t *testing.T) {
	f := &namespaceReferenceFake{releaseErr: errors.New("lost delete acknowledgement")}
	settlements := 0
	settleErr := errors.New("ledger flush failed")
	s := newSharedNamespaces(f, func(context.Context, NamespaceHandle) error { settlements++; return settleErr }, nil)
	owner := strings.Repeat("c", 64)
	h := referenceBorrow(t, s, owner)
	if s.release(context.Background(), owner, h) == nil {
		t.Fatal("unknown release became success")
	}
	if _, e := s.borrow(context.Background(), owner, h.Intent.ContainerID, h.Intent.StartedAt); e == nil {
		t.Fatal("revived unknown helper")
	}
	if s.Renew(context.Background(), h) == nil || s.release(context.Background(), owner, h) == nil || f.releases.Load() != 1 || f.renews.Load() != 0 {
		t.Fatal("retried uncertain mutation/renew")
	}
	r := ServiceRoute{ContainerID: h.Intent.ContainerID, StartedAt: h.Intent.StartedAt, HelperID: h.ID, Port: 80}
	if _, e := s.dialOwner(context.Background(), owner, r); e == nil || f.dials.Load() != 0 {
		t.Fatal("dial revived unknown helper")
	}
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if s.Close(short) == nil || f.closes.Load() != 0 {
		t.Fatal("destroyed retained common owner")
	}
	if s.settleAbsent(context.Background(), owner, h) == nil || len(s.refs) != 1 {
		t.Fatal("failed durable settlement dropped ownership")
	}
	settleErr = nil
	if s.settleAbsent(context.Background(), owner, h) != nil {
		t.Fatal("positive exact absence did not settle")
	}
	bounded, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if s.Wait(bounded) != nil || settlements != 2 || f.closes.Load() != 1 || f.releases.Load() != 1 {
		t.Fatal("settlement retried deletion/close")
	}
}
func TestCloseDeadlineRetainsInFlightBorrowAndLaterClosesExactlyOnce(t *testing.T) {
	f := &namespaceReferenceFake{acquireEntered: make(chan struct{}), acquireReturn: make(chan struct{})}
	s := newSharedNamespaces(f, nil, nil)
	borrow := make(chan NamespaceHandle, 1)
	go func() { h, _ := s.Acquire(context.Background(), strings.Repeat("a", 64), "started"); borrow <- h }()
	<-f.acquireEntered
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if s.Close(short) == nil || f.closes.Load() != 0 {
		t.Fatal("inflight acquire escaped shutdown")
	}
	close(f.acquireReturn)
	h := <-borrow
	if _, e := s.Acquire(context.Background(), h.Intent.ContainerID, h.Intent.StartedAt); e == nil {
		t.Fatal("close admitted a new borrow")
	}
	if s.Release(context.Background(), h) != nil {
		t.Fatal("shutdown lost acknowledged borrower")
	}
	bounded, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if s.Wait(bounded) != nil || s.Close(bounded) != nil || f.closes.Load() != 1 || f.releases.Load() != 1 {
		t.Fatal("not one retained shutdown worker")
	}
}

func TestLANAndHTTPPendingDialsKeepParallelismAndDelayLastRelease(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	f := &namespaceReferenceFake{dialFactory: func() (net.Conn, error) { entered <- struct{}{}; <-release; return &referenceMemoryConn{}, nil }}
	s := newSharedNamespaces(f, nil, nil)
	h := referenceBorrow(t, s, httpNamespaceOwner)
	r := ServiceRoute{ContainerID: h.Intent.ContainerID, StartedAt: h.Intent.StartedAt, HelperID: h.ID, Port: 80}
	results := make(chan net.Conn, 2)
	for i := 0; i < 2; i++ {
		go func() { c, _ := s.Dial(context.Background(), r); results <- c }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("shared bookkeeping serialized existing HTTP setup")
		}
	}
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if s.Release(short, h) == nil || f.releases.Load() != 0 {
		t.Fatal("last release escaped pending setup ownership")
	}
	close(release)
	for i := 0; i < 2; i++ {
		c := <-results
		if c == nil {
			t.Fatal("live parallel HTTP stream refused")
		}
		c.Close()
	}
	if s.Release(context.Background(), h) != nil || s.Close(context.Background()) != nil || f.releases.Load() != 1 {
		t.Fatal("settled setup did not permit single release")
	}
}

func TestLANHelperQuotaIncludesPendingSetupAndCloseJoinsLateResults(t *testing.T) {
	entered := make(chan struct{}, maxLANNamespaceStreams)
	release := make(chan struct{})
	f := &namespaceReferenceFake{dialFactory: func() (net.Conn, error) { entered <- struct{}{}; <-release; return &referenceMemoryConn{}, nil }}
	s := newSharedNamespaces(f, nil, nil)
	owner := strings.Repeat("c", 64)
	h := referenceBorrow(t, s, owner)
	r := ServiceRoute{ContainerID: h.Intent.ContainerID, StartedAt: h.Intent.StartedAt, HelperID: h.ID, Port: 80}
	results := make(chan error, maxLANNamespaceStreams)
	for i := 0; i < maxLANNamespaceStreams; i++ {
		go func() {
			conn, e := s.dialOwner(context.Background(), owner, r)
			if conn != nil {
				conn.Close()
			}
			results <- e
		}()
	}
	for i := 0; i < maxLANNamespaceStreams; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("pending setup was serialized or lost")
		}
	}
	if _, e := s.dialOwner(context.Background(), owner, r); e == nil || f.dials.Load() != maxLANNamespaceStreams {
		t.Fatal("pending execs did not count against shared quota")
	}
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if s.Close(short) == nil || f.closes.Load() != 0 {
		t.Fatal("pending setup lost common owner")
	}
	close(release)
	for i := 0; i < maxLANNamespaceStreams; i++ {
		if <-results == nil {
			t.Fatal("late dial became live after close")
		}
	}
	if s.release(context.Background(), owner, h) != nil {
		t.Fatal("closed late streams left invented live references")
	}
	bounded, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if s.Wait(bounded) != nil || f.closes.Load() != 1 {
		t.Fatal("existing close worker did not join")
	}
}

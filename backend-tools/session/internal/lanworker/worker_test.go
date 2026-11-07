package lanworker

import (
	"context"
	"errors"
	"io"
	"net"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
	"strings"
	"sync"
	"testing"
	"time"
)

type testTimer struct {
	clock   *testClock
	due     time.Time
	ch      chan time.Time
	stopped bool
}

func (t *testTimer) C() <-chan time.Time { return t.ch }
func (t *testTimer) Stop()               { t.clock.mu.Lock(); t.stopped = true; t.clock.mu.Unlock() }

type testClock struct {
	mu         sync.Mutex
	now        time.Time
	timers     []*testTimer
	registered chan struct{}
}

func (c *testClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *testClock) TimerAt(deadline time.Time) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &testTimer{clock: c, due: deadline, ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	if !c.now.Before(deadline) {
		t.ch <- c.now
		t.stopped = true
	}
	select {
	case c.registered <- struct{}{}:
	default:
	}
	return t
}
func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for _, t := range c.timers {
		if !t.stopped && !c.now.Before(t.due) {
			select {
			case t.ch <- c.now:
			default:
			}
			t.stopped = true
		}
	}
}

type testListener struct {
	incoming chan net.Conn
	closed   chan struct{}
	once     sync.Once
}

func (l *testListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.incoming:
		return c, nil
	case <-l.closed:
		return nil, io.EOF
	}
}
func (l *testListener) Close() error { l.once.Do(func() { close(l.closed) }); return nil }
func (l *testListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("192.168.4.10"), Port: 58080}
}

type memoryConn struct {
	net.Conn
	remote string
}

func (c memoryConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP(c.remote), Port: 60000}
}
func (c memoryConn) SetDeadline(time.Time) error { return nil } // Deterministic clock owns expiry in these fake-IO tests.
type workerFixture struct {
	core        *Core
	clock       *testClock
	listener    *testListener
	bindings    Bindings
	grant       lanhelper.Frame
	listenCalls int
	dialled     chan net.Conn
}

func makeWorker(t *testing.T, mutate func(*lanlease.Spec), native func(Bindings) func(context.Context, lanfirewall.Scope) (Bindings, error)) *workerFixture {
	t.Helper()
	now := time.Now().UTC()
	manager := lanfirewall.CallerIdentity{UserSID: "S-1-5-21-1-2-3-1001", PID: 100, CreationTime: "100", ImageSHA256: strings.Repeat("f", 64)}
	child := manager
	child.PID = 101
	child.CreationTime = "200"
	child.ImageSHA256 = strings.Repeat("e", 64)
	request := lanfirewall.Request{Version: 1, Operation: "create", Installation: strings.Repeat("a", 32), UserSID: manager.UserSID, Transaction: strings.Repeat("b", 32), Nonce: strings.Repeat("d", 64), ReviewSHA256: strings.Repeat("9", 64), CallerPID: manager.PID, CallerCreationTime: manager.CreationTime, CallerImageSHA256: manager.ImageSHA256, IssuedAt: now.Add(-time.Second).Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano), Scope: lanfirewall.Scope{Protocol: "tcp", LocalAddress: "192.168.4.10", LocalPort: 58080, InterfaceGUID: "{11111111-1111-1111-1111-111111111111}", InterfaceLUID: "42", Profile: "private", ClientNetworks: []string{"192.168.4.0/24"}, WorkerSHA256: child.ImageSHA256}}
	spec := lanlease.Spec{Request: request, Publication: lanlease.Publication{OwnerInstance: strings.Repeat("7", 32), Daemon: "owned", ContainerID: strings.Repeat("a", 64), StartedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), Generation: 7, HostPort: 58080, ContainerPort: 8080, Protocol: "tcp"}, LeaseID: strings.Repeat("c", 32), WorkerRoot: `C:\ProgramData\OrbWin\LAN`}
	spec.Worker = lanfirewall.WorkerBinding{Path: spec.WorkerRoot + `\` + request.Installation + `\` + spec.LeaseID + `\orbwin-session.exe`, FileID: strings.Repeat("b", 32), VolumeSerial: "17", SHA256: child.ImageSHA256, InterfaceAlias: "Ethernet"}
	if mutate != nil {
		mutate(&spec)
	}
	f := &workerFixture{clock: &testClock{now: now, registered: make(chan struct{}, 16)}, listener: &testListener{incoming: make(chan net.Conn, 20), closed: make(chan struct{})}, bindings: Bindings{manager, child, spec.Worker}, dialled: make(chan net.Conn, 20)}
	nativeCheck := func(context.Context, lanfirewall.Scope) (Bindings, error) { return f.bindings, nil }
	if native != nil {
		nativeCheck = native(f.bindings)
	}
	deps := Dependencies{Clock: f.clock, NativeCheck: nativeCheck, Listen: func(context.Context, string) (net.Listener, error) { f.listenCalls++; return f.listener, nil }, Dial: func(ctx context.Context, _ lanhelper.Stream) (net.Conn, error) {
		a, b := net.Pipe()
		select {
		case f.dialled <- b:
			return memoryConn{a, "127.0.0.1"}, nil
		case <-ctx.Done():
			a.Close()
			b.Close()
			return nil, ctx.Err()
		}
	}}
	core, e := New(spec, f.bindings, deps)
	if e != nil {
		t.Fatal(e)
	}
	f.core = core
	t.Cleanup(func() {
		core.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if core.Wait(ctx) != nil {
			t.Error("worker did not join")
		}
		for len(f.dialled) > 0 {
			(<-f.dialled).Close()
		}
	})
	observed := lanlease.Observation{ReadAt: now, Manager: manager, Worker: child, WorkerFile: spec.Worker, Publication: spec.Publication, Scope: spec.Request.Scope, ManagerPipePID: manager.PID, WorkerPipePID: child.PID, OwnerLeaseHeld: true, ManagerAlive: true, WorkerAlive: true, WorkerFileHeld: true, ProtectedPath: true, PublicationActive: true, InterfacePresent: true}
	ack := lanlease.RuleAcknowledgement{ReadAt: now, LeaseSHA: core.LeaseSHA(), ReviewedScopeSHA: spec.Request.Scope.Digest(), NativeKeysSHA: strings.Repeat("1", 64), TypedSnapshotSHA: strings.Repeat("2", 64), Sequence: 2, Outcome: "created", TerminalSuccess: true, OperationClosed: true, ProtectedReceipt: true, AcknowledgementFlushed: true, RuleProperties: 39, FilterFamilies: 7, ReverseIdentities: 7}
	f.grant = lanhelper.Frame{Version: 2, Kind: "worker-grant", Sequence: 1, LeaseSHA: core.LeaseSHA(), Grant: &lanhelper.Grant{Observation: observed, Rule: ack}}
	return f
}
func grantWorker(t *testing.T, f *workerFixture) {
	t.Helper()
	if e := f.core.Apply(context.Background(), f.grant); e != nil {
		t.Fatal(e)
	}
}
func activeStream(t *testing.T, f *workerFixture) (net.Conn, net.Conn) {
	t.Helper()
	inside, peer := net.Pipe()
	f.listener.incoming <- memoryConn{inside, "192.168.4.20"}
	select {
	case tunnel := <-f.dialled:
		t.Cleanup(func() { peer.Close(); tunnel.Close() })
		return peer, tunnel
	case <-time.After(time.Second):
		t.Fatal("owned tunnel not opened")
		return nil, nil
	}
}
func awaitClosed(t *testing.T, c *Core) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := c.Wait(ctx); e != nil {
		t.Fatal("worker did not close/join active flows", e)
	}
}
func TestDefaultClosedAndUnqualifiedCommitNeverCallListenerFactory(t *testing.T) {
	f := makeWorker(t, nil, nil)
	f.grant.Grant.Rule.AcknowledgementFlushed = false
	if f.core.Apply(context.Background(), f.grant) == nil || f.listenCalls != 0 {
		t.Fatal("unacknowledged creation opened ingress")
	}
	awaitClosed(t, f.core)
}
func TestStalledObservationClosesAlreadyActiveStreamsAndJoins(t *testing.T) {
	f := makeWorker(t, nil, nil)
	grantWorker(t, f)
	peer, tunnel := activeStream(t, f)
	go func() { _, _ = peer.Write([]byte("alive")) }()
	buf := make([]byte, 5)
	if _, e := io.ReadFull(tunnel, buf); e != nil || string(buf) != "alive" {
		t.Fatal("raw stream did not forward")
	}
	f.clock.advance(6 * time.Second)
	awaitClosed(t, f.core)
	peer.SetReadDeadline(time.Now().Add(time.Second))
	if _, e := peer.Read(make([]byte, 1)); e == nil {
		t.Fatal("expired active client remained open")
	}
	select {
	case <-f.listener.closed:
	default:
		t.Fatal("expired listener remained open")
	}
}
func TestBlockedNativeRefreshCannotHoldExpiryClosureLock(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	calls := 0
	f := makeWorker(t, nil, func(binding Bindings) func(context.Context, lanfirewall.Scope) (Bindings, error) {
		return func(context.Context, lanfirewall.Scope) (Bindings, error) {
			calls++
			if calls == 2 {
				close(entered)
				<-release
			}
			return binding, nil
		}
	})
	grantWorker(t, f)
	activeStream(t, f)
	refresh := lanhelper.Frame{Version: 2, Kind: "refresh", Sequence: 2, LeaseSHA: f.core.LeaseSHA(), Observation: &f.grant.Grant.Observation}
	done := make(chan error, 1)
	go func() { done <- f.core.Apply(context.Background(), refresh) }()
	<-entered
	f.clock.advance(6 * time.Second)
	// Ingress closes independently, but a blocked native check remains owned:
	// requesting Stop is not a positive join of that unfinished Apply call.
	f.core.Stop()
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if f.core.Wait(short) == nil {
		t.Fatal("blocked native Apply escaped shutdown join")
	}
	cancel()
	close(release)
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("late refresh reopened expired worker")
		}
	case <-time.After(time.Second):
		t.Fatal("refresh failed to settle")
	}
	awaitClosed(t, f.core)
}
func TestLateBlockedListenerRemainsOwnedUntilReturnedAndClosed(t *testing.T) {
	f := makeWorker(t, nil, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	f.core.deps.Listen = func(context.Context, string) (net.Listener, error) { close(entered); <-release; return f.listener, nil }
	apply := make(chan error, 1)
	go func() { apply <- f.core.Apply(context.Background(), f.grant) }()
	<-entered
	f.clock.advance(6 * time.Second)
	f.core.Stop()
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if f.core.Wait(short) == nil {
		t.Fatal("pending bind reported positive shutdown join")
	}
	cancel()
	close(release)
	select {
	case e := <-apply:
		if e == nil {
			t.Fatal("late listener became active after lease stop")
		}
	case <-time.After(time.Second):
		t.Fatal("late bind did not settle")
	}
	awaitClosed(t, f.core)
	select {
	case <-f.listener.closed:
	default:
		t.Fatal("late listener lost without close")
	}
	if f.core.Apply(context.Background(), f.grant) == nil {
		t.Fatal("shutdown admitted new work after join")
	}
}
func TestChangedPublicationAndReplayCloseExistingFlows(t *testing.T) {
	for _, kind := range []string{"generation", "replay"} {
		t.Run(kind, func(t *testing.T) {
			f := makeWorker(t, nil, nil)
			grantWorker(t, f)
			activeStream(t, f)
			o := f.grant.Grant.Observation
			sequence := uint64(2)
			if kind == "generation" {
				o.Publication.Generation++
			} else {
				sequence = 1
			}
			frame := lanhelper.Frame{Version: 2, Kind: "refresh", Sequence: sequence, LeaseSHA: f.core.LeaseSHA(), Observation: &o}
			if f.core.Apply(context.Background(), frame) == nil {
				t.Fatal("changed/replayed control accepted")
			}
			awaitClosed(t, f.core)
		})
	}
}
func TestUnqualifiedUDPTransportNeverClaimsReadyOrBinds(t *testing.T) {
	f := makeWorker(t, func(s *lanlease.Spec) { s.Request.Scope.Protocol = "udp"; s.Publication.Protocol = "udp" }, nil)
	if !errors.Is(f.core.Apply(context.Background(), f.grant), ErrUDPTransport) || f.listenCalls != 0 {
		t.Fatal("missing datagram ABI became TCP fallback")
	}
	awaitClosed(t, f.core)
}

func TestPartialListenerFailureClosesReturnedResourceBeforeJoin(t *testing.T) {
	f := makeWorker(t, nil, nil)
	f.core.deps.Listen = func(context.Context, string) (net.Listener, error) {
		return f.listener, errors.New("partial bind")
	}
	if f.core.Apply(context.Background(), f.grant) == nil {
		t.Fatal("partial bind became ready")
	}
	awaitClosed(t, f.core)
	select {
	case <-f.listener.closed:
	default:
		t.Fatal("returned listener escaped failed-bind ownership")
	}
}

func TestPartialTunnelFailureClosesReturnedConnection(t *testing.T) {
	f := makeWorker(t, nil, nil)
	entered := make(chan struct{})
	a, b := net.Pipe()
	defer b.Close()
	f.core.deps.Dial = func(context.Context, lanhelper.Stream) (net.Conn, error) {
		close(entered)
		return a, errors.New("partial tunnel")
	}
	grantWorker(t, f)
	client, accepted := net.Pipe()
	defer client.Close()
	f.listener.incoming <- memoryConn{accepted, "192.168.4.20"}
	<-entered
	_ = b.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := b.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("failed tunnel resource was not closed: %v", err)
	}
	f.core.Stop()
	awaitClosed(t, f.core)
}

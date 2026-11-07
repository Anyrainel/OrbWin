// Package lanworker owns the default-closed ordinary LAN worker lifecycle.
// Native identity/store/pipe adapters are separate. This core has injectable
// listener/tunnel/clock dependencies so fault tests need no real network/process.
package lanworker

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"orbwin.local/session/internal/landatagram"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
	"sync"
	"time"
)

var ErrUDPTransport = errors.New("bounded UDP namespace transport is not registered; ingress remains closed")

type Timer interface {
	C() <-chan time.Time
	Stop()
}
type Clock interface {
	Now() time.Time
	TimerAt(time.Time) Timer
}
type wallClock struct{}
type wallTimer struct{ t *time.Timer }

func (wallClock) Now() time.Time { return time.Now() }
func (wallClock) TimerAt(deadline time.Time) Timer {
	return wallTimer{time.NewTimer(time.Until(deadline))}
}
func (t wallTimer) C() <-chan time.Time { return t.t.C }
func (t wallTimer) Stop()               { t.t.Stop() }

type Bindings struct {
	Manager, Self lanfirewall.CallerIdentity
	WorkerFile    lanfirewall.WorkerBinding
}
type Dependencies struct {
	Clock        Clock
	ParentDone   <-chan struct{}
	NativeCheck  func(context.Context, lanfirewall.Scope) (Bindings, error)
	Listen       func(context.Context, string) (net.Listener, error)
	Dial         func(context.Context, lanhelper.Stream) (net.Conn, error)
	ListenPacket func(context.Context, string) (net.PacketConn, error)
	DialDatagram func(context.Context, lanhelper.Stream) (landatagram.Connection, error)
}
type Core struct {
	mu                       sync.Mutex
	spec                     lanlease.Spec
	bindings                 Bindings
	lease                    *lanlease.Lease
	sha                      string
	deps                     Dependencies
	listener                 net.Listener
	packet                   net.PacketConn
	peers                    map[netip.AddrPort]*datagramPeer
	streams                  map[net.Conn]struct{}
	pending                  int
	nextStream, lastSequence uint64
	expires                  time.Time
	closed, granted          bool
	ctx                      context.Context
	cancel                   context.CancelFunc
	stop, wake, done         chan struct{}
	workers                  sync.WaitGroup
}

func New(spec lanlease.Spec, bindings Bindings, deps Dependencies) (*Core, error) {
	if deps.Clock == nil {
		deps.Clock = wallClock{}
	}
	if deps.NativeCheck == nil || (spec.Request.Scope.Protocol == "tcp" && (deps.Listen == nil || deps.Dial == nil)) {
		return nil, errors.New("fixed native/listener/namespace dependencies required")
	}
	owner := lanlease.NewOwner(spec.Request.Installation, bindings.Manager)
	lease, e := owner.Prepare(spec, deps.Clock.Now())
	if e != nil {
		return nil, e
	}
	if bindings.Self.PID == 0 || bindings.Self.UserSID != bindings.Manager.UserSID || bindings.Self.ImageSHA256 != spec.Worker.SHA256 || bindings.WorkerFile != spec.Worker {
		return nil, errors.New("worker boot differs from exact native bindings")
	}
	spec.Request.Scope.ClientNetworks = append([]string(nil), spec.Request.Scope.ClientNetworks...)
	c := &Core{spec: spec, bindings: bindings, lease: lease, sha: lease.Snapshot(deps.Clock.Now()).LeaseSHA, deps: deps, streams: map[net.Conn]struct{}{}, stop: make(chan struct{}), wake: make(chan struct{}, 1), done: make(chan struct{}), expires: deps.Clock.Now().Add(10 * time.Second)}
	expires, _ := time.Parse(time.RFC3339Nano, spec.Request.ExpiresAt)
	if expires.Before(c.expires) {
		c.expires = expires
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.peers = make(map[netip.AddrPort]*datagramPeer)
	c.workers.Add(1)
	go c.watch()
	go func() { c.workers.Wait(); close(c.done) }()
	return c, nil
}
func (c *Core) LeaseSHA() string      { return c.sha }
func (c *Core) Done() <-chan struct{} { return c.done }
func (c *Core) Closed() bool          { c.mu.Lock(); defer c.mu.Unlock(); return c.closed }
func (c *Core) overlay(observed lanlease.Observation, actual Bindings) (lanlease.Observation, error) {
	if actual != c.bindings {
		return observed, errors.New("native manager/worker/file identity changed")
	}
	observed.Manager = actual.Manager
	observed.Worker = actual.Self
	observed.WorkerFile = actual.WorkerFile
	observed.ManagerPipePID = actual.Manager.PID
	observed.WorkerPipePID = actual.Self.PID
	// NativeCheck owns process/file/path validation. Source/publication and
	// exclusive manager-lease facts still arrive on its authenticated control.
	observed.ManagerAlive = true
	observed.WorkerAlive = true
	observed.WorkerFileHeld = true
	observed.ProtectedPath = true
	return observed, nil
}
func (c *Core) Apply(ctx context.Context, frame lanhelper.Frame) error {
	// Admission and Stop share this mutex. The watcher token cannot reach zero
	// before a newly admitted native check/bind is registered, and no work may be
	// admitted after Stop. Late binds therefore remain inside the join barrier.
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("worker control is closed")
	}
	c.workers.Add(1)
	c.mu.Unlock()
	defer c.workers.Done()
	if e := ctx.Err(); e != nil {
		c.Stop()
		return e
	}
	if e := frame.Validate(); e != nil {
		c.Stop()
		return e
	}
	if frame.LeaseSHA != c.LeaseSHA() {
		c.Stop()
		return errors.New("worker control lease digest differs")
	}
	if frame.Kind == "stop" {
		c.mu.Lock()
		valid := frame.Sequence == c.lastSequence+1
		c.mu.Unlock()
		c.Stop()
		if !valid {
			return errors.New("worker stop sequence differs")
		}
		return nil
	}
	if frame.Kind != "worker-grant" && frame.Kind != "refresh" {
		c.Stop()
		return errors.New("worker fixed control tag differs")
	}
	actual, e := c.deps.NativeCheck(ctx, c.spec.Request.Scope)
	if e != nil {
		c.Stop()
		return errors.New("worker native/interface check failed")
	}
	if e := ctx.Err(); e != nil {
		c.Stop()
		return e
	}
	c.mu.Lock()
	if c.closed || frame.Sequence != c.lastSequence+1 {
		c.mu.Unlock()
		c.Stop()
		return errors.New("worker control closed or replayed")
	}
	observed := lanlease.Observation{}
	if frame.Grant != nil {
		observed = frame.Grant.Observation
	} else if frame.Observation != nil {
		observed = *frame.Observation
	}
	observed, e = c.overlay(observed, actual)
	now := c.deps.Clock.Now()
	if e == nil && frame.Kind == "worker-grant" {
		if c.granted {
			e = errors.New("worker grant cannot replay")
		} else if c.spec.Request.Scope.Protocol == "udp" && (c.deps.ListenPacket == nil || c.deps.DialDatagram == nil) {
			e = ErrUDPTransport
		} else {
			e = c.lease.BindChild(1, c.bindings.Self, observed, now)
			if e == nil {
				e = c.lease.AcknowledgeRule(2, frame.Grant.Rule, observed, now)
			}
			if e == nil {
				e = c.lease.ReadyForIngress(3, observed, now)
			}
		}
	} else if e == nil {
		if !c.granted {
			e = errors.New("refresh before grant")
		} else {
			e = c.lease.Observe(c.lease.Snapshot(now).Revision, observed, now)
		}
	}
	if e != nil {
		c.mu.Unlock()
		c.Stop()
		return e
	}
	c.lastSequence = frame.Sequence
	c.expires = observed.ReadAt.Add(lanhelper.ObservationTTL)
	for stream := range c.streams {
		if stream.SetDeadline(c.expires) != nil {
			e = errors.New("worker stream deadline update failed")
			break
		}
	}
	if e == nil && c.packet != nil {
		e = c.packet.SetDeadline(c.expires)
	}
	if e == nil {
		for _, peer := range c.peers {
			if peer.out != nil && !peer.retiring {
				if e = peer.out.SetDeadline(c.expires); e != nil {
					break
				}
			}
		}
	}
	initial := !c.granted
	expires := c.expires
	c.granted = true
	c.mu.Unlock()
	if e != nil {
		c.Stop()
		return e
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
	if initial {
		if c.spec.Request.Scope.Protocol == "udp" {
			return c.bindDatagram(ctx, expires)
		}
		// Binding happens only after all guards; it is performed outside the lock so
		// stalled native/factory work cannot prevent the independent TTL closure.
		bindCtx, cancel := context.WithDeadline(ctx, expires)
		listener, e := c.deps.Listen(bindCtx, net.JoinHostPort(c.spec.Request.Scope.LocalAddress, portString(c.spec.Request.Scope.LocalPort)))
		cancel()
		if e != nil || listener == nil {
			if listener != nil {
				listener.Close()
			}
			c.Stop()
			return errors.New("worker exact listener bind failed")
		}
		if e = ctx.Err(); e != nil {
			listener.Close()
			c.Stop()
			return e
		}
		addr, ok := listener.Addr().(*net.TCPAddr)
		if !ok || addr.Port != int(c.spec.Request.Scope.LocalPort) || addr.IP.String() != c.spec.Request.Scope.LocalAddress {
			listener.Close()
			c.Stop()
			return errors.New("worker listener address differs from reviewed endpoint")
		}
		c.mu.Lock()
		if c.closed || !c.deps.Clock.Now().Before(c.expires) {
			c.mu.Unlock()
			listener.Close()
			c.Stop()
			return errors.New("worker lease expired during bind")
		}
		c.listener = listener
		c.workers.Add(1)
		c.mu.Unlock()
		go c.accept(listener)
	}
	return nil
}
func portString(p uint16) string {
	if p == 0 {
		return "0"
	}
	var b [5]byte
	i := len(b)
	n := uint32(p)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
func (c *Core) watch() {
	defer c.workers.Done()
	for {
		c.expireDatagrams()
		c.mu.Lock()
		closed, expires := c.closed, c.expires
		deadline := expires
		for _, peer := range c.peers {
			if !peer.retiring && peer.last.Add(datagramIdle).Before(deadline) {
				deadline = peer.last.Add(datagramIdle)
			}
		}
		c.mu.Unlock()
		if closed {
			return
		}
		remaining := expires.Sub(c.deps.Clock.Now())
		if remaining <= 0 {
			c.Stop()
			return
		}
		timer := c.deps.Clock.TimerAt(deadline)
		select {
		case <-c.stop:
			timer.Stop()
			return
		case <-c.deps.ParentDone:
			timer.Stop()
			c.Stop()
			return
		case <-c.wake:
			timer.Stop()
		case <-timer.C():
			timer.Stop()
		}
	}
}
func (c *Core) accept(listener net.Listener) {
	defer c.workers.Done()
	for {
		conn, e := listener.Accept()
		if e != nil {
			if conn != nil {
				conn.Close()
			}
			c.Stop()
			return
		}
		remote, ok := conn.RemoteAddr().(*net.TCPAddr)
		peer := netip.Addr{}
		if ok {
			peer, _ = netip.AddrFromSlice(remote.IP)
			peer = peer.Unmap()
		}
		c.mu.Lock()
		allowed := !c.closed && c.pending < lanhelper.MaxStreams && c.nextStream < lanfirewall.MaxSequence && c.lease.AllowsPeer(peer, c.deps.Clock.Now())
		if allowed {
			c.pending++
			c.nextStream++
			c.streams[conn] = struct{}{}
			allowed = conn.SetDeadline(c.expires) == nil
			if allowed {
				c.workers.Add(1)
			} else {
				delete(c.streams, conn)
				c.pending--
			}
		}
		id := c.nextStream
		c.mu.Unlock()
		if !allowed {
			conn.Close()
			continue
		}
		go c.forward(conn, lanhelper.Stream{ID: id, Peer: peer.String()})
	}
}
func (c *Core) forward(in net.Conn, request lanhelper.Stream) {
	defer c.workers.Done()
	defer func() { in.Close(); c.mu.Lock(); delete(c.streams, in); c.pending--; c.mu.Unlock() }()
	c.mu.Lock()
	expires := c.expires
	c.mu.Unlock()
	ctx, cancel := context.WithDeadline(c.ctx, expires)
	defer cancel()
	out, e := c.deps.Dial(ctx, request)
	if e != nil || out == nil {
		if out != nil {
			out.Close()
		}
		return
	}
	c.mu.Lock()
	if c.closed || !c.deps.Clock.Now().Before(c.expires) {
		c.mu.Unlock()
		out.Close()
		return
	}
	c.streams[out] = struct{}{}
	e = out.SetDeadline(c.expires)
	c.mu.Unlock()
	defer func() { out.Close(); c.mu.Lock(); delete(c.streams, out); c.mu.Unlock() }()
	if e != nil {
		return
	}
	copyOne := func(dst, src net.Conn, done chan<- struct{}) {
		_, e := io.CopyBuffer(dst, src, make([]byte, 32<<10))
		if e == nil {
			if half, ok := dst.(interface{ CloseWrite() error }); ok {
				e = half.CloseWrite()
			} else {
				e = errors.New("transport lacks write-half close")
			}
		}
		if e != nil {
			in.Close()
			out.Close()
		}
		done <- struct{}{}
	}
	done := make(chan struct{}, 2)
	go copyOne(out, in, done)
	copyOne(in, out, done)
	<-done
	<-done
}

// Stop revokes ingress and closes every tracked active stream, not just future
// accepts. Wait is the distinct positive join barrier used before retirement.
func (c *Core) Stop() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.cancel()
	close(c.stop)
	listener := c.listener
	packet := c.packet
	for _, peer := range c.peers {
		peer.stopOnce.Do(func() { close(peer.stop) })
	}
	connections := make([]net.Conn, 0, len(c.streams))
	for conn := range c.streams {
		connections = append(connections, conn)
	}
	c.mu.Unlock()
	if listener != nil {
		listener.Close()
	}
	if packet != nil {
		packet.Close()
	}
	for _, conn := range connections {
		conn.Close()
	}
}
func (c *Core) Wait(ctx context.Context) error {
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

package localnet

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

var errNamespaceRetained = errors.New("namespace release is unconfirmed; retain exact ownership for read-only settlement")

const httpNamespaceOwner = "http"

func isHTTPNamespaceOwner(owner string) bool {
	return owner == httpNamespaceOwner || strings.HasPrefix(owner, "http/") && validLANID(strings.TrimPrefix(owner, "http/"))
}

// Pending + active LAN TCP tunnels across ALL leases using one exact helper.
// HTTP's existing limits are unchanged; this budget reserves helper headroom
// for its controller, HTTP traffic and revocation. UDP remains separately gated.
const maxLANNamespaceStreams = 32

type namespaceReference struct {
	handle     NamespaceHandle
	owners     map[string]bool
	unknown    bool
	lanSlots   int
	udpSlots   int
	lanOpen    map[string]int
	operations map[string]int
}

// sharedNamespaces borrows the SAME backend, managers, daemon and ledger used by
// HTTP. It is a reference wrapper, not another namespace or lifecycle manager.
// The gate covers each underlying setup call and shutdown admission. Returned
// streams remain owned by their HTTP/LAN caller and must be joined before release.
type sharedNamespaces struct {
	base         NamespaceBackend
	settle       func(context.Context, NamespaceHandle) error
	quarantine   func(NamespaceHandle)
	streamFailed func(NamespaceHandle) error
	gate         chan struct{}
	refs         map[string]*namespaceReference
	changed      chan struct{}
	closing      atomic.Bool
	once         sync.Once
	done         chan struct{}
	result       error // Published by done.
}

func shareNamespaces(base *OwnedServiceBackend) *sharedNamespaces {
	s := newSharedNamespaces(base, base.settleNamespaceAbsence, base.quarantineNamespace)
	s.streamFailed = func(h NamespaceHandle) error { return base.namespaces.streamFailure(h.ID) }
	return s
}
func newSharedNamespaces(base NamespaceBackend, settle func(context.Context, NamespaceHandle) error, quarantine func(NamespaceHandle)) *sharedNamespaces {
	s := &sharedNamespaces{base: base, settle: settle, quarantine: quarantine, gate: make(chan struct{}, 1), refs: make(map[string]*namespaceReference), changed: make(chan struct{}), done: make(chan struct{})}
	s.gate <- struct{}{}
	return s
}
func (s *sharedNamespaces) lock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.gate:
	}
	if err := ctx.Err(); err != nil {
		s.unlock()
		return err
	}
	return nil
}
func (s *sharedNamespaces) unlock()                   { s.gate <- struct{}{} }
func (s *sharedNamespaces) notify()                   { close(s.changed); s.changed = make(chan struct{}) }
func namespaceReferenceKey(id, started string) string { return id + "\x00" + started }
func namespaceRouteKey(route ServiceRoute) string {
	return namespaceKey(NamespaceIntent{ContainerID: route.ContainerID, StartedAt: route.StartedAt, LANLeaseSHA: route.NamespaceLeaseSHA})
}
func (s *sharedNamespaces) Target(ctx context.Context, id string) (string, error) {
	if e := s.lock(ctx); e != nil {
		return "", e
	}
	defer s.unlock()
	if s.closing.Load() {
		return "", errors.New("namespace owner is closing")
	}
	return s.base.Target(ctx, id)
}
func (s *sharedNamespaces) Acquire(ctx context.Context, id, started string) (NamespaceHandle, error) {
	return s.borrow(ctx, httpNamespaceOwner, id, started)
}
func (s *sharedNamespaces) borrow(ctx context.Context, owner, id, started string) (NamespaceHandle, error) {
	return s.borrowScoped(ctx, owner, id, started, false)
}
func (s *sharedNamespaces) borrowLAN(ctx context.Context, owner, id, started string) (NamespaceHandle, error) {
	if !fullID(owner) {
		return NamespaceHandle{}, errors.New("native LAN namespace requires immutable lease digest")
	}
	return s.borrowScoped(ctx, owner, id, started, true)
}
func (s *sharedNamespaces) borrowScoped(ctx context.Context, owner, id, started string, dedicated bool) (NamespaceHandle, error) {
	if !isHTTPNamespaceOwner(owner) && !fullID(owner) {
		return NamespaceHandle{}, errors.New("exact LAN lease digest required")
	}
	if e := s.lock(ctx); e != nil {
		return NamespaceHandle{}, e
	}
	defer s.unlock()
	if s.closing.Load() {
		return NamespaceHandle{}, errors.New("namespace owner is closing")
	}
	key := namespaceReferenceKey(id, started)
	if dedicated {
		key = namespaceKey(NamespaceIntent{ContainerID: id, StartedAt: started, LANLeaseSHA: owner})
	}
	if ref := s.refs[key]; ref != nil {
		if ref.unknown {
			return NamespaceHandle{}, errNamespaceRetained
		}
		ref.owners[owner] = true
		return ref.handle, nil
	}
	var h NamespaceHandle
	var e error
	if dedicated {
		native, ok := s.base.(interface {
			AcquireLAN(context.Context, string, string, string) (NamespaceHandle, error)
		})
		if !ok {
			return NamespaceHandle{}, errors.New("dedicated native LAN namespace capability unavailable")
		}
		h, e = native.AcquireLAN(ctx, id, started, owner)
	} else {
		h, e = s.base.Acquire(ctx, id, started)
	}
	if e != nil {
		return NamespaceHandle{}, e
	} // Partial/unknown creates are never adopted or released here.
	if h.ID == "" || h.Intent.ContainerID != id || h.Intent.StartedAt != started || (dedicated && h.Intent.LANLeaseSHA != owner) || (!dedicated && h.Intent.LANLeaseSHA != "") {
		return NamespaceHandle{}, errors.New("namespace backend returned a different target; ownership remains with backend")
	}
	s.refs[key] = &namespaceReference{handle: h, owners: map[string]bool{owner: true}, lanOpen: make(map[string]int), operations: make(map[string]int)}
	s.notify()
	return h, nil
}
func (s *sharedNamespaces) Release(ctx context.Context, h NamespaceHandle) error {
	return s.release(ctx, httpNamespaceOwner, h)
}
func (s *sharedNamespaces) release(ctx context.Context, owner string, h NamespaceHandle) error {
	if e := s.lock(ctx); e != nil {
		return e
	}
	key := namespaceKey(h.Intent)
	ref := s.refs[key]
	for ref != nil && ref.handle == h && ref.owners[owner] && ref.operations[owner] > 0 {
		changed := s.changed
		s.unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
		if e := s.lock(ctx); e != nil {
			return e
		}
		ref = s.refs[key]
	}
	defer s.unlock()
	if ref == nil || ref.handle != h || !ref.owners[owner] {
		return errors.New("namespace reference is not owned")
	}
	if ref.lanOpen[owner] != 0 {
		return errors.New("LAN namespace streams must close and join before owner release")
	}
	if len(ref.owners) > 1 {
		delete(ref.owners, owner)
		s.notify()
		return nil
	}
	if ref.unknown {
		return errNamespaceRetained
	}
	if e := s.base.Release(ctx, h); e != nil {
		ref.unknown = true
		if s.quarantine != nil {
			s.quarantine(h)
		}
		s.notify()
		return errors.Join(errNamespaceRetained, e)
	}
	delete(s.refs, key)
	s.notify()
	return nil
}

// settleAbsent never retries Delete/Create or adopts by name. Only an earlier
// acknowledged immutable ID whose Release became uncertain can reach this path.
func (s *sharedNamespaces) settleAbsent(ctx context.Context, owner string, h NamespaceHandle) error {
	if e := s.lock(ctx); e != nil {
		return e
	}
	defer s.unlock()
	key := namespaceKey(h.Intent)
	ref := s.refs[key]
	if ref == nil || ref.handle != h || !ref.owners[owner] || len(ref.owners) != 1 || ref.lanOpen[owner] != 0 || ref.operations[owner] != 0 || !ref.unknown || s.settle == nil {
		return errNamespaceRetained
	}
	if e := s.settle(ctx, h); e != nil {
		return errors.Join(errNamespaceRetained, e)
	}
	delete(s.refs, key)
	s.notify()
	return nil
}
func (s *sharedNamespaces) Renew(ctx context.Context, h NamespaceHandle) error {
	return s.renewOwner(ctx, httpNamespaceOwner, h)
}
func (s *sharedNamespaces) renewOwner(ctx context.Context, owner string, h NamespaceHandle) error {
	if e := s.lock(ctx); e != nil {
		return e
	}
	ref := s.refs[namespaceKey(h.Intent)]
	if s.closing.Load() || ref == nil || ref.handle != h || !ref.owners[owner] || ref.unknown {
		s.unlock()
		return errNamespaceRetained
	}
	ref.operations[owner]++
	s.unlock()
	e := s.base.Renew(ctx, h)
	_ = s.lock(context.Background())
	ref.operations[owner]--
	s.notify()
	s.unlock()
	return e
}
func (s *sharedNamespaces) Dial(ctx context.Context, r ServiceRoute) (net.Conn, error) {
	return s.dialOwner(ctx, httpNamespaceOwner, r)
}
func (s *sharedNamespaces) dialOwner(ctx context.Context, owner string, r ServiceRoute) (net.Conn, error) {
	if e := s.lock(ctx); e != nil {
		return nil, e
	}
	ref := s.refs[namespaceRouteKey(r)]
	if s.closing.Load() || ref == nil || ref.handle.ID != r.HelperID || !ref.owners[owner] || ref.unknown {
		s.unlock()
		return nil, errNamespaceRetained
	}
	lan := !isHTTPNamespaceOwner(owner)
	if lan && ref.lanSlots >= maxLANNamespaceStreams {
		s.unlock()
		return nil, errors.New("shared LAN namespace stream capacity reached")
	}
	if lan {
		ref.lanSlots++
		ref.lanOpen[owner]++
	} // Pending setup included before exec.
	ref.operations[owner]++
	s.unlock()
	conn, e := s.base.Dial(ctx, r)
	_ = s.lock(context.Background())
	valid := !s.closing.Load() && !ref.unknown && ref.owners[owner] && ctx.Err() == nil
	if e != nil || conn == nil || !valid {
		s.unlock()
		var closeErr error
		if conn != nil {
			closeErr = conn.Close()
			e = errors.Join(e, closeErr)
		}
		_ = s.lock(context.Background())
		defer s.unlock()
		ref.operations[owner]--
		if lan {
			ref.lanOpen[owner]--
		}
		if closeErr != nil || s.streamFailed != nil && s.streamFailed(ref.handle) != nil {
			ref.unknown = true
			if s.quarantine != nil {
				s.quarantine(ref.handle)
			}
			s.notify()
		} else if lan {
			ref.lanSlots--
		}
		if e == nil {
			e = errors.New("namespace dial returned no live owned stream")
		}
		s.notify()
		return nil, e
	}
	ref.operations[owner]--
	s.notify()
	s.unlock()
	if !lan {
		return conn, nil
	}
	return &lanReferenceConn{Conn: conn, settle: func(closeErr error) {
		_ = s.lock(context.Background())
		defer s.unlock()
		ref.lanOpen[owner]--
		// A failed revocation retains capacity/ownership. No new lease can reuse
		// that slot merely because its local pipe was closed.
		if closeErr != nil || (s.streamFailed != nil && s.streamFailed(ref.handle) != nil) {
			ref.unknown = true
			if s.quarantine != nil {
				s.quarantine(ref.handle)
			}
		} else {
			ref.lanSlots--
		}
		s.notify()
	}}, nil
}

type lanReferenceConn struct {
	net.Conn
	once   sync.Once
	result error
	settle func(error)
}

func (c *lanReferenceConn) Close() error {
	c.once.Do(func() { c.result = c.Conn.Close(); c.settle(c.result) })
	return c.result
}
func (c *lanReferenceConn) CloseWrite() error {
	if half, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return half.CloseWrite()
	}
	return errors.New("namespace transport lacks write-half close")
}
func (s *sharedNamespaces) Close(ctx context.Context) error {
	// Start ONE shutdown owner; caller deadlines do not drop references or retry
	// an uncertain physical release. A later settlement only unblocks this worker.
	s.once.Do(func() {
		s.closing.Store(true)
		go func() {
			_ = s.lock(context.Background())
			s.notify()
			for len(s.refs) > 0 {
				changed := s.changed
				s.unlock()
				<-changed
				_ = s.lock(context.Background())
			}
			s.unlock()
			s.result = s.base.Close(context.Background())
			close(s.done)
		}()
	})
	return s.Wait(ctx)
}
func (s *sharedNamespaces) Wait(ctx context.Context) error {
	select {
	case <-s.done:
		return s.result
	default:
	}
	select {
	case <-s.done:
		return s.result
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (backend *OwnedServiceBackend) quarantineNamespace(h NamespaceHandle) {
	m := backend.namespaces
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active[namespaceKey(h.Intent)] != h {
		return
	}
	m.poisonMu.Lock()
	defer m.poisonMu.Unlock()
	if m.poisoned == nil {
		m.poisoned = make(map[string]error)
	}
	m.poisoned[h.ID] = errNamespaceRetained
}
func (backend *OwnedServiceBackend) settleNamespaceAbsence(ctx context.Context, h NamespaceHandle) error {
	m := backend.namespaces
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || !fullID(h.ID) || m.active[namespaceKey(h.Intent)] != h {
		return errNamespaceRetained
	}
	if e := m.streamFailure(h.ID); !errors.Is(e, errNamespaceRetained) {
		return errNamespaceRetained
	}
	if e := m.verifyDaemon(ctx); e != nil {
		return e
	}
	_, e := m.cli.ContainerInspect(ctx, h.ID, client.ContainerInspectOptions{})
	if !cerrdefs.IsNotFound(e) {
		if e != nil {
			return e
		}
		return errNamespaceRetained
	}
	if e = m.verifyDaemon(ctx); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	// Docker IDs are immutable. A late deletion of this ID cannot recreate it.
	// This does NOT supply compare-and-delete semantics for firewall rule keys.
	if e = m.ledger.settled(namespaceRecord{Version: 1, Daemon: m.daemon, Intent: h.Intent, ID: h.ID}); e != nil {
		return e
	}
	if e = m.settleDatagramAbsenceLocked(h.ID); e != nil {
		return e
	}
	delete(m.active, namespaceKey(h.Intent))
	m.poisonMu.Lock()
	delete(m.poisoned, h.ID)
	m.poisonMu.Unlock()
	return nil
}

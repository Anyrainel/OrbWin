// Package sdkstate owns fixed native Windows SDK state. It exposes no path,
// raw-handle, JSON recovery constructor or trust-store/DPAPI operation.
package sdkstate

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"sync"
)

var ErrState = errors.New("native SDK state ownership unavailable")
var ErrRecoveryRequired = errors.New("native SDK state needs explicit recovery; no replay or fresh reset")
var ErrLegacyState = errors.New("legacy SDK state needs explicit cold transition")
var ErrBusy = errors.New("native SDK state transaction already active")
var ErrNotExist = fs.ErrNotExist
var ErrExist = fs.ErrExist

const maxReadBytes = 128 << 20
const maxEntries = 2048

type recordLease interface {
	data() []byte
	check() error
	close() error
}
type backend interface {
	check() error
	read(context.Context, string, int64) (recordLease, error)
	publish(context.Context, string, recordLease, []byte) (recordLease, error)
	remove(context.Context, string, recordLease) error
	list(context.Context) ([]string, error)
	close() error
}
type Root struct{ s *rootState }
type rootState struct {
	mu             sync.Mutex
	b              backend
	active         *Guard
	poison, closed bool
	closeErr       error
}
type Guard struct{ s *guardState }
type guardState struct {
	mu       sync.Mutex
	root     *rootState
	ctx      context.Context
	reads    map[string]recordLease
	used     int64
	closed   bool
	closeErr error
}

func (*Root) String() string    { return "SDKStateRoot(private native owner)" }
func (*Root) GoString() string  { return "SDKStateRoot(private native owner)" }
func (*Guard) String() string   { return "SDKStateGuard(retained native records)" }
func (*Guard) GoString() string { return "SDKStateGuard(retained native records)" }
func Open(ctx context.Context, installation, daemon, legacyHome string) (*Root, error) {
	if !bounded(ctx) || !installationID(installation) || !validDaemon(daemon) {
		return nil, ErrState
	}
	return openWithMode(ctx, installation, daemon, legacyHome, ordinaryOpen)
}

// OpenNewService restricts the real native admission; an existing empty root is refused.
func OpenNewService(ctx context.Context, installation, daemon, legacyHome string) (*Root, error) {
	return openWithMode(ctx, installation, daemon, legacyHome, newServiceOnly)
}
func openWithMode(ctx context.Context, installation, daemon, legacyHome string, mode openRestriction) (*Root, error) {
	if !bounded(ctx) || !installationID(installation) || !validDaemon(daemon) {
		return nil, ErrState
	}
	b, e := openNative(ctx, installation, daemon, legacyHome, mode)
	if b == nil {
		return nil, e
	}
	return &Root{&rootState{b: b, poison: e != nil}}, e
}
func bounded(ctx context.Context) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	_, ok := ctx.Deadline()
	return ok
}
func (r *Root) Check() error {
	if r == nil || r.s == nil {
		return ErrState
	}
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poison {
		return ErrRecoveryRequired
	}
	return s.b.check()
}
func (r *Root) Begin(ctx context.Context) (*Guard, error) {
	if r == nil || r.s == nil || !bounded(ctx) {
		return nil, ErrState
	}
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.poison {
		return nil, ErrRecoveryRequired
	}
	if s.active != nil {
		return nil, ErrBusy
	}
	if s.b.check() != nil {
		return nil, ErrState
	}
	g := &Guard{&guardState{root: s, ctx: ctx, reads: map[string]recordLease{}}}
	s.active = g
	return g, nil
}
func (r *Root) Close() error {
	if r == nil || r.s == nil {
		return nil
	}
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	if s.poison {
		return ErrRecoveryRequired
	}
	if s.active != nil {
		return ErrBusy
	}
	if s.b.check() != nil {
		s.poison = true
		s.closeErr = ErrRecoveryRequired
		return s.closeErr
	}
	s.closed = true
	s.closeErr = s.b.close()
	if s.closeErr != nil {
		s.poison = true
	}
	return s.closeErr
}
func (s *guardState) usable() error {
	if s.closed || !bounded(s.ctx) {
		return ErrState
	}
	s.root.mu.Lock()
	defer s.root.mu.Unlock()
	if s.root.closed || s.root.poison || s.root.active == nil || s.root.active.s != s {
		return ErrRecoveryRequired
	}
	return s.root.b.check()
}
func (s *guardState) poison() { s.root.mu.Lock(); s.root.poison = true; s.root.mu.Unlock() }
func (s *guardState) read(name string, budget int64) (recordLease, error) {
	limit := recordBudget(name)
	if limit == 0 || budget < 1 || budget > limit {
		return nil, ErrState
	}
	if e := s.usable(); e != nil {
		return nil, e
	}
	if r, ok := s.reads[name]; ok {
		if r == nil {
			return nil, ErrNotExist
		}
		if int64(len(r.data())) > budget || r.check() != nil {
			return nil, ErrState
		}
		return r, nil
	}
	if len(s.reads) >= maxEntries || s.used+budget > maxReadBytes {
		return nil, ErrState
	}
	r, e := s.root.b.read(s.ctx, name, budget)
	if r != nil {
		s.reads[name] = r
		s.used += int64(len(r.data()))
	}
	if errors.Is(e, ErrNotExist) {
		s.reads[name] = nil
		return nil, ErrNotExist
	}
	if e != nil {
		if r != nil {
			s.poison()
		}
		return nil, e
	}
	if r == nil || int64(len(r.data())) > budget || s.used > maxReadBytes || r.check() != nil {
		s.poison()
		return nil, ErrState
	}
	return r, nil
}
func (g *Guard) Read(name string, budget int64) ([]byte, error) {
	if g == nil || g.s == nil {
		return nil, ErrState
	}
	s := g.s
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.read(name, budget)
	if e != nil {
		return nil, e
	}
	return append([]byte(nil), r.data()...), nil
}
func (g *Guard) Check() error {
	if g == nil || g.s == nil {
		return ErrState
	}
	s := g.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.usable(); e != nil {
		return e
	}
	for _, r := range s.reads {
		if r != nil && r.check() != nil {
			return ErrState
		}
	}
	return nil
}
func (g *Guard) List() ([]string, error) {
	if g == nil || g.s == nil {
		return nil, ErrState
	}
	s := g.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.usable(); e != nil {
		return nil, e
	}
	names, e := s.root.b.list(s.ctx)
	if e != nil || len(names) > maxEntries {
		return nil, ErrRecoveryRequired
	}
	seen := map[string]bool{}
	for _, n := range names {
		if recordBudget(n) == 0 || seen[n] {
			return nil, ErrRecoveryRequired
		}
		seen[n] = true
	}
	return append([]string(nil), names...), nil
}
func (g *Guard) change(name string, expected, data []byte, kind string) error {
	if g == nil || g.s == nil {
		return ErrState
	}
	s := g.s
	s.mu.Lock()
	defer s.mu.Unlock()
	limit := recordBudget(name)
	if limit == 0 || len(data) == 0 && kind != "delete" || int64(len(data)) > limit {
		return ErrState
	}
	if e := s.usable(); e != nil {
		return e
	}
	old, seen := s.reads[name]
	if kind == "create" {
		if !seen {
			var e error
			old, e = s.read(name, limit)
			if !errors.Is(e, ErrNotExist) && e != nil {
				return e
			}
		}
		if old != nil {
			return ErrExist
		}
	} else {
		if !seen || old == nil || old.check() != nil || !bytes.Equal(expected, old.data()) {
			return ErrState
		}
	}
	next := append([]byte(nil), data...)
	if kind == "append" {
		if int64(len(expected)+len(data)) > limit {
			return ErrState
		}
		next = append(append([]byte(nil), expected...), data...)
	}
	priorSize := int64(0)
	if old != nil {
		priorSize = int64(len(old.data()))
	}
	projected := s.used - priorSize + int64(len(next))
	if kind == "delete" {
		projected = s.used - priorSize
	}
	if projected < 0 || projected > maxReadBytes {
		return ErrState
	}
	// Admission latches before ANY native stage/intent/lease exchange. Only a
	// positive native completion clears it; an error can never replay this owner.
	s.poison()
	var fresh recordLease
	var e error
	if kind == "delete" {
		e = s.root.b.remove(s.ctx, name, old)
	} else {
		fresh, e = s.root.b.publish(s.ctx, name, old, next)
	}
	if fresh != nil {
		s.reads[name] = fresh
		s.used = s.used - priorSize + int64(len(fresh.data()))
	} else if e == nil {
		s.reads[name] = nil
		s.used -= priorSize
	}
	if s.used < 0 || s.used > maxReadBytes || fresh != nil && int64(len(fresh.data())) > limit {
		return ErrRecoveryRequired
	}
	if e != nil || s.ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	if fresh != nil && (fresh.check() != nil || !bytes.Equal(fresh.data(), next)) {
		return ErrRecoveryRequired
	}
	s.root.mu.Lock()
	s.root.poison = false
	s.root.mu.Unlock()
	return nil
}
func (g *Guard) Create(n string, data []byte) error       { return g.change(n, nil, data, "create") }
func (g *Guard) Replace(n string, old, data []byte) error { return g.change(n, old, data, "replace") }
func (g *Guard) Append(n string, old, data []byte) error  { return g.change(n, old, data, "append") }
func (g *Guard) Delete(n string, old []byte) error        { return g.change(n, old, nil, "delete") }
func (g *Guard) Close() error {
	if g == nil || g.s == nil {
		return nil
	}
	s := g.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	if s.ctx.Err() != nil {
		s.closeErr = ErrState
	}
	if s.root.b.check() != nil {
		s.closeErr = ErrRecoveryRequired
		s.poison()
	}
	for _, r := range s.reads {
		if r != nil && r.check() != nil {
			s.closeErr = ErrRecoveryRequired
			s.poison()
		}
	}
	for _, r := range s.reads {
		if r != nil && r.close() != nil {
			s.closeErr = ErrRecoveryRequired
			s.poison()
		}
	}
	s.root.mu.Lock()
	if s.root.active != nil && s.root.active.s == s {
		s.root.active = nil
	}
	s.root.mu.Unlock()
	return s.closeErr
}

// Shared by the actual native close path and pure fault tests. Never release
// the real data-sharing owner lock after any earlier unknown native Close.
func closeOwnerLast(resources []func() error, owner func() error) error {
	for _, close := range resources {
		if close() == nil {
			continue
		}
		return ErrRecoveryRequired
	}
	if owner() != nil {
		return ErrRecoveryRequired
	}
	return nil
}

// Only the sole native owner FILE_OBJECT uses retainOnUnknown. A failed origin
// precheck cannot physically release that exclusion. A failure returned by the
// final Close syscall itself remains unknown, never positive retirement proof.
func closeCheckedRecord(retainOnUnknown bool, origin func() error, close func() error) error {
	checked := origin()
	if checked != nil && retainOnUnknown {
		return ErrRecoveryRequired
	}
	closed := close()
	if checked != nil || closed != nil {
		return ErrRecoveryRequired
	}
	return nil
}

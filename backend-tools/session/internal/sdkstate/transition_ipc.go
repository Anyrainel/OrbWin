package sdkstate

import (
	"context"
	"net"
	"sync"
)

// ColdIPCLease has no caller-listener constructor. The actual native cold
// authority issues it for its already reserved product pipe, without rebind.
// The SDK's existing IPC server supplies the sole serving loop and must return
// only after joining every worker. This lease does not prove runtime readiness
// or grant historical cleanup authority.
type ColdIPCLease struct{ s *coldIPCState }

type coldIPCState struct {
	mu                                sync.Mutex
	listener                          *coldIPCListener
	claimed, serving, joined, unknown bool
}

type coldIPCListener struct {
	net.Listener
	once sync.Once
	err  error
}

func (l *coldIPCListener) Close() error {
	l.once.Do(func() { l.err = l.Listener.Close() })
	return l.err
}

// Serve consumes the lease once. The callback is the existing joined IPC loop,
// not an authority observer. A positive original-listener close is retained
// independently of its serve result. A lost close cannot be retried by reusing
// the lease or by opening another pipe with the same name.
func (l *ColdIPCLease) Serve(ctx context.Context, run func(net.Listener) error) error {
	if l == nil || l.s == nil || ctx == nil || ctx.Err() != nil || run == nil {
		return ErrState
	}
	s := l.s
	s.mu.Lock()
	if s.claimed || s.listener == nil || s.unknown {
		s.mu.Unlock()
		return ErrRecoveryRequired
	}
	s.claimed, s.serving, s.unknown = true, true, true
	s.mu.Unlock()
	result := run(s.listener)
	closed := s.listener.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serving = false
	if closed != nil {
		s.unknown = true
		return ErrRecoveryRequired
	}
	s.joined = true
	s.unknown = false
	return result
}

func (t *ColdTransition) IPCLease(ctx context.Context) (*ColdIPCLease, error) {
	if t == nil || !bounded(ctx) {
		return nil, ErrState
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.unknown || t.native == nil || t.root == nil || t.root.Check() != nil || !t.receipt.valid() {
		return nil, ErrRecoveryRequired
	}
	provider, ok := t.native.(interface {
		reserveIPC(context.Context, TransitionReceipt) (*coldIPCState, error)
	})
	if !ok {
		return nil, ErrRecoveryRequired
	}
	s, err := provider.reserveIPC(ctx, t.receipt)
	if err != nil || s == nil {
		return nil, ErrRecoveryRequired
	}
	return &ColdIPCLease{s}, nil
}

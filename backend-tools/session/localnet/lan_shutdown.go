package localnet

import (
	"context"
	"errors"
	"sync"
)

type generationHostOwner interface {
	Close(context.Context) error
	Wait(context.Context) error
}
type generationLANOwner interface {
	Close(context.Context) error
	Wait(context.Context) error
}
type generationBackendOwner interface{ Wait(context.Context) error }

// generationShutdown owns ONE graceful shutdown operation. Caller deadlines do
// not discard it or re-dispatch mutations. LAN's positive native/flow join comes
// first, then host Close ONCE, host join, and the SAME shared backend's join.
// An unknown LAN outcome deliberately leaves done open and retains the owner.
// Forced process death is outside this graceful ownership guarantee.
type generationShutdown struct {
	once    sync.Once
	done    chan struct{}
	host    generationHostOwner
	lan     generationLANOwner
	backend generationBackendOwner
	result  error
}

func newGenerationShutdown(host generationHostOwner, lan generationLANOwner, backend generationBackendOwner) *generationShutdown {
	return &generationShutdown{done: make(chan struct{}), host: host, lan: lan, backend: backend}
}
func (s *generationShutdown) Close(ctx context.Context) error {
	s.once.Do(func() {
		go func() {
			if s.lan != nil {
				s.result = s.lan.Close(context.Background())
				if e := s.lan.Wait(context.Background()); e != nil {
					// Never turn a terminal unknown into owner release. This worker is
					// retained; a future reviewed recovery must settle the SAME owner.
					s.result = errors.Join(s.result, e)
					return
				}
			}
			s.result = errors.Join(s.result, s.host.Close(context.Background()), s.host.Wait(context.Background()))
			if s.backend != nil {
				if closer, ok := s.backend.(interface{ Close(context.Context) error }); ok {
					s.result = errors.Join(s.result, closer.Close(context.Background()))
				}
				s.result = errors.Join(s.result, s.backend.Wait(context.Background()))
			}
			close(s.done)
		}()
	})
	return s.Wait(ctx)
}
func (s *generationShutdown) Wait(ctx context.Context) error {
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

// Wait observes the already requested Runtime shutdown. It never cancels,
// repeats a mutation, or closes the ledger while a generation is still owned.
func (runtime *ServiceRuntime) Wait(ctx context.Context) error {
	select {
	case <-runtime.done:
		return runtime.result
	default:
	}
	select {
	case <-runtime.done:
		return runtime.result
	case <-ctx.Done():
		return ctx.Err()
	}
}

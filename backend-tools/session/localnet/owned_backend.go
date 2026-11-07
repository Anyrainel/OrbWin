package localnet

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// OwnedServiceBackend transfers both managers into the route host's lifetime.
// Its one shutdown worker settles relay receipts before NamespaceManager releases
// the common ledger. A caller deadline does not falsely report stopped or abandon
// the bounded cleanup worker. Renderer contexts never own this lifetime.
type OwnedServiceBackend struct {
	namespaces *NamespaceManager
	relay      *RelayManager
	closing    atomic.Bool
	once       sync.Once
	done       chan struct{}
	result     error // Published by closing done; read only after that barrier.
}

func OwnServiceBackend(namespaces *NamespaceManager, relay *RelayManager) (*OwnedServiceBackend, error) {
	if namespaces == nil || relay == nil || namespaces.ledger == nil || namespaces.ledger != relay.ledger || namespaces.cli != relay.cli || namespaces.image != relay.image.ID || namespaces.asset != relay.image.AssetSHA {
		return nil, errors.New("matching namespace/relay managers and one exclusive owner required")
	}
	return &OwnedServiceBackend{namespaces: namespaces, relay: relay, done: make(chan struct{})}, nil
}
func (backend *OwnedServiceBackend) available() error {
	if backend.closing.Load() {
		return errors.New("local-services backend is stopping")
	}
	return nil
}
func (backend *OwnedServiceBackend) Target(ctx context.Context, id string) (string, error) {
	if err := backend.available(); err != nil {
		return "", err
	}
	return backend.namespaces.Target(ctx, id)
}
func (backend *OwnedServiceBackend) Acquire(ctx context.Context, id, started string) (NamespaceHandle, error) {
	if err := backend.available(); err != nil {
		return NamespaceHandle{}, err
	}
	return backend.namespaces.Acquire(ctx, id, started)
}
func (backend *OwnedServiceBackend) AcquireLAN(ctx context.Context, id, started, leaseSHA string) (NamespaceHandle, error) {
	if err := backend.available(); err != nil {
		return NamespaceHandle{}, err
	}
	return backend.namespaces.AcquireLAN(ctx, id, started, leaseSHA)
}
func (backend *OwnedServiceBackend) Renew(ctx context.Context, handle NamespaceHandle) error {
	if err := backend.available(); err != nil {
		return err
	}
	return backend.namespaces.Renew(ctx, handle)
}
func (backend *OwnedServiceBackend) Release(ctx context.Context, handle NamespaceHandle) error {
	if err := backend.available(); err != nil {
		return err
	}
	return backend.namespaces.Release(ctx, handle)
}
func (backend *OwnedServiceBackend) Dial(ctx context.Context, route ServiceRoute) (net.Conn, error) {
	if err := backend.available(); err != nil {
		return nil, err
	}
	return backend.namespaces.Dial(ctx, route)
}
func (backend *OwnedServiceBackend) Close(ctx context.Context) error {
	backend.once.Do(func() {
		backend.closing.Store(true)
		go func() {
			defer close(backend.done)
			relayCtx, relayCancel := context.WithTimeout(context.Background(), 10*time.Second)
			relayErr := backend.relay.Close(relayCtx)
			relayCancel()
			namespaceCtx, namespaceCancel := context.WithTimeout(context.Background(), 10*time.Second)
			namespaceErr := backend.namespaces.Close(namespaceCtx)
			namespaceCancel()
			backend.result = errors.Join(relayErr, namespaceErr)
		}()
	})
	select {
	case <-backend.done:
		return backend.result
	default:
	}
	select {
	case <-backend.done:
		return backend.result
	case <-ctx.Done():
		return ctx.Err()
	}
}

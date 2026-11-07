package localnet

import (
	"context"
	"errors"
	"net"
	"sync"
)

// HTTP owns a view of the same backend, not the backend lifetime. Its controller
// first closes/joins route streams, then this view drops only HTTP references.
// LAN references and its heartbeat remain independent across HTTP configuration.
type httpNamespaceView struct {
	*sharedNamespaces
	once    sync.Once
	done    chan struct{}
	result  error
	owner   string
	gate    sync.RWMutex
	closing bool
}

func newHTTPNamespaceView(shared *sharedNamespaces) (*httpNamespaceView, error) {
	id, err := lanRandomID()
	if err != nil {
		return nil, err
	}
	return &httpNamespaceView{sharedNamespaces: shared, done: make(chan struct{}), owner: "http/" + id}, nil
}
func (view *httpNamespaceView) Acquire(ctx context.Context, id, started string) (NamespaceHandle, error) {
	view.gate.RLock()
	defer view.gate.RUnlock()
	if view.closing {
		return NamespaceHandle{}, errors.New("HTTP namespace view is closed")
	}
	return view.borrow(ctx, view.owner, id, started)
}
func (view *httpNamespaceView) Renew(ctx context.Context, h NamespaceHandle) error {
	view.gate.RLock()
	defer view.gate.RUnlock()
	if view.closing {
		return errors.New("HTTP namespace view is closed")
	}
	return view.renewOwner(ctx, view.owner, h)
}
func (view *httpNamespaceView) Release(ctx context.Context, h NamespaceHandle) error {
	view.gate.RLock()
	defer view.gate.RUnlock()
	if view.closing {
		return errors.New("HTTP namespace view is closed")
	}
	return view.release(ctx, view.owner, h)
}
func (view *httpNamespaceView) Dial(ctx context.Context, r ServiceRoute) (net.Conn, error) {
	view.gate.RLock()
	defer view.gate.RUnlock()
	if view.closing {
		return nil, errors.New("HTTP namespace view is closed")
	}
	return view.dialOwner(ctx, view.owner, r)
}
func (view *httpNamespaceView) Close(ctx context.Context) error {
	view.once.Do(func() {
		go func() {
			defer close(view.done)
			view.gate.Lock()
			view.closing = true
			view.gate.Unlock()
			if err := view.sharedNamespaces.lock(context.Background()); err != nil {
				view.result = err
				return
			}
			var handles []NamespaceHandle
			for _, ref := range view.refs {
				if ref.owners[view.owner] {
					handles = append(handles, ref.handle)
				}
			}
			view.sharedNamespaces.unlock()
			for _, handle := range handles {
				view.result = errors.Join(view.result, view.release(context.Background(), view.owner, handle))
			}
		}()
	})
	return view.Wait(ctx)
}
func (view *httpNamespaceView) Wait(ctx context.Context) error {
	select {
	case <-view.done:
		return view.result
	case <-ctx.Done():
		return ctx.Err()
	}
}

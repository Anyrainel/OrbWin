package localnet

import (
	"context"
	"errors"
	"orbwin.local/session/internal/landatagram"
	"sync"
)

// Same owned manager and verified namespace, never another helper/owner ledger.
func (b *OwnedServiceBackend) DialDatagram(ctx context.Context, r ServiceRoute) (DatagramConnection, error) {
	if e := b.available(); e != nil {
		return nil, e
	}
	return b.namespaces.DialDatagram(ctx, r)
}

func (s *sharedNamespaces) dialDatagramOwner(ctx context.Context, owner string, r ServiceRoute) (landatagram.Connection, error) {
	if e := s.lock(ctx); e != nil {
		return nil, e
	}
	ref := s.refs[namespaceRouteKey(r)]
	base, ok := s.base.(interface {
		DialDatagram(context.Context, ServiceRoute) (DatagramConnection, error)
	})
	if !ok || owner == httpNamespaceOwner || s.closing.Load() || ref == nil || ref.handle.ID != r.HelperID || !ref.owners[owner] || ref.unknown {
		s.unlock()
		return nil, errNamespaceRetained
	}
	if ref.lanSlots >= maxLANNamespaceStreams || ref.udpSlots >= landatagram.MaxPeers {
		s.unlock()
		return nil, errors.New("shared LAN/UDP helper capacity reached")
	}
	ref.lanSlots++
	ref.udpSlots++
	ref.lanOpen[owner]++
	ref.operations[owner]++
	s.unlock()
	conn, e := base.DialDatagram(ctx, r)
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
		ref.operations[owner]--
		ref.lanOpen[owner]--
		if closeErr != nil || s.streamFailed != nil && s.streamFailed(ref.handle) != nil {
			ref.unknown = true
			if s.quarantine != nil {
				s.quarantine(ref.handle)
			}
		} else {
			ref.lanSlots--
			ref.udpSlots--
		}
		s.notify()
		s.unlock()
		if e == nil {
			e = errors.New("datagram setup returned no live owned association")
		}
		return nil, e
	}
	ref.operations[owner]--
	s.notify()
	s.unlock()
	return &lanReferenceDatagram{Connection: conn, settle: func(e error) {
		_ = s.lock(context.Background())
		defer s.unlock()
		ref.lanOpen[owner]--
		if e != nil || s.streamFailed != nil && s.streamFailed(ref.handle) != nil {
			ref.unknown = true
			if s.quarantine != nil {
				s.quarantine(ref.handle)
			}
		} else {
			ref.lanSlots--
			ref.udpSlots--
		}
		s.notify()
	}}, nil
}

type lanReferenceDatagram struct {
	landatagram.Connection
	once   sync.Once
	result error
	settle func(error)
}

func (c *lanReferenceDatagram) Close() error {
	c.once.Do(func() { c.result = c.Connection.Close(); c.settle(c.result) })
	return c.result
}

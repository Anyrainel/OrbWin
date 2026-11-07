package lanworker

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"orbwin.local/session/internal/landatagram"
	"orbwin.local/session/internal/lanhelper"
	"sync"
	"time"
)

const datagramIdle = 15 * time.Second
const datagramQueue = 4

type datagramPeer struct {
	address  netip.AddrPort
	id       uint64
	last     time.Time
	retiring bool
	packets  chan []byte
	stop     chan struct{}
	stopOnce sync.Once
	out      landatagram.Connection // Accessed under Core.mu.
}

func (c *Core) bindDatagram(ctx context.Context, expires time.Time) error {
	bind, cancel := context.WithDeadline(ctx, expires)
	socket, e := c.deps.ListenPacket(bind, net.JoinHostPort(c.spec.Request.Scope.LocalAddress, portString(c.spec.Request.Scope.LocalPort)))
	cancel()
	if e != nil || socket == nil {
		if socket != nil {
			socket.Close()
		}
		c.Stop()
		return errors.New("exact UDP listener bind failed")
	}
	addr, ok := socket.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP.String() != c.spec.Request.Scope.LocalAddress || addr.Port != int(c.spec.Request.Scope.LocalPort) || ctx.Err() != nil {
		socket.Close()
		c.Stop()
		return errors.New("UDP listener differs from reviewed tuple")
	}
	c.mu.Lock()
	if c.closed || !c.deps.Clock.Now().Before(c.expires) {
		c.mu.Unlock()
		socket.Close()
		c.Stop()
		return errors.New("UDP lease expired during bind")
	}
	if e = socket.SetDeadline(c.expires); e != nil {
		c.mu.Unlock()
		socket.Close()
		c.Stop()
		return e
	}
	c.packet = socket
	c.workers.Add(1)
	c.mu.Unlock()
	go c.readDatagrams(socket)
	return nil
}
func (c *Core) readDatagrams(socket net.PacketConn) {
	defer c.workers.Done()
	var buffer [landatagram.MaxPayload + 1]byte
	for {
		n, remote, e := socket.ReadFrom(buffer[:])
		if e != nil {
			c.Stop()
			return
		}
		if n < 0 || n > landatagram.MaxPayload {
			continue
		}
		address, ok := remote.(*net.UDPAddr)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(address.IP)
		if !ok || !ip.Unmap().Is4() || address.Port <= 0 || address.Port > 65535 {
			continue
		}
		peer := netip.AddrPortFrom(ip.Unmap(), uint16(address.Port))
		c.mu.Lock()
		now := c.deps.Clock.Now()
		if c.closed || !c.lease.AllowsPeer(peer.Addr(), now) {
			c.mu.Unlock()
			continue
		}
		p := c.peers[peer]
		if p == nil {
			if len(c.peers) >= landatagram.MaxPeers || c.nextStream >= 1<<53-1 {
				c.mu.Unlock()
				continue
			}
			c.nextStream++
			p = &datagramPeer{address: peer, id: c.nextStream, last: now, packets: make(chan []byte, datagramQueue), stop: make(chan struct{})}
			c.peers[peer] = p
			c.workers.Add(1)
			go c.runDatagramPeer(socket, p)
		}
		if !p.retiring && len(p.packets) < cap(p.packets) {
			packet := append([]byte{}, buffer[:n]...)
			select {
			case p.packets <- packet:
				p.last = now
			default:
			}
		} // Finite drop-on-full UDP queue.
		c.mu.Unlock()
	}
}
func (c *Core) expireDatagrams() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.deps.Clock.Now()
	for _, p := range c.peers {
		if !p.retiring && !now.Before(p.last.Add(datagramIdle)) {
			p.retiring = true
			p.stopOnce.Do(func() { close(p.stop) })
		}
	}
}
func (c *Core) runDatagramPeer(socket net.PacketConn, p *datagramPeer) {
	defer c.workers.Done()
	defer func() {
		c.mu.Lock()
		if c.peers[p.address] == p {
			delete(c.peers, p.address)
		}
		c.mu.Unlock()
	}()
	c.mu.Lock()
	expires := c.expires
	c.mu.Unlock()
	ctx, cancel := context.WithDeadline(c.ctx, expires)
	defer cancel()
	// Peer expiry cancels pending setup too. This watcher is joined locally.
	setupDone := make(chan struct{})
	setupJoined := make(chan struct{})
	go func() {
		defer close(setupJoined)
		select {
		case <-p.stop:
			cancel()
		case <-setupDone:
		}
	}()
	out, e := c.deps.DialDatagram(ctx, lanhelper.Stream{ID: p.id, Peer: p.address.String()})
	close(setupDone)
	<-setupJoined
	if e != nil || out == nil {
		if out != nil {
			out.Close()
		}
		return
	}
	c.mu.Lock()
	if c.closed || p.retiring || !c.deps.Clock.Now().Before(c.expires) {
		c.mu.Unlock()
		out.Close()
		return
	}
	p.out = out
	e = out.SetDeadline(c.expires)
	c.mu.Unlock()
	defer out.Close()
	if e != nil {
		return
	}
	responses := make(chan struct{})
	go func() {
		defer close(responses)
		var buffer [landatagram.MaxPayload]byte
		for {
			n, e := out.ReadPacket(buffer[:])
			if e != nil || n < 0 || n > len(buffer) {
				return
			}
			c.mu.Lock()
			allowed := !c.closed && !p.retiring && c.lease.AllowsPeer(p.address.Addr(), c.deps.Clock.Now())
			if allowed {
				p.last = c.deps.Clock.Now()
			}
			c.mu.Unlock()
			if !allowed {
				return
			}
			written, e := socket.WriteTo(buffer[:n], net.UDPAddrFromAddrPort(p.address))
			if e != nil || written != n {
				return
			}
		}
	}()
	defer func() { out.Close(); <-responses }()
	for {
		select {
		case <-c.stop:
			return
		case <-p.stop:
			return
		case <-responses:
			return
		case packet := <-p.packets:
			c.mu.Lock()
			allowed := !c.closed && !p.retiring && c.lease.AllowsPeer(p.address.Addr(), c.deps.Clock.Now())
			c.mu.Unlock()
			if !allowed {
				return
			}
			if n, e := out.WritePacket(packet); e != nil || n != len(packet) {
				return
			}
		}
	}
}

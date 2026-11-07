package lanworker

import (
	"context"
	"io"
	"net"
	"orbwin.local/session/internal/landatagram"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
	"sync"
	"testing"
	"time"
)

type memoryPacket struct {
	data []byte
	peer *net.UDPAddr
}
type memoryPacketSocket struct {
	incoming, outgoing chan memoryPacket
	closed             chan struct{}
	once               sync.Once
}

func (s *memoryPacketSocket) ReadFrom(dst []byte) (int, net.Addr, error) {
	select {
	case p := <-s.incoming:
		n := copy(dst, p.data)
		return n, p.peer, nil
	case <-s.closed:
		return 0, nil, io.EOF
	}
}
func (s *memoryPacketSocket) WriteTo(data []byte, peer net.Addr) (int, error) {
	p := memoryPacket{append([]byte{}, data...), peer.(*net.UDPAddr)}
	select {
	case s.outgoing <- p:
		return len(data), nil
	case <-s.closed:
		return 0, io.ErrClosedPipe
	}
}
func (s *memoryPacketSocket) Close() error { s.once.Do(func() { close(s.closed) }); return nil }
func (s *memoryPacketSocket) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("192.168.4.10"), Port: 58080}
}
func (s *memoryPacketSocket) SetDeadline(time.Time) error      { return nil }
func (s *memoryPacketSocket) SetReadDeadline(time.Time) error  { return nil }
func (s *memoryPacketSocket) SetWriteDeadline(time.Time) error { return nil }

type memoryDatagram struct {
	requests, replies chan []byte
	closed            chan struct{}
	once              sync.Once
}

func newMemoryDatagram() *memoryDatagram {
	return &memoryDatagram{requests: make(chan []byte, 16), replies: make(chan []byte, 16), closed: make(chan struct{})}
}
func (c *memoryDatagram) ReadPacket(dst []byte) (int, error) {
	select {
	case p := <-c.replies:
		return copy(dst, p), nil
	case <-c.closed:
		return 0, io.EOF
	}
}
func (c *memoryDatagram) WritePacket(p []byte) (int, error) {
	select {
	case c.requests <- append([]byte{}, p...):
		return len(p), nil
	case <-c.closed:
		return 0, io.ErrClosedPipe
	}
}
func (c *memoryDatagram) Close() error                     { c.once.Do(func() { close(c.closed) }); return nil }
func (c *memoryDatagram) SetDeadline(time.Time) error      { return nil }
func (c *memoryDatagram) SetReadDeadline(time.Time) error  { return nil }
func (c *memoryDatagram) SetWriteDeadline(time.Time) error { return nil }
func udpWorker(t *testing.T) (*workerFixture, *memoryPacketSocket) {
	f := makeWorker(t, func(s *lanlease.Spec) { s.Request.Scope.Protocol = "udp"; s.Publication.Protocol = "udp" }, nil)
	socket := &memoryPacketSocket{incoming: make(chan memoryPacket, 32), outgoing: make(chan memoryPacket, 32), closed: make(chan struct{})}
	f.core.deps.ListenPacket = func(context.Context, string) (net.PacketConn, error) { return socket, nil }
	return f, socket
}
func getPacket(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	select {
	case p := <-ch:
		return p
	case <-time.After(time.Second):
		t.Fatal("packet not delivered")
		return nil
	}
}
func TestDatagramWorkerPreservesPacketsEmptyPayloadAndCIDRThenTTLClosesFlows(t *testing.T) {
	f, socket := udpWorker(t)
	association := newMemoryDatagram()
	requests := make(chan lanhelper.Stream, 2)
	f.core.deps.DialDatagram = func(_ context.Context, request lanhelper.Stream) (landatagram.Connection, error) {
		requests <- request
		return association, nil
	}
	grantWorker(t, f)
	socket.incoming <- memoryPacket{[]byte("denied"), &net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 1234}}
	peer := &net.UDPAddr{IP: net.ParseIP("192.168.4.20"), Port: 60000}
	socket.incoming <- memoryPacket{[]byte{}, peer}
	socket.incoming <- memoryPacket{[]byte{0, 1, 2, 3}, peer}
	var request lanhelper.Stream
	select {
	case request = <-requests:
	case <-time.After(time.Second):
		t.Fatal("no typed UDP association")
	}
	if request.Peer != "192.168.4.20:60000" {
		t.Fatal("unauthorized peer reached namespace")
	}
	if len(getPacket(t, association.requests)) != 0 || string(getPacket(t, association.requests)) != string([]byte{0, 1, 2, 3}) {
		t.Fatal("packet boundaries/empty packet changed")
	}
	association.replies <- []byte{}
	association.replies <- []byte("answer")
	for i := 0; i < 2; i++ {
		select {
		case p := <-socket.outgoing:
			if p.peer.String() != peer.String() || (i == 0 && len(p.data) != 0) || (i == 1 && string(p.data) != "answer") {
				t.Fatal("reply identity/boundary changed")
			}
		case <-time.After(time.Second):
			t.Fatal("reply missing")
		}
	}
	select {
	case <-requests:
		t.Fatal("denied or repeated peer created extra association")
	default:
	}
	f.clock.advance(6 * time.Second)
	awaitClosed(t, f.core)
	select {
	case <-association.closed:
	default:
		t.Fatal("TTL retained active datagram channel")
	}
	select {
	case <-socket.closed:
	default:
		t.Fatal("TTL retained UDP ingress")
	}
}
func TestDatagramWorkerBoundedPendingPeersRemainOwnedUntilLateDialCloses(t *testing.T) {
	f, socket := udpWorker(t)
	entered := make(chan struct{}, landatagram.MaxPeers)
	release := make(chan struct{})
	created := make(chan *memoryDatagram, landatagram.MaxPeers)
	f.core.deps.DialDatagram = func(context.Context, lanhelper.Stream) (landatagram.Connection, error) {
		entered <- struct{}{}
		<-release
		c := newMemoryDatagram()
		created <- c
		return c, nil
	}
	grantWorker(t, f)
	for i := 0; i < landatagram.MaxPeers; i++ {
		socket.incoming <- memoryPacket{[]byte("first"), &net.UDPAddr{IP: net.ParseIP("192.168.4.20"), Port: 60000 + i}}
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("pending peer not admitted")
		}
	}
	socket.incoming <- memoryPacket{[]byte("over-capacity"), &net.UDPAddr{IP: net.ParseIP("192.168.4.20"), Port: 61000}}
	f.core.Stop()
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if f.core.Wait(short) == nil {
		t.Fatal("blocked datagram setup escaped join")
	}
	close(release)
	awaitClosed(t, f.core)
	if len(created) != landatagram.MaxPeers {
		t.Fatal("peer pool exceeded sixteen pending associations")
	}
	for len(created) > 0 {
		c := <-created
		select {
		case <-c.closed:
		default:
			t.Fatal("late UDP setup leaked")
		}
	}
}
func TestDatagramWorkerIdlePeersRetireWithoutClosingHealthyLease(t *testing.T) {
	f, socket := udpWorker(t)
	association := newMemoryDatagram()
	f.core.deps.DialDatagram = func(context.Context, lanhelper.Stream) (landatagram.Connection, error) { return association, nil }
	grantWorker(t, f)
	socket.incoming <- memoryPacket{[]byte("one"), &net.UDPAddr{IP: net.ParseIP("192.168.4.20"), Port: 60000}}
	getPacket(t, association.requests)
	for sequence := uint64(2); sequence <= 5; sequence++ {
		f.clock.advance(4 * time.Second)
		o := f.grant.Grant.Observation
		o.ReadAt = f.clock.Now()
		if e := f.core.Apply(context.Background(), lanhelper.Frame{Version: 2, Kind: "refresh", Sequence: sequence, LeaseSHA: f.core.LeaseSHA(), Observation: &o}); e != nil {
			t.Fatal(e)
		}
	}
	select {
	case <-association.closed:
	case <-time.After(time.Second):
		t.Fatal("idle peer was not retired")
	}
	if f.core.Closed() {
		t.Fatal("idle association retired entire refreshed lease")
	}
	f.core.Stop()
	awaitClosed(t, f.core)
}

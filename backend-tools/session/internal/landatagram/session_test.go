package landatagram

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestLeasePeerCapacityAndIndependentRelease(t *testing.T) {
	var slots PeerSlots
	var release []func()
	for i := 1; i <= MaxPeers; i++ {
		f, e := slots.Reserve(netip.AddrPortFrom(netip.MustParseAddr("192.168.1.2"), uint16(i)))
		if e != nil {
			t.Fatal(e)
		}
		release = append(release, f)
	}
	if _, e := slots.Reserve(netip.MustParseAddrPort("192.168.1.3:1")); e == nil {
		t.Fatal("17th peer admitted")
	}
	if _, e := slots.Reserve(netip.MustParseAddrPort("192.168.1.2:1")); e == nil {
		t.Fatal("duplicate association")
	}
	release[0]()
	release[0]()
	if _, e := slots.Reserve(netip.MustParseAddrPort("192.168.1.3:1")); e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{"[::1]:1", "0.0.0.0:1", "192.168.1.2:0"} {
		if _, e := slots.Reserve(netip.MustParseAddrPort(p)); e == nil {
			t.Fatal("invalid peer", p)
		}
	}
}

type fakePacket struct {
	read    chan []byte
	writes  chan []byte
	closed  chan struct{}
	once    sync.Once
	stall   bool
	entered chan struct{}
}

func packetFake() *fakePacket {
	return &fakePacket{read: make(chan []byte, 2), writes: make(chan []byte, 2), closed: make(chan struct{}), entered: make(chan struct{}, 1)}
}
func (f *fakePacket) ReadPacket(b []byte) (int, error) {
	select {
	case <-f.closed:
		return 0, io.EOF
	case p := <-f.read:
		copy(b, p)
		return len(p), nil
	}
}
func (f *fakePacket) WritePacket(b []byte) (int, error) {
	select {
	case f.entered <- struct{}{}:
	default:
	}
	if f.stall {
		<-f.closed
		return 0, io.ErrClosedPipe
	}
	select {
	case <-f.closed:
		return 0, io.ErrClosedPipe
	case f.writes <- append([]byte{}, b...):
		return len(b), nil
	}
}
func (f *fakePacket) Close() error                     { f.once.Do(func() { close(f.closed) }); return nil }
func (f *fakePacket) SetDeadline(time.Time) error      { return nil }
func (f *fakePacket) SetReadDeadline(time.Time) error  { return nil }
func (f *fakePacket) SetWriteDeadline(time.Time) error { return nil }
func TestBridgeEmptyPacketAndCancellationJoinsStalledWrite(t *testing.T) {
	a, b := packetFake(), packetFake()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Bridge(ctx, a, b) }()
	a.read <- []byte{}
	select {
	case p := <-b.writes:
		if len(p) != 0 {
			t.Fatal(p)
		}
	case <-time.After(time.Second):
		t.Fatal("empty packet lost")
	}
	// Drain the earlier write-entry observation, then wait for the stalled one.
	<-b.entered
	b.stall = true // previous write completed; only the next writer uses this flag.
	a.read <- []byte("blocked")
	select {
	case <-b.entered:
	case <-time.After(time.Second):
		t.Fatal("write did not start")
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("pumps not joined")
	}
	select {
	case <-a.closed:
	default:
		t.Fatal("left retained")
	}
	select {
	case <-b.closed:
	default:
		t.Fatal("right retained")
	}
}

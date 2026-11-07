//go:build windows

package lanworker

import (
	"context"
	"net"
	"orbwin.local/session/internal/landatagram"
	"orbwin.local/session/internal/lanhelper"
	"time"
)

func (c *NativeChild) receiveDatagram(ctx context.Context, conn net.Conn, f lanhelper.Frame) {
	out, e := c.dialDatagram(ctx, *f.Stream)
	if e != nil || out == nil {
		if out != nil {
			out.Close()
		}
		return
	}
	c.mu.Lock()
	if c.stopping {
		c.mu.Unlock()
		out.Close()
		return
	}
	c.datagrams[out] = true
	c.mu.Unlock()
	defer func() { out.Close(); c.mu.Lock(); delete(c.datagrams, out); c.mu.Unlock() }()
	ready := lanhelper.Frame{Version: 2, Kind: "datagram-ready", Sequence: f.Sequence, LeaseSHA: f.LeaseSHA, Stream: &lanhelper.Stream{ID: f.Stream.ID}}
	if lanhelper.Write(conn, ready) != nil || conn.SetDeadline(time.Time{}) != nil {
		return
	}
	wire := landatagram.New(conn)
	defer wire.Close()
	copyNativeDatagrams(wire, out)
}
func copyNativeDatagrams(a, b landatagram.Connection) {
	done := make(chan struct{}, 2)
	copyOne := func(dst, src landatagram.Connection) {
		defer func() { a.Close(); b.Close(); done <- struct{}{} }()
		var buffer [landatagram.MaxPayload]byte
		for {
			n, e := src.ReadPacket(buffer[:])
			if e != nil || n < 0 || n > len(buffer) {
				return
			}
			if written, e := dst.WritePacket(buffer[:n]); e != nil || written != n {
				return
			}
		}
	}
	go copyOne(a, b)
	copyOne(b, a)
	<-done
	<-done
}

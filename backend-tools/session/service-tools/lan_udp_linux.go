//go:build linux

package main

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"orbwin.local/session/internal/landatagram"
	"orbwin.local/session/internal/streamcontrol"
)

func udpArguments(rawPort, rawToken string) (string, streamcontrol.Token, error) {
	p, err := port(rawPort)
	if err != nil {
		return "", streamcontrol.Token{}, err
	}
	t, err := streamcontrol.ParseToken(rawToken)
	if err != nil {
		return "", t, err
	}
	return "127.0.0.1:" + p, t, nil
}

type stdioDatagram struct {
	input, output *os.File
	watchdog      *landatagram.ProcessWatchdog
}

func (s stdioDatagram) Read(b []byte) (int, error) {
	done, err := s.watchdog.BeginRead()
	if err != nil {
		return 0, err
	}
	defer done()
	return s.input.Read(b)
}
func (s stdioDatagram) Write(b []byte) (int, error) {
	done, err := s.watchdog.BeginWrite()
	if err != nil {
		return 0, err
	}
	defer done()
	return s.output.Write(b)
}
func (s stdioDatagram) Close() error {
	s.watchdog.StartClose()
	return errors.Join(s.input.Close(), s.output.Close())
}
func (s stdioDatagram) SetReadDeadline(t time.Time) error  { return s.watchdog.SetReadDeadline(t) }
func (s stdioDatagram) SetWriteDeadline(t time.Time) error { return s.watchdog.SetWriteDeadline(t) }

// UDP is connected to one literal loopback destination. ReadMsgUDP reports
// truncation explicitly, including a too-large datagram which fills the buffer.
type udpSocket interface {
	ReadMsgUDP([]byte, []byte) (int, int, int, *net.UDPAddr, error)
	Write([]byte) (int, error)
	Close() error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}
type udpDatagram struct {
	socket                      udpSocket
	mu                          sync.Mutex
	readDeadline, writeDeadline time.Time
	now                         func() time.Time
}

func (u *udpDatagram) deadline(requested time.Time, budget time.Duration) time.Time {
	now := time.Now()
	if u.now != nil {
		now = u.now()
	}
	limit := now.Add(budget)
	if !requested.IsZero() && requested.Before(limit) {
		return requested
	}
	return limit
}

func (u *udpDatagram) ReadPacket(b []byte) (int, error) {
	u.mu.Lock()
	err := u.socket.SetReadDeadline(u.deadline(u.readDeadline, landatagram.ReadBudget))
	u.mu.Unlock()
	if err != nil {
		return 0, err
	}
	n, _, flags, _, err := u.socket.ReadMsgUDP(b, nil)
	if err != nil {
		return 0, err
	}
	if flags&syscall.MSG_TRUNC != 0 || n > landatagram.MaxPayload {
		return 0, landatagram.ErrOversize
	}
	return n, nil
}
func (u *udpDatagram) WritePacket(b []byte) (int, error) {
	if len(b) > landatagram.MaxPayload {
		return 0, landatagram.ErrOversize
	}
	u.mu.Lock()
	err := u.socket.SetWriteDeadline(u.deadline(u.writeDeadline, landatagram.WriteBudget))
	u.mu.Unlock()
	if err != nil {
		return 0, err
	}
	return u.socket.Write(b)
}
func (u *udpDatagram) Close() error { return u.socket.Close() }
func (u *udpDatagram) SetDeadline(t time.Time) error {
	return errors.Join(u.SetReadDeadline(t), u.SetWriteDeadline(t))
}
func (u *udpDatagram) SetReadDeadline(t time.Time) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.readDeadline = t
	return u.socket.SetReadDeadline(u.deadline(t, landatagram.ReadBudget))
}
func (u *udpDatagram) SetWriteDeadline(t time.Time) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.writeDeadline = t
	return u.socket.SetWriteDeadline(u.deadline(t, landatagram.WriteBudget))
}

func dialUDPStdio(ctx context.Context, rawPort, rawToken string) error {
	address, token, err := udpArguments(rawPort, rawToken)
	if err != nil {
		return err
	}
	control, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", streamcontrol.SocketPath)
	if err != nil {
		return err
	}
	// Revocation/control loss terminates this dedicated exec, including inherited
	// stdin/stdout. It cannot terminate the netns owner or another association.
	lease, err := streamcontrol.Register(ctx, control, token, func() { os.Exit(125) })
	if err != nil {
		return err
	}
	defer lease.Close()
	raw, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "udp4", address)
	if err != nil {
		return err
	}
	socket, ok := raw.(*net.UDPConn)
	if !ok {
		raw.Close()
		return errors.New("connected IPv4 UDP socket required")
	}
	if err = socket.SetReadBuffer(128 << 10); err == nil {
		err = socket.SetWriteBuffer(128 << 10)
	}
	if err != nil {
		socket.Close()
		return err
	}
	watchdog := landatagram.NewProcessWatchdog(func() { os.Exit(125) })
	stdio := landatagram.New(stdioDatagram{os.Stdin, os.Stdout, watchdog})
	err = landatagram.Bridge(ctx, stdio, &udpDatagram{socket: socket})
	watchdog.Joined() // Bridge returned only after both pumps joined.
	return err
}

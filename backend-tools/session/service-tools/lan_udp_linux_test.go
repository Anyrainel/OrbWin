//go:build linux

package main

import (
	"context"
	"errors"
	"net"
	"orbwin.local/session/internal/landatagram"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestUDPArgumentsOnlyLiteralLoopbackCanonicalPortAndToken(t *testing.T) {
	address, _, e := udpArguments("65535", strings.Repeat("a", 32))
	if e != nil || address != "127.0.0.1:65535" {
		t.Fatal(address, e)
	}
	for _, p := range []string{"0", "01", "65536", "-1", "localhost:53", "127.0.0.2:53", "53;true", " 53"} {
		if _, _, e := udpArguments(p, strings.Repeat("a", 32)); e == nil {
			t.Fatal("accepted", p)
		}
	}
	for _, token := range []string{"", strings.Repeat("A", 32), strings.Repeat("a", 31), strings.Repeat("g", 32)} {
		if _, _, e := udpArguments("53", token); e == nil {
			t.Fatal("accepted token")
		}
	}
}

// Opt-in fresh anonymous-pipe subprocess fixture, compile-only until root
// review. It never invokes run(), a socket, Docker, a workload or inherited
// application stdio in the parent. Every child is this exact test executable.
func TestUDPProcessWatchdogAnonymousPipes(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_UDP_PIPE") != "1" {
		t.Skip("explicit isolated pipe qualification required")
	}
	for _, mode := range []string{"read", "write", "close", "joined"} {
		t.Run(mode, func(t *testing.T) {
			input, feed, e := os.Pipe()
			if e != nil {
				t.Fatal(e)
			}
			defer input.Close()
			defer feed.Close()
			drain, output, e := os.Pipe()
			if e != nil {
				t.Fatal(e)
			}
			defer drain.Close()
			defer output.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestUDPWatchdogChild$")
			cmd.Env = append(os.Environ(), "ORBWIN_UDP_PIPE_CHILD="+mode)
			cmd.Stdin = input
			cmd.Stdout = output
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			if mode == "joined" {
				if _, e = feed.Write([]byte{1}); e != nil {
					t.Fatal(e)
				}
			}
			e = cmd.Wait()
			if ctx.Err() != nil {
				t.Fatal("watchdog required parent timeout", ctx.Err())
			}
			code := 0
			if e != nil {
				var x *exec.ExitError
				if !errors.As(e, &x) {
					t.Fatal(e)
				}
				code = x.ExitCode()
			}
			if mode == "joined" {
				if code != 0 {
					t.Fatal("joined timer killed", code)
				}
			} else if mode == "close" {
				if code != 0 && code != 125 {
					t.Fatal(code)
				}
			} else if code != 125 {
				t.Fatal("deadline did not terminate dedicated child", code)
			}
		})
	}
}
func TestUDPWatchdogChild(t *testing.T) {
	mode := os.Getenv("ORBWIN_UDP_PIPE_CHILD")
	if mode == "" {
		t.Skip("fixture child only")
	}
	if mode != "read" && mode != "write" && mode != "close" && mode != "joined" {
		os.Exit(123)
	}
	// Prove this fixture exercises inherited BLOCKING stdio, not os.Pipe's
	// ordinary Go pollable handles. Neither production nor this check changes it.
	for _, fd := range []uintptr{0, 1} {
		flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
		if errno != 0 || flags&syscall.O_NONBLOCK != 0 {
			os.Exit(123)
		}
	}
	w := landatagram.NewProcessWatchdog(func() { os.Exit(125) })
	s := stdioDatagram{os.Stdin, os.Stdout, w}
	if mode == "write" {
		_ = s.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
		_, _ = s.Write(make([]byte, 4<<20))
		os.Exit(122)
	}
	if mode == "close" {
		go func() { time.Sleep(30 * time.Millisecond); _ = s.Close() }()
		_, _ = s.Read(make([]byte, 1))
		w.Joined()
		return
	}
	_ = s.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, _ = s.Read(make([]byte, 1))
	if mode == "read" {
		os.Exit(122)
	}
	_ = s.Close()
	w.Joined()
	time.Sleep(80 * time.Millisecond)
}

type fakeUDPSocket struct {
	n, flags        int
	readAt, writeAt time.Time
}

func (f *fakeUDPSocket) ReadMsgUDP([]byte, []byte) (int, int, int, *net.UDPAddr, error) {
	return f.n, 0, f.flags, nil, nil
}
func (f *fakeUDPSocket) Write(b []byte) (int, error)        { return len(b), nil }
func (f *fakeUDPSocket) Close() error                       { return nil }
func (f *fakeUDPSocket) SetReadDeadline(t time.Time) error  { f.readAt = t; return nil }
func (f *fakeUDPSocket) SetWriteDeadline(t time.Time) error { f.writeAt = t; return nil }
func TestUDPTruncationEmptyAndDeadlineUsingFakeSocket(t *testing.T) {
	now := time.Unix(100, 0)
	s := &fakeUDPSocket{}
	u := &udpDatagram{socket: s, now: func() time.Time { return now }}
	if n, e := u.ReadPacket(make([]byte, landatagram.MaxPayload+1)); n != 0 || e != nil {
		t.Fatal("empty lost", n, e)
	}
	for _, v := range []struct{ n, flags int }{{5, syscall.MSG_TRUNC}, {landatagram.MaxPayload + 1, 0}} {
		s.n = v.n
		s.flags = v.flags
		if _, e := u.ReadPacket(make([]byte, landatagram.MaxPayload+1)); e != landatagram.ErrOversize {
			t.Fatal("truncation accepted", e)
		}
	}
	early := now.Add(time.Second)
	_ = u.SetDeadline(early)
	s.n = 0
	s.flags = 0
	_, _ = u.ReadPacket(nil)
	_, _ = u.WritePacket(nil)
	if !s.readAt.Equal(early) || !s.writeAt.Equal(early) {
		t.Fatal("caller deadline overwritten")
	}
}

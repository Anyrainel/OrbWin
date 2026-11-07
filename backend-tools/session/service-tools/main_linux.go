//go:build linux

// Static local-service tools. Containers receive this binary, never a daemon
// socket, Windows key or target filesystem/PID namespace. Network-namespace
// helpers can dial only their target's loopback. The front relay authenticates a
// fixed Windows gateway with a dedicated public root; application HTTP/TLS bytes
// stay opaque and application certificates/signing keys remain on Windows.
package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"

	"orbwin.local/session/internal/gateway"
	"orbwin.local/session/internal/streamcontrol"
)

const leaseDirectory = "/run/orbwin"
const leaseSeconds int64 = 45

func port(raw string) (string, error) {
	value, err := strconv.ParseUint(raw, 10, 16)
	if err != nil || value == 0 || strconv.FormatUint(value, 10) != raw {
		return "", errors.New("canonical TCP port in 1–65535 required")
	}
	return raw, nil
}
func uptime() (int64, error) {
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return 0, err
	}
	return info.Uptime, nil
}
func sessionValid(session string) bool {
	return regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(session)
}
func createLease(directory, session string, now int64) error {
	if !sessionValid(session) || now < 0 {
		return errors.New("invalid service lease identity")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(directory, "identity"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = file.WriteString(session)
	closed := file.Close()
	if err != nil {
		return err
	}
	if closed != nil {
		return closed
	}
	return renewLease(directory, session, now)
}
func renewLease(directory, session string, now int64) error {
	if !sessionValid(session) || now < 0 {
		return errors.New("invalid service lease renewal")
	}
	file, err := os.Open(filepath.Join(directory, "identity"))
	if err != nil {
		return err
	}
	identity, err := io.ReadAll(io.LimitReader(file, 33))
	file.Close()
	if err != nil {
		return err
	}
	if string(identity) != session {
		return errors.New("service lease belongs to another session")
	}
	temporary, err := os.CreateTemp(directory, ".renew-")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	var frame [8]byte
	binary.LittleEndian.PutUint64(frame[:], uint64(now))
	if _, err = temporary.Write(frame[:]); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), filepath.Join(directory, "lease"))
}
func liveLease(directory string, now int64) bool {
	file, err := os.Open(filepath.Join(directory, "lease"))
	if err != nil {
		return false
	}
	defer file.Close()
	var frame [9]byte
	count, _ := io.ReadFull(file, frame[:])
	if count != 8 {
		return false
	}
	previous := binary.LittleEndian.Uint64(frame[:8])
	return now >= 0 && previous <= uint64(now) && uint64(now)-previous <= uint64(leaseSeconds)
}

func waitRelayReady(ctx context.Context, directory, session string) error {
	if !sessionValid(session) {
		return errors.New("exact relay readiness session required")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for {
		file, err := os.Open(filepath.Join(directory, "ready"))
		if err == nil {
			marker, readErr := io.ReadAll(io.LimitReader(file, 33))
			closed := file.Close()
			if readErr != nil || closed != nil {
				return errors.Join(readErr, closed)
			}
			if string(marker) != session {
				return errors.New("relay readiness belongs to a different session")
			}
			return ctx.Err()
		}
		if !os.IsNotExist(err) {
			return err
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func leaseContext(parent context.Context, directory string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now, err := uptime()
				if err != nil || !liveLease(directory, now) {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, cancel
}

func dialStdio(ctx context.Context, address string, input io.Reader, output io.Writer) error {
	connection, err := (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer connection.Close()
	tcp, ok := connection.(*net.TCPConn)
	if !ok {
		return errors.New("TCP stream required")
	}
	stream := &activeTCP{connection: tcp}
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			tcp.Close()
		case <-stopped:
		}
	}()
	uploaded := make(chan error, 1)
	go func() {
		_, err := io.Copy(stream, input)
		closed := tcp.CloseWrite()
		if err == nil {
			err = closed
		}
		uploaded <- err
	}()
	_, downloadErr := io.Copy(output, stream)
	// When the remote closes first, ordinary process exit closes the inherited
	// exec stdin. Do not block forever waiting for a caller that has no more data.
	select {
	case uploadErr := <-uploaded:
		return errors.Join(downloadErr, uploadErr)
	default:
		return downloadErr
	}
}

// Bound a detached exec whose caller disappeared without stopping the target or
// a helper serving other connections. Ongoing bytes renew the deadline; a silent
// stream expires after two minutes. The controller must expose this idle limit.
type activeTCP struct{ connection *net.TCPConn }

func (connection *activeTCP) Read(data []byte) (int, error) {
	if err := connection.connection.SetDeadline(time.Now().Add(2 * time.Minute)); err != nil {
		return 0, err
	}
	return connection.connection.Read(data)
}
func (connection *activeTCP) Write(data []byte) (int, error) {
	if err := connection.connection.SetDeadline(time.Now().Add(2 * time.Minute)); err != nil {
		return 0, err
	}
	return connection.connection.Write(data)
}
func relay(ctx context.Context, listener net.Listener, destination, session string, root []byte, mode gateway.Mode) error {
	ctx, cancel := context.WithCancel(ctx)
	var active sync.WaitGroup
	var firstFailure sync.Once
	slots := make(chan struct{}, 128)
	defer func() { cancel(); listener.Close(); active.Wait() }()
	go func() { <-ctx.Done(); listener.Close() }()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
		default:
			connection.Close()
			continue
		}
		active.Add(1)
		go func() {
			defer active.Done()
			defer func() { <-slots }()
			defer connection.Close()
			peer, err := gateway.Dial(ctx, destination, session, root, mode)
			if err != nil {
				firstFailure.Do(func() {
					message := err.Error()
					if len(message) > 512 {
						message = message[:512]
					}
					fmt.Fprintln(os.Stderr, "OrbWin relay gateway:", message)
				})
				return
			}
			defer peer.Close()
			_ = gateway.CopyDuplex(ctx, connection, peer)
		}()
	}
}
func run(ctx context.Context, args []string) error {
	if len(args) == 3 && args[0] == "dial-udp" {
		return dialUDPStdio(ctx, args[1], args[2])
	}
	if len(args) == 3 && args[0] == "dial" {
		selected, err := port(args[1])
		if err != nil {
			return err
		}
		token, err := streamcontrol.ParseToken(args[2])
		if err != nil {
			return err
		}
		control, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", streamcontrol.SocketPath)
		if err != nil {
			return err
		}
		lease, err := streamcontrol.Register(ctx, control, token, func() { os.Exit(125) })
		if err != nil {
			return err
		}
		defer lease.Close()
		return dialStdio(ctx, "127.0.0.1:"+selected, os.Stdin, os.Stdout)
	}
	if len(args) == 2 && args[0] == "revoke" {
		token, err := streamcontrol.ParseToken(args[1])
		if err != nil {
			return err
		}
		control, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", streamcontrol.SocketPath)
		if err != nil {
			return err
		}
		if err := streamcontrol.Revoke(ctx, control, token); err != nil {
			return err
		}
		_, err = fmt.Fprint(os.Stdout, "revoked\n")
		return err
	}
	if len(args) == 4 && args[0] == "ready-relay" {
		gatewayPort, root, err := parseRelayGateway(args[1], args[2], args[3])
		if err != nil {
			return err
		}
		if err = waitRelayReady(ctx, leaseDirectory, args[1]); err != nil {
			return err
		}
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		peer, err := gateway.Dial(probeCtx, "host.docker.internal:"+gatewayPort, args[1], root, gateway.HTTP)
		if err != nil {
			return err
		}
		_ = peer.NetConn().Close()
		now, err := uptime()
		if err != nil {
			return err
		}
		if err = renewLease(leaseDirectory, args[1], now); err != nil {
			return err
		}
		_, err = fmt.Fprint(os.Stdout, "renewed\n")
		return err
	}
	if len(args) == 2 && (args[0] == "renew" || args[0] == "renew-netns" || args[0] == "ready-netns") {
		if args[0] != "renew" {
			readyCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			for {
				control, err := (&net.Dialer{Timeout: time.Second}).DialContext(readyCtx, "unix", streamcontrol.SocketPath)
				if err == nil {
					if err := streamcontrol.Healthy(readyCtx, control); err != nil {
						return err
					}
					break
				}
				if args[0] != "ready-netns" {
					return err
				}
				timer := time.NewTimer(20 * time.Millisecond)
				select {
				case <-readyCtx.Done():
					timer.Stop()
					return readyCtx.Err()
				case <-timer.C:
				}
			}
		}
		now, err := uptime()
		if err != nil {
			return err
		}
		if err := renewLease(leaseDirectory, args[1], now); err != nil {
			return err
		}
		_, err = fmt.Fprint(os.Stdout, "renewed\n")
		return err
	}
	if (len(args) != 2 || args[0] != "netns") && (len(args) != 4 || args[0] != "relay") {
		return errors.New("unsupported service tool invocation")
	}
	var gatewayPort string
	var root []byte
	if args[0] == "relay" {
		var err error
		gatewayPort, root, err = parseRelayGateway(args[1], args[2], args[3])
		if err != nil {
			return err
		}
	}
	now, err := uptime()
	if err != nil {
		return err
	}
	if err = createLease(leaseDirectory, args[1], now); err != nil {
		return err
	}
	owned, cancel := leaseContext(ctx, leaseDirectory)
	defer cancel()
	if args[0] == "netns" {
		listener, err := net.Listen("unix", streamcontrol.SocketPath)
		if err != nil {
			return err
		}
		if err := os.Chmod(streamcontrol.SocketPath, 0600); err != nil {
			listener.Close()
			return err
		}
		server := streamcontrol.NewServer(listener)
		stop := context.AfterFunc(owned, func() { server.Close() })
		defer stop()
		return server.Serve()
	}
	httpListener, err := net.Listen("tcp", ":80")
	if err != nil {
		return err
	}
	defer httpListener.Close()
	tlsListener, err := net.Listen("tcp", ":443")
	if err != nil {
		return err
	}
	defer tlsListener.Close()
	// A running container/lease alone is not listener readiness. Publish only
	// after both binds succeed; TCP accepts can queue before workers are scheduled.
	if err = os.WriteFile(filepath.Join(leaseDirectory, "ready"), []byte(args[1]), 0600); err != nil {
		return err
	}
	result := make(chan error, 2)
	go func() {
		result <- relay(owned, httpListener, "host.docker.internal:"+gatewayPort, args[1], root, gateway.HTTP)
	}()
	go func() {
		result <- relay(owned, tlsListener, "host.docker.internal:"+gatewayPort, args[1], root, gateway.HTTPS)
	}()
	first := <-result
	cancel()
	second := <-result
	return errors.Join(first, second)
}

func parseRelayGateway(session, rawPort, rawRoot string) (string, []byte, error) {
	selected, err := port(rawPort)
	if err != nil {
		return "", nil, err
	}
	if len(rawRoot) > gateway.RootBudget*2 {
		return "", nil, errors.New("gateway public root exceeds its input budget")
	}
	root, err := base64.StdEncoding.Strict().DecodeString(rawRoot)
	if err != nil || base64.StdEncoding.EncodeToString(root) != rawRoot {
		return "", nil, errors.New("canonical gateway public certificate required")
	}
	if _, err = gateway.ClientConfig(session, root); err != nil {
		return "", nil, err
	}
	return selected, root, nil
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "OrbWin service tools:", err)
		os.Exit(125)
	}
}

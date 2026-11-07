package localnet

// Non-TTY Docker exec uses Moby's eight-byte stdcopy framing. Keep HTTP/TLS/
// websocket bytes unmodified, bound frame/error lengths before allocation, and
// retain ordinary net.Conn deadlines and independent write-half closure.
import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/client"
)

const maxExecFrame = 1 << 20

type execConnection struct {
	net.Conn
	reader     io.Reader
	readLock   sync.Mutex
	remaining  uint32
	stream     byte
	header     [8]byte
	headerRead int
	diagnostic []byte
	closeWrite func() error
	closeOnce  sync.Once
	closeErr   error
	revoke     func() error
}

func (connection *execConnection) Close() error {
	connection.closeOnce.Do(func() {
		connection.closeErr = connection.Conn.Close()
		if connection.revoke != nil {
			connection.closeErr = errors.Join(connection.closeErr, connection.revoke())
		}
	})
	return connection.closeErr
}
func (connection *execConnection) CloseWrite() error { return connection.closeWrite() }
func (connection *execConnection) Read(destination []byte) (int, error) {
	connection.readLock.Lock()
	defer connection.readLock.Unlock()
	if len(destination) == 0 {
		return 0, nil
	}
	for {
		if connection.remaining > 0 {
			if connection.stream != 1 {
				var diagnostic [8192]byte
				selected := diagnostic[:int(connection.remaining)]
				count, err := connection.reader.Read(selected)
				connection.diagnostic = append(connection.diagnostic, selected[:count]...)
				connection.remaining -= uint32(count)
				if err != nil {
					return 0, connection.readError(err)
				}
				if connection.remaining == 0 && connection.stream == 3 {
					_ = connection.Close()
					return 0, errors.New("Docker exec system error: " + strings.TrimSpace(string(connection.diagnostic)))
				}
				continue
			}
			selected := destination
			if len(selected) > int(connection.remaining) {
				selected = selected[:connection.remaining]
			}
			count, err := connection.reader.Read(selected)
			connection.remaining -= uint32(count)
			if err != nil {
				err = connection.readError(err)
			}
			return count, err
		}
		count, err := io.ReadFull(connection.reader, connection.header[connection.headerRead:])
		connection.headerRead += count
		if err != nil {
			return 0, connection.readError(err)
		}
		header := connection.header
		connection.headerRead = 0
		length := binary.BigEndian.Uint32(header[4:])
		if header[1] != 0 || header[2] != 0 || header[3] != 0 || length > maxExecFrame || (header[0] != 1 && header[0] != 2 && header[0] != 3) {
			_ = connection.Close()
			return 0, errors.New("invalid or excessive Docker exec stream frame")
		}
		if header[0] != 1 && (length > 8192 || len(connection.diagnostic)+int(length) > 8192) {
			_ = connection.Close()
			return 0, errors.New("namespace tunnel diagnostics exceeded 8 KiB")
		}
		connection.stream = header[0]
		connection.remaining = length
		if header[0] == 3 && length == 0 {
			_ = connection.Close()
			return 0, errors.New("Docker exec system error: " + strings.TrimSpace(string(connection.diagnostic)))
		}
	}
}

func (connection *execConnection) readError(err error) error {
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		return err
	}
	_ = connection.Close()
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		if connection.remaining > 0 || connection.headerRead > 0 {
			return io.ErrUnexpectedEOF
		}
		if len(connection.diagnostic) > 0 {
			return fmt.Errorf("namespace TCP tunnel failed: %s", strings.TrimSpace(string(connection.diagnostic)))
		}
	}
	return err
}

// The caller must own/verify the exact helper and its target incarnation. This
// only starts the fixed trusted dial command; no target executable, shell,
// arbitrary host, Docker socket or PID namespace is involved. DialContext governs
// setup only, like net.Dialer; the returned connection owns its later lifetime.
func DialNamespace(ctx context.Context, cli *client.Client, helperID string, port uint16) (net.Conn, error) {
	return dialNamespace(ctx, cli, helperID, port, nil)
}
func dialNamespace(ctx context.Context, cli *client.Client, helperID string, port uint16, cleanup func(context.Context, string) error) (net.Conn, error) {
	id, err := hex.DecodeString(helperID)
	if err != nil || len(id) != 32 || port == 0 {
		return nil, errors.New("exact namespace helper and TCP port required")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(nonce[:])
	if cleanup == nil {
		cleanup = func(ctx context.Context, token string) error { return revokeNamespaceStream(ctx, cli, helperID, token) }
	}
	revoke := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return cleanup(ctx, token)
	}
	created, err := cli.ExecCreate(ctx, helperID, client.ExecCreateOptions{Cmd: []string{"/orbwin-services", "dial", strconv.Itoa(int(port)), token}, AttachStdin: true, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return nil, err
	}
	attached, err := cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return nil, errors.Join(err, revoke())
	}
	return &execConnection{Conn: attached.Conn, reader: attached.Reader, closeWrite: attached.CloseWrite, revoke: revoke}, nil
}
func revokeNamespaceStream(ctx context.Context, cli *client.Client, helperID, token string) error {
	return runNamespaceCommand(ctx, cli, helperID, "revoke", token, "revoked\n")
}
func runNamespaceCommand(ctx context.Context, cli *client.Client, helperID, kind, token, expected string) error {
	if (kind != "revoke" && kind != "renew-netns" && kind != "ready-netns" && kind != "renew") || !fullID(helperID) {
		return errors.New("fixed owned namespace control command required")
	}
	raw, err := hex.DecodeString(token)
	if err != nil || len(raw) != 16 || token != strings.ToLower(token) {
		return errors.New("exact namespace control nonce/session required")
	}
	return runOwnedControlCommand(ctx, cli, helperID, []string{"/orbwin-services", kind, token}, expected)
}

func runRelayReady(ctx context.Context, cli *client.Client, record relayRecord) error {
	if err := record.Intent.validate(); err != nil {
		return err
	}
	if !fullID(record.ID) {
		return errors.New("exact owned relay ID required")
	}
	return runOwnedControlCommand(ctx, cli, record.ID, []string{"/orbwin-services", "ready-relay", record.Intent.Session, strconv.Itoa(int(record.Intent.Gateway.Port)), base64.StdEncoding.EncodeToString(record.Intent.Gateway.PublicRoot)}, "renewed\n")
}
func runOwnedControlCommand(ctx context.Context, cli *client.Client, helperID string, command []string, expected string) error {
	created, err := cli.ExecCreate(ctx, helperID, client.ExecCreateOptions{Cmd: command, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return err
	}
	attached, err := cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return err
	}
	stream := &execConnection{Conn: attached.Conn, reader: attached.Reader, closeWrite: attached.CloseWrite}
	defer stream.Close()
	stop := context.AfterFunc(ctx, func() { stream.Conn.Close() })
	defer stop()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(3 * time.Second)
	}
	if err := stream.SetDeadline(deadline); err != nil {
		return err
	}
	ack, err := io.ReadAll(io.LimitReader(stream, 9))
	if err != nil {
		return err
	}
	if !bytes.Equal(ack, []byte(expected)) {
		return errors.New("namespace control command did not return its exact acknowledgement")
	}
	for {
		state, err := cli.ExecInspect(ctx, created.ID, client.ExecInspectOptions{})
		if err != nil {
			return err
		}
		if state.ContainerID != helperID {
			return errors.New("namespace control exec belongs to a different helper")
		}
		if !state.Running {
			if state.ExitCode != 0 {
				return errors.New("namespace stream revocation command failed")
			}
			return nil
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

// Package streamcontrol owns diagnostic exec admission and revocation inside one
// isolated helper. Production transport is a private Unix socket, never a Docker
// socket or a target namespace filesystem/PID channel.
package streamcontrol

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

const SocketPath = "/run/orbwin/control.sock"
const HistoryLimit = 4096
const (
	register byte = 1
	revoke   byte = 2
	health   byte = 3
	accepted byte = 0
	denied   byte = 1
	full     byte = 2
)

type Token [16]byte

func ParseToken(text string) (Token, error) {
	var token Token
	raw, err := hex.DecodeString(text)
	if err != nil || len(raw) != 16 || text != strings.ToLower(text) {
		return token, errors.New("exact lowercase stream nonce required")
	}
	copy(token[:], raw)
	return token, nil
}
func write(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		count, err := writer.Write(data)
		if err != nil {
			return err
		}
		if count <= 0 || count > len(data) {
			return io.ErrShortWrite
		}
		data = data[count:]
	}
	return nil
}
func setup(ctx context.Context, connection net.Conn, operation byte, token Token) (result byte, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	interrupted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { connection.Close(); close(interrupted) })
	defer func() {
		if !stop() {
			<-interrupted
		}
		if ctx.Err() != nil {
			resultErr = ctx.Err()
		}
	}()
	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return 0, err
	}
	var frame [17]byte
	frame[0] = operation
	copy(frame[1:], token[:])
	if err := write(connection, frame[:]); err != nil {
		return 0, err
	}
	var ack [1]byte
	if _, err := io.ReadFull(connection, ack[:]); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		return 0, err
	}
	return ack[0], nil
}

type Lease struct {
	connection net.Conn
	mu         sync.Mutex
	closed     bool
	done       chan struct{}
}

// The callback must terminate the owned dial or close all its data resources.
// It runs on revocation AND unexpected control-channel loss. Normal Close after
// target TCP teardown suppresses it and joins the watcher.
func Register(ctx context.Context, connection net.Conn, token Token, onRevoked func()) (*Lease, error) {
	if onRevoked == nil {
		connection.Close()
		return nil, errors.New("owned stream termination callback required")
	}
	ack, err := setup(ctx, connection, register, token)
	if err != nil || ack != accepted {
		connection.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("stream nonce revoked, repeated or admission history exhausted")
	}
	lease := &Lease{connection: connection, done: make(chan struct{})}
	go func() {
		defer close(lease.done)
		var event [1]byte
		_, _ = io.ReadFull(connection, event[:])
		lease.mu.Lock()
		closed := lease.closed
		lease.mu.Unlock()
		if !closed {
			onRevoked()
		}
	}()
	return lease, nil
}
func (lease *Lease) Close() error {
	lease.mu.Lock()
	lease.closed = true
	lease.mu.Unlock()
	err := lease.connection.Close()
	<-lease.done
	return err
}
func Revoke(ctx context.Context, connection net.Conn, token Token) error {
	defer connection.Close()
	ack, err := setup(ctx, connection, revoke, token)
	if err != nil {
		return err
	}
	if ack != accepted {
		return errors.New("stream revocation was not acknowledged")
	}
	return nil
}
func Healthy(ctx context.Context, connection net.Conn) error {
	defer connection.Close()
	ack, err := setup(ctx, connection, health, Token{})
	if err != nil {
		return err
	}
	if ack != accepted {
		return errors.New("namespace stream admission history requires helper replacement")
	}
	return nil
}

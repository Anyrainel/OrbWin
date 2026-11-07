package streamcontrol

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

type entry struct {
	connection net.Conn
	done       chan struct{}
	revokeOnce sync.Once
}
type Server struct {
	mu              sync.Mutex
	listener        net.Listener
	active          map[Token]*entry
	seen            map[Token]struct{}
	connections     map[net.Conn]struct{}
	limit           int
	slots           chan struct{}
	ctx             context.Context
	cancel          context.CancelFunc
	workers         sync.WaitGroup
	closed, serving bool
	closeDone       chan struct{}
}

func NewServer(listener net.Listener) *Server { return newServer(listener, HistoryLimit) }
func newServer(listener net.Listener, limit int) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{listener: listener, active: make(map[Token]*entry), seen: make(map[Token]struct{}), connections: make(map[net.Conn]struct{}), limit: limit, slots: make(chan struct{}, 32), ctx: ctx, cancel: cancel, closeDone: make(chan struct{})}
}
func (server *Server) Serve() error {
	server.mu.Lock()
	if server.closed || server.serving || server.listener == nil {
		server.mu.Unlock()
		return errors.New("stream control server closed/already serving")
	}
	server.serving = true
	server.mu.Unlock()
	defer server.Close()
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			server.mu.Lock()
			closed := server.closed
			server.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}
		server.mu.Lock()
		if server.closed {
			server.mu.Unlock()
			connection.Close()
			return nil
		}
		select {
		case server.slots <- struct{}{}:
		default:
			server.mu.Unlock()
			connection.Close()
			continue
		}
		server.connections[connection] = struct{}{}
		server.workers.Add(1)
		server.mu.Unlock()
		go func() {
			defer server.workers.Done()
			defer func() {
				connection.Close()
				server.mu.Lock()
				delete(server.connections, connection)
				server.mu.Unlock()
				<-server.slots
			}()
			server.handle(connection)
		}()
	}
}
func (server *Server) handle(connection net.Conn) {
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	var frame [17]byte
	if _, err := io.ReadFull(connection, frame[:]); err != nil {
		return
	}
	var token Token
	copy(token[:], frame[1:])
	switch frame[0] {
	case register:
		server.mu.Lock()
		_, seen := server.seen[token]
		if seen || len(server.seen) >= server.limit || len(server.active) >= 8 {
			server.mu.Unlock()
			_ = write(connection, []byte{denied})
			return
		}
		current := &entry{connection: connection, done: make(chan struct{})}
		server.seen[token] = struct{}{}
		server.active[token] = current
		server.mu.Unlock()
		defer func() { server.mu.Lock(); delete(server.active, token); close(current.done); server.mu.Unlock() }()
		if err := write(connection, []byte{accepted}); err != nil {
			return
		}
		_ = connection.SetReadDeadline(time.Time{})
		var unused [1]byte
		_, _ = connection.Read(unused[:]) // EOF only; no client commands on this lease.
	case revoke:
		server.mu.Lock()
		current := server.active[token]
		if current == nil {
			if _, seen := server.seen[token]; !seen {
				if len(server.seen) >= server.limit {
					server.mu.Unlock()
					_ = write(connection, []byte{full})
					return
				}
				server.seen[token] = struct{}{}
			}
			server.mu.Unlock()
			_ = write(connection, []byte{accepted})
			return
		}
		server.mu.Unlock()
		current.revokeOnce.Do(func() {
			_ = current.connection.SetWriteDeadline(time.Now().Add(time.Second))
			_ = write(current.connection, []byte{denied})
		})
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-current.done:
			_ = write(connection, []byte{accepted})
		case <-server.ctx.Done():
		case <-timer.C:
		}
	case health:
		if token != (Token{}) {
			return
		}
		server.mu.Lock()
		exhausted := len(server.seen) >= server.limit
		server.mu.Unlock()
		ack := accepted
		if exhausted {
			ack = full
		}
		_ = write(connection, []byte{ack})
	}
}
func (server *Server) Close() error {
	server.mu.Lock()
	if server.closed {
		server.mu.Unlock()
		<-server.closeDone
		return nil
	}
	server.closed = true
	server.cancel()
	connections := make([]net.Conn, 0, len(server.connections))
	for connection := range server.connections {
		connections = append(connections, connection)
	}
	server.mu.Unlock()
	var err error
	if server.listener != nil {
		err = server.listener.Close()
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
	}
	for _, connection := range connections {
		connection.Close()
	}
	server.workers.Wait()
	close(server.closeDone)
	return err
}

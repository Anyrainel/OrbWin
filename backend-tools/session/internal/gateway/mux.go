package gateway

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
)

// Mux accepts only a loopback TCP listener. It verifies/bounds gateway setup,
// consumes the mode prefix and hands an opaque connection to one HTTP listener.
// It does not terminate application HTTPS, process HTTP or control any engine.
// The owner must monitor Serve's result and close its route controller separately.
type Mux struct {
	mu              sync.Mutex
	listener        net.Listener
	identity        *Identity
	plain, secure   *modeListener
	connections     map[*muxConnection]struct{}
	slots           chan struct{}
	done            chan struct{}
	closeDone       chan struct{}
	ctx             context.Context
	cancel          context.CancelFunc
	workers         sync.WaitGroup
	closed, serving bool
}
type modeListener struct {
	owner *Mux
	queue chan net.Conn
}
type muxConnection struct {
	net.Conn
	transport net.Conn
	owner     *Mux
	once      sync.Once
	err       error
}

func NewMux(listener net.Listener, identity *Identity) (*Mux, error) {
	if listener == nil || identity == nil {
		return nil, errors.New("loopback gateway listener and private process identity required")
	}
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !address.IP.IsLoopback() || address.Port == 0 {
		return nil, errors.New("gateway must bind an explicit loopback TCP endpoint")
	}
	ctx, cancel := context.WithCancel(context.Background())
	mux := &Mux{listener: listener, identity: identity, connections: make(map[*muxConnection]struct{}), slots: make(chan struct{}, 128), done: make(chan struct{}), closeDone: make(chan struct{}), ctx: ctx, cancel: cancel}
	mux.plain = &modeListener{owner: mux, queue: make(chan net.Conn, 32)}
	mux.secure = &modeListener{owner: mux, queue: make(chan net.Conn, 32)}
	return mux, nil
}
func (mux *Mux) HTTPListener() net.Listener   { return mux.plain }
func (mux *Mux) HTTPSListener() net.Listener  { return mux.secure }
func (listener *modeListener) Addr() net.Addr { return listener.owner.listener.Addr() }
func (listener *modeListener) Close() error   { return listener.owner.Close() }
func (listener *modeListener) Accept() (net.Conn, error) {
	select {
	case <-listener.owner.done:
		return nil, net.ErrClosed
	case connection := <-listener.queue:
		select {
		case <-listener.owner.done:
			connection.Close()
			return nil, net.ErrClosed
		default:
			return connection, nil
		}
	}
}
func (connection *muxConnection) Close() error {
	connection.once.Do(func() {
		connection.err = connection.Conn.Close()
		connection.transport.Close()
		connection.owner.mu.Lock()
		delete(connection.owner.connections, connection)
		connection.owner.mu.Unlock()
		<-connection.owner.slots
	})
	return connection.err
}
func (connection *muxConnection) CloseWrite() error {
	if half, ok := connection.Conn.(interface{ CloseWrite() error }); ok {
		return half.CloseWrite()
	}
	return errors.New("gateway connection lacks write-half closure")
}
func (mux *Mux) Serve() error {
	mux.mu.Lock()
	if mux.closed || mux.serving {
		mux.mu.Unlock()
		return errors.New("gateway already serving or closed")
	}
	mux.serving = true
	mux.mu.Unlock()
	defer mux.Close()
	for {
		transport, err := mux.listener.Accept()
		if err != nil {
			mux.mu.Lock()
			closed := mux.closed
			mux.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}
		mux.mu.Lock()
		if mux.closed {
			mux.mu.Unlock()
			transport.Close()
			return nil
		}
		select {
		case mux.slots <- struct{}{}:
		default:
			mux.mu.Unlock()
			transport.Close()
			continue
		}
		secure := tls.Server(transport, mux.identity.ServerConfig())
		connection := &muxConnection{Conn: secure, transport: transport, owner: mux}
		mux.connections[connection] = struct{}{}
		mux.workers.Add(1)
		mux.mu.Unlock()
		go func() {
			defer mux.workers.Done()
			mode, err := AcceptMode(mux.ctx, secure)
			if err != nil {
				transport.Close()
				connection.Close()
				return
			}
			destination := mux.plain
			if mode == HTTPS {
				destination = mux.secure
			}
			select {
			case destination.queue <- connection:
			case <-mux.done:
				transport.Close()
				connection.Close()
			}
		}()
	}
}
func (mux *Mux) Close() error {
	mux.mu.Lock()
	if mux.closed {
		mux.mu.Unlock()
		<-mux.closeDone
		return nil
	}
	mux.closed = true
	close(mux.done)
	mux.cancel()
	connections := make([]*muxConnection, 0, len(mux.connections))
	for connection := range mux.connections {
		connections = append(connections, connection)
	}
	mux.mu.Unlock()
	err := mux.listener.Close()
	if errors.Is(err, net.ErrClosed) {
		err = nil
	}
	// Abort raw transports first so TLS close-notify or an application write
	// cannot serialize five-second Close delays across all active connections.
	for _, connection := range connections {
		connection.transport.Close()
	}
	for _, connection := range connections {
		connection.Close()
	}
	mux.workers.Wait()
	close(mux.closeDone)
	return err
}

package localnet

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"orbwin.local/session/internal/gateway"
)

type GatewayDescriptor struct {
	Port       uint16    `json:"port"`
	Session    string    `json:"session"`
	PublicRoot []byte    `json:"publicRoot"`
	ExpiresAt  time.Time `json:"expiresAt"`
}
type hostLog struct {
	mu   sync.Mutex
	text string
}

func (buffer *hostLog) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if len(data) >= 16384 {
		buffer.text = string(data[len(data)-16384:])
		return len(data), nil
	}
	buffer.text += string(data)
	if len(buffer.text) > 16384 {
		buffer.text = buffer.text[len(buffer.text)-16384:]
	}
	return len(data), nil
}
func (buffer *hostLog) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return strings.ToValidUTF8(buffer.text, "�")
}

// ServiceHost is independent of renderer lifetime and never manages rdd. It
// owns only its private loopback listener, route controller and supplied owned
// namespace backend. A permanent process must drive discovery/reconciliation
// and relay renewal; HTTP is never an administrative endpoint.
type ServiceHost struct {
	controller    *ServiceController
	mux           *gateway.Mux
	plain, secure *http.Server
	descriptor    GatewayDescriptor
	diagnostics   hostLog
	stop, done    chan struct{}
	stopOnce      sync.Once
	workers       sync.WaitGroup
	mu            sync.Mutex
	result        error
}

// After argument acceptance, startup consumes backend ownership, including on
// failure. No public root trust, published port, Docker relay or UI is created.
func StartServiceHost(backend NamespaceBackend, authority *Authority, session string) (*ServiceHost, error) {
	return StartServiceHostWithPorts(backend, authority, session, DefaultServicePorts())
}
func StartServiceHostWithPorts(backend NamespaceBackend, authority *Authority, session string, ports ServicePorts) (*ServiceHost, error) {
	controller, err := NewServiceControllerWithPorts(backend, ports)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*ServiceHost, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return nil, errors.Join(err, controller.Close(ctx))
	}
	configuration, err := controller.TLSConfig(authority)
	if err != nil {
		return fail(err)
	}
	if authority.RevocationHTTPPort() != ports.HTTP {
		return fail(errors.New("integration HTTP port requires a reviewed authority rotation"))
	}
	identity, err := gateway.NewIdentity(session, time.Now())
	if err != nil {
		return fail(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fail(err)
	}
	mux, err := gateway.NewMux(listener, identity)
	if err != nil {
		listener.Close()
		return fail(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	host := &ServiceHost{controller: controller, mux: mux, descriptor: GatewayDescriptor{Port: uint16(address.Port), Session: session, PublicRoot: identity.PublicRoot(), ExpiresAt: identity.ExpiresAt()}, stop: make(chan struct{}), done: make(chan struct{})}
	logger := log.New(&host.diagnostics, "", log.LstdFlags)
	host.plain = &http.Server{Handler: authority.PublicCRLHandler(controller.Handler(false)), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 64 << 10, ErrorLog: logger}
	host.secure = &http.Server{Handler: controller.Handler(true), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 64 << 10, ErrorLog: logger}
	results := make(chan error, 3)
	host.workers.Add(3)
	go func() { defer host.workers.Done(); results <- mux.Serve() }()
	go func() { defer host.workers.Done(); results <- host.plain.Serve(mux.HTTPListener()) }()
	go func() {
		defer host.workers.Done()
		results <- host.secure.Serve(tls.NewListener(mux.HTTPSListener(), configuration))
	}()
	go host.supervise(results)
	return host, nil
}
func (host *ServiceHost) supervise(results <-chan error) {
	var failure error
	select {
	case <-host.stop:
	case err := <-results:
		if err == nil {
			err = errors.New("local service gateway stopped unexpectedly")
		}
		failure = err
	}
	// Stop ingress and private tunneled clients first. Then retire all route
	// streams before exact namespace cleanup. Raw gateway closure also unblocks
	// nested TLS/HTTP handshakes, so server Close cannot wait on stalled peers.
	failure = errors.Join(failure, host.mux.Close())
	// Route retirement can revoke streams before the composed backend spends
	// up to ten seconds each on relay and namespace cleanup. Join that owner
	// before publishing host completion, including fatal-listener shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	failure = errors.Join(failure, host.controller.Close(ctx))
	cancel()
	failure = errors.Join(failure, host.plain.Close(), host.secure.Close())
	host.workers.Wait()
	host.mu.Lock()
	host.result = failure
	host.mu.Unlock()
	close(host.done)
}
func (host *ServiceHost) Gateway() GatewayDescriptor {
	descriptor := host.descriptor
	descriptor.PublicRoot = append([]byte(nil), descriptor.PublicRoot...)
	return descriptor
}
func (host *ServiceHost) Reconcile(ctx context.Context, candidates []ServiceCandidate, retry bool) ([]ServiceStatus, error) {
	return host.controller.Reconcile(ctx, candidates, retry)
}
func (host *ServiceHost) Invalidate() error   { return host.controller.Invalidate() }
func (host *ServiceHost) Diagnostics() string { return host.diagnostics.String() }
func (host *ServiceHost) Wait(ctx context.Context) error {
	select {
	case <-host.done:
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.result
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (host *ServiceHost) Close(ctx context.Context) error {
	host.stopOnce.Do(func() { close(host.stop) })
	return host.Wait(ctx)
}

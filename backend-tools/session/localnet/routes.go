package localnet

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A route selects one immutable container incarnation and its owned namespace
// helper. The engine reconciler must verify ownership and the current StartedAt
// before publishing a snapshot. No IP, host-port or caller-supplied URL is used.
type ServiceRoute struct {
	NamespaceLeaseSHA                      string
	Name, ContainerID, StartedAt, HelperID string
	HTTPS                                  bool
	Port                                   uint16
}
type NamespaceDialer func(context.Context, ServiceRoute) (net.Conn, error)

// Replace publishes a complete validated snapshot atomically. Unchanged routes
// retain connections; changed/removed routes cancel requests and close every
// backend connection, including websocket upgrades. A late dial cannot escape
// retirement. The registry never starts an engine or installs DNS/trust.
type Routes struct {
	mu      sync.RWMutex
	entries map[string]*serviceRoute
	dial    NamespaceDialer
	slots   chan struct{}
	helpers map[string]*helperCapacity
	closed  bool
	ports   ServicePorts
}
type helperCapacity struct {
	slots      chan struct{}
	references int
}
type serviceRoute struct {
	spec            ServiceRoute
	ctx             context.Context
	cancel          context.CancelFunc
	transport       *http.Transport
	proxy           *httputil.ReverseProxy
	mu              sync.Mutex
	closed          bool
	connections     map[*routeConnection]struct{}
	certificateMu   sync.Mutex
	certificate     *tls.Certificate
	certificateRoot string
}
type routeConnection struct {
	net.Conn
	owner   *serviceRoute
	once    sync.Once
	err     error
	release func()
}

func (connection *routeConnection) Close() error {
	connection.once.Do(func() {
		connection.err = connection.Conn.Close()
		connection.release()
		connection.owner.mu.Lock()
		delete(connection.owner.connections, connection)
		connection.owner.mu.Unlock()
	})
	return connection.err
}
func (connection *routeConnection) CloseWrite() error {
	if half, ok := connection.Conn.(interface{ CloseWrite() error }); ok {
		return half.CloseWrite()
	}
	return errors.New("namespace connection does not support write-half closure")
}
func NewRoutes(dial NamespaceDialer) (*Routes, error) {
	return NewRoutesWithPorts(dial, DefaultServicePorts())
}
func NewRoutesWithPorts(dial NamespaceDialer, ports ServicePorts) (*Routes, error) {
	if err := ports.validate(); err != nil {
		return nil, err
	}
	if dial == nil {
		return nil, errors.New("verified namespace dialer required")
	}
	routes := &Routes{entries: make(map[string]*serviceRoute), slots: make(chan struct{}, 32), helpers: make(map[string]*helperCapacity), ports: ports}
	routes.dial = func(ctx context.Context, spec ServiceRoute) (net.Conn, error) {
		routes.mu.Lock()
		capacity := routes.helpers[spec.HelperID]
		if capacity == nil {
			capacity = &helperCapacity{slots: make(chan struct{}, 4)}
			routes.helpers[spec.HelperID] = capacity
		}
		capacity.references++
		routes.mu.Unlock()
		releaseReference := func() {
			routes.mu.Lock()
			capacity.references--
			if capacity.references == 0 {
				delete(routes.helpers, spec.HelperID)
			}
			routes.mu.Unlock()
		}
		select {
		case capacity.slots <- struct{}{}:
		case <-ctx.Done():
			releaseReference()
			return nil, ctx.Err()
		}
		connection, err := dial(ctx, spec)
		if err != nil {
			<-capacity.slots
			releaseReference()
			return nil, err
		}
		return &capacityConnection{Conn: connection, release: func() { <-capacity.slots; releaseReference() }}, nil
	}
	return routes, nil
}

type capacityConnection struct {
	net.Conn
	once    sync.Once
	err     error
	release func()
}

func (connection *capacityConnection) Close() error {
	connection.once.Do(func() { connection.err = connection.Conn.Close(); connection.release() })
	return connection.err
}
func (connection *capacityConnection) CloseWrite() error {
	if half, ok := connection.Conn.(interface{ CloseWrite() error }); ok {
		return half.CloseWrite()
	}
	return errors.New("namespace connection does not support write-half closure")
}
func fullID(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && value == strings.ToLower(value)
}
func (routes *Routes) Replace(snapshot []ServiceRoute) error {
	if len(snapshot) > 256 {
		return errors.New("local service routes exceed 256 names")
	}
	validated := make(map[string]ServiceRoute, len(snapshot))
	for _, spec := range snapshot {
		canonical, err := CanonicalServiceName(spec.Name)
		started, timeErr := time.Parse(time.RFC3339Nano, spec.StartedAt)
		if err != nil || canonical != spec.Name || !fullID(spec.ContainerID) || !fullID(spec.HelperID) || timeErr != nil || started.IsZero() || spec.Port == 0 {
			return errors.New("canonical service name, exact target/helper incarnation and TCP port required")
		}
		if _, exists := validated[spec.Name]; exists {
			return errors.New("duplicate local service name; review the conflicting routes")
		}
		validated[spec.Name] = spec
	}
	routes.mu.Lock()
	if routes.closed {
		routes.mu.Unlock()
		return errors.New("local service routes are closed")
	}
	next := make(map[string]*serviceRoute, len(snapshot))
	for name, spec := range validated {
		if previous := routes.entries[name]; previous != nil && previous.spec == spec {
			next[name] = previous
		} else {
			next[name] = newServiceRoute(spec, routes.dial, routes.slots)
		}
	}
	var retired []*serviceRoute
	for name, previous := range routes.entries {
		if next[name] != previous {
			retired = append(retired, previous)
		}
	}
	routes.entries = next
	routes.mu.Unlock()
	closeServiceRoutes(retired)
	return nil
}
func (routes *Routes) Close() error {
	routes.mu.Lock()
	routes.closed = true
	previous := routes.entries
	routes.entries = make(map[string]*serviceRoute)
	routes.mu.Unlock()
	retired := make([]*serviceRoute, 0, len(previous))
	for _, entry := range previous {
		retired = append(retired, entry)
	}
	closeServiceRoutes(retired)
	return nil
}
func newServiceRoute(spec ServiceRoute, dial NamespaceDialer, slots chan struct{}) *serviceRoute {
	ctx, cancel := context.WithCancel(context.Background())
	entry := &serviceRoute{spec: spec, ctx: ctx, cancel: cancel, connections: make(map[*routeConnection]struct{})}
	entry.transport = &http.Transport{
		MaxIdleConns: 4, MaxIdleConnsPerHost: 4, MaxConnsPerHost: 4,
		IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: time.Second, MaxResponseHeaderBytes: 1 << 20,
		TLSHandshakeTimeout: 5 * time.Second,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, ServerName: spec.Name},
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			stop := context.AfterFunc(entry.ctx, cancel)
			defer stop()
			if entry.ctx.Err() != nil {
				return nil, errors.New("local service route is retired")
			}
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			connection, err := dial(ctx, spec)
			if err != nil {
				<-slots
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				connection.Close()
				<-slots
				return nil, err
			}
			tracked := &routeConnection{Conn: connection, owner: entry, release: func() { <-slots }}
			entry.mu.Lock()
			if entry.closed {
				entry.mu.Unlock()
				connection.Close()
				<-slots
				return nil, errors.New("service incarnation retired during dial")
			}
			entry.connections[tracked] = struct{}{}
			entry.mu.Unlock()
			return tracked, nil
		},
	}
	entry.proxy = &httputil.ReverseProxy{
		Transport: entry.transport,
		ErrorLog:  log.New(io.Discard, "", 0),
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(&url.URL{Scheme: routeScheme(spec), Host: "127.0.0.1:" + strconv.Itoa(int(spec.Port))})
			request.Out.Host = request.In.Host
			for key := range request.Out.Header {
				if strings.EqualFold(key, "Forwarded") || strings.EqualFold(key, "X-Real-IP") || strings.HasPrefix(strings.ToLower(key), "x-forwarded-") {
					request.Out.Header.Del(key)
				}
			}
			request.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "Local service is unavailable. Check its container and selected port.", http.StatusBadGateway)
		},
	}
	return entry
}

// Certificates are issued lazily only for currently selected names. No wildcard
// or default certificate authenticates unknown hosts. A cached leaf is renewed
// with an hour remaining; authority expiry remains a visible handshake failure.
func (routes *Routes) TLSConfig(authority *Authority) (*tls.Config, error) {
	if err := authority.validate(time.Now()); err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		name, err := CanonicalServiceName(hello.ServerName)
		if err != nil {
			return nil, err
		}
		routes.mu.RLock()
		entry := routes.entries[name]
		routes.mu.RUnlock()
		if entry == nil || entry.ctx.Err() != nil {
			return nil, errors.New("local TLS service route is not active")
		}
		entry.certificateMu.Lock()
		defer entry.certificateMu.Unlock()
		now := time.Now()
		if entry.certificate != nil && entry.certificateRoot == authority.Fingerprint() && entry.certificate.Leaf.NotAfter.After(now.Add(time.Hour)) {
			return entry.certificate, nil
		}
		leaf, err := authority.Leaf(name, now)
		if err != nil {
			return nil, err
		}
		if leaf.Leaf == nil {
			leaf.Leaf, err = x509.ParseCertificate(leaf.Certificate[0])
			if err != nil {
				return nil, err
			}
		}
		entry.certificate = &leaf
		entry.certificateRoot = authority.Fingerprint()
		return entry.certificate, nil
	}}, nil
}
func (entry *serviceRoute) close() {
	entry.mu.Lock()
	entry.closed = true
	connections := make([]*routeConnection, 0, len(entry.connections))
	for connection := range entry.connections {
		connections = append(connections, connection)
	}
	entry.mu.Unlock()
	entry.cancel()
	var closing sync.WaitGroup
	for _, connection := range connections {
		closing.Add(1)
		go func() { defer closing.Done(); connection.Close() }()
	}
	closing.Wait()
	entry.transport.CloseIdleConnections()
}
func closeServiceRoutes(entries []*serviceRoute) {
	var closing sync.WaitGroup
	for _, entry := range entries {
		closing.Add(1)
		go func() { defer closing.Done(); entry.close() }()
	}
	closing.Wait()
}

// Secure handles an already terminated TLS connection; SNI must match Host.
// Plain HTTP redirects only selected routes. Unknown names never reach a target.
func (routes *Routes) Handler(secure bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodConnect || request.URL.IsAbs() || request.URL.Host != "" {
			http.Error(w, "Origin requests only.", http.StatusBadRequest)
			return
		}
		name, external, err := routes.ports.requestName(request.Host, secure)
		if err != nil {
			http.Error(w, "Unknown local service name.", http.StatusNotFound)
			return
		}
		routes.mu.RLock()
		entry := routes.entries[name]
		routes.mu.RUnlock()
		if entry == nil {
			http.Error(w, "Local service route is not active.", http.StatusNotFound)
			return
		}
		if !secure {
			http.Redirect(w, request, routes.ports.redirect(name, external, request.URL), http.StatusPermanentRedirect)
			return
		}
		if request.TLS == nil {
			http.Error(w, "TLS connection required.", http.StatusBadRequest)
			return
		}
		sni, err := CanonicalServiceName(request.TLS.ServerName)
		if err != nil || sni != name {
			http.Error(w, "TLS name differs from the selected service.", http.StatusMisdirectedRequest)
			return
		}
		ctx, cancel := context.WithCancel(request.Context())
		stop := context.AfterFunc(entry.ctx, cancel)
		defer func() { stop(); cancel() }()
		if entry.ctx.Err() != nil {
			http.Error(w, "Local service route changed. Retry the current link.", http.StatusServiceUnavailable)
			return
		}
		entry.proxy.ServeHTTP(w, request.WithContext(ctx))
	})
}

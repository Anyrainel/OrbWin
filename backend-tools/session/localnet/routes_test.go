package localnet

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func routeFixture() ServiceRoute {
	return ServiceRoute{Name: "app.orbwin.localhost", ContainerID: strings.Repeat("a", 64), StartedAt: "2026-10-24T10:00:00Z", HelperID: strings.Repeat("b", 64), Port: 8080}
}

func TestVerifiedTLSUpgradeIsClosedWhenTargetIncarnationChanges(t *testing.T) {
	backendDone := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, stream, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer close(backendDone)
		defer connection.Close()
		_, _ = stream.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: orbwin-test\r\n\r\n")
		_ = stream.Flush()
		data := make([]byte, 4)
		if _, err = io.ReadFull(stream, data); err != nil {
			t.Error(err)
			return
		}
		_, _ = connection.Write(data)
		_, _ = io.Copy(io.Discard, stream)
	}))
	defer backend.Close()
	routes, _ := NewRoutes(func(ctx context.Context, _ ServiceRoute) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", backend.Listener.Addr().String())
	})
	defer routes.Close()
	spec := routeFixture()
	_ = routes.Replace([]ServiceRoute{spec})
	authority, err := NewAuthority(strings.Repeat("c", 32), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := routes.TLSConfig(authority)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(routes.Handler(true))
	server.TLS = configuration
	server.StartTLS()
	defer server.Close()
	pool := x509.NewCertPool()
	root, err := x509.ParseCertificate(authority.RootCertificate())
	if err != nil {
		t.Fatal(err)
	}
	pool.AddCert(root)
	connection, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", server.Listener.Addr().String(), &tls.Config{RootCAs: pool, ServerName: spec.Name, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
	_, err = fmt.Fprintf(connection, "GET /upgrade HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: orbwin-test\r\n\r\n", spec.Name)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if err != nil || response.StatusCode != 101 {
		t.Fatalf("verified TLS upgrade failed: %v %v", response, err)
	}
	payload := []byte{0, 255, 13, 10}
	if _, err := connection.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 4)
	if _, err := io.ReadFull(reader, got); err != nil || string(got) != string(payload) {
		t.Fatalf("upgrade corrupted binary payload: %v %v", got, err)
	}
	spec.StartedAt = "2026-10-24T11:00:00Z"
	if err := routes.Replace([]ServiceRoute{spec}); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("upgraded client survived target replacement")
	}
	select {
	case <-backendDone:
	case <-time.After(time.Second):
		t.Fatal("upgrade backend remained connected after replacement")
	}
}

func TestTLSCertificateSelectionRejectsUnknownNamesAndChangesAuthority(t *testing.T) {
	routes, _ := NewRoutes(func(context.Context, ServiceRoute) (net.Conn, error) {
		t.Error("certificate issuance dialed a workload")
		return nil, io.EOF
	})
	defer routes.Close()
	spec := routeFixture()
	_ = routes.Replace([]ServiceRoute{spec})
	first, _ := NewAuthority(strings.Repeat("c", 32), time.Now())
	second, _ := NewAuthority(strings.Repeat("d", 32), time.Now())
	firstConfig, _ := routes.TLSConfig(first)
	secondConfig, _ := routes.TLSConfig(second)
	hello := &tls.ClientHelloInfo{ServerName: spec.Name}
	one, err := firstConfig.GetCertificate(hello)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := firstConfig.GetCertificate(hello)
	if again != one {
		t.Fatal("unchanged name/authority regenerated a valid cached certificate")
	}
	two, err := secondConfig.GetCertificate(hello)
	if err != nil || string(one.Certificate[1]) == string(two.Certificate[1]) {
		t.Fatal("authority rotation reused the previous issuer")
	}
	for _, name := range []string{"", "evil.example", "other.orbwin.localhost", "*.orbwin.localhost"} {
		if _, err := firstConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: name}); err == nil {
			t.Errorf("issued certificate for unknown name %q", name)
		}
	}
	_ = routes.Replace(nil)
	if _, err := secondConfig.GetCertificate(hello); err == nil {
		t.Fatal("removed route still issued certificates")
	}
}
func routeRequest(method, target, host, sni string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.Host = host
	if sni != "" {
		request.TLS = &tls.ConnectionState{ServerName: sni}
	}
	return request
}
func TestRoutesForwardOnlySelectedOriginAndReplaceUntrustedForwarding(t *testing.T) {
	observed := make(chan *http.Request, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed <- r.Clone(context.Background())
		w.Header().Set("X-Workload", "selected")
		_, _ = w.Write([]byte("binary\x00response"))
	}))
	defer backend.Close()
	spec := routeFixture()
	var calls atomic.Int32
	routes, _ := NewRoutes(func(ctx context.Context, selected ServiceRoute) (net.Conn, error) {
		calls.Add(1)
		if selected != spec {
			t.Error("dial selected another target incarnation")
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", backend.Listener.Addr().String())
	})
	defer routes.Close()
	if err := routes.Replace([]ServiceRoute{spec}); err != nil {
		t.Fatal(err)
	}
	request := routeRequest("GET", "/a%2Fb?q=1", spec.Name, spec.Name)
	request.Header.Set("Forwarded", "host=evil.example;proto=http")
	request.Header.Set("X-Forwarded-For", "attacker")
	request.Header.Set("X-Forwarded-Host", "evil.example")
	request.Header.Set("X-Forwarded-Proto", "http")
	response := httptest.NewRecorder()
	routes.Handler(true).ServeHTTP(response, request)
	if response.Code != 200 || response.Body.String() != "binary\x00response" || response.Header().Get("X-Workload") != "selected" {
		t.Fatalf("forwarding corrupted response: %d %q", response.Code, response.Body.String())
	}
	got := <-observed
	if got.Host != spec.Name || got.URL.RequestURI() != "/a%2Fb?q=1" || got.Header.Get("Forwarded") != "" || strings.Contains(got.Header.Get("X-Forwarded-For"), "attacker") || got.Header.Get("X-Forwarded-Host") != spec.Name || got.Header.Get("X-Forwarded-Proto") != "https" {
		t.Fatalf("untrusted forwarding/path survived: %#v %s", got.Header, got.URL)
	}
	for _, fixture := range []struct {
		method, path, host, sni string
		code                    int
	}{
		{"GET", "/", "other.orbwin.localhost", "other.orbwin.localhost", 404},
		{"GET", "/", "evil.example", "evil.example", 404},
		{"GET", "/", spec.Name + ":1234", spec.Name, 404},
		{"GET", "/", spec.Name, "other.orbwin.localhost", 421},
		{"GET", "/", spec.Name, "", 400},
		{"CONNECT", "/", spec.Name, spec.Name, 400},
		{"GET", "http://evil.example/", spec.Name, spec.Name, 400},
	} {
		response := httptest.NewRecorder()
		routes.Handler(true).ServeHTTP(response, routeRequest(fixture.method, fixture.path, fixture.host, fixture.sni))
		if response.Code != fixture.code {
			t.Errorf("unexpected response for %+v: %d", fixture, response.Code)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("refused request reached a namespace helper")
	}
	response = httptest.NewRecorder()
	routes.Handler(false).ServeHTTP(response, routeRequest("POST", "/a%2Fb?q=1", "APP.ORBWIN.LOCALHOST:80", ""))
	if response.Code != 308 || response.Header().Get("Location") != "https://app.orbwin.localhost/a%2Fb?q=1" {
		t.Fatal("redirect changed selected name or escaped request path")
	}
}

func TestRouteReplacementClosesConnectionsAndUnchangedSnapshotPreservesThem(t *testing.T) {
	peer := make(chan net.Conn, 2)
	routes, _ := NewRoutes(func(context.Context, ServiceRoute) (net.Conn, error) {
		left, right := net.Pipe()
		peer <- right
		return left, nil
	})
	defer routes.Close()
	spec := routeFixture()
	if err := routes.Replace([]ServiceRoute{spec}); err != nil {
		t.Fatal(err)
	}
	entry := routes.entries[spec.Name]
	connection, err := entry.transport.DialContext(context.Background(), "tcp", "ignored:9999")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	other := <-peer
	defer other.Close()
	if err := routes.Replace([]ServiceRoute{spec}); err != nil {
		t.Fatal(err)
	}
	if routes.entries[spec.Name] != entry || entry.ctx.Err() != nil {
		t.Fatal("unchanged discovery snapshot retired a live route")
	}
	invalid := spec
	invalid.HelperID = "short"
	if routes.Replace([]ServiceRoute{invalid}) == nil || routes.entries[spec.Name] != entry {
		t.Fatal("invalid replacement modified the active snapshot")
	}
	if routes.Replace([]ServiceRoute{spec, spec}) == nil {
		t.Fatal("duplicate service names were silently selected")
	}
	spec.StartedAt = "2026-10-24T10:01:00Z"
	if err := routes.Replace([]ServiceRoute{spec}); err != nil {
		t.Fatal(err)
	}
	_ = other.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := other.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("old incarnation connection remained reachable: %v", err)
	}
	if entry.ctx.Err() == nil || len(routes.slots) != 0 {
		t.Fatal("retirement did not cancel requests/release connection capacity")
	}
	routes.Close()
	if routes.Replace([]ServiceRoute{spec}) == nil {
		t.Fatal("closed integration was resurrected by discovery")
	}
}

func TestRouteRetirementRejectsLateDialAndCancelsInFlightRequest(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	left, right := net.Pipe()
	defer right.Close()
	routes, _ := NewRoutes(func(context.Context, ServiceRoute) (net.Conn, error) { close(started); <-release; return left, nil })
	defer routes.Close()
	spec := routeFixture()
	_ = routes.Replace([]ServiceRoute{spec})
	entry := routes.entries[spec.Name]
	finished := make(chan error, 1)
	go func() {
		connection, err := entry.transport.DialContext(context.Background(), "tcp", "ignored:0")
		if connection != nil {
			connection.Close()
		}
		finished <- err
	}()
	<-started
	_ = routes.Replace(nil)
	close(release)
	if err := <-finished; err == nil {
		t.Fatal("late connection escaped route retirement")
	}
	if len(routes.slots) != 0 {
		t.Fatal("late dial leaked capacity")
	}
	_ = right.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := right.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("late namespace connection remained open: %v", err)
	}

	requestStarted := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(requestStarted); <-r.Context().Done() }))
	defer backend.Close()
	active, _ := NewRoutes(func(ctx context.Context, _ ServiceRoute) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", backend.Listener.Addr().String())
	})
	defer active.Close()
	_ = active.Replace([]ServiceRoute{spec})
	done := make(chan struct{})
	go func() {
		defer close(done)
		active.Handler(true).ServeHTTP(httptest.NewRecorder(), routeRequest("GET", "/", spec.Name, spec.Name))
	}()
	<-requestStarted
	_ = active.Replace(nil)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("retired HTTP request remained blocked")
	}
}

func TestRouteConnectionBudgetHonorsCancellationAndReleasesOnClose(t *testing.T) {
	var peers []net.Conn
	routes, _ := NewRoutes(func(context.Context, ServiceRoute) (net.Conn, error) {
		left, right := net.Pipe()
		peers = append(peers, right)
		return left, nil
	})
	defer func() {
		routes.Close()
		for _, peer := range peers {
			peer.Close()
		}
	}()
	var snapshot []ServiceRoute
	for index := range 32 {
		spec := routeFixture()
		spec.Name = fmt.Sprintf("app%d.orbwin.localhost", index)
		spec.HelperID = fmt.Sprintf("%064x", index+1)
		snapshot = append(snapshot, spec)
	}
	_ = routes.Replace(snapshot)
	for _, spec := range snapshot {
		entry := routes.entries[spec.Name]
		if _, err := entry.transport.DialContext(context.Background(), "tcp", "ignored:0"); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := routes.entries[snapshot[0].Name].transport.DialContext(ctx, "tcp", "ignored:0"); err == nil || len(peers) != 32 {
		t.Fatal("global connection budget admitted an excess namespace exec")
	}
	routes.Close()
	if len(routes.slots) != 0 || len(routes.helpers) != 0 {
		t.Fatal("shutdown retained connection capacity")
	}
}

func TestAliasesShareNamespaceCapacity(t *testing.T) {
	var peers []net.Conn
	routes, _ := NewRoutes(func(context.Context, ServiceRoute) (net.Conn, error) {
		left, right := net.Pipe()
		peers = append(peers, right)
		return left, nil
	})
	defer func() {
		routes.Close()
		for _, peer := range peers {
			peer.Close()
		}
	}()
	first := routeFixture()
	alias := first
	alias.Name = "app.orbwin.test"
	_ = routes.Replace([]ServiceRoute{first, alias})
	for range 4 {
		if _, err := routes.entries[first.Name].transport.DialContext(context.Background(), "tcp", "ignored:0"); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := routes.entries[alias.Name].transport.DialContext(ctx, "tcp", "ignored:0"); err == nil || len(peers) != 4 {
		t.Fatal("service aliases bypassed the namespace capacity bound")
	}
	routes.Close()
	if len(routes.helpers) != 0 || len(routes.slots) != 0 {
		t.Fatal("cancelled capacity wait leaked state")
	}
}

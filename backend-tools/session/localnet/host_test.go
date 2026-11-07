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

	"orbwin.local/session/internal/gateway"
)

func hostFixture(t *testing.T) (*ServiceHost, *Authority, *controllerBackend, *atomic.Int32) {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == "HEAD" {
			w.WriteHeader(204)
			return
		}
		if r.Header.Get("X-Forwarded-Proto") != "https" {
			t.Error("private transport TLS confused application scheme")
		}
		_, _ = w.Write([]byte("selected workload\x00response"))
	}))
	t.Cleanup(server.Close)
	backend := &controllerBackend{started: "2026-10-24T12:00:00Z", address: server.Listener.Addr().String()}
	authority, err := NewAuthority(strings.Repeat("c", 32), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	host, err := StartServiceHost(backend, authority, strings.Repeat("d", 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = host.Close(ctx)
	})
	return host, authority, backend, requests
}
func hostStream(t *testing.T, host *ServiceHost, authority *Authority, name string, secure bool) net.Conn {
	t.Helper()
	descriptor := host.Gateway()
	mode := gateway.HTTP
	if secure {
		mode = gateway.HTTPS
	}
	outer, err := gateway.Dial(context.Background(), fmt.Sprintf("127.0.0.1:%d", descriptor.Port), descriptor.Session, descriptor.PublicRoot, mode)
	if err != nil {
		t.Fatal(err)
	}
	if !secure {
		return outer
	}
	root, err := x509.ParseCertificate(authority.RootCertificate())
	if err != nil {
		outer.Close()
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	inner := tls.Client(outer, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: name, RootCAs: pool, NextProtos: []string{"http/1.1"}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := inner.HandshakeContext(ctx); err != nil {
		outer.Close()
		t.Fatal(err)
	}
	return inner
}
func hostGet(t *testing.T, connection net.Conn, name, path string) *http.Response {
	t.Helper()
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	request, _ := http.NewRequest("GET", "https://"+name+path, nil)
	if err := request.Write(connection); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(connection), request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
func TestServiceHostPrivateGatewayToVerifiedApplicationHTTPS(t *testing.T) {
	host, authority, backend, requests := hostFixture(t)
	candidate := controllerCandidate()
	status, err := host.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || status[0].State != "active" {
		t.Fatalf("host did not publish verified route: %+v %v", status, err)
	}
	plain := hostStream(t, host, authority, candidate.Names[0], false)
	redirect := hostGet(t, plain, candidate.Names[0], "/a%2Fb?q=1")
	if redirect.StatusCode != 308 || redirect.Header.Get("Location") != "https://app.orbwin.localhost/a%2Fb?q=1" {
		t.Fatal("HTTP gateway did not preserve the selected HTTPS redirect")
	}
	redirect.Body.Close()
	plain.Close()
	secure := hostStream(t, host, authority, candidate.Names[0], true)
	response := hostGet(t, secure, candidate.Names[0], "/hello")
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	secure.Close()
	if err != nil || response.StatusCode != 200 || string(data) != "selected workload\x00response" || requests.Load() != 2 {
		t.Fatalf("nested HTTPS/proxy journey changed response: %q %v", data, err)
	}
	descriptor := host.Gateway()
	descriptor.PublicRoot[0] ^= 1
	if descriptor.PublicRoot[0] == host.Gateway().PublicRoot[0] {
		t.Fatal("gateway descriptor exposes mutable identity")
	}
	if err := host.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	backend.mu.Lock()
	closed := backend.closed
	backend.mu.Unlock()
	if !closed {
		t.Fatal("host shutdown retained namespace ownership")
	}
	if connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", descriptor.Port), 100*time.Millisecond); err == nil {
		connection.Close()
		t.Fatal("stopped host still accepts gateway traffic")
	}
}
func TestServiceHostUnexpectedGatewayLossClosesOwnedBackend(t *testing.T) {
	host, _, backend, _ := hostFixture(t)
	if err := host.mux.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := host.Wait(ctx); err == nil {
		t.Fatal("unexpected gateway loss reported graceful service health")
	}
	backend.mu.Lock()
	closed := backend.closed
	backend.mu.Unlock()
	if !closed {
		t.Fatal("gateway failure did not close its owned backend")
	}
}
func TestServiceHostCloseCancelsReconcileWithoutWorkloadLifecycleActions(t *testing.T) {
	host, _, backend, _ := hostFixture(t)
	entered := make(chan struct{})
	backend.mu.Lock()
	backend.targetEntered = entered
	backend.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := host.Reconcile(context.Background(), []ServiceCandidate{controllerCandidate()}, false)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reconcile did not begin target inspection")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := host.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stopped reconcile reported success")
		}
	case <-ctx.Done():
		t.Fatal("host retained reconciliation worker")
	}
	backend.mu.Lock()
	acquires, closed := backend.acquires, backend.closed
	backend.mu.Unlock()
	if acquires != 0 || !closed {
		t.Fatal("shutdown created helpers or retained backend ownership")
	}
}
func TestServiceHostStartupFailureClosesConsumedBackend(t *testing.T) {
	backend := &controllerBackend{}
	if host, err := StartServiceHost(backend, nil, strings.Repeat("d", 32)); err == nil {
		host.Close(context.Background())
		t.Fatal("missing application authority started a service")
	}
	backend.mu.Lock()
	closed := backend.closed
	backend.mu.Unlock()
	if !closed {
		t.Fatal("failed startup stranded backend ownership")
	}
}

package gateway

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func muxFixture(t *testing.T) (*Mux, *Identity, string) {
	t.Helper()
	identity := testIdentity(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux, err := NewMux(listener, identity)
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- mux.Serve() }()
	t.Cleanup(func() {
		if err := mux.Close(); err != nil {
			t.Error(err)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("gateway accept loop did not exit")
		}
	})
	return mux, identity, listener.Addr().String()
}
func TestGatewayMuxSeparatesHTTPAndNestedVerifiedHTTPS(t *testing.T) {
	mux, identity, address := muxFixture(t)
	plain := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			t.Error("private transport TLS became application HTTPS")
		}
		_, _ = w.Write([]byte("plain\x00response"))
	})}
	application, err := NewIdentity(strings.Repeat("b", 32), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	innerConfig := application.ServerConfig()
	innerConfig.NextProtos = []string{"http/1.1"}
	secure := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.TLS.ServerName != hostname(application.session) {
			t.Error("nested application TLS was lost")
		}
		_, _ = w.Write([]byte("secure\x00response"))
	})}
	var serving sync.WaitGroup
	serving.Add(2)
	go func() { defer serving.Done(); _ = plain.Serve(mux.HTTPListener()) }()
	go func() { defer serving.Done(); _ = secure.Serve(tls.NewListener(mux.HTTPSListener(), innerConfig)) }()
	t.Cleanup(func() { mux.Close(); plain.Close(); secure.Close(); serving.Wait() })
	for _, mode := range []Mode{HTTP, HTTPS} {
		connection, err := Dial(context.Background(), address, identity.session, identity.PublicRoot(), mode)
		if err != nil {
			t.Fatal(err)
		}
		var stream net.Conn = connection
		if mode == HTTPS {
			configuration, _ := ClientConfig(application.session, application.PublicRoot())
			configuration.NextProtos = []string{"http/1.1"}
			nested := tls.Client(connection, configuration)
			if err := nested.HandshakeContext(context.Background()); err != nil {
				connection.Close()
				t.Fatal(err)
			}
			stream = nested
		}
		_ = stream.SetDeadline(time.Now().Add(time.Second))
		request, _ := http.NewRequest("GET", "http://app.orbwin.localhost/", nil)
		if err := request.Write(stream); err != nil {
			stream.Close()
			t.Fatal(err)
		}
		response, err := http.ReadResponse(bufio.NewReader(stream), request)
		if err != nil {
			stream.Close()
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		stream.Close()
		expected := "plain\x00response"
		if mode == HTTPS {
			expected = "secure\x00response"
		}
		if err != nil || string(data) != expected {
			t.Fatalf("gateway routed the wrong protocol: %q %v", data, err)
		}
	}
}
func TestGatewayMuxCloseJoinsUnhandshakenAndQueuedConnections(t *testing.T) {
	mux, identity, address := muxFixture(t)
	queued, err := Dial(context.Background(), address, identity.session, identity.PublicRoot(), HTTP)
	if err != nil {
		t.Fatal(err)
	}
	defer queued.Close()
	unhandshaken, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer unhandshaken.Close()
	deadline := time.Now().Add(time.Second)
	for {
		mux.mu.Lock()
		count := len(mux.connections)
		mux.mu.Unlock()
		if count == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("gateway did not register the setup connection")
		}
		time.Sleep(time.Millisecond)
	}
	done := make(chan error, 1)
	go func() { done <- mux.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("gateway close blocked on TLS setup or queue")
	}
	if _, err := mux.HTTPListener().Accept(); err == nil {
		t.Fatal("closed mode listener returned a queued stream")
	}
	mux.mu.Lock()
	count := len(mux.connections)
	mux.mu.Unlock()
	if count != 0 || len(mux.slots) != 0 {
		t.Fatal("gateway shutdown retained connections/capacity")
	}
	for _, connection := range []net.Conn{queued, unhandshaken} {
		_ = connection.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := connection.Read(make([]byte, 1)); err == nil {
			t.Fatal("closed gateway still delivered a stream")
		}
	}
}
func TestGatewayMuxRefusesWildcardListener(t *testing.T) {
	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if mux, err := NewMux(listener, testIdentity(t)); err == nil {
		mux.Close()
		t.Fatal("private gateway accepted a non-loopback bind")
	}
}

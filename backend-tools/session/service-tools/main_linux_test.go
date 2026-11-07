//go:build linux

package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/gateway"
)

func TestRelayReadinessRequiresItsExactCompletedListenerMarker(t *testing.T) {
	directory := t.TempDir()
	session := strings.Repeat("a", 32)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	if err := waitRelayReady(ctx, directory, session); err == nil {
		t.Fatal("missing listener marker appeared ready")
	}
	cancel()
	if err := os.WriteFile(filepath.Join(directory, "ready"), []byte(strings.Repeat("b", 32)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := waitRelayReady(context.Background(), directory, session); err == nil {
		t.Fatal("foreign readiness accepted")
	}
	if err := os.WriteFile(filepath.Join(directory, "ready"), []byte(session), 0600); err != nil {
		t.Fatal(err)
	}
	if err := waitRelayReady(context.Background(), directory, session); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseUsesExactSessionAndMonotonicExpiry(t *testing.T) {
	directory := t.TempDir()
	session := strings.Repeat("a", 32)
	if err := createLease(directory, session, 100); err != nil {
		t.Fatal(err)
	}
	if !liveLease(directory, 145) || liveLease(directory, 146) || liveLease(directory, 99) {
		t.Fatal("lease expiry/future timestamp boundary changed")
	}
	if err := renewLease(directory, strings.Repeat("b", 32), 110); err == nil {
		t.Fatal("foreign session renewed a lease")
	}
	if err := renewLease(directory, session, 120); err != nil || !liveLease(directory, 160) {
		t.Fatal("owned lease could not renew")
	}
	if err := createLease(directory, session, 130); err == nil {
		t.Fatal("a second main process adopted the old lease")
	}
}
func TestStdioTunnelPreservesBinaryResponseAfterRequestHalfClose(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	request := bytes.Repeat([]byte{0, 1, 255, 13, 10}, 100000)
	response := bytes.Repeat([]byte{254, 0, 255, 10}, 100000)
	server := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			server <- err
			return
		}
		defer connection.Close()
		got, err := io.ReadAll(connection)
		if err == nil && !bytes.Equal(got, request) {
			err = io.ErrUnexpectedEOF
		}
		if err == nil {
			_, err = connection.Write(response)
		}
		server <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var output bytes.Buffer
	if err = dialStdio(ctx, listener.Addr().String(), bytes.NewReader(request), &output); err != nil {
		t.Fatal(err)
	}
	if err = <-server; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), response) {
		t.Fatal("binary response or half-close was corrupted")
	}
}
func TestDuplexCancellationClosesBlockedPeers(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	clients := make([]net.Conn, 0, 2)
	servers := make([]*net.TCPConn, 0, 2)
	for range 2 {
		client, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, client)
		server, err := listener.Accept()
		if err != nil {
			t.Fatal(err)
		}
		servers = append(servers, server.(*net.TCPConn))
	}
	defer func() {
		for _, connection := range clients {
			connection.Close()
		}
		for _, connection := range servers {
			connection.Close()
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- gateway.CopyDuplex(ctx, servers[0], servers[1]) }()
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancellation was hidden")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked relay did not cancel")
	}
}

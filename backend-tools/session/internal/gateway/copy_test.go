package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"
)

func tcpPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server, err := listener.Accept()
	if err != nil {
		client.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close(); server.Close() })
	return client.(*net.TCPConn), server.(*net.TCPConn)
}
func TestGatewayCopyPreservesTCPToTLSBinaryHalfClose(t *testing.T) {
	identity := testIdentity(t)
	request := bytes.Repeat([]byte{0, 255, 13, 10}, 100000)
	response := bytes.Repeat([]byte{254, 0, 1, 10}, 100000)
	address, upstreamDone := singleGateway(t, identity, "", func(connection *tls.Conn, mode Mode) {
		got, err := io.ReadAll(connection)
		if err != nil || !bytes.Equal(got, request) {
			t.Errorf("TCP/TLS request changed: %d %v", len(got), err)
			return
		}
		_, _ = connection.Write(response)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	secure, err := Dial(ctx, address, identity.session, identity.PublicRoot(), HTTP)
	if err != nil {
		t.Fatal(err)
	}
	defer secure.Close()
	client, incoming := tcpPair(t)
	done := make(chan error, 1)
	go func() { done <- CopyDuplex(ctx, incoming, secure); incoming.Close(); secure.Close() }()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(client)
	if err != nil || !bytes.Equal(got, response) {
		t.Fatalf("TCP/TLS response after request EOF changed: %d %v", len(got), err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("TCP/TLS copy did not complete")
	}
	awaitGateway(t, upstreamDone, false)
}
func TestGatewayCopyCancellationAndIdleExpiryJoinBothWorkers(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		leftPeer, left := tcpPair(t)
		rightPeer, right := tcpPair(t)
		ctx, cancel := context.WithCancel(context.Background())
		if cancelled {
			cancel()
		}
		done := make(chan error, 1)
		go func() { done <- copyDuplex(ctx, left, right, 20*time.Millisecond) }()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("cancelled/idle stream was reported as complete")
			}
		case <-time.After(time.Second):
			t.Fatal("copy workers did not settle")
		}
		cancel()
		for _, peer := range []net.Conn{leftPeer, rightPeer} {
			_ = peer.SetReadDeadline(time.Now().Add(time.Second))
			if _, err := peer.Read(make([]byte, 1)); err != io.EOF {
				t.Fatalf("closed stream peer remained connected: %v", err)
			}
		}
	}
}

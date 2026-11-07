package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testIdentity(t *testing.T) *Identity {
	t.Helper()
	identity, err := NewIdentity(strings.Repeat("a", 32), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return identity
}
func singleGateway(t *testing.T, identity *Identity, address string, handle func(*tls.Conn, Mode)) (string, <-chan error) {
	t.Helper()
	if address == "" {
		address = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	done := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer connection.Close()
		secure := tls.Server(connection, identity.ServerConfig())
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		mode, err := AcceptMode(ctx, secure)
		if err == nil && handle != nil {
			handle(secure, mode)
		}
		done <- err
	}()
	return listener.Addr().String(), done
}
func awaitGateway(t *testing.T, done <-chan error, wantError bool) {
	t.Helper()
	select {
	case err := <-done:
		if (err != nil) != wantError {
			t.Fatalf("unexpected gateway setup result: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("gateway worker did not settle")
	}
}
func TestGatewayVerifiedModesPreserveBinaryResponseAfterRequestEOF(t *testing.T) {
	for _, mode := range []Mode{HTTP, HTTPS} {
		t.Run(string(rune('0'+mode)), func(t *testing.T) {
			identity := testIdentity(t)
			request := bytes.Repeat([]byte{0, 255, 13, 10, 1}, 100000)
			response := bytes.Repeat([]byte{254, 0, 10, 255}, 100000)
			address, done := singleGateway(t, identity, "", func(connection *tls.Conn, gotMode Mode) {
				if gotMode != mode {
					t.Error("gateway changed selected protocol")
				}
				got, err := io.ReadAll(connection)
				if err != nil || !bytes.Equal(got, request) {
					t.Errorf("binary request/half-close changed: %d %v", len(got), err)
					return
				}
				_, _ = connection.Write(response)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			connection, err := Dial(ctx, address, identity.session, identity.PublicRoot(), mode)
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
			if _, err := connection.Write(request); err != nil {
				t.Fatal(err)
			}
			if err := connection.CloseWrite(); err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(connection)
			if err != nil || !bytes.Equal(got, response) {
				t.Fatalf("binary response after EOF changed: %d %v", len(got), err)
			}
			awaitGateway(t, done, false)
		})
	}
}
func TestGatewayRejectsReusedPortWithDifferentIdentityBeforeApplicationData(t *testing.T) {
	old := testIdentity(t)
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := reserved.Addr().String()
	reserved.Close()
	replacement := testIdentity(t)
	var accepted atomic.Int32
	address, done := singleGateway(t, replacement, address, func(*tls.Conn, Mode) { accepted.Add(1) })
	if connection, err := Dial(context.Background(), address, old.session, old.PublicRoot(), HTTP); err == nil {
		connection.Close()
		t.Fatal("reused port authenticated with a different process key")
	}
	awaitGateway(t, done, true)
	if accepted.Load() != 0 {
		t.Fatal("replacement received an application stream")
	}
}
func TestGatewayRefusesExpiryWrongSNIAndProtocol(t *testing.T) {
	for _, fault := range []string{"expired", "sni", "alpn", "no-alpn"} {
		t.Run(fault, func(t *testing.T) {
			identity := testIdentity(t)
			if fault == "expired" {
				var err error
				identity, err = NewIdentity(identity.session, time.Now().AddDate(-2, 0, 0))
				if err != nil {
					t.Fatal(err)
				}
			}
			var accepted atomic.Int32
			address, done := singleGateway(t, identity, "", func(*tls.Conn, Mode) { accepted.Add(1) })
			configuration, err := ClientConfig(identity.session, identity.PublicRoot())
			if err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "sni":
				configuration.ServerName = "other.orbwin.invalid"
			case "alpn":
				configuration.NextProtos = []string{"http/1.1"}
			case "no-alpn":
				configuration.NextProtos = nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			connection, err := (&tls.Dialer{Config: configuration}).DialContext(ctx, "tcp", address)
			if err == nil {
				secure := connection.(*tls.Conn)
				frame, _ := modeFrame(HTTP)
				_ = secure.SetDeadline(time.Now().Add(time.Second))
				_ = writeAll(secure, frame[:])
				var ack [2]byte
				_, err = io.ReadFull(secure, ack[:])
				secure.Close()
			}
			if err == nil {
				t.Fatal("invalid identity/protocol passed gateway setup")
			}
			awaitGateway(t, done, true)
			if accepted.Load() != 0 {
				t.Fatal("invalid identity/protocol received application access")
			}
		})
	}
}
func TestGatewayConsumesExactlyModeFrameAndRejectsModifiedFrames(t *testing.T) {
	for _, fault := range []string{"", "magic", "version", "reserved", "mode"} {
		t.Run(fault, func(t *testing.T) {
			identity := testIdentity(t)
			payload := []byte("\x00coalesced binary\xffpayload")
			var received atomic.Int32
			address, done := singleGateway(t, identity, "", func(connection *tls.Conn, mode Mode) {
				got := make([]byte, len(payload))
				if _, err := io.ReadFull(connection, got); err != nil || !bytes.Equal(got, payload) {
					t.Errorf("prefix parser consumed payload: %q %v", got, err)
				}
				received.Add(1)
			})
			configuration, _ := ClientConfig(identity.session, identity.PublicRoot())
			connection, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address, configuration)
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			_ = connection.SetDeadline(time.Now().Add(time.Second))
			frame, _ := modeFrame(HTTPS)
			switch fault {
			case "magic":
				frame[0] = 'X'
			case "version":
				frame[4] = 2
			case "reserved":
				frame[6] = 1
			case "mode":
				frame[7] = 9
			}
			if err := writeAll(connection, append(frame[:], payload...)); err != nil {
				t.Fatal(err)
			}
			var ack [2]byte
			_, err = io.ReadFull(connection, ack[:])
			if fault == "" {
				if err != nil || ack != [2]byte{1, byte(HTTPS)} {
					t.Fatalf("valid mode not acknowledged: %v %v", ack, err)
				}
			} else if err == nil {
				t.Fatal("modified mode frame was acknowledged")
			}
			awaitGateway(t, done, fault != "")
			if fault != "" && received.Load() != 0 {
				t.Fatal("malformed frame reached application")
			}
		})
	}
}
func TestGatewayRootIsPublicOnlyAndRejectsForeignSessionOrMalformedInput(t *testing.T) {
	identity := testIdentity(t)
	serialized, err := json.Marshal(identity)
	if err != nil || string(serialized) != "{}" {
		t.Fatal("gateway identity accidentally exposes serializable key fields")
	}
	root := identity.PublicRoot()
	root[0] ^= 1
	if bytes.Equal(root, identity.PublicRoot()) {
		t.Fatal("public-root getter exposed mutable identity state")
	}
	for _, fixture := range []struct {
		session string
		root    []byte
	}{{strings.Repeat("b", 32), identity.PublicRoot()}, {identity.session, root}, {identity.session, bytes.Repeat([]byte{'x'}, RootBudget+1)}, {"short", identity.PublicRoot()}} {
		if _, err := ClientConfig(fixture.session, fixture.root); err == nil {
			t.Fatal("foreign/malformed gateway identity was accepted")
		}
	}
}

func TestGatewayModeFrameAcrossSeparateTLSRecords(t *testing.T) {
	identity := testIdentity(t)
	address, done := singleGateway(t, identity, "", func(connection *tls.Conn, mode Mode) {
		if mode != HTTP {
			t.Error("fragmented frame changed mode")
		}
		var data [3]byte
		if _, err := io.ReadFull(connection, data[:]); err != nil || data != [3]byte{0, 255, 10} {
			t.Errorf("fragmented payload changed: %v %v", data, err)
		}
		_, _ = connection.Write([]byte("ok"))
	})
	configuration, _ := ClientConfig(identity.session, identity.PublicRoot())
	connection, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	frame, _ := modeFrame(HTTP)
	for _, value := range frame {
		if _, err := connection.Write([]byte{value}); err != nil {
			t.Fatal(err)
		}
	}
	var ack [2]byte
	if _, err := io.ReadFull(connection, ack[:]); err != nil || ack != [2]byte{1, byte(HTTP)} {
		t.Fatalf("fragmented mode was not acknowledged: %v %v", ack, err)
	}
	for _, value := range []byte{0, 255, 10} {
		if _, err := connection.Write([]byte{value}); err != nil {
			t.Fatal(err)
		}
	}
	var response [2]byte
	if _, err := io.ReadFull(connection, response[:]); err != nil || string(response[:]) != "ok" {
		t.Fatal("fragmented stream did not complete")
	}
	awaitGateway(t, done, false)
}

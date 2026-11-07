package networkdiag

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResolutionOrderBoundsAndPrivateErrorRedaction(t *testing.T) {
	addresses := []net.IPAddr{{IP: net.ParseIP("::1")}, {IP: net.ParseIP("127.0.0.1")}}
	got := summarize("localhost", addresses, nil)
	if got.Resolution != "observed" || got.FirstFamily != "ipv6" || strings.Join(got.Families, ",") != "ipv6,ipv4" {
		t.Fatalf("order lost: %+v", got)
	}
	for _, bad := range [][]net.IPAddr{nil, make([]net.IPAddr, maxAnswers+1), {{IP: net.IP{1, 2, 3}}}, {{IP: net.ParseIP("::1"), Zone: "private-adapter"}}, {{IP: net.ParseIP("192.168.77.88")}}} {
		if result := summarize("localhost", bad, nil); result.Resolution == "observed" {
			t.Fatalf("unsafe answers accepted: %+v", result)
		}
	}
	got = summarize("localhost", nil, &net.DNSError{Name: "private.corp.example", Server: "192.168.77.88", Err: "secret-proxy-password", IsNotFound: true})
	data, _ := json.Marshal(got)
	if got.Resolution != "not-found" || strings.Contains(string(data), "private") || strings.Contains(string(data), "192.168") || strings.Contains(string(data), "secret") {
		t.Fatalf("private error leaked: %s", data)
	}
	if errorClass(context.Canceled) != "canceled" || errorClass(context.DeadlineExceeded) != "timeout" || errorClass(errors.New("secret")) != "unavailable" {
		t.Fatal("error classes changed")
	}
}

func TestConnectGuardRequiresExactRetainedListener(t *testing.T) {
	for _, bad := range []string{"192.168.1.1:40000", "example.com:40000", "127.0.0.2:40000", "127.0.0.1:40001", "[::1%private]:40000", "[::1]:40000", "bad"} {
		if err := ownedDestination(bad, "40000", false); err == nil {
			t.Fatalf("foreign destination accepted: %s", bad)
		}
	}
	for _, address := range []string{"127.0.0.1:40000", "[::1]:40000"} {
		if err := ownedDestination(address, "40000", true); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNonceProofRefusesOtherServiceAndJoinsOnCancellation(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	owned := startOwned(ctx, listener, strings.Repeat("a", 64))
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	if got := probeOwned(ctx, "tcp4", listener.Addr().String(), strings.Repeat("a", 64), port, false); got.State != "verified-owned-listener" {
		t.Fatalf("own nonce failed: %+v", got)
	}
	if got := probeOwned(ctx, "tcp4", listener.Addr().String(), strings.Repeat("b", 64), port, false); got.State == "verified-owned-listener" {
		t.Fatal("foreign nonce accepted")
	}
	// An extra client cannot retain the diagnostic lifetime after cancellation.
	connection, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	cancel()
	joined := make(chan struct{})
	go func() { owned.close(); close(joined) }()
	select {
	case <-joined:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled listener workers did not join")
	}
	if _, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second); err == nil {
		t.Fatal("listener survived close")
	}
}

func TestSuccessfulConnectAloneCannotProveOwnership(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(time.Second))
		io.WriteString(conn, strings.Repeat("x", 64))
	}()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	got := probeOwned(context.Background(), "tcp4", listener.Addr().String(), strings.Repeat("a", 64), port, false)
	<-done
	if got.State != "failed" || got.ErrorClass != "ownership-mismatch" {
		t.Fatalf("arbitrary TCP service accepted: %+v", got)
	}
}

func TestEchoServiceCannotReflectProbeIntoOwnership(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(time.Second))
		io.Copy(conn, conn)
	}()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	got := probeOwned(context.Background(), "tcp4", listener.Addr().String(), strings.Repeat("a", 64), port, false)
	<-done
	if got.State == "verified-owned-listener" {
		t.Fatal("echo service reflected probe into false ownership")
	}
}

func TestCanceledObservationStartsNoListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := ObserveLoopback(ctx)
	if !errors.Is(err, context.Canceled) || got.IPv4.State != "not-run" || len(got.Names) != 0 {
		t.Fatalf("canceled observation: %+v %v", got, err)
	}
}

func TestProxyConfigurationIsExplicitlyConfiguredNotEffective(t *testing.T) {
	for _, test := range []struct {
		auto, pac, manual bool
		mode              string
	}{{false, false, false, "direct-configured"}, {true, false, false, "automatic-configured"}, {false, true, false, "automatic-configured"}, {false, false, true, "manual-configured"}, {true, false, true, "mixed-configured"}} {
		got := configuration("fixed-source", test.auto, test.pac, test.manual, true)
		if got.Mode != test.mode || got.EffectiveProxy != "not-evaluated" || !got.BypassConfigured {
			t.Fatalf("configuration claim changed: %+v", got)
		}
	}
	got := unknownConfiguration("fixed-source")
	if got.Mode != "unknown" || got.State != "unavailable" || got.EffectiveProxy != "not-evaluated" {
		t.Fatal("failure inferred direct networking")
	}
}

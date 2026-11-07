// Package networkdiag provides bounded local network observations. Results do
// not establish browser, workload, proxy, VPN or enterprise DNS compatibility.
package networkdiag

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const MaxOutput = 16 << 10
const maxAnswers = 16

var fixedNames = [...]string{"localhost", "network-check.orbwin.localhost"}

type Probe struct {
	State      string `json:"state"`
	ErrorClass string `json:"errorClass,omitempty"`
}
type NameObservation struct {
	Name               string   `json:"name"`
	Resolution         string   `json:"resolution"`
	Families           []string `json:"families"`
	NonLoopbackAnswers int      `json:"nonLoopbackAnswers"`
	FirstFamily        string   `json:"firstFamily,omitempty"`
	NativeDial         Probe    `json:"nativeDial"`
	IPv4OnlyDial       Probe    `json:"ipv4OnlyDial"`
}
type LoopbackSnapshot struct {
	Version int               `json:"version"`
	ReadAt  string            `json:"readAt"`
	Source  string            `json:"source"`
	IPv4    Probe             `json:"ipv4"`
	IPv6    Probe             `json:"ipv6"`
	Names   []NameObservation `json:"names"`
}

func errorClass(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) && dns.IsNotFound {
		return "not-found"
	}
	return "unavailable"
}

func summarize(name string, addresses []net.IPAddr, err error) NameObservation {
	result := NameObservation{Name: name, Resolution: "observed", Families: []string{}, NativeDial: Probe{State: "not-run"}, IPv4OnlyDial: Probe{State: "not-run"}}
	if err != nil {
		result.Resolution = errorClass(err)
		return result
	}
	if len(addresses) == 0 || len(addresses) > maxAnswers {
		result.Resolution = "invalid-answer-count"
		return result
	}
	for _, entry := range addresses {
		address, ok := netip.AddrFromSlice(entry.IP)
		if !ok || entry.Zone != "" {
			result.Resolution = "malformed-answer"
			return result
		}
		address = address.Unmap()
		family := "ipv6"
		if address.Is4() {
			family = "ipv4"
		}
		result.Families = append(result.Families, family)
		if !address.IsLoopback() {
			result.NonLoopbackAnswers++
		}
	}
	result.FirstFamily = result.Families[0]
	if result.NonLoopbackAnswers != 0 {
		result.Resolution = "non-loopback-answer"
	}
	return result
}

// loopbackDestination is also checked immediately before every native connect.
// DNS can change after lookup; validation of an earlier answer is insufficient.
func loopbackDestination(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errors.New("loopback-only diagnostic destination required")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || ip.Zone() != "" || !ip.Unmap().IsLoopback() {
		return errors.New("loopback-only diagnostic destination required")
	}
	return nil
}

type ownedListener struct {
	listener net.Listener
	workers  sync.WaitGroup
}

func startOwned(ctx context.Context, listener net.Listener, nonce string) *ownedListener {
	owned := &ownedListener{listener: listener}
	owned.workers.Add(1)
	go func() {
		defer owned.workers.Done()
		defer listener.Close()
		slots := make(chan struct{}, 4)
		for accepted := 0; accepted < 32; accepted++ {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			select {
			case slots <- struct{}{}:
			default:
				connection.Close()
				continue
			}
			owned.workers.Add(1)
			go func() {
				defer owned.workers.Done()
				defer func() { <-slots }()
				defer connection.Close()
				stop := context.AfterFunc(ctx, func() { connection.Close() })
				defer stop()
				connection.SetDeadline(time.Now().Add(time.Second))
				// Server-first proof: do not reveal the expected nonce to an
				// unrelated echo service and mistake reflection for ownership.
				io.WriteString(connection, nonce)
			}()
		}
	}()
	return owned
}
func (owned *ownedListener) close() { owned.listener.Close(); owned.workers.Wait() }

func ownedDestination(address, port string, ipv6 bool) error {
	if err := loopbackDestination(address); err != nil {
		return err
	}
	host, actualPort, _ := net.SplitHostPort(address)
	ip, _ := netip.ParseAddr(host)
	ip = ip.Unmap()
	if actualPort != port || ip.String() != "127.0.0.1" && !(ipv6 && ip.String() == "::1") {
		return errors.New("diagnostic destination has no retained owned listener")
	}
	return nil
}

func probeOwned(ctx context.Context, network, destination, nonce, port string, ipv6 bool) Probe {
	probeCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	dialer := net.Dialer{FallbackDelay: 300 * time.Millisecond, ControlContext: func(_ context.Context, _ string, address string, _ syscall.RawConn) error {
		return ownedDestination(address, port, ipv6)
	}}
	connection, err := dialer.DialContext(probeCtx, network, destination)
	if err != nil {
		return Probe{State: "failed", ErrorClass: errorClass(err)}
	}
	defer connection.Close()
	stop := context.AfterFunc(probeCtx, func() { connection.Close() })
	defer stop()
	if deadline, ok := probeCtx.Deadline(); ok {
		connection.SetDeadline(deadline)
	}
	buffer := make([]byte, len(nonce))
	if _, err := io.ReadFull(connection, buffer); err != nil {
		return Probe{State: "failed", ErrorClass: errorClass(err)}
	}
	if string(buffer) != nonce {
		return Probe{State: "failed", ErrorClass: "ownership-mismatch"}
	}
	return Probe{State: "verified-owned-listener"}
}

// ObserveLoopback owns temporary IPv4/IPv6 loopback listeners at one ephemeral
// port and closes/joins them before return. It accepts no hostname, destination,
// caller-provided port, URL, proxy or credential. Each connection proves a fresh
// random nonce; a successful TCP connect alone is never reported as our listener.
// No LAN binding, engine/helper mutation or external endpoint request is made.
func ObserveLoopback(ctx context.Context) (LoopbackSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	result := LoopbackSnapshot{Version: 1, ReadAt: time.Now().UTC().Format(time.RFC3339Nano), Source: "os-resolver-and-nonce-owned-loopback", IPv4: Probe{State: "not-run"}, IPv6: Probe{State: "not-run"}, Names: []NameObservation{}}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return result, errors.New("could not create diagnostic listener identity")
	}
	nonce := hex.EncodeToString(random)
	v4, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", "127.0.0.1:0")
	if err != nil {
		result.IPv4 = Probe{State: "listen-unavailable", ErrorClass: errorClass(err)}
		return result, errors.New("owned IPv4 loopback listener unavailable")
	}
	owned4 := startOwned(ctx, v4, nonce)
	defer owned4.close()
	port := strconv.Itoa(v4.Addr().(*net.TCPAddr).Port)
	v6, err := (&net.ListenConfig{}).Listen(ctx, "tcp6", net.JoinHostPort("::1", port))
	if err == nil {
		owned6 := startOwned(ctx, v6, nonce)
		defer owned6.close()
		result.IPv6 = probeOwned(ctx, "tcp6", net.JoinHostPort("::1", port), nonce, port, true)
	} else {
		result.IPv6 = Probe{State: "listen-unavailable", ErrorClass: errorClass(err)}
	}
	result.IPv4 = probeOwned(ctx, "tcp4", net.JoinHostPort("127.0.0.1", port), nonce, port, false)
	for _, name := range fixedNames {
		lookupCtx, stop := context.WithTimeout(ctx, time.Second)
		addresses, err := net.DefaultResolver.LookupIPAddr(lookupCtx, name)
		stop()
		observation := summarize(name, addresses, err)
		if observation.Resolution == "observed" {
			observation.NativeDial = probeOwned(ctx, "tcp", net.JoinHostPort(name, port), nonce, port, v6 != nil)
			// Explicitly exercise Go's named-dial fallback with only our IPv4
			// destination authorized. IPv6 is refused before connect, not probed
			// on an unowned socket. This is not browser fallback qualification.
			observation.IPv4OnlyDial = probeOwned(ctx, "tcp", net.JoinHostPort(name, port), nonce, port, false)
		}
		result.Names = append(result.Names, observation)
	}
	return result, ctx.Err()
}

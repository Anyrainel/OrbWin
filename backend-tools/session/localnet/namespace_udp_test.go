package localnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"orbwin.local/session/internal/landatagram"
)

func TestDatagramCommandRefusesSelectorsAndMalformedIdentity(t *testing.T) {
	h, tok := strings.Repeat("a", 64), strings.Repeat("b", 32)
	cmd, e := datagramCommand(h, 53, tok)
	if e != nil || strings.Join(cmd, " ") != "/orbwin-services dial-udp 53 "+tok {
		t.Fatal(cmd, e)
	}
	for _, x := range []struct {
		h string
		p uint16
		t string
	}{{"", 53, tok}, {strings.ToUpper(h), 53, tok}, {h, 0, tok}, {h, 53, strings.ToUpper(tok)}, {h, 53, "tcp://foreign"}} {
		if _, e := datagramCommand(x.h, x.p, x.t); e == nil {
			t.Fatal("invalid command accepted", x)
		}
	}
}
func TestDatagramCapacityAcrossLeasesAndUnknownRetention(t *testing.T) {
	h := NamespaceHandle{Intent: namespaceFixture(), ID: strings.Repeat("e", 64)}
	m := &NamespaceManager{active: map[string]NamespaceHandle{namespaceKey(h.Intent): h}}
	var releases []func()
	// Distinct callers/leases all share the SAME helper budget.
	for i := 0; i < landatagram.MaxPeers; i++ {
		f, e := m.reserveDatagram(h)
		if e != nil {
			t.Fatal(e)
		}
		releases = append(releases, f)
	}
	if _, e := m.reserveDatagram(h); e == nil {
		t.Fatal("17th global association admitted")
	}
	// Unknown cleanup deliberately does not call release, retaining its slot.
	for _, f := range releases[1:] {
		f()
	}
	if m.datagramSlots[h.ID] != 1 {
		t.Fatal("uncertain association capacity lost")
	}
	releases[0]()
	releases[0]()
	if len(m.datagramSlots) != 0 {
		t.Fatal("double release")
	}
	m.closed = true
	if _, e := m.reserveDatagram(h); e == nil {
		t.Fatal("shutdown admission")
	}
}

func TestDatagramFailedRevocationPoisonsAndLateCloseCannotReinsertSettlement(t *testing.T) {
	h := NamespaceHandle{Intent: namespaceFixture(), ID: strings.Repeat("e", 64)}
	m := &NamespaceManager{active: map[string]NamespaceHandle{namespaceKey(h.Intent): h}}
	release, e := m.reserveDatagram(h)
	if e != nil {
		t.Fatal(e)
	}
	fault := errors.New("revocation acknowledgement lost")
	if e = m.finishDatagram(h, release, fault); !errors.Is(e, fault) || m.datagramSlots[h.ID] != 1 || m.streamFailure(h.ID) == nil {
		t.Fatal("uncertainty lost", e)
	}
	if _, e = m.reserveDatagram(h); e == nil {
		t.Fatal("poisoned helper admitted")
	}
	m.mu.Lock()
	e = m.settleDatagramAbsenceLocked(h.ID)
	m.mu.Unlock()
	if e != nil {
		t.Fatal(e)
	}
	release()
	release()
	if len(m.datagramSlots) != 0 {
		t.Fatal("late close reinserted/negated count")
	}
	if m.streamFailure(h.ID) == nil {
		t.Fatal("capacity settlement cleared namespace poison")
	}
	// The owning physical-absence path clears its poison after ledger settlement.
	delete(m.poisoned, h.ID)
	if e = m.finishDatagram(h, release, fault); e != nil || m.streamFailure(h.ID) != nil {
		t.Fatal("late failure re-poisoned retired helper", e)
	}
}
func TestDatagramRetainedHelperHistoryIsBoundedWithoutEviction(t *testing.T) {
	h := NamespaceHandle{Intent: namespaceFixture(), ID: strings.Repeat("e", 64)}
	m := &NamespaceManager{active: map[string]NamespaceHandle{namespaceKey(h.Intent): h}, datagramSlots: make(map[string]int)}
	for i := 0; i < maxDatagramHelpers; i++ {
		m.datagramSlots[fmt.Sprintf("%064x", i)] = 1
	}
	if _, e := m.reserveDatagram(h); e == nil || len(m.datagramSlots) != maxDatagramHelpers {
		t.Fatal("history grew/evicted")
	}
}

func TestDatagramSettlementRequiresSameDaemonExactNotFound(t *testing.T) {
	for _, fault := range []string{"", "foreign-daemon", "daemon-drift-after-inspect", "inspect-error", "still-exists"} {
		t.Run(fault, func(t *testing.T) {
			h := strings.Repeat("a", 64)
			infoReads := 0
			transport := datagramRoundTrip(func(r *http.Request) (*http.Response, error) {
				status, body := 200, `{}`
				if strings.HasSuffix(r.URL.Path, "/info") {
					infoReads++
					id := "owned"
					if fault == "foreign-daemon" || (fault == "daemon-drift-after-inspect" && infoReads > 1) {
						id = "foreign"
					}
					body = fmt.Sprintf(`{"ID":%q,"OSType":"linux"}`, id)
				} else if strings.HasSuffix(r.URL.Path, "/containers/"+h+"/json") {
					if fault == "inspect-error" {
						return nil, errors.New("synthetic loss")
					}
					if fault != "still-exists" {
						status = 404
						body = `{"message":"No such container"}`
					}
				} else {
					t.Fatal("unexpected request", r.Method, r.URL.Path)
				}
				if r.Method != "GET" {
					t.Fatal("settlement mutated", r.Method)
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			cli, e := client.New(client.WithHost("http://127.0.0.1:1"), client.WithVersion("1.54"), client.WithHTTPClient(&http.Client{Transport: transport}))
			if e != nil {
				t.Fatal(e)
			}
			defer cli.Close()
			m := &NamespaceManager{cli: cli, daemon: "owned", datagramSlots: map[string]int{h: 1}}
			e = m.SettleDatagramAbsence(context.Background(), h)
			if fault == "" {
				if e != nil || len(m.datagramSlots) != 0 || infoReads != 2 {
					t.Fatal("absence not settled", e)
				}
			} else if e == nil || m.datagramSlots[h] != 1 {
				t.Fatal("uncertainty/foreign accepted", e)
			}
		})
	}
}

type datagramRoundTrip func(*http.Request) (*http.Response, error)

func (f datagramRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDatagramUnknownCreateAndFailedAttachAlwaysFenceToken(t *testing.T) {
	for _, createAccepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown-create", true: "failed-attach"}[createAccepted], func(t *testing.T) {
			transport := datagramRoundTrip(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "/exec") && createAccepted {
					return &http.Response{StatusCode: 201, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"Id":"` + strings.Repeat("f", 64) + `"}`)), Request: r}, nil
				}
				return nil, errors.New("synthetic uncertain transport")
			})
			cli, e := client.New(client.WithHost("http://127.0.0.1:1"), client.WithVersion("1.54"), client.WithHTTPClient(&http.Client{Transport: transport}))
			if e != nil {
				t.Fatal(e)
			}
			defer cli.Close()
			for _, fenceFails := range []bool{false, true} {
				fenced, released := 0, false
				_, err := dialNamespaceDatagram(context.Background(), cli, strings.Repeat("a", 64), 53, func(ctx context.Context, token, execID string) error {
					fenced++
					if len(token) != 32 {
						t.Fatal("token missing")
					}
					if createAccepted && !fullID(execID) || !createAccepted && execID != "" {
						t.Fatal("wrong exec identity", execID)
					}
					if _, ok := ctx.Deadline(); !ok {
						t.Fatal("unbounded revoke")
					}
					if fenceFails {
						return errors.New("revoke uncertain")
					}
					if execID == "" {
						return errors.New("exec identity unknown")
					}
					released = true
					return nil
				}, func() { released = true })
				if err == nil || fenced != 1 || released != (createAccepted && !fenceFails) {
					t.Fatal("unknown exec freed without fence", err, fenced, released)
				}
			}
		})
	}
}

func TestDatagramExitBarrierRequiresExactStoppedExecAndBoundedDeadline(t *testing.T) {
	h, id := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, fault := range []string{"", "foreign-exec", "foreign-helper", "running", "inspect-loss", "unknown-id"} {
		t.Run(fault, func(t *testing.T) {
			transport := datagramRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/exec/"+id+"/json") {
					t.Fatal("unexpected request", r.Method, r.URL.Path)
				}
				if fault == "inspect-loss" {
					return nil, errors.New("synthetic transport loss")
				}
				ri, rh := id, h
				if fault == "foreign-exec" {
					ri = strings.Repeat("c", 64)
				}
				if fault == "foreign-helper" {
					rh = strings.Repeat("d", 64)
				}
				body := fmt.Sprintf(`{"ID":%q,"ContainerID":%q,"Running":%t}`, ri, rh, fault == "running")
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			cli, e := client.New(client.WithHost("http://127.0.0.1:1"), client.WithVersion("1.54"), client.WithHTTPClient(&http.Client{Transport: transport}))
			if e != nil {
				t.Fatal(e)
			}
			defer cli.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			execID := id
			if fault == "unknown-id" {
				execID = ""
			}
			e = waitDatagramExit(ctx, cli, h, execID)
			if (e == nil) != (fault == "") {
				t.Fatal("barrier result", e)
			}
			if e = waitDatagramExit(context.Background(), cli, h, id); e == nil {
				t.Fatal("unbounded cleanup")
			}
		})
	}
}

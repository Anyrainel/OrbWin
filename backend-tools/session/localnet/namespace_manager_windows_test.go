//go:build windows

package localnet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"orbwin.local/session/internal/streamcontrol"
)

type managerDaemon struct {
	streamToken                        string
	readies, revocations               int
	mu                                 sync.Mutex
	t                                  *testing.T
	manager                            *NamespaceManager
	intent                             NamespaceIntent
	config                             *container.Config
	host                               *container.HostConfig
	name, fault                        string
	exists, running                    bool
	creates, starts, removes, renewals int
	dials                              int
	renewStarted                       chan struct{}
}

func (daemon *managerDaemon) serve(w http.ResponseWriter, r *http.Request) {
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	intent := daemon.intent
	id := strings.Repeat("e", 64)
	encode := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	missing := func() { w.WriteHeader(404); encode(map[string]string{"message": "No such container"}) }
	switch r.Method + " " + r.URL.Path {
	case "GET /v1.56/info":
		identity := "owned-daemon"
		if daemon.fault == "foreign-daemon" {
			identity = "other-daemon"
		}
		encode(map[string]string{"ID": identity, "OSType": "linux"})
	case "GET /v1.56/images/" + intent.ImageID + "/json":
		encode(map[string]any{"Id": intent.ImageID, "Os": "linux", "Architecture": "amd64", "Config": map[string]any{"Labels": map[string]string{"dev.orbwin.component": "local-services-tools", "dev.orbwin.source-sha256": intent.AssetSHA}}})
	case "GET /v1.56/containers/" + intent.ContainerID + "/json":
		started := intent.StartedAt
		if daemon.fault == "target-restarted" && daemon.starts > 0 {
			started = "2026-10-24T13:00:00Z"
		}
		encode(map[string]any{"Id": intent.ContainerID, "State": map[string]any{"Running": true, "StartedAt": started}, "Config": map[string]any{}, "HostConfig": map[string]string{"NetworkMode": "bridge"}})
	case "POST /v1.56/containers/create":
		daemon.creates++
		var body struct {
			container.Config
			HostConfig *container.HostConfig
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			daemon.t.Error(err)
			return
		}
		daemon.config, daemon.host = &body.Config, body.HostConfig
		daemon.name = r.URL.Query().Get("name")
		if daemon.manager == nil {
			daemon.t.Error("creation preceded manager ownership")
			return
		}
		records, err := daemon.manager.ledger.records()
		if err != nil || len(records) != 1 || records[0].ID != "" || records[0].Intent.name() != daemon.name {
			daemon.t.Errorf("create preceded durable immutable intent: %+v %v", records, err)
			return
		}
		daemon.exists = true
		if daemon.fault == "uncertain-create" {
			http.Error(w, "accepted but reply failed", 500)
			return
		}
		if daemon.fault == "damaged-accepted-record" {
			file, err := os.OpenFile(daemon.manager.ledger.path(records[0].Intent.Session), os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				daemon.t.Error(err)
				return
			}
			_, _ = file.WriteString(`{"id":"partial`)
			file.Close()
		}
		w.WriteHeader(201)
		encode(map[string]string{"Id": id})
	case "POST /v1.56/containers/" + id + "/start":
		daemon.starts++
		records, err := daemon.manager.ledger.records()
		if err != nil || len(records) != 1 || records[0].ID != id {
			daemon.t.Error("helper started before accepted ID was flushed")
		}
		if daemon.fault == "failed-start" {
			http.Error(w, "fixture start failed", 500)
			return
		}
		daemon.running = true
		w.WriteHeader(204)
	case "GET /v1.56/containers/" + id + "/json":
		if !daemon.exists {
			missing()
			return
		}
		labels := daemon.config.Labels
		if daemon.fault == "foreign-helper" {
			labels = map[string]string{"dev.orbwin.session": "foreign"}
		}
		configuration := *daemon.config
		configuration.Labels = labels
		encode(map[string]any{"Id": id, "Name": "/" + daemon.name, "Image": intent.ImageID, "Config": configuration, "HostConfig": daemon.host, "State": map[string]any{"Running": daemon.running}})
	case "DELETE /v1.56/containers/" + id:
		if !daemon.exists {
			missing()
			return
		}
		if r.URL.Query().Get("force") != "1" && r.URL.Query().Get("force") != "true" {
			daemon.t.Error("expected exact-owned helper cleanup")
		}
		daemon.removes++
		daemon.exists = false
		w.WriteHeader(204)
	case "POST /v1.56/containers/" + id + "/exec":
		var body struct{ Cmd []string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Cmd) == 4 && strings.Join(body.Cmd[:3], " ") == "/orbwin-services dial 8080" {
			if _, err := streamcontrol.ParseToken(body.Cmd[3]); err != nil {
				daemon.t.Error(err)
			}
			daemon.streamToken = body.Cmd[3]
			daemon.dials++
			w.WriteHeader(201)
			encode(map[string]string{"Id": "dial-exec"})
			return
		}
		if len(body.Cmd) == 3 && body.Cmd[0] == "/orbwin-services" && body.Cmd[1] == "revoke" {
			if body.Cmd[2] != daemon.streamToken {
				daemon.t.Error("revocation did not name its exact dial nonce")
			}
			daemon.revocations++
			w.WriteHeader(201)
			encode(map[string]string{"Id": "revoke-exec"})
			return
		}
		if strings.Join(body.Cmd, " ") == "/orbwin-services ready-netns "+daemon.config.Labels["dev.orbwin.session"] {
			daemon.readies++
			w.WriteHeader(201)
			encode(map[string]string{"Id": "ready-exec"})
			return
		}
		if strings.Join(body.Cmd, " ") != "/orbwin-services renew-netns "+daemon.config.Labels["dev.orbwin.session"] {
			daemon.t.Error("renewal invoked an unexpected executable/identity")
		}
		daemon.renewals++
		w.WriteHeader(201)
		encode(map[string]string{"Id": "renew-exec"})
	case "POST /v1.56/exec/renew-exec/start", "POST /v1.56/exec/ready-exec/start", "POST /v1.56/exec/revoke-exec/start":
		var attach struct{ Detach, Tty bool }
		if err := json.NewDecoder(r.Body).Decode(&attach); err != nil || attach.Detach || attach.Tty {
			daemon.t.Error("control command must use attached non-TTY output")
			return
		}
		connection, stream, err := w.(http.Hijacker).Hijack()
		if err != nil {
			daemon.t.Error(err)
			return
		}
		defer connection.Close()
		_, _ = stream.WriteString("HTTP/1.1 101 UPGRADED\r\nConnection: Upgrade\r\nUpgrade: tcp\r\nContent-Type: application/vnd.docker.raw-stream\r\n\r\n")
		_ = stream.Flush()
		ack := "renewed\n"
		if strings.Contains(r.URL.Path, "revoke-exec") {
			ack = "revoked\n"
			if daemon.fault == "failed-revocation" {
				ack = "wrong\n"
			}
		}
		_, _ = connection.Write(execFrame(1, []byte(ack)))
	case "POST /v1.56/exec/dial-exec/start":
		if daemon.fault == "failed-dial-attach" {
			http.Error(w, "uncertain dial start", 500)
			return
		}
		var attach struct{ Detach, Tty bool }
		if err := json.NewDecoder(r.Body).Decode(&attach); err != nil || attach.Detach || attach.Tty {
			daemon.t.Error("HTTP tunnel did not use an attached non-TTY exec")
			return
		}
		connection, stream, err := w.(http.Hijacker).Hijack()
		if err != nil {
			daemon.t.Error(err)
			return
		}
		defer connection.Close()
		_, _ = stream.WriteString("HTTP/1.1 101 UPGRADED\r\nConnection: Upgrade\r\nUpgrade: tcp\r\nContent-Type: application/vnd.docker.raw-stream\r\n\r\n")
		_ = stream.Flush()
		request, err := http.ReadRequest(stream.Reader)
		if err != nil {
			daemon.t.Error(err)
			return
		}
		request.Body.Close()
		if (request.Method != http.MethodHead && request.Method != http.MethodGet) || request.URL.Path != "/" || request.Host != "app.orbwin.localhost" {
			daemon.t.Error("probe altered target method/path/host")
		}
		response := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
		if request.Method == http.MethodGet {
			if request.Header.Get("X-Forwarded-Proto") != "https" {
				daemon.t.Error("public HTTPS scheme was not preserved across SDK namespace tunnel")
			}
			response = "HTTP/1.1 200 OK\r\nContent-Length: 16\r\nConnection: close\r\n\r\nowned\x00api-result"
		}
		_, _ = connection.Write(execFrame(1, []byte(response)))
	case "GET /v1.56/exec/renew-exec/json", "GET /v1.56/exec/ready-exec/json", "GET /v1.56/exec/revoke-exec/json":
		if daemon.fault == "blocked-renewal" && strings.Contains(r.URL.Path, "renew-exec") {
			close(daemon.renewStarted)
			<-r.Context().Done()
			return
		}
		exit := 0
		if daemon.fault == "failed-renewal" && strings.Contains(r.URL.Path, "renew-exec") {
			exit = 125
		}
		encode(map[string]any{"Running": false, "ExitCode": exit, "ContainerID": id})
	default:
		if r.Method == "GET" && daemon.name != "" && r.URL.Path == "/v1.56/containers/"+daemon.name+"/json" {
			if !daemon.exists {
				missing()
				return
			}
			encode(map[string]any{"Id": id})
			return
		}
		daemon.t.Errorf("unexpected/unowned API call: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected API", 500)
	}
}
func managerFixture(t *testing.T, fault string) (*managerDaemon, *client.Client, string) {
	t.Helper()
	daemon := &managerDaemon{t: t, intent: namespaceFixture(), fault: fault}
	server := httptest.NewServer(http.HandlerFunc(daemon.serve))
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost("tcp://"+server.Listener.Addr().String()), client.WithAPIVersion("1.56"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cli.Close() })
	return daemon, cli, t.TempDir()
}
func openManagerFixture(t *testing.T, daemon *managerDaemon, cli *client.Client, directory string) *NamespaceManager {
	t.Helper()
	manager, err := OpenNamespaceManager(context.Background(), cli, directory, "owned-daemon", daemon.intent.ImageID, daemon.intent.AssetSHA)
	if err != nil {
		t.Fatal(err)
	}
	daemon.mu.Lock()
	daemon.manager = manager
	daemon.mu.Unlock()
	t.Cleanup(func() { _ = manager.Close(context.Background()) })
	return manager
}
func TestNamespaceManagerCreatesRenewsAndReleasesOnlyExactOwnedHelper(t *testing.T) {
	daemon, cli, directory := managerFixture(t, "")
	manager := openManagerFixture(t, daemon, cli, directory)
	handle, err := manager.Acquire(context.Background(), daemon.intent.ContainerID, daemon.intent.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	again, err := manager.Acquire(context.Background(), daemon.intent.ContainerID, daemon.intent.StartedAt)
	if err != nil || again != handle {
		t.Fatal("unchanged target recreated its namespace helper")
	}
	if err := manager.Renew(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	foreign := handle
	foreign.ID = strings.Repeat("f", 64)
	if manager.Release(context.Background(), foreign) == nil {
		t.Fatal("unowned helper release was accepted")
	}
	if err := manager.Release(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	if daemon.creates != 1 || daemon.starts != 1 || daemon.removes != 1 || daemon.renewals != 1 {
		t.Fatalf("unexpected lifecycle: %+v", daemon)
	}
	records, err := manager.ledger.records()
	if err != nil || len(records) != 0 {
		t.Fatal("confirmed cleanup retained accepted ownership")
	}
}

func TestNamespaceManagerLANIntentSurvivesCreateAndExactRelease(t *testing.T) {
	daemon, cli, directory := managerFixture(t, "")
	manager := openManagerFixture(t, daemon, cli, directory)
	leaseSHA := strings.Repeat("c", 64)
	handle, err := manager.AcquireLAN(context.Background(), daemon.intent.ContainerID, daemon.intent.StartedAt, leaseSHA)
	if err != nil {
		t.Fatal(err)
	}
	records, err := manager.ledger.records()
	if err != nil || len(records) != 1 || records[0].Intent.LANLeaseSHA != leaseSHA || records[0].ID != handle.ID {
		t.Fatal("LAN helper lost immutable ledger purpose", err)
	}
	daemon.mu.Lock()
	label := daemon.config.Labels["dev.orbwin.lan-lease"]
	daemon.mu.Unlock()
	if label != leaseSHA {
		t.Fatal("actual create lost immutable LAN lease label")
	}
	if err = manager.Release(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
}

func TestNamespaceManagerReservesSeparateHTTPAndLANHelperBudgets(t *testing.T) {
	for _, requestedLAN := range []bool{false, true} {
		for _, existingLAN := range []bool{false, true} {
			t.Run(fmt.Sprintf("requestLAN=%t/existingLAN=%t", requestedLAN, existingLAN), func(t *testing.T) {
				daemon, cli, directory := managerFixture(t, "")
				manager := openManagerFixture(t, daemon, cli, directory)
				// Admission-only stand-ins: they cannot trigger API cleanup or be
				// adopted as the newly requested exact workload incarnation.
				for i := 0; i < 16; i++ {
					intent := daemon.intent
					intent.ContainerID = fmt.Sprintf("%064x", i+1)
					if existingLAN {
						intent.LANLeaseSHA = fmt.Sprintf("%064x", i+32)
					}
					manager.active[namespaceKey(intent)] = NamespaceHandle{Intent: intent, ID: fmt.Sprintf("%064x", i+64)}
				}
				defer func() { clear(manager.active) }()
				var handle NamespaceHandle
				var err error
				if requestedLAN {
					handle, err = manager.AcquireLAN(context.Background(), daemon.intent.ContainerID, daemon.intent.StartedAt, strings.Repeat("c", 64))
				} else {
					handle, err = manager.Acquire(context.Background(), daemon.intent.ContainerID, daemon.intent.StartedAt)
				}
				if requestedLAN == existingLAN {
					if err == nil || daemon.creates != 0 {
						t.Fatal("full purpose budget admitted another helper")
					}
				} else if err != nil {
					t.Fatal("one purpose consumed the other purpose's helper budget", err)
				} else if err = manager.Release(context.Background(), handle); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestNamespaceManagerFailuresRetainUncertaintyAndRecoverByExactIntent(t *testing.T) {
	for _, fault := range []string{"uncertain-create", "failed-start", "target-restarted", "damaged-accepted-record", "foreign-helper"} {
		t.Run(fault, func(t *testing.T) {
			daemon, cli, directory := managerFixture(t, fault)
			manager := openManagerFixture(t, daemon, cli, directory)
			if _, err := manager.Acquire(context.Background(), daemon.intent.ContainerID, daemon.intent.StartedAt); err == nil {
				t.Fatal("failed/changed helper was activated")
			}
			_ = manager.Close(context.Background())
			daemon.mu.Lock()
			creates, removes := daemon.creates, daemon.removes
			daemon.mu.Unlock()
			if creates != 1 {
				t.Fatal("uncertain create was retried")
			}
			if fault == "foreign-helper" || fault == "uncertain-create" {
				if removes != 0 {
					t.Fatal("unverified/uncertain helper was deleted during failed acquisition")
				}
			}
			if fault == "foreign-helper" {
				if unexpected, err := OpenNamespaceManager(context.Background(), cli, directory, "owned-daemon", daemon.intent.ImageID, daemon.intent.AssetSHA); err == nil {
					unexpected.Close(context.Background())
					t.Fatal("recovery adopted a foreign helper")
				}
			}
			daemon.mu.Lock()
			daemon.fault = ""
			daemon.mu.Unlock()
			reopened, err := OpenNamespaceManager(context.Background(), cli, directory, "owned-daemon", daemon.intent.ImageID, daemon.intent.AssetSHA)
			if fault == "damaged-accepted-record" {
				if err == nil {
					reopened.Close(context.Background())
					t.Fatal("damaged journal was silently repaired/adopted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close(context.Background())
			records, err := reopened.ledger.records()
			if err != nil || len(records) != 0 {
				t.Fatalf("inactive exact helper did not recover: %+v %v", records, err)
			}
			daemon.mu.Lock()
			defer daemon.mu.Unlock()
			if daemon.exists || daemon.removes != 1 {
				t.Fatal("startup recovery did not remove exactly the recorded helper")
			}
		})
	}
}
func TestNamespaceManagerForeignDaemonAndFailedRenewalAreExplicit(t *testing.T) {
	daemon, cli, directory := managerFixture(t, "foreign-daemon")
	if manager, err := OpenNamespaceManager(context.Background(), cli, directory, "owned-daemon", daemon.intent.ImageID, daemon.intent.AssetSHA); err == nil {
		manager.Close(context.Background())
		t.Fatal("foreign daemon was adopted")
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("foreign daemon caused a ledger write")
	}
	daemon.mu.Lock()
	daemon.fault = ""
	daemon.mu.Unlock()
	manager := openManagerFixture(t, daemon, cli, directory)
	handle, err := manager.Acquire(context.Background(), daemon.intent.ContainerID, daemon.intent.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	daemon.mu.Lock()
	daemon.fault = "failed-renewal"
	daemon.mu.Unlock()
	if err := manager.Renew(context.Background(), handle); err == nil {
		t.Fatal("failed guest renewal became a healthy lease")
	}
	daemon.mu.Lock()
	daemon.fault = "foreign-daemon"
	daemon.mu.Unlock()
	if err := manager.Release(context.Background(), handle); err == nil {
		t.Fatal("changed endpoint accepted helper removal")
	}
	if err := manager.Close(context.Background()); err == nil {
		t.Fatal("changed endpoint cleanup reported success")
	}
	daemon.mu.Lock()
	removed := daemon.removes
	daemon.fault = ""
	daemon.mu.Unlock()
	if removed != 0 {
		t.Fatal("changed endpoint received a destructive cleanup call")
	}
	reopened, err := OpenNamespaceManager(context.Background(), cli, directory, "owned-daemon", daemon.intent.ImageID, daemon.intent.AssetSHA)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close(context.Background())
}

func TestNamespaceManagerRetainsMissingUnknownCreateAndRecoversLateAcceptance(t *testing.T) {
	daemon, cli, directory := managerFixture(t, "")
	ledger, err := OpenNamespaceLedger(directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Begin(daemon.intent); err != nil {
		t.Fatal(err)
	}
	ledger.Close()
	daemon.mu.Lock()
	daemon.name = daemon.intent.name()
	daemon.mu.Unlock()
	manager := openManagerFixture(t, daemon, cli, directory)
	if count, err := manager.Recover(context.Background()); err != nil || count != 0 {
		t.Fatalf("missing unaccepted create was incorrectly recovered: %d %v", count, err)
	}
	if items, err := manager.Pending(); err != nil || len(items) != 1 || items[0].State != "create-acceptance-unknown" {
		t.Fatalf("retained uncertainty is absent from service status: %+v %v", items, err)
	}
	records, err := manager.ledger.records()
	if err != nil || len(records) != 1 || records[0].ID != "" {
		t.Fatal("unknown intent disappeared before delayed create acceptance")
	}
	options, _ := NamespaceConfiguration(daemon.intent)
	daemon.mu.Lock()
	daemon.config = options.Config
	daemon.host = options.HostConfig
	daemon.exists = true
	daemon.mu.Unlock()
	if count, err := manager.Recover(context.Background()); err != nil || count != 1 {
		t.Fatalf("late accepted owned helper did not recover: %d %v", count, err)
	}
	records, err = manager.ledger.records()
	if err != nil || len(records) != 0 {
		t.Fatal("confirmed late accepted helper retained its record")
	}
	if items, err := manager.Pending(); err != nil || len(items) != 0 {
		t.Fatal("confirmed cleanup retained stale pending status")
	}
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	if daemon.creates != 0 || daemon.starts != 0 || daemon.removes != 1 {
		t.Fatal("recovery replayed create/start instead of cleaning the exact late helper")
	}
}

func TestNamespaceManagerCloseCancelsAndJoinsInFlightRenewal(t *testing.T) {
	daemon, cli, directory := managerFixture(t, "")
	manager := openManagerFixture(t, daemon, cli, directory)
	handle, err := manager.Acquire(context.Background(), daemon.intent.ContainerID, daemon.intent.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	daemon.mu.Lock()
	daemon.fault = "blocked-renewal"
	daemon.renewStarted = started
	daemon.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- manager.Renew(context.Background(), handle) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("renewal did not reach blocked guest inspection")
	}
	if err := manager.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled renewal reported healthy lease")
		}
	case <-time.After(time.Second):
		t.Fatal("controller close left renewal running")
	}
	reopened, err := OpenNamespaceLedger(directory, "owned-daemon")
	if err != nil {
		t.Fatalf("close did not release process lease after joining workers: %v", err)
	}
	defer reopened.Close()
	if records, err := reopened.records(); err != nil || len(records) != 0 {
		t.Fatal("shutdown failed to settle the exact helper")
	}
}

func TestServiceControllerUsesOwnedManagerAndFramedExecTransport(t *testing.T) {
	daemon, cli, directory := managerFixture(t, "")
	manager := openManagerFixture(t, daemon, cli, directory)
	controller, err := NewServiceController(manager)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Close(context.Background())
	candidate := controllerCandidate()
	status, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || len(status) != 1 || status[0].State != "active" || len(controller.ActiveRoutes()) != 2 {
		t.Fatalf("controller/SDK/namespace tunnel contract failed: %+v %v", status, err)
	}
	status, err = controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || status[0].State != "active" {
		t.Fatal("owned manager renewal lost a verified route")
	}
	if err := controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	if daemon.creates != 1 || daemon.starts != 1 || daemon.dials != 1 || daemon.renewals != 1 || daemon.removes != 1 {
		t.Fatalf("unexpected owned contract journey: create=%d start=%d dial=%d renew=%d remove=%d", daemon.creates, daemon.starts, daemon.dials, daemon.renewals, daemon.removes)
	}
}

func TestServiceHostProtectedAuthorityThroughGatewayAndOwnedSDKNamespace(t *testing.T) {
	daemon, cli, directory := managerFixture(t, "")
	manager := openManagerFixture(t, daemon, cli, directory)
	installation := strings.Repeat("c", 32)
	created, err := manager.ledger.CreateAuthority(installation, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := manager.ledger.LoadAuthority(installation, created.Fingerprint(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	host, err := StartServiceHost(manager, loaded, strings.Repeat("f", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close(context.Background())
	candidate := controllerCandidate()
	status, err := host.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || status[0].State != "active" {
		t.Fatalf("combined service setup failed: %+v %v", status, err)
	}
	connection := hostStream(t, host, loaded, candidate.Names[0], true)
	response := hostGet(t, connection, candidate.Names[0], "/")
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	connection.Close()
	if err != nil || string(data) != "owned\x00api-result" {
		t.Fatalf("combined protected-state/TLS/SDK response changed: %q %v", data, err)
	}
	if err := host.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	daemon.mu.Lock()
	creates, starts, dials, removes := daemon.creates, daemon.starts, daemon.dials, daemon.removes
	daemon.mu.Unlock()
	if creates != 1 || starts != 1 || dials != 2 || removes != 1 {
		t.Fatalf("combined service journey touched unexpected resources: %d/%d/%d/%d", creates, starts, dials, removes)
	}
}

func TestNamespaceCancellationFailureBlocksFurtherAdmissionUntilOwnedRecovery(t *testing.T) {
	daemon, cli, directory := managerFixture(t, "")
	manager := openManagerFixture(t, daemon, cli, directory)
	handle, err := manager.Acquire(context.Background(), daemon.intent.ContainerID, daemon.intent.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	daemon.mu.Lock()
	daemon.fault = "failed-revocation"
	daemon.mu.Unlock()
	route := ServiceRoute{Name: "app.orbwin.localhost", ContainerID: handle.Intent.ContainerID, StartedAt: handle.Intent.StartedAt, HelperID: handle.ID, Port: 8080}
	connection, err := manager.Dial(context.Background(), route)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(connection, "HEAD / HTTP/1.1\r\nHost: app.orbwin.localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(connection)
	if err := connection.Close(); err == nil {
		t.Fatal("unacknowledged revocation became successful cleanup")
	}
	if next, err := manager.Dial(context.Background(), route); err == nil {
		next.Close()
		t.Fatal("uncertain helper admitted another guest dial")
	}
	if err := manager.Renew(context.Background(), handle); err == nil {
		t.Fatal("uncertain helper renewed a healthy state")
	}
	daemon.mu.Lock()
	dials, revoke, renewals, ready := daemon.dials, daemon.revocations, daemon.renewals, daemon.readies
	daemon.mu.Unlock()
	if dials != 1 || revoke != 1 || renewals != 0 || ready != 1 {
		t.Fatalf("uncertain cancellation leaked admission or skipped readiness: %d/%d/%d/%d", dials, revoke, renewals, ready)
	}
	if err := manager.Release(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	if manager.streamFailure(handle.ID) != nil {
		t.Fatal("settled helper retained stale uncertainty")
	}
}
func TestNamespaceLostExecAttachRevokesItsExactNonceWithoutReplayingStart(t *testing.T) {
	daemon, cli, directory := managerFixture(t, "")
	manager := openManagerFixture(t, daemon, cli, directory)
	handle, err := manager.Acquire(context.Background(), daemon.intent.ContainerID, daemon.intent.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	daemon.mu.Lock()
	daemon.fault = "failed-dial-attach"
	daemon.mu.Unlock()
	route := ServiceRoute{ContainerID: handle.Intent.ContainerID, StartedAt: handle.Intent.StartedAt, HelperID: handle.ID, Port: 8080}
	if connection, err := manager.Dial(context.Background(), route); err == nil {
		connection.Close()
		t.Fatal("uncertain attach became successful dial")
	}
	if manager.streamFailure(handle.ID) != nil {
		t.Fatal("acknowledged revocation left false helper uncertainty")
	}
	daemon.mu.Lock()
	dials, revocations := daemon.dials, daemon.revocations
	daemon.mu.Unlock()
	if dials != 1 || revocations != 1 {
		t.Fatal("lost attach was retried or its exact nonce was not revoked")
	}
}

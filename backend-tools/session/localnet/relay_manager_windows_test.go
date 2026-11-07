//go:build windows

package localnet

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

type relayDaemon struct {
	mu                               sync.Mutex
	t                                *testing.T
	ledger                           *NamespaceLedger
	images                           *imageStoreDaemon
	fault                            string
	object                           container.InspectResponse
	creates, starts, removes, renews int
	readies                          int
	exists                           bool
	removeEntered, removeGate        chan struct{}
}

func (daemon *relayDaemon) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1.56/info" || strings.HasPrefix(r.URL.Path, "/v1.56/images/") {
		daemon.images.serve(w, r)
		return
	}
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	encode := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	id := strings.Repeat("f", 64)
	bridge := strings.Repeat("d", 64)
	if strings.HasPrefix(r.URL.Path, "/v1.56/networks/") {
		requested := strings.TrimPrefix(r.URL.Path, "/v1.56/networks/")
		if requested == "bridge" || requested == bridge {
			encode(map[string]any{"Id": bridge, "Name": "bridge", "Scope": "local", "Driver": "bridge"})
			return
		}
		encode(map[string]any{"Id": requested, "Name": "selected", "Scope": "local", "Driver": "bridge"})
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /v1.56/containers/json":
		encode([]any{})
	case "POST /v1.56/containers/create":
		daemon.creates++
		records, err := daemon.ledger.relayRecords()
		if err != nil || len(records) != 1 || records[0].ID != "" {
			daemon.t.Error("create preceded flushed intent", records, err)
		}
		var body struct {
			container.Config
			HostConfig       *container.HostConfig
			NetworkingConfig *network.NetworkingConfig
		}
		if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
			daemon.t.Error(err)
			return
		}
		daemon.object = container.InspectResponse{ID: id, Name: "/" + r.URL.Query().Get("name"), Image: body.Image, Config: &body.Config, HostConfig: body.HostConfig, State: &container.State{}, NetworkSettings: &container.NetworkSettings{Networks: body.NetworkingConfig.EndpointsConfig}}
		daemon.exists = daemon.fault != "uncertain-missing"
		if strings.HasPrefix(daemon.fault, "uncertain") {
			http.Error(w, "acceptance response lost", 500)
			return
		}
		w.WriteHeader(201)
		encode(map[string]string{"Id": id})
	case "POST /v1.56/containers/" + id + "/start":
		daemon.starts++
		records, err := daemon.ledger.relayRecords()
		if err != nil || len(records) != 1 || records[0].ID != id {
			daemon.t.Error("start preceded accepted ID flush", records, err)
		}
		if daemon.fault == "failed-start" {
			http.Error(w, "port conflict", 500)
			return
		}
		daemon.object.State.Running = true
		w.WriteHeader(204)
	case "DELETE /v1.56/containers/" + id:
		if daemon.fault == "blocked-remove" {
			close(daemon.removeEntered)
			select {
			case <-daemon.removeGate:
			case <-r.Context().Done():
				return
			}
		}
		if daemon.fault == "failed-remove" {
			http.Error(w, "cleanup failed", 500)
			return
		}
		daemon.removes++
		daemon.exists = false
		w.WriteHeader(204)
	case "POST /v1.56/containers/" + id + "/exec":
		var command struct{ Cmd []string }
		_ = json.NewDecoder(r.Body).Decode(&command)
		joined := strings.Join(command.Cmd, " ")
		if joined != "/orbwin-services renew "+daemon.object.Config.Labels["dev.orbwin.session"] && !(len(command.Cmd) == 5 && command.Cmd[1] == "ready-relay" && command.Cmd[2] == daemon.object.Config.Labels["dev.orbwin.session"]) {
			daemon.t.Error("foreign renewal command", command)
		}
		if len(command.Cmd) == 5 && command.Cmd[1] == "ready-relay" {
			daemon.readies++
		} else {
			daemon.renews++
		}
		w.WriteHeader(201)
		encode(map[string]string{"Id": "relay-renew"})
	case "POST /v1.56/exec/relay-renew/start":
		var options struct{ Detach, Tty bool }
		if err := json.NewDecoder(r.Body).Decode(&options); err != nil || options.Detach || options.Tty {
			daemon.t.Error("renewal must attach non-TTY", err)
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
		if daemon.fault == "failed-renew" {
			ack = "wrong\n"
		}
		_, _ = connection.Write(execFrame(1, []byte(ack)))
	case "GET /v1.56/exec/relay-renew/json":
		encode(map[string]any{"ContainerID": id, "Running": false, "ExitCode": 0})
	default:
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1.56/containers/") && strings.HasSuffix(r.URL.Path, "/json") {
			if !daemon.exists {
				http.Error(w, "not found", 404)
				return
			}
			if daemon.fault == "foreign-relay" {
				copy := daemon.object
				config := *copy.Config
				config.Labels = map[string]string{"dev.orbwin.services": "foreign"}
				copy.Config = &config
				encode(copy)
				return
			}
			encode(daemon.object)
			return
		}
		daemon.t.Errorf("unexpected relay SDK call %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", 500)
	}
}
func managedRelayFixture(t *testing.T, fault string) (*RelayManager, *relayDaemon, RelayIntent) {
	t.Helper()
	ledger, err := OpenNamespaceLedger(t.TempDir(), "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ledger.Close() })
	binary, asset := testToolsAsset()
	images := &imageStoreDaemon{t: t, ledger: ledger, asset: asset}
	daemon := &relayDaemon{t: t, ledger: ledger, images: images, fault: fault}
	server := httptest.NewServer(http.HandlerFunc(daemon.serve))
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost("tcp://"+server.Listener.Addr().String()), client.WithAPIVersion("1.56"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cli.Close() })
	image, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := OpenRelayManager(context.Background(), cli, ledger, image)
	if err != nil {
		t.Fatal(err)
	}
	manager.portProbe = func(ServicePorts) error { return nil } // Pure contract fixture has no host listeners.
	t.Cleanup(manager.cancel)
	intent, _, _ := relayFixture(t)
	intent.ImageID = image.ID
	intent.AssetSHA = asset
	return manager, daemon, intent
}
func TestManagedRelayFlushesBeforeMutationAndRecoversOnlyExactAcceptance(t *testing.T) {
	manager, daemon, intent := managedRelayFixture(t, "")
	ctx := context.Background()
	id, err := manager.Start(ctx, intent.Gateway, intent.Networks)
	if err != nil {
		t.Fatal(err)
	}
	intent.Networks[0].Aliases[0] = "changed.orbwin.test" // Caller cannot mutate owned intent.
	if err = manager.Renew(ctx); err != nil {
		t.Fatal(err)
	}
	if err = manager.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if records, e := manager.ledger.relayRecords(); e != nil || len(records) != 0 {
		t.Fatal(records, e)
	}
	if id == "" || daemon.creates != 1 || daemon.starts != 1 || daemon.removes != 1 || daemon.renews != 1 {
		t.Fatal("unexpected lifecycle counts", daemon)
	}
	if err = manager.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Start(ctx, intent.Gateway, nil); err == nil {
		t.Fatal("closed manager started relay")
	}
	// Borrowed ledger remains owned for namespace/authority shutdown.
	if _, err = manager.ledger.relayRecords(); err != nil {
		t.Fatal("relay released common owner", err)
	}
}
func TestManagedRelayFailedStartSettlesOnlyItsKnownHelper(t *testing.T) {
	manager, daemon, intent := managedRelayFixture(t, "failed-start")
	if _, err := manager.Start(context.Background(), intent.Gateway, intent.Networks); err == nil {
		t.Fatal("start succeeded")
	}
	if daemon.removes != 1 || daemon.exists {
		t.Fatal("failed start leaked exact helper")
	}
	if records, err := manager.ledger.relayRecords(); err != nil || len(records) != 0 {
		t.Fatal(records, err)
	}
}
func TestManagedRelayUnknownLateAcceptanceIsRecoveredWithoutReplay(t *testing.T) {
	for _, fault := range []string{"uncertain-create", "uncertain-missing"} {
		t.Run(fault, func(t *testing.T) {
			manager, daemon, intent := managedRelayFixture(t, fault)
			ctx := context.Background()
			if _, err := manager.Start(ctx, intent.Gateway, intent.Networks); err == nil {
				t.Fatal("lost reply succeeded")
			}
			if _, err := manager.Start(ctx, intent.Gateway, intent.Networks); err == nil || daemon.creates != 1 {
				t.Fatal("replayed unknown create")
			}
			if fault == "uncertain-missing" {
				if _, err := manager.Recover(ctx); err == nil || daemon.removes != 0 {
					t.Fatal("forgot missing unknown acceptance")
				}
				daemon.mu.Lock()
				daemon.exists = true
				daemon.mu.Unlock()
			}
			if count, err := manager.Recover(ctx); err != nil || count != 1 || daemon.removes != 1 || daemon.creates != 1 {
				t.Fatal(count, err, daemon.removes, daemon.creates)
			}
		})
	}
}
func TestManagedRelayForeignOwnershipAndFailedRemovalRetainReceipts(t *testing.T) {
	for _, fault := range []string{"foreign-relay", "failed-remove"} {
		t.Run(fault, func(t *testing.T) {
			manager, daemon, intent := managedRelayFixture(t, "")
			ctx := context.Background()
			if _, err := manager.Start(ctx, intent.Gateway, intent.Networks); err != nil {
				t.Fatal(err)
			}
			daemon.mu.Lock()
			daemon.fault = fault
			daemon.mu.Unlock()
			if err := manager.Stop(ctx); err == nil {
				t.Fatal("cleanup ignored failure")
			}
			if records, err := manager.ledger.relayRecords(); err != nil || len(records) != 1 {
				t.Fatal(records, err)
			}
			if daemon.removes != 0 {
				t.Fatal("removed foreign or failed helper")
			}
			daemon.mu.Lock()
			daemon.fault = ""
			daemon.mu.Unlock()
			if err := manager.Close(ctx); err != nil || daemon.removes != 1 {
				t.Fatal(err)
			}
		})
	}
}
func TestManagedRelayKnownPortConflictPrecedesIntentAndCreate(t *testing.T) {
	manager, daemon, intent := managedRelayFixture(t, "")
	manager.portProbe = func(ServicePorts) error { return ErrRelayPortUnavailable }
	if _, err := manager.Start(context.Background(), intent.Gateway, intent.Networks); !errors.Is(err, ErrRelayPortUnavailable) {
		t.Fatal(err)
	}
	if records, err := manager.ledger.relayRecords(); err != nil || len(records) != 0 || daemon.creates != 0 {
		t.Fatal("known conflict mutated daemon or journal", records, err)
	}
}

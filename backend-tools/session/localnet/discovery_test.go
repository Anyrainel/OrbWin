package localnet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func serviceSummary(index int, name string, ports ...uint16) container.Summary {
	item := container.Summary{ID: fmt.Sprintf("%064x", index), Names: []string{"/" + name}, State: "running", Labels: make(map[string]string)}
	item.HostConfig.NetworkMode = "bridge"
	for _, port := range ports {
		item.Ports = append(item.Ports, container.PortSummary{PrivatePort: port, Type: "tcp"})
	}
	return item
}
func composeSummary(index int, project, service, replica string) container.Summary {
	item := serviceSummary(index, project+"-"+service+"-"+replica, 8080)
	item.Labels = map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": service, "com.docker.compose.container-number": replica}
	return item
}
func containsName(candidate ServiceCandidate, name string) bool {
	for _, candidateName := range candidate.Names {
		if candidateName == name {
			return true
		}
	}
	return false
}

func TestServiceDiscoveryProposesHTTPPortsWithoutClaimingPublishedRoutes(t *testing.T) {
	one := serviceSummary(1, "app", 8080, 8080)
	one.Ports = append(one.Ports, container.PortSummary{PrivatePort: 53, PublicPort: 5353, Type: "udp"})
	multiple := serviceSummary(2, "multiple", 8000, 9000)
	none := serviceSummary(3, "none")
	stopped := serviceSummary(4, "stopped", 80)
	stopped.State = "exited"
	host := serviceSummary(5, "host", 80)
	host.HostConfig.NetworkMode = "host"
	shared := serviceSummary(6, "shared", 80)
	shared.HostConfig.NetworkMode = "container:" + one.ID
	paused := serviceSummary(7, "paused", 80)
	paused.State = "paused"
	restarting := serviceSummary(8, "restarting", 80)
	restarting.State = "restarting"
	items, err := BuildCandidates([]container.Summary{shared, stopped, none, multiple, one, host, paused, restarting}, nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]ServiceCandidate)
	for _, item := range items {
		byName[item.ContainerName] = item
	}
	if item := byName["app"]; item.State != "probe-required" || item.SelectedPort != 8080 || len(item.Ports) != 1 || !containsName(item, "app.orbwin.localhost") || !containsName(item, "app.orbwin.test") {
		t.Fatalf("single TCP proposal is wrong: %+v", item)
	}
	if byName["multiple"].State != "needs-port" || byName["none"].State != "needs-port" || byName["stopped"].State != "stopped" || byName["host"].State != "unsupported-network" || byName["shared"].State != "unsupported-network" {
		t.Fatalf("unsupported/ambiguous service was inferred: %+v", items)
	}
	if !reflect.DeepEqual(byName["multiple"].Ports, []uint16{8000, 9000}) {
		t.Fatal("port order is unstable")
	}
	if byName["paused"].State != "paused" || byName["restarting"].State != "restarting" {
		t.Fatal("paused/restarting workloads became stopped or routable")
	}
}
func TestComposeDiscoveryKeepsRecreationIdentityAndNeverInventsReplicaSelection(t *testing.T) {
	first := composeSummary(1, "project", "web", "1")
	old := first
	old.ID = fmt.Sprintf("%064x", 2)
	old.Names = []string{"/old"}
	old.State = "exited"
	items, err := BuildCandidates([]container.Summary{first, old}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ContainerID == first.ID && (!containsName(item, "web.project.orbwin.localhost") || !containsName(item, "web-1.project.orbwin.localhost") || item.State != "probe-required") {
			t.Fatalf("stopped former replica blocked running replacement: %+v", item)
		}
	}
	second := composeSummary(3, "project", "web", "2")
	items, err = BuildCandidates([]container.Summary{first, second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if containsName(item, "web.project.orbwin.localhost") || item.State != "probe-required" {
			t.Fatalf("ambiguous service alias was published: %+v", item)
		}
	}
	preference := ServicePreference{Selector: ServiceSelector{Project: "project", Service: "web", Replica: 1}, Name: "chosen.orbwin.localhost", Port: 3000}
	first.ID = fmt.Sprintf("%064x", 4)
	first.Names = []string{"/externally-recreated"}
	items, err = BuildCandidates([]container.Summary{first}, []ServicePreference{preference})
	if err != nil || len(items) != 1 || items[0].SelectedPort != 3000 || !containsName(items[0], "chosen.orbwin.localhost") || items[0].ContainerID != first.ID {
		t.Fatalf("reviewed coherent Compose choice did not survive recreation: %+v %v", items, err)
	}
	first.Labels["com.docker.compose.service"] = "another"
	items, _ = BuildCandidates([]container.Summary{first}, []ServicePreference{preference})
	if containsName(items[0], "chosen.orbwin.localhost") {
		t.Fatal("preference guessed membership from a container name")
	}
}
func TestDiscoveryShowsConflictsAndRequiresExplicitRepairForIncompleteLabels(t *testing.T) {
	first, second := serviceSummary(1, "first", 80), serviceSummary(2, "second", 80)
	preferences := []ServicePreference{{Selector: ServiceSelector{ContainerID: first.ID}, Name: "same.orbwin.test"}, {Selector: ServiceSelector{ContainerID: second.ID}, Name: "same.orbwin.localhost"}}
	items, err := BuildCandidates([]container.Summary{first, second}, preferences)
	if err != nil || items[0].State != "name-conflict" || items[1].State != "name-conflict" {
		t.Fatalf("conflicting names silently selected a workload: %+v %v", items, err)
	}
	preferences[1].Disabled = true
	items, _ = BuildCandidates([]container.Summary{first, second}, preferences)
	if items[0].State != "probe-required" || items[1].State != "disabled" {
		t.Fatal("explicit disable did not resolve active collision")
	}
	partial := composeSummary(3, "project", "web", "01")
	items, _ = BuildCandidates([]container.Summary{partial}, nil)
	if items[0].State == "probe-required" || len(items[0].Names) != 0 {
		t.Fatal("malformed Compose number became guessed membership")
	}
	items, _ = BuildCandidates([]container.Summary{partial}, []ServicePreference{{Selector: ServiceSelector{ContainerID: partial.ID}, Name: "explicit.orbwin.localhost", Port: 8080}})
	if items[0].State != "probe-required" || !containsName(items[0], "explicit.orbwin.test") {
		t.Fatal("explicit container-level adaptation could not repair partial metadata")
	}
	duplicate := composeSummary(4, "project", "web", "1")
	other := duplicate
	other.ID = fmt.Sprintf("%064x", 5)
	other.Names = []string{"/other"}
	items, _ = BuildCandidates([]container.Summary{duplicate, other}, nil)
	if items[0].State != "name-conflict" || items[1].State != "name-conflict" {
		t.Fatal("duplicate running replica identities were guessed")
	}
}
func TestDiscoveryExcludesHelpersOneOffsAndRejectsInvalidPreferences(t *testing.T) {
	helper := serviceSummary(1, "helper", 80)
	helper.Labels["dev.orbwin.services"] = "relay"
	debug := serviceSummary(2, "debug", 80)
	debug.Labels["dev.orbwin.transient"] = "debug"
	oneoff := composeSummary(3, "project", "job", "1")
	oneoff.Labels["com.docker.compose.oneoff"] = "True"
	items, err := BuildCandidates([]container.Summary{helper, debug, oneoff}, nil)
	if err != nil || len(items) != 0 {
		t.Fatal("tool/one-off helper was proposed as an application route")
	}
	item := serviceSummary(4, "app", 80)
	valid := ServicePreference{Selector: ServiceSelector{ContainerID: item.ID}, Name: "app.orbwin.localhost"}
	for _, preferences := range [][]ServicePreference{
		{valid, valid},
		{{Selector: ServiceSelector{ContainerID: item.ID, Project: "project"}}},
		{{Selector: ServiceSelector{Project: "project", Service: "web"}}},
		{{Selector: ServiceSelector{ContainerID: item.ID}, Name: "evil.example"}},
		{{Selector: ServiceSelector{ContainerID: item.ID}, Name: "APP.ORBWIN.LOCALHOST"}},
	} {
		if _, err := BuildCandidates([]container.Summary{item}, preferences); err == nil {
			t.Fatal("invalid preferences reached discovery")
		}
	}
	if _, err := BuildCandidates([]container.Summary{item, item}, nil); err == nil {
		t.Fatal("duplicate full IDs were accepted")
	}
}
func TestServiceDNSNormalizationKeepsDistinctionsAndStaysBounded(t *testing.T) {
	sources := []string{"app", "App", "app_", "app-", strings.Repeat("x", 64), strings.Repeat("x", 63) + "y", "你好", "---"}
	seen := make(map[string]bool)
	for _, source := range sources {
		label := serviceDNSLabel(source)
		if !plainDNSLabel.MatchString(label) || len(label) > 63 || seen[label] || serviceDNSLabel(source) != label {
			t.Fatalf("unstable/colliding/invalid normalized label: %q => %q", source, label)
		}
		seen[label] = true
	}
}

func TestServiceNamesRefuseUnicodeBeforeCaseNormalization(t *testing.T) {
	for _, name := range []string{"K.orbwin.localhost", "app.orbwİn.localhost", "你好.orbwin.test"} {
		if _, err := CanonicalServiceName(name); err == nil {
			t.Fatalf("non-ASCII source was converted into an accepted service name: %q", name)
		}
	}
}
func TestServiceInventoryReadsOnlyRegisteredDaemon(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		lists := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Error("discovery mutated the daemon")
				http.Error(w, "mutation", 500)
				return
			}
			switch r.URL.Path {
			case "/v1.56/info":
				id := "owned"
				if foreign {
					id = "other"
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"ID": id, "OSType": "linux"})
			case "/v1.56/containers/json":
				lists++
				if r.URL.Query().Get("all") != "1" && r.URL.Query().Get("all") != "true" {
					t.Error("stopped/replaced inventory was omitted")
				}
				if r.URL.Query().Get("limit") != "513" {
					t.Error("inventory is not bounded")
				}
				_ = json.NewEncoder(w).Encode([]container.Summary{serviceSummary(1, "app", 80)})
			default:
				t.Errorf("unexpected discovery API: %s", r.URL.Path)
				http.NotFound(w, r)
			}
		}))
		cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.56"))
		if err != nil {
			t.Fatal(err)
		}
		items, err := DiscoverServices(context.Background(), cli, "owned", nil)
		cli.Close()
		server.Close()
		if foreign {
			if err == nil || lists != 0 {
				t.Fatal("foreign daemon was inventoried")
			}
		} else if err != nil || lists != 1 || len(items) != 1 {
			t.Fatalf("owned read-only discovery failed: %+v %v", items, err)
		}
	}
}

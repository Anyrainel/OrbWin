package localnet

import (
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"orbwin.local/session/internal/gateway"
)

func relayFixture(t *testing.T) (RelayIntent, container.InspectResponse, string) {
	t.Helper()
	session := strings.Repeat("a", 32)
	identity, err := gateway.NewIdentity(session, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	intent := RelayIntent{Session: session, ImageID: "sha256:" + strings.Repeat("b", 64), AssetSHA: strings.Repeat("c", 64), BridgeID: strings.Repeat("d", 64), Gateway: GatewayDescriptor{Session: session, Port: 32123, PublicRoot: identity.PublicRoot(), ExpiresAt: identity.ExpiresAt()}, Networks: []RelayNetwork{{ID: strings.Repeat("e", 64), Aliases: []string{"app.orbwin.test"}}}}
	options, err := RelayConfiguration(intent)
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("f", 64)
	object := container.InspectResponse{ID: id, Name: "/" + intent.name(), Image: intent.ImageID, HostConfig: options.HostConfig, State: &container.State{Running: true}, Config: options.Config, NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{"bridge": {NetworkID: intent.BridgeID, GwPriority: 1}, "selected": {NetworkID: intent.Networks[0].ID, Aliases: []string{"app.orbwin.test"}}}}}
	return intent, object, id
}
func TestRelayOwnershipRefusesForeignIdentityPublicationsAndIsolation(t *testing.T) {
	changes := map[string]func(*container.InspectResponse){
		"ID":            func(o *container.InspectResponse) { o.ID = strings.Repeat("1", 64) },
		"name":          func(o *container.InspectResponse) { o.Name = "/other" },
		"image":         func(o *container.InspectResponse) { o.Image = "sha256:" + strings.Repeat("1", 64) },
		"owner":         func(o *container.InspectResponse) { o.Config.Labels["dev.orbwin.session"] = "other" },
		"command":       func(o *container.InspectResponse) { o.Config.Cmd[1] = "netns" },
		"root user":     func(o *container.InspectResponse) { o.Config.User = "0" },
		"entrypoint":    func(o *container.InspectResponse) { o.Config.Entrypoint = []string{"/other"} },
		"root writable": func(o *container.InspectResponse) { o.HostConfig.ReadonlyRootfs = false },
		"privileged":    func(o *container.InspectResponse) { o.HostConfig.Privileged = true },
		"capability":    func(o *container.InspectResponse) { o.HostConfig.CapAdd = []string{"NET_ADMIN"} },
		"host PID":      func(o *container.InspectResponse) { o.HostConfig.PidMode = "host" },
		"host IPC":      func(o *container.InspectResponse) { o.HostConfig.IpcMode = "host" },
		"bind":          func(o *container.InspectResponse) { o.HostConfig.Binds = []string{"/:/host"} },
		"port": func(o *container.InspectResponse) {
			p := network.MustParsePort("80/tcp")
			o.HostConfig.PortBindings[p][0].HostPort = "8080"
		},
		"resource": func(o *container.InspectResponse) { o.HostConfig.Memory = 0 },
		"extra network": func(o *container.InspectResponse) {
			o.NetworkSettings.Networks["foreign"] = &network.EndpointSettings{NetworkID: strings.Repeat("1", 64)}
		},
		"wrong network": func(o *container.InspectResponse) {
			o.NetworkSettings.Networks["selected"].NetworkID = strings.Repeat("1", 64)
		},
		"extra alias": func(o *container.InspectResponse) {
			o.NetworkSettings.Networks["selected"].Aliases = append(o.NetworkSettings.Networks["selected"].Aliases, "foreign.test")
		},
		"default gateway": func(o *container.InspectResponse) { o.NetworkSettings.Networks["selected"].GwPriority = 2 },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			intent, object, id := relayFixture(t)
			if err := verifyRelayObject(intent, id, object, true); err != nil {
				t.Fatal(err)
			}
			change(&object)
			if err := verifyRelayObject(intent, id, object, false); err == nil {
				t.Fatal("foreign profile could authorize removal")
			}
		})
	}
	intent, object, id := relayFixture(t)
	object.State.Running = false
	if err := verifyRelayObject(intent, id, object, true); err == nil {
		t.Fatal("stopped relay available")
	}
	if err := verifyRelayObject(intent, id, object, false); err != nil {
		t.Fatal("exact stopped relay cannot be cleaned", err)
	}
}
func TestRelayConfigurationRejectsAliasEscapesAndCopiesReviewedNetworks(t *testing.T) {
	for _, alias := range []string{"public.example", "app.orbwin.localhost", "APP.orbwin.test", "*.orbwin.test", "app.orbwin.test:443"} {
		intent, _, _ := relayFixture(t)
		intent.Networks[0].Aliases = []string{alias}
		if _, err := RelayConfiguration(intent); err == nil {
			t.Fatalf("accepted alias %q", alias)
		}
	}
	intent, _, _ := relayFixture(t)
	options, err := RelayConfiguration(intent)
	if err != nil {
		t.Fatal(err)
	}
	options.NetworkingConfig.EndpointsConfig[intent.Networks[0].ID].Aliases[0] = "changed.orbwin.test"
	if intent.Networks[0].Aliases[0] != "app.orbwin.test" {
		t.Fatal("configuration mutated immutable intent")
	}
	intent.Networks = append(intent.Networks, intent.Networks[0])
	if _, err = RelayConfiguration(intent); err == nil {
		t.Fatal("duplicate network accepted")
	}
}

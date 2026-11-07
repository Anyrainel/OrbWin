package localnet

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"orbwin.local/session/internal/gateway"
	"orbwin.local/session/internal/streamcontrol"
)

type RelayNetwork struct {
	ID      string   `json:"id"`
	Aliases []string `json:"aliases"`
}

// The default bridge is resolved to its exact ID before an intent is committed.
// Additional local bridge networks and aliases are explicitly reviewed. No target
// endpoint is disconnected or modified. All gateway material is public.
type RelayIntent struct {
	Session  string            `json:"session"`
	ImageID  string            `json:"imageId"`
	AssetSHA string            `json:"assetSha"`
	BridgeID string            `json:"bridgeId"`
	Gateway  GatewayDescriptor `json:"gateway"`
	Networks []RelayNetwork    `json:"networks"`
	Ports    ServicePorts      `json:"ports"`
}

func (intent RelayIntent) servicePorts() ServicePorts {
	if intent.Ports == (ServicePorts{}) {
		return DefaultServicePorts()
	}
	return intent.Ports
}

func (intent RelayIntent) validate() error {
	if err := intent.servicePorts().validate(); err != nil {
		return err
	}
	if _, err := streamcontrol.ParseToken(intent.Session); err != nil {
		return err
	}
	if intent.Gateway.Session != intent.Session || intent.Gateway.Port == 0 || !fullID(intent.BridgeID) || !fullID(intent.AssetSHA) || !strings.HasPrefix(intent.ImageID, "sha256:") || !fullID(strings.TrimPrefix(intent.ImageID, "sha256:")) || len(intent.Networks) > 8 {
		return errors.New("exact relay image, gateway, bridge and bounded networks required")
	}
	if _, err := gateway.ClientConfig(intent.Session, intent.Gateway.PublicRoot); err != nil {
		return err
	}
	root, err := x509.ParseCertificate(intent.Gateway.PublicRoot)
	if err != nil || !root.NotAfter.Equal(intent.Gateway.ExpiresAt) {
		return errors.New("relay gateway expiry differs from its exact public certificate")
	}
	last := ""
	total := 0
	for _, selected := range intent.Networks {
		if !fullID(selected.ID) || selected.ID == intent.BridgeID || selected.ID <= last || len(selected.Aliases) == 0 || len(selected.Aliases) > 256 {
			return errors.New("unique sorted selected network IDs and aliases required")
		}
		last = selected.ID
		previous := ""
		for _, alias := range selected.Aliases {
			canonical, err := CanonicalServiceName(alias)
			if err != nil || alias != canonical || !strings.HasSuffix(alias, ".orbwin.test") || alias <= previous {
				return errors.New("unique sorted reserved container-client aliases required")
			}
			previous = alias
		}
		total += len(selected.Aliases)
	}
	if total > 256 {
		return errors.New("relay alias budget exceeded")
	}
	return nil
}
func (intent RelayIntent) name() string { return "orbwin-service-relay-" + intent.Session }
func (intent RelayIntent) labels() map[string]string {
	return map[string]string{"dev.orbwin.services": "relay", "dev.orbwin.session": intent.Session, "dev.orbwin.source-sha256": intent.AssetSHA}
}
func (intent RelayIntent) command() []string {
	return []string{"/orbwin-services", "relay", intent.Session, strconv.Itoa(int(intent.Gateway.Port)), base64.StdEncoding.EncodeToString(intent.Gateway.PublicRoot)}
}
func RelayConfiguration(intent RelayIntent) (client.ContainerCreateOptions, error) {
	if err := intent.validate(); err != nil {
		return client.ContainerCreateOptions{}, err
	}
	p80, p443 := network.MustParsePort("80/tcp"), network.MustParsePort("443/tcp")
	pids := int64(128)
	ports := intent.servicePorts()
	endpoints := map[string]*network.EndpointSettings{intent.BridgeID: {NetworkID: intent.BridgeID, GwPriority: 1}}
	for _, selected := range intent.Networks {
		endpoints[selected.ID] = &network.EndpointSettings{NetworkID: selected.ID, Aliases: slices.Clone(selected.Aliases)}
	}
	return client.ContainerCreateOptions{Name: intent.name(), Config: &container.Config{Image: intent.ImageID, User: "65532:65532", Cmd: intent.command(), Labels: intent.labels(), WorkingDir: "/", StopTimeout: new(2), ExposedPorts: network.PortSet{p80: {}, p443: {}}}, HostConfig: &container.HostConfig{
		AutoRemove: true, ReadonlyRootfs: true, NetworkMode: container.NetworkMode(intent.BridgeID), CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"},
		Tmpfs: map[string]string{"/run/orbwin": "rw,noexec,nosuid,nodev,size=64k,mode=0700,uid=65532,gid=65532"},
		// rdd forwards Windows loopback to the guest tap address. This owned relay
		// must accept that guest-interface destination; guest-loopback-only binds
		// do not receive it. The qualified Windows publication remains 127.0.0.1.
		PortBindings: network.PortMap{p80: {{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: strconv.Itoa(int(ports.HTTP))}}, p443: {{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: strconv.Itoa(int(ports.HTTPS))}}},
		Resources:    container.Resources{Memory: 64 << 20, PidsLimit: &pids, NanoCPUs: 500_000_000},
	}, NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: endpoints}}, nil
}

// Network names are never sufficient to adopt an endpoint. Default bridge and
// selected networks are rechecked by ID immediately before creation/start.
func VerifyRelayNetworks(ctx context.Context, cli *client.Client, intent RelayIntent) error {
	if err := intent.validate(); err != nil {
		return err
	}
	for index, id := range append([]string{intent.BridgeID}, relayNetworkIDs(intent.Networks)...) {
		result, err := cli.NetworkInspect(ctx, id, client.NetworkInspectOptions{})
		if err != nil {
			return err
		}
		object := result.Network
		if object.ID != id || object.Driver != "bridge" || object.Scope != "local" || object.Ingress || (index == 0 && (object.Name != "bridge" || object.Internal)) || (index > 0 && (object.Name == "bridge" || object.Name == "host" || object.Name == "none")) {
			return errors.New("relay supports only the exact default bridge and selected local user-defined bridge networks")
		}
	}
	return nil
}
func relayNetworkIDs(networks []RelayNetwork) []string {
	ids := make([]string, len(networks))
	for i, selected := range networks {
		ids[i] = selected.ID
	}
	return ids
}

func VerifyRelay(ctx context.Context, cli *client.Client, intent RelayIntent, id string, available bool) error {
	if err := intent.validate(); err != nil {
		return err
	}
	if !fullID(id) {
		return errors.New("exact accepted relay ID required")
	}
	result, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	return verifyRelayObject(intent, id, result.Container, available)
}
func verifyRelayObject(intent RelayIntent, id string, object container.InspectResponse, available bool) error {
	if err := intent.validate(); err != nil {
		return err
	}
	if !fullID(id) || object.ID != id || object.Name != "/"+intent.name() || object.Image != intent.ImageID || object.Config == nil || object.HostConfig == nil || object.State == nil {
		return errors.New("relay identity differs from its committed intent")
	}
	if available && (!object.State.Running || object.State.Paused || object.State.Restarting || object.State.Dead) {
		return errors.New("owned service relay is unavailable")
	}
	config, host := object.Config, object.HostConfig
	for key, value := range intent.labels() {
		if config.Labels[key] != value {
			return errors.New("relay ownership labels differ")
		}
	}
	expected, _ := RelayConfiguration(intent)
	if config.Image != intent.ImageID || config.User != "65532:65532" || !slices.Equal(config.Cmd, intent.command()) || len(config.Entrypoint) != 0 || len(config.Volumes) != 0 || !reflect.DeepEqual(config.ExposedPorts, expected.Config.ExposedPorts) {
		return errors.New("relay command or image configuration differs")
	}
	if host.Privileged || !host.ReadonlyRootfs || !host.AutoRemove || host.NetworkMode != container.NetworkMode(intent.BridgeID) || host.PidMode != "" || (host.IpcMode != "" && host.IpcMode != "private") || host.UTSMode != "" || host.UsernsMode != "" || host.CgroupnsMode == "host" || len(host.CapAdd) != 0 || !slices.Equal(host.CapDrop, []string{"ALL"}) || len(host.Binds) != 0 || len(host.Mounts) != 0 || len(host.Devices) != 0 || len(host.DeviceRequests) != 0 || len(host.VolumesFrom) != 0 || !reflect.DeepEqual(host.PortBindings, expected.HostConfig.PortBindings) || !slices.Equal(host.SecurityOpt, expected.HostConfig.SecurityOpt) || !reflect.DeepEqual(host.Tmpfs, expected.HostConfig.Tmpfs) || host.Memory != 64<<20 || host.NanoCPUs != 500_000_000 || host.PidsLimit == nil || *host.PidsLimit != 128 {
		return errors.New("relay isolation, publications or resource limits differ")
	}
	if object.NetworkSettings == nil || len(object.NetworkSettings.Networks) != len(intent.Networks)+1 {
		return errors.New("relay attached network set differs")
	}
	wanted := map[string][]string{intent.BridgeID: nil}
	for _, selected := range intent.Networks {
		wanted[selected.ID] = selected.Aliases
	}
	for _, endpoint := range object.NetworkSettings.Networks {
		if endpoint == nil {
			return errors.New("missing relay network endpoint")
		}
		aliases, exists := wanted[endpoint.NetworkID]
		if !exists || !slices.Equal(endpoint.Aliases, aliases) {
			return fmt.Errorf("relay network aliases differ for %s", endpoint.NetworkID)
		}
		if (endpoint.NetworkID == intent.BridgeID && endpoint.GwPriority != 1) || (endpoint.NetworkID != intent.BridgeID && endpoint.GwPriority != 0) {
			return errors.New("relay gateway priority differs")
		}
		delete(wanted, endpoint.NetworkID)
	}
	if len(wanted) != 0 {
		return errors.New("relay endpoint identities repeated or absent")
	}
	return nil
}

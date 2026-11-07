package localnet

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// Persist this immutable intent before ContainerCreate, and the returned full ID
// before Start. The long-lived route controller owns that ledger/exclusive lease;
// this module never adopts or deletes resources by a broad label/name query.
type NamespaceIntent struct {
	Session, ImageID, AssetSHA, ContainerID, StartedAt string
	LANLeaseSHA                                        string `json:",omitempty"`
}

func (intent NamespaceIntent) validate() error {
	if intent.LANLeaseSHA != "" && !fullID(intent.LANLeaseSHA) {
		return errors.New("exact native LAN namespace lease purpose required")
	}
	raw, err := hex.DecodeString(intent.Session)
	started, timeErr := time.Parse(time.RFC3339Nano, intent.StartedAt)
	if err != nil || len(raw) != 16 || intent.Session != strings.ToLower(intent.Session) || !strings.HasPrefix(intent.ImageID, "sha256:") || !fullID(strings.TrimPrefix(intent.ImageID, "sha256:")) || !fullID(intent.AssetSHA) || !fullID(intent.ContainerID) || timeErr != nil || started.IsZero() {
		return errors.New("exact service namespace intent required")
	}
	return nil
}
func (intent NamespaceIntent) name() string { return "orbwin-service-netns-" + intent.Session }
func (intent NamespaceIntent) labels() map[string]string {
	labels := map[string]string{"dev.orbwin.services": "netns", "dev.orbwin.session": intent.Session, "dev.orbwin.target": intent.ContainerID, "dev.orbwin.target-started": intent.StartedAt, "dev.orbwin.source-sha256": intent.AssetSHA}
	if intent.LANLeaseSHA != "" {
		labels["dev.orbwin.lan-lease"] = intent.LANLeaseSHA
	}
	return labels
}
func NamespaceConfiguration(intent NamespaceIntent) (client.ContainerCreateOptions, error) {
	if err := intent.validate(); err != nil {
		return client.ContainerCreateOptions{}, err
	}
	pids := int64(128)
	return client.ContainerCreateOptions{
		Name:   intent.name(),
		Config: &container.Config{Image: intent.ImageID, User: "65532:65532", Cmd: []string{"/orbwin-services", "netns", intent.Session}, Labels: intent.labels(), WorkingDir: "/", StopTimeout: new(2)},
		HostConfig: &container.HostConfig{
			AutoRemove: true, ReadonlyRootfs: true, NetworkMode: container.NetworkMode("container:" + intent.ContainerID),
			CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"},
			Tmpfs:     map[string]string{"/run/orbwin": "rw,noexec,nosuid,nodev,size=64k,mode=0700,uid=65532,gid=65532"},
			Resources: container.Resources{Memory: 64 << 20, PidsLimit: &pids, NanoCPUs: 500_000_000},
		},
	}, nil
}

// Both before helper creation and after helper start, compare the target's full
// incarnation. Paused/restarting/exited targets do not receive new connections.
func VerifyServiceTarget(ctx context.Context, cli *client.Client, intent NamespaceIntent) error {
	if err := intent.validate(); err != nil {
		return err
	}
	result, err := cli.ContainerInspect(ctx, intent.ContainerID, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	target := result.Container
	return validateServiceTarget(target, intent.ContainerID, intent.StartedAt)
}
func validateServiceTarget(target container.InspectResponse, id, startedAt string) error {
	if target.ID != id || target.State == nil || !target.State.Running || target.State.Paused || target.State.Restarting || target.State.Dead || target.State.StartedAt != startedAt {
		return errors.New("local service target stopped, paused or changed incarnation; refresh the route")
	}
	if target.Config != nil && (target.Config.Labels["dev.orbwin.services"] != "" || target.Config.Labels["dev.orbwin.transient"] != "") {
		return errors.New("OrbWin helper resources are not application route targets")
	}
	if target.Config == nil || target.HostConfig == nil || target.HostConfig.NetworkMode == "host" || strings.HasPrefix(string(target.HostConfig.NetworkMode), "container:") {
		return errors.New("local service routing requires a private target network namespace; retain published access for host/shared namespace workloads")
	}
	return nil
}

// Verification is read-only. Dialing additionally requires an available helper.
func VerifyNamespace(ctx context.Context, cli *client.Client, intent NamespaceIntent, helperID string) error {
	return verifyNamespace(ctx, cli, intent, helperID, true)
}

// Recovery must also recognize a stopped or start-failed helper. It may remove
// only a full ID returned by a recorded create after this ownership check; no
// force/remove or resource search is exposed here. Target availability is not a
// cleanup prerequisite: deleting a target must not strand its owned helper.
func VerifyNamespaceOwnership(ctx context.Context, cli *client.Client, intent NamespaceIntent, helperID string) error {
	return verifyNamespace(ctx, cli, intent, helperID, false)
}
func verifyNamespace(ctx context.Context, cli *client.Client, intent NamespaceIntent, helperID string, requireAvailable bool) error {
	if err := intent.validate(); err != nil {
		return err
	}
	if !fullID(helperID) || helperID == intent.ContainerID {
		return errors.New("exact owned namespace helper ID required")
	}
	result, err := cli.ContainerInspect(ctx, helperID, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	helper := result.Container
	if helper.ID != helperID || helper.Name != "/"+intent.name() || helper.Config == nil || helper.HostConfig == nil || helper.Image != intent.ImageID || helper.Config.Image != intent.ImageID {
		return errors.New("service namespace helper identity differs from its intent")
	}
	for key, expected := range intent.labels() {
		if helper.Config.Labels[key] != expected {
			return errors.New("service namespace helper ownership differs from its intent")
		}
	}
	if helper.State == nil || (requireAvailable && (!helper.State.Running || helper.State.Paused || helper.State.Restarting || helper.State.Dead)) {
		return errors.New("local service namespace helper is not available")
	}
	configuration := helper.HostConfig
	if configuration.Privileged || !configuration.ReadonlyRootfs || !configuration.AutoRemove || configuration.NetworkMode != container.NetworkMode("container:"+intent.ContainerID) || configuration.PidMode != "" || configuration.IpcMode == "host" || len(configuration.CapAdd) != 0 || len(configuration.CapDrop) != 1 || configuration.CapDrop[0] != "ALL" || len(configuration.Binds) != 0 || len(configuration.Mounts) != 0 || len(configuration.Devices) != 0 || len(configuration.DeviceRequests) != 0 || len(configuration.VolumesFrom) != 0 || len(configuration.PortBindings) != 0 || helper.Config.User != "65532:65532" || len(helper.Config.Volumes) != 0 {
		return errors.New("local service helper isolation differs from the supported profile")
	}
	if strings.Join(helper.Config.Cmd, "\x00") != "/orbwin-services\x00netns\x00"+intent.Session {
		return errors.New("local service helper command differs from the trusted profile")
	}
	if len(helper.Config.Entrypoint) != 0 || len(configuration.SecurityOpt) != 1 || configuration.SecurityOpt[0] != "no-new-privileges:true" || len(configuration.Tmpfs) != 1 || configuration.Tmpfs["/run/orbwin"] != "rw,noexec,nosuid,nodev,size=64k,mode=0700,uid=65532,gid=65532" || (configuration.IpcMode != "" && configuration.IpcMode != "private") || configuration.UTSMode != "" || configuration.UsernsMode != "" || configuration.CgroupnsMode == "host" || configuration.Memory != 64<<20 || configuration.NanoCPUs != 500_000_000 || configuration.PidsLimit == nil || *configuration.PidsLimit != 128 {
		return errors.New("local service helper security/resource profile differs")
	}
	return nil
}

func DialVerifiedNamespace(ctx context.Context, cli *client.Client, intent NamespaceIntent, helperID string, port uint16) (net.Conn, error) {
	return dialVerifiedNamespace(ctx, cli, intent, helperID, port, nil)
}
func dialVerifiedNamespace(ctx context.Context, cli *client.Client, intent NamespaceIntent, helperID string, port uint16, cleanup func(context.Context, string) error) (net.Conn, error) {
	if err := VerifyServiceTarget(ctx, cli, intent); err != nil {
		return nil, err
	}
	if err := VerifyNamespace(ctx, cli, intent, helperID); err != nil {
		return nil, err
	}
	return dialNamespace(ctx, cli, helperID, port, cleanup)
}

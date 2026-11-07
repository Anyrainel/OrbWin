package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"reflect"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

func migrationVolumeFingerprint(value volume.Volume) (string, error) {
	data, err := json.Marshal(struct {
		Name, Driver, Scope, CreatedAt, Mountpoint string
		Options, Labels                            map[string]string
	}{value.Name, value.Driver, value.Scope, value.CreatedAt, value.Mountpoint, value.Options, value.Labels})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
func sourceVolumeSnapshot(ctx context.Context, cli *client.Client, name string) (volume.Volume, string, string, error) {
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return volume.Volume{}, "", "", err
	}
	selected, err := transferVolume(ctx, cli, name)
	if err != nil {
		return selected, "", "", err
	}
	root := info.Info.DockerRootDir
	if selected.Name != name || selected.Scope != "local" || selected.CreatedAt == "" || root == "/" || !path.IsAbs(root) || path.Clean(root) != root || len(root) > 4096 || strings.ContainsAny(root, "\x00\r\n") || selected.Mountpoint != path.Join(root, "volumes", name, "_data") {
		return selected, "", "", errors.New("source volume is outside the supported ordinary local-volume layout")
	}
	fingerprint, err := migrationVolumeFingerprint(selected)
	return selected, root, fingerprint, err
}
func migrationVolumeConfiguration(intent migrationVolumeIntent, image string) (client.ContainerCreateOptions, error) {
	if err := intent.validate(); err != nil {
		return client.ContainerCreateOptions{}, err
	}
	if !imageIDValid(image) {
		return client.ContainerCreateOptions{}, errors.New("acknowledged maintenance image required")
	}
	pids := int64(16)
	return client.ContainerCreateOptions{
		Name: intent.helperName(),
		Config: &container.Config{
			Image: image, User: "0:0", WorkingDir: "/",
			Cmd: []string{"/orbwin-fs", "hold"}, NetworkDisabled: true, StopTimeout: new(2),
			Labels: map[string]string{
				"dev.orbwin.component":        "migration-volume-helper",
				"dev.orbwin.import-token":     intent.Token,
				"dev.orbwin.source-sha256":    intent.AssetSHA,
				"dev.orbwin.migration-source": intent.Source.Daemon,
			},
		},
		HostConfig: &container.HostConfig{
			AutoRemove: true, ReadonlyRootfs: true, NetworkMode: "none", IpcMode: "none",
			CapDrop: []string{"ALL"}, CapAdd: []string{"CAP_DAC_READ_SEARCH"},
			SecurityOpt: []string{"no-new-privileges:true"},
			Mounts: []mount.Mount{{
				Type: mount.TypeBind, Source: intent.Mountpoint, Target: "/source", ReadOnly: true,
				BindOptions: &mount.BindOptions{CreateMountpoint: false},
			}},
			Resources: container.Resources{Memory: 64 << 20, PidsLimit: &pids, NanoCPUs: 250_000_000},
		},
	}, nil
}
func verifyMigrationVolumeHelper(ctx context.Context, cli *client.Client, record migrationVolumeRecord, query string) (string, error) {
	inspected, err := cli.ContainerInspect(ctx, query, client.ContainerInspectOptions{})
	if err != nil {
		return "", err
	}
	object := inspected.Container
	expected, err := migrationVolumeConfiguration(record.migrationVolumeIntent, record.ImageID)
	if err != nil {
		return "", err
	}
	if !fullContainerID(object.ID) || (record.ContainerID != "" && object.ID != record.ContainerID) || object.Name != "/"+expected.Name || object.Config == nil || object.HostConfig == nil || object.Config.Image != record.ImageID || object.Image != record.ImageID || object.Config.User != "0:0" || !reflect.DeepEqual(object.Config.Cmd, expected.Config.Cmd) || len(object.Config.Entrypoint) != 0 || len(object.Config.Volumes) != 0 || !reflect.DeepEqual(object.Config.Labels, expected.Config.Labels) {
		return "", errors.New("source maintenance helper identity/provenance differs; retained")
	}
	host := object.HostConfig
	if host.Privileged || !host.ReadonlyRootfs || !host.AutoRemove || host.NetworkMode != "none" || host.IpcMode != "none" || host.PidMode != "" || len(host.Binds) != 0 || len(host.Mounts) != 1 || host.Mounts[0].Type != mount.TypeBind || host.Mounts[0].Source != record.Mountpoint || host.Mounts[0].Target != "/source" || !host.Mounts[0].ReadOnly || (host.Mounts[0].BindOptions != nil && host.Mounts[0].BindOptions.CreateMountpoint) || !reflect.DeepEqual(host.CapDrop, expected.HostConfig.CapDrop) || !reflect.DeepEqual(host.CapAdd, expected.HostConfig.CapAdd) || !reflect.DeepEqual(host.SecurityOpt, expected.HostConfig.SecurityOpt) || len(host.Devices) != 0 || len(host.DeviceRequests) != 0 || len(host.VolumesFrom) != 0 || len(host.PortBindings) != 0 || host.Memory != 64<<20 || host.NanoCPUs != 250_000_000 || host.PidsLimit == nil || *host.PidsLimit != 16 {
		return "", errors.New("source maintenance helper isolation differs; retained")
	}
	return object.ID, nil
}
func verifyMigrationVolumeImage(ctx context.Context, cli *client.Client, record migrationVolumeRecord) error {
	inspected, err := cli.ImageInspect(ctx, record.ImageID)
	if err != nil {
		return err
	}
	image := inspected.InspectResponse
	if image.ID != record.ImageID || image.Os != "linux" || image.Architecture != "amd64" || image.Config == nil || len(image.Config.Labels) != 3 || image.Config.Labels["dev.orbwin.component"] != "file-maintenance-tools" || image.Config.Labels["dev.orbwin.source-sha256"] != record.AssetSHA || image.Config.Labels["dev.orbwin.import-token"] != record.Token {
		return errors.New("source maintenance image provenance differs; retained")
	}
	for _, tag := range image.RepoTags {
		if tag != "<none>:<none>" {
			return errors.New("maintenance image acquired a tag; retained for explicit review")
		}
	}
	return nil
}

func cleanupMigrationVolume(ctx context.Context, cli *client.Client, scope *migrationVolumeScope, job *migrationVolumeJob) error {
	if job.failed {
		return errors.New("source maintenance receipt persistence is uncertain; no cleanup mutation sent")
	}
	guard := func() error {
		_, err := verifyVolumeMigrationSource(ctx, cli, job.record.Target, job.record.Source)
		return err
	}
	if err := guard(); err != nil {
		return err
	}
	if job.record.Phase == "cleaned" {
		return nil
	}
	if job.record.Phase == "intent" {
		return job.append(migrationVolumeEvent{Phase: "cleaned"})
	}
	if job.record.Phase == "importing" {
		listed, err := cli.ImageList(ctx, client.ImageListOptions{All: true, Filters: client.Filters{"label": {"dev.orbwin.component=file-maintenance-tools": true, "dev.orbwin.source-sha256=" + job.record.AssetSHA: true, "dev.orbwin.import-token=" + job.record.Token: true}}})
		if err != nil {
			return err
		}
		if len(listed.Items) != 1 {
			return errors.New("source tools import is absent or ambiguous; unknown acceptance receipt retained without retry")
		}
		candidate := job.record
		candidate.ImageID = listed.Items[0].ID
		if err := verifyMigrationVolumeImage(ctx, cli, candidate); err != nil {
			return err
		}
		if err := job.append(migrationVolumeEvent{Phase: "image", ID: candidate.ImageID}); err != nil {
			return err
		}
	}
	scope.mu.Lock()
	scope.image = job.record.ImageID
	scope.mu.Unlock()
	if job.record.Phase == "creating" {
		id, err := verifyMigrationVolumeHelper(ctx, cli, job.record, job.record.helperName())
		if err != nil {
			return errors.Join(errors.New("source helper creation remains unconfirmed; retain receipt for later exact recovery"), err)
		}
		if err := job.append(migrationVolumeEvent{Phase: "container", ID: id}); err != nil {
			return err
		}
	}
	if job.record.Phase == "container" || job.record.Phase == "exported" {
		if err := scope.acceptContainer(job.record.ContainerID); err != nil {
			return err
		}
		_, err := verifyMigrationVolumeHelper(ctx, cli, job.record, job.record.ContainerID)
		if !cerrdefs.IsNotFound(err) {
			if err != nil {
				return err
			}
			if err := guard(); err != nil {
				return err
			}
			_, err = cli.ContainerRemove(ctx, job.record.ContainerID, client.ContainerRemoveOptions{Force: true})
			if err != nil && !cerrdefs.IsNotFound(err) {
				return err
			}
		}
		if err := job.append(migrationVolumeEvent{Phase: "helper-removed", ID: job.record.ContainerID}); err != nil {
			return err
		}
	}
	if job.record.Phase == "image" || job.record.Phase == "helper-removed" {
		err := verifyMigrationVolumeImage(ctx, cli, job.record)
		if !cerrdefs.IsNotFound(err) {
			if err != nil {
				return err
			}
			if err := guard(); err != nil {
				return err
			}
			_, err = cli.ImageRemove(ctx, job.record.ImageID, client.ImageRemoveOptions{})
			if err != nil && !cerrdefs.IsNotFound(err) {
				return err
			}
		}
		if err := job.append(migrationVolumeEvent{Phase: "image-removed", ID: job.record.ImageID}); err != nil {
			return err
		}
	}
	if job.record.Phase == "image-removed" {
		return job.append(migrationVolumeEvent{Phase: "cleaned"})
	}
	return errors.New("source maintenance cleanup has no supported continuation")
}

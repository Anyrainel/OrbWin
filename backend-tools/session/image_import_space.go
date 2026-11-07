package main

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// Read-only df on the owned daemon's reported storage root, through already
// pinned tools. The imported image is never executed. Space is shared and not
// reserved: this rejects an observed shortfall, not a promise against disk full.
func imageImportGuestSpace(ctx context.Context, cli *client.Client, index imageArchiveIndex, daemon string) (resultErr error) {
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return err
	}
	if info.Info.ID != daemon || info.Info.OSType != "linux" {
		return errors.New("image-storage preflight endpoint no longer matches the owned Linux daemon")
	}
	root := info.Info.DockerRootDir
	if !strings.HasPrefix(root, "/") || root == "/" || len(root) > 4096 || path.Clean(root) != root || strings.ContainsAny(root, "\x00\r\n\\") {
		return errors.New("owned daemon has an unsupported image-storage root")
	}
	if err := ensureMaintenanceImage(ctx, cli); err != nil {
		return err
	}
	session, err := transferToken()
	if err != nil {
		return err
	}
	intent, err := beginTransient(cli, "files", session, "image-import-storage-space")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, intent.close()) }()
	pids := int64(16)
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: intent.intent.Name, Config: &container.Config{Image: maintenanceImageID, Cmd: []string{"/bin/sleep", "120"}, NetworkDisabled: true, Labels: map[string]string{"dev.orbwin.transient": "files", "dev.orbwin.session": session, "dev.orbwin.target": "image-import-storage-space"}}, HostConfig: &container.HostConfig{NetworkMode: "none", ReadonlyRootfs: true, AutoRemove: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}, Mounts: []mount.Mount{{Type: mount.TypeBind, Source: root, Target: "/source", ReadOnly: true}}, Resources: container.Resources{Memory: 32 << 20, PidsLimit: &pids, NanoCPUs: 250_000_000}}})
	if err != nil {
		if cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) {
			intent.settled = true
		}
		return err
	}
	defer func() {
		cleanup, cancel := bounded()
		defer cancel()
		_, err := cli.ContainerRemove(cleanup, created.ID, client.ContainerRemoveOptions{Force: true})
		if err == nil || cerrdefs.IsNotFound(err) {
			intent.settled = true
			err = nil
		}
		resultErr = errors.Join(resultErr, err)
	}()
	if err := intent.created(created.ID); err != nil {
		return err
	}
	if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return err
	}
	required := index.Bytes + index.ExpandedBytes + (256 << 20)
	if err := guestVolumeSpace(ctx, cli, created.ID, required); err != nil {
		return fmt.Errorf("image storage free-space preflight failed; no selected archive load was sent: %w", err)
	}
	return nil
}

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

type saveRequest struct {
	Volume   string `json:"volume"`
	Path     string `json:"path"`
	Expected string `json:"expected"`
	Text     string `json:"text"`
}

func volumeIdle(ctx context.Context, cli *client.Client, name, except string) error {
	listed, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{"volume": {name: true}}})
	if err != nil {
		return err
	}
	if len(listed.Items) > 512 {
		return errors.New("more than 512 volume references require a narrower maintenance window")
	}
	for _, item := range listed.Items {
		if item.ID == except {
			continue
		}
		inspected, err := cli.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if err != nil {
			return err
		}
		object := inspected.Container
		if object.State == nil {
			return errors.New("a volume user's state is unknown")
		}
		if !object.State.Running && !object.State.Restarting && !object.State.Paused {
			continue
		}
		for _, attachment := range object.Mounts {
			if attachment.Type == mount.TypeVolume && attachment.Name == name && attachment.RW {
				return fmt.Errorf("stop writable volume user %s (%s) before volume maintenance", object.Name, object.ID)
			}
		}
	}
	return nil
}

type boundedBuffer struct {
	bytes.Buffer
	left int
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	if len(data) > b.left {
		return 0, errors.New("file transaction output exceeded its limit")
	}
	b.left -= len(data)
	return b.Buffer.Write(data)
}
func saveVolumeFile(input io.Reader, output io.Writer, daemon string) (resultErr error) {
	decoder := json.NewDecoder(io.LimitReader(input, 2<<20))
	decoder.DisallowUnknownFields()
	var request saveRequest
	if err := decoder.Decode(&request); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("unexpected file edit input")
	}
	if err := resourceFileArguments("volume", request.Volume); err != nil {
		return err
	}
	if err := fileArguments(daemon, strings.Repeat("a", 64), request.Path, "read"); err != nil {
		return err
	}
	hash, err := hex.DecodeString(request.Expected)
	if err != nil || len(hash) != 32 || len(request.Text) > previewBudget || strings.ContainsRune(request.Text, 0) {
		return errors.New("invalid bounded text edit version or content")
	}
	cli, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return err
	}
	if info.Info.ID != daemon || info.Info.OSType != "linux" {
		return errors.New("file edit endpoint does not match the recorded owned Linux daemon")
	}
	inspected, err := cli.VolumeInspect(ctx, request.Volume, client.VolumeInspectOptions{})
	if err != nil {
		return err
	}
	original := inspected.Volume
	if original.Driver != "local" || len(original.Options) != 0 {
		return errors.New("file edits currently support ordinary local named volumes without driver options")
	}
	if err = volumeIdle(ctx, cli, request.Volume, ""); err != nil {
		return err
	}
	if err := ensureMaintenanceImage(ctx, cli); err != nil {
		return err
	}
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		return err
	}
	session := hex.EncodeToString(token)
	intent, err := beginTransient(cli, "edit", session, request.Volume)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, intent.close()) }()
	pids := int64(32)
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: intent.intent.Name,
		Config:     &container.Config{Image: maintenanceImageID, Cmd: []string{"/bin/sleep", "90"}, NetworkDisabled: true, Labels: map[string]string{"dev.orbwin.transient": "edit", "dev.orbwin.session": session, "dev.orbwin.target": request.Volume}},
		HostConfig: &container.HostConfig{NetworkMode: "none", ReadonlyRootfs: false, AutoRemove: true, CapDrop: []string{"ALL"}, CapAdd: []string{"CHOWN", "FOWNER", "DAC_OVERRIDE"}, SecurityOpt: []string{"no-new-privileges:true"}, Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: request.Volume, Target: "/source", VolumeOptions: &mount.VolumeOptions{NoCopy: true}}}, Resources: container.Resources{Memory: 128 << 20, PidsLimit: &pids, NanoCPUs: 500_000_000}},
	})
	if err != nil {
		if cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) {
			intent.settled = true
		}
		return err
	}
	defer func() {
		cleanupCtx, cancel := bounded()
		defer cancel()
		_, err := cli.ContainerRemove(cleanupCtx, created.ID, client.ContainerRemoveOptions{Force: true})
		if err != nil && !cerrdefs.IsNotFound(err) {
			resultErr = errors.Join(resultErr, err)
		} else {
			intent.settled = true
		}
	}()
	if err = intent.created(created.ID); err != nil {
		return err
	}
	if _, err = cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return err
	}
	if err = stageFileHelper(ctx, cli, created.ID); err != nil {
		return err
	}
	after, err := cli.VolumeInspect(ctx, request.Volume, client.VolumeInspectOptions{})
	if err != nil {
		return err
	}
	if !volumeSame(original, after.Volume) {
		return errors.New("volume changed during edit setup; no file transaction was sent")
	}
	if err = volumeIdle(ctx, cli, request.Volume, created.ID); err != nil {
		return err
	}
	operation, err := cli.ExecCreate(ctx, created.ID, client.ExecCreateOptions{Cmd: []string{"/usr/local/bin/orbwin-fs"}, AttachStdin: true, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return err
	}
	attached, err := cli.ExecAttach(ctx, operation.ID, client.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer attached.Close()
	if err = attached.Conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return err
	}
	frame, err := json.Marshal(map[string]string{"path": request.Path, "expected": request.Expected, "text": request.Text})
	if err != nil {
		return err
	}
	if err = writeFull(attached.Conn, append(frame, '\n')); err != nil {
		return err
	}
	if err = attached.CloseWrite(); err != nil {
		return err
	}
	stdout, stderr := &boundedBuffer{left: 32768}, &boundedBuffer{left: 32768}
	if _, err = stdcopy.StdCopy(stdout, stderr, attached.Reader); err != nil {
		return err
	}
	if _, err = output.Write(stdout.Bytes()); err != nil {
		return err
	}
	var completed client.ExecInspectResult
	for {
		completed, err = cli.ExecInspect(ctx, operation.ID, client.ExecInspectOptions{})
		if err != nil {
			return err
		}
		if completed.ContainerID != created.ID {
			return errors.New("file transaction target ownership changed")
		}
		if !completed.Running {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	if completed.ExitCode != 0 {
		return fmt.Errorf("file transaction returned %d: %s", completed.ExitCode, stderr.String())
	}
	return volumeIdle(ctx, cli, request.Volume, created.ID)
}

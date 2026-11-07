package main

// Standalone reads use Moby's image/volume mounts. Selected images never run.
// A short-lived pinned tools container bounds cleanup even if this client dies.
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

func resourceFileArguments(kind, target string) error {
	switch kind {
	case "container":
		return nil // Full container ID validated by files().
	case "image":
		raw := strings.TrimPrefix(target, "sha256:")
		if len(raw) != 64 {
			return errors.New("select a full image digest")
		}
		_, err := hex.DecodeString(raw)
		return err
	case "volume":
		if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`).MatchString(target) {
			return errors.New("select an existing named volume")
		}
		return nil
	default:
		return errors.New("unsupported file source")
	}
}
func volumeSame(a, b volume.Volume) bool {
	return a.Name == b.Name && a.Driver == b.Driver && a.CreatedAt == b.CreatedAt && a.Mountpoint == b.Mountpoint && reflect.DeepEqual(a.Options, b.Options) && reflect.DeepEqual(a.Labels, b.Labels)
}

type fileEditSource struct {
	Volume string `json:"volume,omitempty"`
	Path   string `json:"path,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func containerEditSource(ctx context.Context, cli *client.Client, id, source string) *fileEditSource {
	unavailable := func(reason string) *fileEditSource { return &fileEditSource{Reason: reason} }
	object, err := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return unavailable("Current mount information is unavailable; browse the intended volume directly.")
	}
	if object.Container.ID != id {
		return unavailable("Container identity changed; refresh before editing.")
	}
	selected := path.Clean(source)
	var candidate *fileEditSource
	longest := 0
	for _, attachment := range object.Container.Mounts {
		prefix := strings.TrimSuffix(attachment.Destination, "/")
		if selected != prefix && !strings.HasPrefix(selected, prefix+"/") {
			continue
		}
		if len(prefix) < longest {
			continue
		}
		longest = len(prefix)
		if attachment.Type != mount.TypeVolume {
			candidate = unavailable("This path is a bind or other mount. Edit it through its selected Windows/WSL project source.")
			continue
		}
		if !attachment.RW {
			candidate = unavailable("This container mount is read-only; editing from this view is refused.")
			continue
		}
		candidate = &fileEditSource{Volume: attachment.Name, Path: "/" + strings.TrimPrefix(strings.TrimPrefix(selected, prefix), "/")}
	}
	if candidate == nil {
		return unavailable("Image/writable-layer files are read-only in this view. Copy a file to edit locally, or change the project/Dockerfile and recreate the container.")
	}
	if candidate.Volume == "" {
		return candidate
	}
	components := strings.Split(strings.TrimPrefix(selected, "/"), "/")
	if len(components) > 64 {
		return unavailable("Too many path components to map safely; browse the intended volume directly.")
	}
	current := "/"
	for _, component := range components {
		current = path.Join(current, component)
		stat, err := cli.ContainerStatPath(ctx, id, client.ContainerStatPathOptions{Path: current})
		if err != nil {
			return unavailable("Path ancestry could not be verified; browse the intended volume directly.")
		}
		if stat.Stat.Mode&os.ModeSymlink != 0 || stat.Stat.LinkTarget != "" {
			return unavailable("Link ancestry cannot be mapped safely to a volume; browse the intended volume directly.")
		}
	}
	return candidate
}

func resourceFiles(output io.Writer, daemon, kind, target, source, action string, destinations ...string) (resultErr error) {
	if err := resourceFileArguments(kind, target); err != nil {
		return err
	}
	copying := action == "copy"
	if copying && len(destinations) != 1 {
		return errors.New("copy requires an explicit new Windows destination")
	}
	if kind == "container" && !copying {
		return files(output, daemon, target, source, action)
	}
	if kind == "container" && copying {
		return copyContainerFile(output, daemon, target, source, destinations[0])
	}
	// Reuse path/action validation without treating an image/volume name as a
	// container ID. No untrusted value enters a shell command.
	validationAction := action
	if copying {
		validationAction = "read"
	}
	if err := fileArguments(daemon, strings.Repeat("a", 64), source, validationAction); err != nil {
		return err
	}
	cli, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()
	deadline := 15 * time.Second
	expiry := "30"
	if copying {
		deadline = 600 * time.Second
		expiry = "610"
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return err
	}
	if info.Info.ID != daemon || info.Info.OSType != "linux" {
		return errors.New("file endpoint does not match the recorded owned Linux daemon")
	}
	selected := mount.Mount{Source: target, Target: "/source", ReadOnly: true}
	var before volume.Volume
	if kind == "image" {
		selected.Type = mount.TypeImage
	} else {
		selected.Type = mount.TypeVolume
		selected.VolumeOptions = &mount.VolumeOptions{NoCopy: true}
		inspected, err := cli.VolumeInspect(ctx, target, client.VolumeInspectOptions{})
		if err != nil {
			return err
		}
		before = inspected.Volume
		if before.Driver != "local" || len(before.Options) != 0 {
			return errors.New("standalone file reads currently support ordinary local named volumes without driver options")
		}
	}
	if err := ensureMaintenanceImage(ctx, cli); err != nil {
		return err
	}
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		return err
	}
	session := hex.EncodeToString(token)
	intent, err := beginTransient(cli, "files", session, target)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, intent.close()) }()
	pids := int64(32)
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: "orbwin-files-" + session,
		Config:     &container.Config{Image: maintenanceImageID, Cmd: []string{"/bin/sleep", expiry}, NetworkDisabled: true, Labels: map[string]string{"dev.orbwin.transient": "files", "dev.orbwin.session": session, "dev.orbwin.target": target}},
		HostConfig: &container.HostConfig{NetworkMode: "none", ReadonlyRootfs: true, AutoRemove: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}, Mounts: []mount.Mount{selected}, Resources: container.Resources{Memory: 64 << 20, PidsLimit: &pids, NanoCPUs: 500_000_000}},
	})
	if err != nil {
		if cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) {
			intent.settled = true
		}
		return fmt.Errorf("create bundled read-only tools helper: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := bounded()
		defer cancel()
		_, err := cli.ContainerRemove(cleanupCtx, created.ID, client.ContainerRemoveOptions{Force: true})
		if err != nil && !cerrdefs.IsNotFound(err) {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove exact files helper %s: %w", created.ID, err))
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
	if kind == "volume" {
		after, err := cli.VolumeInspect(ctx, target, client.VolumeInspectOptions{})
		if err != nil {
			return err
		}
		if !volumeSame(before, after.Volume) {
			return errors.New("volume metadata changed during attachment; no data accepted and no source volume deleted")
		}
	}
	current := "/source"
	for _, part := range strings.Split(strings.TrimPrefix(path.Clean(source), "/"), "/") {
		if part == "" {
			continue
		}
		current = path.Join(current, part)
		entry, err := cli.ContainerStatPath(ctx, created.ID, client.ContainerStatPathOptions{Path: current})
		if err != nil {
			return err
		}
		if entry.Stat.Mode&os.ModeSymlink != 0 || entry.Stat.LinkTarget != "" {
			return errors.New("standalone image/volume reads do not traverse symbolic links; inspect the link in its parent directory")
		}
	}
	archive, err := cli.CopyFromContainer(ctx, created.ID, client.CopyFromContainerOptions{SourcePath: current})
	if err != nil {
		return err
	}
	defer archive.Content.Close()
	if copying {
		result, err := copyFileArchive(archive.Content, destinations[0])
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(result)
	}
	result, err := readArchive(archive.Content, action)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(result)
}

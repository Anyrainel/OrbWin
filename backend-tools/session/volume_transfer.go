package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

func transferClient(daemon string) (*client.Client, context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
	cli, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err == nil && (info.Info.ID != daemon || info.Info.OSType != "linux") {
		err = errors.New("volume transfer endpoint does not match the recorded owned Linux daemon")
	}
	if err != nil {
		cli.Close()
		cancel()
		return nil, nil, nil, err
	}
	return cli, ctx, cancel, nil
}
func transferToken() (string, error) {
	var token [16]byte
	_, err := rand.Read(token[:])
	return hex.EncodeToString(token[:]), err
}

// The helper never deletes its mounted volume. Durable helper reconciliation
// covers create/start loss. Restore destinations are intentionally retained on
// every uncertain/failing outcome, with persistent ownership and phase receipts.
func volumeTools(ctx context.Context, cli *client.Client, name string, readonly bool) (id string, cleanup func() error, err error) {
	if err := ensureMaintenanceImage(ctx, cli); err != nil {
		return "", nil, err
	}
	session, err := transferToken()
	if err != nil {
		return "", nil, err
	}
	intent, err := beginTransient(cli, "files", session, name)
	if err != nil {
		return "", nil, err
	}
	cleanup = func() error { return intent.close() }
	pids := int64(32)
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: intent.intent.Name, Config: &container.Config{Image: maintenanceImageID, Cmd: []string{"/bin/sleep", "1830"}, NetworkDisabled: true, Labels: map[string]string{"dev.orbwin.transient": "files", "dev.orbwin.session": session, "dev.orbwin.target": name}}, HostConfig: &container.HostConfig{NetworkMode: "none", ReadonlyRootfs: false, AutoRemove: true, CapDrop: []string{"ALL"}, CapAdd: []string{"DAC_READ_SEARCH"}, SecurityOpt: []string{"no-new-privileges:true"}, Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: name, Target: "/source", ReadOnly: readonly, VolumeOptions: &mount.VolumeOptions{NoCopy: true}}}, Resources: container.Resources{Memory: 128 << 20, PidsLimit: &pids, NanoCPUs: 500_000_000}}})
	if err != nil {
		if cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) {
			intent.settled = true
		}
		return "", cleanup, err
	}
	id = created.ID
	cleanup = func() error {
		cleanupCtx, cancel := bounded()
		defer cancel()
		_, err := cli.ContainerRemove(cleanupCtx, id, client.ContainerRemoveOptions{Force: true})
		if err == nil || cerrdefs.IsNotFound(err) {
			intent.settled = true
			err = nil
		}
		return errors.Join(err, intent.close())
	}
	if err = intent.created(id); err != nil {
		return id, cleanup, err
	}
	_, err = cli.ContainerStart(ctx, id, client.ContainerStartOptions{})
	if err == nil {
		err = stageFileHelper(ctx, cli, id)
	}
	return id, cleanup, err
}
func transferVolume(ctx context.Context, cli *client.Client, name string) (volume.Volume, error) {
	result, err := cli.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return volume.Volume{}, err
	}
	if result.Volume.Driver != "local" || len(result.Volume.Options) != 0 {
		return volume.Volume{}, errors.New("portable transfer supports ordinary local named volumes without driver options")
	}
	return result.Volume, nil
}
func volumeUnchanged(ctx context.Context, cli *client.Client, before volume.Volume, helper string) error {
	after, err := transferVolume(ctx, cli, before.Name)
	if err != nil {
		return err
	}
	if !volumeSame(before, after) {
		return errors.New("volume metadata changed; source and any pending destination were retained")
	}
	return volumeIdle(ctx, cli, before.Name, helper)
}

func isolatedDestination(ctx context.Context, cli *client.Client, before volume.Volume, helper string) error {
	if err := volumeUnchanged(ctx, cli, before, helper); err != nil {
		return err
	}
	listed, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{"volume": {before.Name: true}}})
	if err != nil {
		return err
	}
	for _, item := range listed.Items {
		if item.ID != helper {
			return fmt.Errorf("pending destination was attached by another client (%s); retained for inspection", item.ID)
		}
	}
	return nil
}

type freeSpaceWriter struct {
	file      *os.File
	directory string
	left      int64
}

func (writer *freeSpaceWriter) Write(data []byte) (int, error) {
	if writer.left <= int64(len(data)) {
		available, err := hostFreeSpace(writer.directory)
		if err != nil {
			return 0, err
		}
		if available < uint64(len(data))+(64<<20) {
			return 0, errors.New("insufficient host space; source retained")
		}
		writer.left = 8 << 20
	}
	n, err := writer.file.Write(data)
	writer.left -= int64(n)
	return n, err
}

func exportVolume(output io.Writer, daemon, name, destination string) (resultErr error) {
	if err := resourceFileArguments("volume", name); err != nil {
		return err
	}
	if err := validateVolumeExportDestination(destination); err != nil {
		return err
	}
	cli, ctx, cancel, err := transferClient(daemon)
	if err != nil {
		return err
	}
	defer cli.Close()
	defer cancel()
	before, err := transferVolume(ctx, cli, name)
	if err != nil {
		return err
	}
	if err = volumeIdle(ctx, cli, name, ""); err != nil {
		return err
	}
	id, cleanup, err := volumeTools(ctx, cli, name, true)
	if cleanup != nil {
		defer func() { resultErr = errors.Join(resultErr, cleanup()) }()
	}
	if err != nil {
		return err
	}
	if err = volumeUnchanged(ctx, cli, before, id); err != nil {
		return err
	}
	return exportVolumeContents(ctx, cli, output, daemon, name, destination, id,
		func() error { return volumeUnchanged(ctx, cli, before, id) },
		func() error { return auditVolumeAttributes(ctx, cli, id) })
}

func validateVolumeExportDestination(destination string) error {
	if !filepath.IsAbs(destination) || len(destination) < 3 || len(destination) > 4096 || destination[1] != ':' || strings.ContainsAny(destination[2:], "\x00\r\n:") {
		return errors.New("select a new local Windows package destination")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		if err == nil {
			return errors.New("export never overwrites an existing destination")
		}
		return err
	}
	return nil
}

// The caller owns the client and helper. Publication is shared by owned and
// selected-source volumes; callbacks enforce their distinct identity contracts.
func exportVolumeContents(ctx context.Context, cli *client.Client, output io.Writer, daemon, name, destination, id string, guard, audit func() error) (resultErr error) {
	session, err := transferToken()
	if err != nil {
		return err
	}
	partial := destination + ".orbwin-partial-" + session
	payload, err := os.OpenFile(partial+".payload", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() {
		payload.Close()
		if err := os.Remove(payload.Name()); err != nil && !os.IsNotExist(err) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	fmt.Fprintln(os.Stderr, "Exporting quiesced volume through exclusive staging files:", partial)
	if err = audit(); err != nil {
		return err
	}
	archive, err := cli.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: "/source"})
	if err != nil {
		return err
	}
	manifest, err := collectVolumeArchive(archive.Content, &freeSpaceWriter{file: payload, directory: filepath.Dir(destination)}, daemon, name)
	closeErr := archive.Content.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	manifestHash, err := volumeManifestIdentity(manifest)
	if err != nil {
		return err
	}
	if err = payload.Sync(); err != nil {
		return err
	}
	available, err := hostFreeSpace(filepath.Dir(destination))
	if err != nil {
		return err
	}
	if available < uint64(manifest.Bytes+volumeManifestLimit+65536) {
		return errors.New("insufficient host space for final package; source retained")
	}
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		if err := os.Remove(partial); err != nil && !os.IsNotExist(err) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	if _, err = payload.Seek(0, 0); err != nil {
		return err
	}
	if err = writeVolumePackage(&freeSpaceWriter{file: file, directory: filepath.Dir(destination)}, manifest, payload); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = guard(); err != nil {
		return err
	}
	if err = audit(); err != nil {
		return err
	}
	if err = os.Link(partial, destination); err != nil {
		return fmt.Errorf("publish portable package without replacement: %w", err)
	}
	return json.NewEncoder(output).Encode(map[string]any{"path": destination, "volume": name, "bytes": manifest.Bytes, "sha256": manifest.SHA256, "manifestSha256": manifestHash, "sourceEngine": manifest.Engine, "entries": len(manifest.Entries), "version": 1})
}

// Append-only local receipts are fsynced before Create and before reporting each
// phase. A killed client can leave a pending volume, never a falsely completed
// restore. Labels are authority checks, not a global lock against other clients.
type restoreReceipt struct {
	file  *os.File
	lease *os.File
	path  string
	token string
	name  string
}

func beginRestore(name, daemon string, manifest volumeManifest) (*restoreReceipt, error) {
	manifestHash, err := volumeManifestIdentity(manifest)
	if err != nil {
		return nil, err
	}
	directory, err := transientDirectory()
	if err != nil {
		return nil, err
	}
	directory = filepath.Join(filepath.Dir(directory), "volume-transfers")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	token, err := transferToken()
	if err != nil {
		return nil, err
	}
	filename := filepath.Join(directory, token+".jsonl")
	lease, err := transientLease(filepath.Join(directory, token+".lock"), true)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		lease.Close()
		return nil, err
	}
	receipt := &restoreReceipt{file: file, lease: lease, path: filename, token: token, name: name}
	if err = receipt.append(map[string]any{"version": 1, "phase": "intent", "transaction": token, "volume": name, "daemon": daemon, "sourceSha256": manifest.SHA256, "manifestSha256": manifestHash, "sourceEngine": manifest.Engine, "sourceVolume": manifest.Volume}); err != nil {
		file.Close()
		lease.Close()
		return nil, err
	}
	return receipt, nil
}
func (receipt *restoreReceipt) append(value any) error {
	if err := json.NewEncoder(receipt.file).Encode(value); err != nil {
		return err
	}
	return receipt.file.Sync()
}
func guestVolumeSpace(ctx context.Context, cli *client.Client, id string, required int64) error {
	exec, err := cli.ExecCreate(ctx, id, client.ExecCreateOptions{Cmd: []string{"df", "-Pk", "/source"}, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return err
	}
	response, err := cli.ExecAttach(ctx, exec.ID, client.ExecAttachOptions{})
	if err != nil {
		return err
	}
	defer response.Close()
	if err = response.Conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		return err
	}
	stdout, stderr := &boundedBuffer{left: 32768}, &boundedBuffer{left: 8192}
	if _, err = stdcopy.StdCopy(stdout, stderr, response.Reader); err != nil {
		return err
	}
	inspected, err := cli.ExecInspect(ctx, exec.ID, client.ExecInspectOptions{})
	if err != nil {
		return err
	}
	if inspected.Running || inspected.ExitCode != 0 {
		return fmt.Errorf("guest free-space check failed: %s", stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) < 2 {
		return errors.New("guest free-space result is incomplete")
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 6 {
		return errors.New("guest free-space result is invalid")
	}
	available, err := strconv.ParseUint(fields[len(fields)-3], 10, 64)
	if err != nil {
		return err
	}
	if available < uint64(required/1024)+65536 {
		return errors.New("insufficient guest free space; selected source data retained")
	}
	return nil
}

func volumeTreeDifference(a, b volumeManifest) string {
	// Tar may choose either hard-link path as its data-bearing representative.
	// Compare each inode group's sorted path set and file metadata/content.
	normalize := func(manifest volumeManifest) map[string]string {
		entries := map[string]volumeEntry{}
		groups := map[string][]string{}
		for _, entry := range manifest.Entries {
			entries[entry.Path] = entry
			if entry.Kind == "file" {
				groups[entry.Path] = append(groups[entry.Path], entry.Path)
			}
		}
		for _, entry := range manifest.Entries {
			if entry.Kind == "hardlink" {
				groups[entry.Link] = append(groups[entry.Link], entry.Path)
			}
		}
		result := map[string]string{}
		for _, entry := range manifest.Entries {
			resultKey := entry.Path
			group := ""
			if entry.Kind == "file" || entry.Kind == "hardlink" {
				target := entry.Path
				if entry.Kind == "hardlink" {
					target = entry.Link
				}
				entry = entries[target]
				members := groups[target]
				sort.Strings(members)
				group = strings.Join(members, "\x00")
			}
			entry.Path = ""
			entry.Offset = 0
			data, _ := json.Marshal(entry)
			result[resultKey] = string(data) + group
		}
		return result
	}
	left, right := normalize(a), normalize(b)
	if reflect.DeepEqual(left, right) {
		return ""
	}
	for key, want := range left {
		if got, ok := right[key]; !ok || want != got {
			return fmt.Sprintf("entry %q expected %s; observed %s", key, want, got)
		}
	}
	return "restored volume contains additional entries"
}
func equivalentVolumeTrees(a, b volumeManifest) bool { return volumeTreeDifference(a, b) == "" }

func restoreVolume(output io.Writer, daemon, source, name string) (resultErr error) {
	if err := resourceFileArguments("volume", name); err != nil {
		return err
	}
	packageFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer packageFile.Close()
	stat, err := packageFile.Stat()
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() {
		return errors.New("select an existing portable-volume package file")
	}
	manifest, start, err := readVolumePackage(packageFile, stat.Size())
	if err != nil {
		return err
	}
	cli, ctx, cancel, err := transferClient(daemon)
	if err != nil {
		return err
	}
	defer cli.Close()
	defer cancel()
	if _, err = cli.VolumeInspect(ctx, name, client.VolumeInspectOptions{}); !cerrdefs.IsNotFound(err) {
		if err == nil {
			return errors.New("restore requires a new volume name; existing data is never overwritten")
		}
		return err
	}
	receipt, err := beginRestore(name, daemon, manifest)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, receipt.file.Close(), receipt.lease.Close()) }()
	fmt.Fprintln(os.Stderr, "Restore ownership receipt:", receipt.path, "; interrupted or failed destinations remain isolated and must not be attached to workloads.")
	created, err := cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name, Driver: "local", Labels: map[string]string{"dev.orbwin.transfer": receipt.token, "dev.orbwin.source-sha256": manifest.SHA256}})
	if err != nil {
		return err
	}
	if created.Volume.Labels["dev.orbwin.transfer"] != receipt.token {
		return errors.New("destination name collided with another client's volume; no destination write attempted")
	}
	destinationFingerprint, err := migrationVolumeFingerprint(created.Volume)
	if err != nil {
		return err
	}
	if err = receipt.append(map[string]string{"phase": "created", "volume": name, "destinationFingerprint": destinationFingerprint}); err != nil {
		return err
	}
	before, err := transferVolume(ctx, cli, name)
	if err != nil {
		return err
	}
	if !volumeSame(created.Volume, before) {
		return errors.New("destination metadata changed before attachment")
	}
	if err = isolatedDestination(ctx, cli, before, ""); err != nil {
		return err
	}
	id, cleanup, err := volumeTools(ctx, cli, name, false)
	if cleanup != nil {
		defer func() { resultErr = errors.Join(resultErr, cleanup()) }()
	}
	if err != nil {
		return err
	}
	if err = isolatedDestination(ctx, cli, before, id); err != nil {
		return err
	}
	// Include directory/inode and small-file allocation overhead. This remains
	// a conservative preflight, not a reservation against other engine clients.
	if err = guestVolumeSpace(ctx, cli, id, manifest.Bytes+int64(len(manifest.Entries))*8192); err != nil {
		return err
	}
	if err = receipt.append(map[string]string{"phase": "importing"}); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Importing into isolated pending volume:", name)
	reader, writer := io.Pipe()
	completed := make(chan error, 1)
	go func() {
		err := volumeRestoreArchive(writer, packageFile, start, manifest)
		_ = writer.CloseWithError(err)
		completed <- err
	}()
	_, copyErr := cli.CopyToContainer(ctx, id, client.CopyToContainerOptions{DestinationPath: "/", Content: reader})
	_ = reader.CloseWithError(copyErr)
	encodeErr := <-completed
	if err = errors.Join(copyErr, encodeErr); err != nil {
		return fmt.Errorf("restore incomplete; retained pending volume %s (transaction %s): %w", name, receipt.token, err)
	}
	if err = receipt.append(map[string]string{"phase": "verifying"}); err != nil {
		return err
	}
	if err = auditVolumeAttributes(ctx, cli, id); err != nil {
		return err
	}
	archive, err := cli.CopyFromContainer(ctx, id, client.CopyFromContainerOptions{SourcePath: "/source"})
	if err != nil {
		return err
	}
	actual, err := collectVolumeArchive(archive.Content, io.Discard, daemon, name)
	closed := archive.Content.Close()
	if err != nil {
		return err
	}
	if closed != nil {
		return closed
	}
	if difference := volumeTreeDifference(manifest, actual); difference != "" {
		return fmt.Errorf("restored content/metadata differs; pending volume retained, not accepted: %s", difference)
	}
	if err = isolatedDestination(ctx, cli, before, id); err != nil {
		return err
	}
	if err = receipt.append(map[string]string{"phase": "complete"}); err != nil {
		return err
	}
	manifestHash, err := volumeManifestIdentity(manifest)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{"complete": true, "volume": name, "transaction": receipt.token, "receipt": receipt.path, "sourceSha256": manifest.SHA256, "manifestSha256": manifestHash, "sourceEngine": manifest.Engine, "sourceVolume": manifest.Volume, "entries": len(manifest.Entries), "bytes": manifest.Bytes})
}

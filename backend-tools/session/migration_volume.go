package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/moby/moby/client"
	"orbwin.local/session/localnet"
)

func reviewDesktopMigrationVolume(output io.Writer, target, source, name string) error {
	return reviewMigrationVolume(output, target, migrationSourceIdentity{Kind: "docker-desktop", Daemon: source}, name)
}
func reviewMigrationVolume(output io.Writer, target string, selectedSource migrationSourceIdentity, name string) error {
	source := selectedSource.Daemon
	if source == "" || source == target {
		return errors.New("select the distinct reviewed source daemon")
	}
	if err := resourceFileArguments("volume", name); err != nil {
		return err
	}
	owned, _, cancel, err := transferClient(target)
	if err != nil {
		return err
	}
	defer cancel()
	defer owned.Close()
	scope := &migrationVolumeScope{}
	ctx, cancelSource := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancelSource()
	cli, selectedSource, closeSource, err := openMigrationVolumeSource(ctx, scope, selectedSource)
	if err != nil {
		return err
	}
	defer closeSource()
	identity, err := verifyVolumeMigrationSource(ctx, cli, target, selectedSource)
	if err != nil {
		return err
	}
	selected, _, fingerprint, err := sourceVolumeSnapshot(ctx, cli, name)
	if err != nil {
		return err
	}
	if err := volumeIdle(ctx, cli, name, ""); err != nil {
		return err
	}
	if _, err := verifyVolumeMigrationSource(ctx, cli, target, identity); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{"version": 1, "source": identity, "target": target, "volume": name, "createdAt": selected.CreatedAt, "fingerprint": fingerprint, "quiescentObserved": true, "message": "Coordinate external writers until export finishes. Export uses a temporary networkless read-only source helper and a verified bundled scratch image; it does not stop workloads. Restore into OrbWin is a separate reviewed operation."})
}

func exportDesktopMigrationVolume(output io.Writer, target, source, name, expected, destination string) (resultErr error) {
	return exportMigrationVolume(output, target, migrationSourceIdentity{Kind: "docker-desktop", Daemon: source}, name, expected, destination)
}
func exportMigrationVolume(output io.Writer, target string, selectedSource migrationSourceIdentity, name, expected, destination string) (resultErr error) {
	source := selectedSource.Daemon
	if err := resourceFileArguments("volume", name); err != nil {
		return err
	}
	if !digestValid(expected) || source == "" || source == target {
		return errors.New("review the distinct source and exact volume metadata before export")
	}
	if err := validateVolumeExportDestination(destination); err != nil {
		return err
	}
	available, err := hostFreeSpace(filepath.Dir(destination))
	if err != nil {
		return err
	}
	if available < 128<<20 {
		return errors.New("insufficient host headroom for source volume export")
	}
	binary, err := trustedFileHelper()
	if err != nil {
		return err
	}
	owned, _, cancel, err := transferClient(target)
	if err != nil {
		return err
	}
	defer cancel()
	defer owned.Close()
	scope := &migrationVolumeScope{}
	ctx, cancelSource := context.WithTimeout(context.Background(), 1800*time.Second)
	defer cancelSource()
	cli, selectedSource, closeSource, err := openMigrationVolumeSource(ctx, scope, selectedSource)
	if err != nil {
		return err
	}
	defer closeSource()
	identity, err := verifyVolumeMigrationSource(ctx, cli, target, selectedSource)
	if err != nil {
		return err
	}
	selected, root, fingerprint, err := sourceVolumeSnapshot(ctx, cli, name)
	if err != nil {
		return err
	}
	if fingerprint != expected {
		return errors.New("source volume changed since review; no helper created")
	}
	if err := volumeIdle(ctx, cli, name, ""); err != nil {
		return err
	}
	token, err := transferToken()
	if err != nil {
		return err
	}
	intent := migrationVolumeIntent{Version: 1, Token: token, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: identity, Target: target, Volume: name, Fingerprint: fingerprint, DataRoot: root, Mountpoint: selected.Mountpoint, AssetSHA: os.Getenv("ORBWIN_FS_HELPER_SHA256"), Destination: destination}
	job, err := beginMigrationVolume(intent)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, job.close()) }()
	fmt.Fprintln(os.Stderr, "Source volume maintenance receipt:", job.file.Name())
	scope.token = token
	scope.asset = intent.AssetSHA
	scope.helper = intent.helperName()
	cleanup := func() error {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancelCleanup()
		return cleanupMigrationVolume(cleanupCtx, cli, scope, job)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, cleanup())
		}
	}()
	guard := func() error {
		if _, err := verifyVolumeMigrationSource(ctx, cli, target, identity); err != nil {
			return err
		}
		_, _, actual, err := sourceVolumeSnapshot(ctx, cli, name)
		if err != nil {
			return err
		}
		if actual != fingerprint {
			return errors.New("source volume metadata changed; source and any package retained for review")
		}
		return volumeIdle(ctx, cli, name, "")
	}
	if err := guard(); err != nil {
		return err
	}
	if err := job.append(migrationVolumeEvent{Phase: "importing"}); err != nil {
		return err
	}
	scope.mu.Lock()
	scope.importing = true
	scope.mu.Unlock()
	image, err := localnet.ImportFileToolsImage(ctx, cli, binary, intent.AssetSHA, token)
	scope.mu.Lock()
	scope.importing = false
	scope.mu.Unlock()
	if err != nil {
		return err
	}
	if err := job.append(migrationVolumeEvent{Phase: "image", ID: image}); err != nil {
		return err
	}
	scope.mu.Lock()
	scope.image = image
	scope.mu.Unlock()
	if err := guard(); err != nil {
		return err
	}
	if err := job.append(migrationVolumeEvent{Phase: "creating"}); err != nil {
		return err
	}
	configuration, err := migrationVolumeConfiguration(intent, image)
	if err != nil {
		return err
	}
	scope.mu.Lock()
	scope.creating = true
	scope.mu.Unlock()
	created, err := cli.ContainerCreate(ctx, configuration)
	scope.mu.Lock()
	scope.creating = false
	scope.mu.Unlock()
	if err != nil {
		return err
	}
	if err := job.append(migrationVolumeEvent{Phase: "container", ID: created.ID}); err != nil {
		return err
	}
	if err := scope.acceptContainer(created.ID); err != nil {
		return err
	}
	if _, err := verifyMigrationVolumeHelper(ctx, cli, job.record, created.ID); err != nil {
		return err
	}
	if err := guard(); err != nil {
		return err
	}
	if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return err
	}
	if _, err := verifyMigrationVolumeHelper(ctx, cli, job.record, created.ID); err != nil {
		return err
	}
	if err := guard(); err != nil {
		return err
	}
	var exported bytes.Buffer
	if err := exportVolumeContents(ctx, cli, &exported, source, name, destination, created.ID, guard, func() error { return auditVolumeAttributesScoped(ctx, cli, created.ID, "/orbwin-fs", scope.acceptExec) }); err != nil {
		return err
	}
	if err := job.append(migrationVolumeEvent{Phase: "exported", Result: json.RawMessage(exported.Bytes())}); err != nil {
		return err
	}
	if err := cleanup(); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{"complete": true, "source": identity, "target": target, "export": job.record.Export, "receipt": job.file.Name(), "toolsCleaned": true, "message": "Source volume was retained; the verified package can be reviewed and restored into a fresh OrbWin volume."})
}

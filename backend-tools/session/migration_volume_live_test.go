package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"orbwin.local/session/localnet"
)

func TestDesktopMigrationVolumeNative(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_OWNED") != "1" || os.Getenv("ORBWIN_QUALIFY_MIGRATION_SOURCE") != "docker-desktop" {
		t.Skip("explicit Desktop and owned-engine disposable fixture opt-ins required")
	}
	qualifyMigrationVolumeNative(t, migrationVolumeNativeOptions{kind: "docker-desktop", sourceName: "desktop"})
}

type migrationVolumeNativeOptions struct {
	kind, sourceName string
	wsl              *migrationWSLIdentity
	open             func(context.Context) (*client.Client, func(), error)
}

func qualifyMigrationVolumeNative(t *testing.T, options migrationVolumeNativeOptions) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var identity struct {
		DaemonID      string `json:"daemonId"`
		Endpoint      string `json:"endpoint"`
		Coexistence   bool   `json:"coexistenceQualified"`
		Qualification struct {
			Engines []struct{ Name, DaemonID string } `json:"engines"`
		} `json:"qualification"`
	}
	data, err := os.ReadFile(filepath.Join(root, "artifacts/qualification/engine-identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &identity); err != nil {
		t.Fatal(err)
	}
	if identity.Endpoint != endpoint || !identity.Coexistence || identity.DaemonID == "" {
		t.Fatal("qualified owned engine required")
	}
	sourceID := ""
	for _, engine := range identity.Qualification.Engines {
		if engine.Name == options.sourceName {
			sourceID = engine.DaemonID
		}
	}
	if sourceID == "" || sourceID == identity.DaemonID {
		t.Fatal("distinct qualified source required")
	}
	token, err := transferToken()
	if err != nil {
		t.Fatal(err)
	}
	name := "orbwin-migration-volume-" + token
	folder := filepath.Join(root, "artifacts/qualification", name)
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	transient := filepath.Join(folder, "transient")
	if err := os.Mkdir(transient, 0700); err != nil {
		t.Fatal(err)
	}
	var fsManifest struct {
		SHA string `json:"binarySha256"`
	}
	data, err = os.ReadFile(filepath.Join(root, "artifacts/backend/fs-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fsManifest); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORBWIN_FS_HELPER", filepath.Join(root, "artifacts/backend/orbwin-fs"))
	t.Setenv("ORBWIN_FS_HELPER_SHA256", fsManifest.SHA)
	t.Setenv("ORBWIN_TRANSIENT_DIR", transient)
	proof := map[string]any{"schema": 1, "sourceKind": options.kind, "wsl": options.wsl, "source": sourceID, "target": identity.DaemonID, "fixture": name, "started": time.Now().UTC(), "success": false}
	steps := []map[string]any{}
	save := func() {
		proof["steps"] = steps
		encoded, _ := json.MarshalIndent(proof, "", "  ")
		_ = os.WriteFile(filepath.Join(folder, "proof.json"), encoded, 0600)
	}
	save()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	owned, _, cancelOwned, err := transferClient(identity.DaemonID)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelOwned()
	defer owned.Close()
	var source *client.Client
	var closeSource func()
	if options.open != nil {
		source, closeSource, err = options.open(ctx)
	} else {
		source, err = client.New(client.WithHost(desktopMigrationEndpoint), client.WithAPIVersionNegotiation())
		closeSource = func() { source.Close() }
	}
	if err != nil {
		t.Fatal(err)
	}
	defer closeSource()
	verifySource := func(ctx context.Context) (migrationSourceIdentity, error) {
		return verifyVolumeMigrationSource(ctx, source, identity.DaemonID, migrationSourceIdentity{Kind: options.kind, Daemon: sourceID, WSL: options.wsl})
	}
	migrationArgs := func(operation string, rest ...string) []string {
		args := []string{"migration-volume-" + operation, "docker-desktop", identity.DaemonID, sourceID, name}
		if options.wsl != nil {
			args = []string{"migration-rancher-volume-" + operation, identity.DaemonID, sourceID, options.wsl.Registration, options.wsl.Machine, name}
		}
		return append(args, rest...)
	}
	if _, err := verifySource(ctx); err != nil {
		t.Fatal(err)
	}
	var sourceVolumeCreated, targetVolumeCreated bool
	var fixtureImage, seeder string
	destination := name + "-restored"
	cleanupErrors := []string{}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cleanupCancel()
		if _, err := verifySource(cleanupCtx); err != nil {
			cleanupErrors = append(cleanupErrors, err.Error())
		} else {
			if seeder != "" {
				object, err := source.ContainerInspect(cleanupCtx, seeder, client.ContainerInspectOptions{})
				if err == nil {
					if object.Container.Config == nil || object.Container.Config.Labels["dev.orbwin.qualification"] != token {
						err = fmt.Errorf("fixture seeder ownership changed; retained")
					} else {
						_, err = source.ContainerRemove(cleanupCtx, seeder, client.ContainerRemoveOptions{Force: true})
					}
				}
				if err != nil && !cerrdefs.IsNotFound(err) {
					cleanupErrors = append(cleanupErrors, err.Error())
				}
			}
			if sourceVolumeCreated {
				volume, err := source.VolumeInspect(cleanupCtx, name, client.VolumeInspectOptions{})
				if err == nil && volume.Volume.Labels["dev.orbwin.qualification"] == token {
					_, err = source.VolumeRemove(cleanupCtx, name, client.VolumeRemoveOptions{})
				} else if err == nil {
					err = fmt.Errorf("source fixture volume ownership changed; retained")
				}
				if err != nil {
					cleanupErrors = append(cleanupErrors, err.Error())
				}
			}
			if fixtureImage != "" {
				image, err := source.ImageInspect(cleanupCtx, fixtureImage)
				if err == nil && image.Config != nil && image.Config.Labels["dev.orbwin.import-token"] == token {
					_, err = source.ImageRemove(cleanupCtx, fixtureImage, client.ImageRemoveOptions{})
				} else if err == nil {
					err = fmt.Errorf("fixture image ownership changed; retained")
				}
				if err != nil {
					cleanupErrors = append(cleanupErrors, err.Error())
				}
			}
		}
		if targetVolumeCreated {
			volume, err := owned.VolumeInspect(cleanupCtx, destination, client.VolumeInspectOptions{})
			if err == nil && volume.Volume.Labels["dev.orbwin.source-sha256"] == proof["payloadSha256"] {
				_, err = owned.VolumeRemove(cleanupCtx, destination, client.VolumeRemoveOptions{})
			} else if err == nil {
				err = fmt.Errorf("target fixture volume ownership changed; retained")
			}
			if err != nil {
				cleanupErrors = append(cleanupErrors, err.Error())
			}
		}
		proof["cleanupErrors"] = cleanupErrors
		proof["finished"] = time.Now().UTC()
		proof["success"] = !t.Failed() && len(cleanupErrors) == 0
		save()
		if len(cleanupErrors) > 0 {
			t.Errorf("cleanup: %v; evidence %s", cleanupErrors, folder)
		}
	}()
	run := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, filepath.Join(root, "artifacts/backend/orbwin-session.exe"), args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		output, err := cmd.Output()
		steps = append(steps, map[string]any{"args": args, "stdout": string(output), "stderr": stderr.String(), "error": fmt.Sprint(err)})
		save()
		return output, err
	}
	pass := func(args ...string) []byte {
		t.Helper()
		output, err := run(args...)
		if err != nil {
			t.Fatalf("%v: %v; evidence %s", args, err, folder)
		}
		return output
	}
	if _, err := source.VolumeInspect(ctx, name, client.VolumeInspectOptions{}); !cerrdefs.IsNotFound(err) {
		t.Fatal("source fixture name not confirmed absent")
	}
	if _, err := owned.VolumeInspect(ctx, destination, client.VolumeInspectOptions{}); !cerrdefs.IsNotFound(err) {
		t.Fatal("target fixture name not confirmed absent")
	}
	created, err := source.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name, Labels: map[string]string{"dev.orbwin.qualification": token}})
	if err != nil {
		t.Fatal(err)
	}
	if created.Volume.Labels["dev.orbwin.qualification"] != token {
		t.Fatal("source fixture collision")
	}
	sourceVolumeCreated = true
	binary, err := trustedFileHelper()
	if err != nil {
		t.Fatal(err)
	}
	proof["fixtureImageIntent"] = map[string]string{"token": token, "asset": fsManifest.SHA}
	save()
	fixtureImage, err = localnet.ImportFileToolsImage(ctx, source, binary, fsManifest.SHA, token)
	if err != nil {
		t.Fatal(err)
	}
	proof["fixtureImage"] = fixtureImage
	save()
	helper, err := source.ContainerCreate(ctx, client.ContainerCreateOptions{Name: name + "-seed", Config: &container.Config{Image: fixtureImage, Cmd: []string{"/orbwin-fs", "hold"}, Labels: map[string]string{"dev.orbwin.qualification": token}}, HostConfig: &container.HostConfig{AutoRemove: true, NetworkMode: "none", ReadonlyRootfs: true, Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: name, Target: "/source", VolumeOptions: &mount.VolumeOptions{NoCopy: true}}}}})
	if err != nil {
		t.Fatal(err)
	}
	seeder = helper.ID
	proof["fixtureSeeder"] = seeder
	save()
	if _, err := source.ContainerStart(ctx, seeder, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	var seed bytes.Buffer
	writer := tar.NewWriter(&seed)
	payload := []byte("selected-source volume payload 例\x00binary\n")
	for _, header := range []*tar.Header{{Name: "data", Typeflag: tar.TypeDir, Mode: 0750, Uid: 1234, Gid: 2345}, {Name: "data/payload 例.bin", Typeflag: tar.TypeReg, Mode: 0640, Uid: 1234, Gid: 2345, Size: int64(len(payload))}, {Name: "data/hard", Typeflag: tar.TypeLink, Linkname: "data/payload 例.bin", Mode: 0640, Uid: 1234, Gid: 2345}, {Name: "data/link", Typeflag: tar.TypeSymlink, Linkname: "payload 例.bin", Mode: 0777, Uid: 1234, Gid: 2345}} {
		header.ModTime = time.Unix(1700000000, 0)
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := writer.Write(payload); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := source.CopyToContainer(ctx, seeder, client.CopyToContainerOptions{DestinationPath: "/source", Content: &seed, CopyUIDGID: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := run(migrationArgs("review")...); err == nil {
		t.Fatal("running writable source user accepted")
	}
	baselineArchive, err := source.CopyFromContainer(ctx, seeder, client.CopyFromContainerOptions{SourcePath: "/source"})
	if err != nil {
		t.Fatal(err)
	}
	var baselineData bytes.Buffer
	baseline, err := collectVolumeArchive(baselineArchive.Content, &baselineData, sourceID, name)
	baselineArchive.Content.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ContainerRemove(ctx, seeder, client.ContainerRemoveOptions{Force: true}); err != nil && !cerrdefs.IsNotFound(err) {
		t.Fatal(err)
	}
	seeder = ""
	var review struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.Unmarshal(pass(migrationArgs("review")...), &review); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(folder, "selected 例.orbvolume")
	if _, err := run(migrationArgs("export", strings.Repeat("0", 64), packagePath)...); err == nil {
		t.Fatal("stale source metadata accepted")
	}
	exportOutput := pass(migrationArgs("export", review.Fingerprint, packagePath)...)
	file, err := os.Open(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := file.Stat()
	manifest, _, err := readVolumePackage(file, info.Size())
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if difference := volumeTreeDifference(baseline, manifest); difference != "" {
		t.Fatal(difference)
	}
	proof["payloadSha256"] = manifest.SHA256
	manifestHash, err := volumeManifestIdentity(manifest)
	if err != nil {
		t.Fatal(err)
	}
	proof["manifestSha256"] = manifestHash
	var exportDocument struct {
		Export struct {
			ManifestSHA256 string `json:"manifestSha256"`
		} `json:"export"`
	}
	if err := json.Unmarshal(exportOutput, &exportDocument); err != nil || exportDocument.Export.ManifestSHA256 != manifestHash {
		t.Fatalf("source export provenance differs: %s %v", exportOutput, err)
	}
	proof["manifestEntries"] = len(manifest.Entries)
	if _, err := run(migrationArgs("export", review.Fingerprint, packagePath)...); err == nil {
		t.Fatal("existing source package overwritten")
	}
	restoreOutput := pass("volume-transfer", "restore", packagePath, destination, identity.DaemonID)
	targetVolumeCreated = true
	var restoreDocument struct{ Transaction, ManifestSHA256, SourceEngine, SourceVolume string }
	if err := json.Unmarshal(restoreOutput, &restoreDocument); err != nil || restoreDocument.ManifestSHA256 != manifestHash || restoreDocument.SourceEngine != sourceID || restoreDocument.SourceVolume != name {
		t.Fatalf("restore provenance differs: %s %v", restoreOutput, err)
	}
	proof["restoreTransaction"] = restoreDocument.Transaction
	historyOutput := pass("migration-volume-history", identity.DaemonID)
	var history struct {
		Records []migrationVolumeHistoryEntry `json:"records"`
	}
	if err := json.Unmarshal(historyOutput, &history); err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, record := range history.Records {
		for _, restored := range record.Restores {
			if record.Volume == name && record.SourceDaemon == sourceID && record.ManifestSHA256 == manifestHash && restored.Transaction == restoreDocument.Transaction && restored.Volume == destination && restored.Phase == "complete" {
				matched = true
			}
		}
	}
	if !matched {
		t.Fatalf("source-to-restore history not linked: %s", historyOutput)
	}
	proof["sourceRestoreReceiptLinked"] = true
	destinationOutput := pass("migration-destination-review", identity.DaemonID, sourceID, name, destination)
	var checked migrationDestinationResult
	if err := json.Unmarshal(destinationOutput, &checked); err != nil || checked.RestoreTransaction != restoreDocument.Transaction || checked.ManifestSHA256 != manifestHash || checked.Destination != destination {
		t.Fatalf("destination ownership review failed: %s %v", destinationOutput, err)
	}
	if _, err := run("migration-destination-review", identity.DaemonID, sourceID, name+"-foreign", destination); err == nil {
		t.Fatal("foreign source volume accepted by destination review")
	}
	proof["destinationReceiptAndMetadataMatched"] = true
	targetPackage := filepath.Join(folder, "target-readback.orbvolume")
	pass("volume-transfer", "export", destination, targetPackage, identity.DaemonID)
	file, err = os.Open(targetPackage)
	if err != nil {
		t.Fatal(err)
	}
	info, _ = file.Stat()
	restored, _, err := readVolumePackage(file, info.Size())
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if difference := volumeTreeDifference(manifest, restored); difference != "" {
		t.Fatal(difference)
	}
	// Kill only our SDK child after its helper ID is durably acknowledged. This
	// exercises real Windows lease release and source cleanup without stopping
	// either engine or any unrelated workload. Never retry export automatically.
	jobsDirectory := filepath.Join(folder, "migration-volume-jobs")
	existingJobs, err := os.ReadDir(jobsDirectory)
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, entry := range existingJobs {
		known[entry.Name()] = true
	}
	lossDestination := filepath.Join(folder, "client-loss.orbvolume")
	lossCommand := exec.CommandContext(ctx, filepath.Join(root, "artifacts/backend/orbwin-session.exe"), migrationArgs("export", review.Fingerprint, lossDestination)...)
	var lossOutput, lossError bytes.Buffer
	lossCommand.Stdout = &lossOutput
	lossCommand.Stderr = &lossError
	if err := lossCommand.Start(); err != nil {
		t.Fatal(err)
	}
	lossToken := ""
	lossDeadline := time.Now().Add(45 * time.Second)
	for lossToken == "" && time.Now().Before(lossDeadline) {
		entries, err := os.ReadDir(jobsDirectory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if known[entry.Name()] || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			file, err := os.Open(filepath.Join(jobsDirectory, entry.Name()))
			if err != nil {
				continue
			}
			record, err := readMigrationVolumeRecord(file)
			file.Close()
			if err == nil && record.Phase == "container" {
				lossToken = record.Token
				break
			}
		}
		if lossToken == "" {
			time.Sleep(5 * time.Millisecond)
		}
	}
	killError := lossCommand.Process.Kill()
	waitError := lossCommand.Wait()
	steps = append(steps, map[string]any{"clientLossToken": lossToken, "killError": fmt.Sprint(killError), "exitError": fmt.Sprint(waitError), "stdout": lossOutput.String(), "stderr": lossError.String()})
	save()
	if lossToken == "" || killError != nil || waitError == nil {
		t.Fatal("could not qualify the exact live SDK child interruption")
	}
	job, err := openMigrationVolumeJob(identity.DaemonID, lossToken)
	if err != nil {
		t.Fatal(err)
	}
	proof["clientLossPhase"] = job.record.Phase
	job.close()
	pass("migration-volume-recover", identity.DaemonID, lossToken)
	job, err = openMigrationVolumeJob(identity.DaemonID, lossToken)
	if err != nil {
		t.Fatal(err)
	}
	if job.record.Phase != "cleaned" {
		job.close()
		t.Fatal("interrupted source tools did not settle")
	}
	job.close()
	proof["clientLossRecovered"] = true
	// A controlled receipt with an unavailable target proves cleanup does not
	// require dialing/restarting the owned daemon. No engine is actually stopped.
	offlineToken, err := transferToken()
	if err != nil {
		t.Fatal(err)
	}
	selected, rootPath, offlineFingerprint, err := sourceVolumeSnapshot(ctx, source, name)
	if err != nil {
		t.Fatal(err)
	}
	sourceIdentity, err := verifySource(ctx)
	if err != nil {
		t.Fatal(err)
	}
	offlineIntent := migrationVolumeIntent{Version: 1, Token: offlineToken, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: sourceIdentity, Target: "controlled-unavailable-target", Volume: name, Fingerprint: offlineFingerprint, DataRoot: rootPath, Mountpoint: selected.Mountpoint, AssetSHA: fsManifest.SHA, Destination: filepath.Join(folder, "never-dispatched.orbvolume")}
	offlineJob, err := beginMigrationVolume(offlineIntent)
	if err != nil {
		t.Fatal(err)
	}
	offlineJob.close()
	pass("migration-volume-recover", offlineIntent.Target, offlineToken)
	proof["unavailableTargetMetadataRecovery"] = true
	_, _, fingerprint, err := sourceVolumeSnapshot(ctx, source, name)
	if err != nil || fingerprint != review.Fingerprint {
		t.Fatalf("source metadata changed: %v", err)
	}
	proof["sourceMetadataPreserved"] = true
	proof["sourceTreeMatchedBaseline"] = true
	proof["restoredTreeMatched"] = true
	save()
}

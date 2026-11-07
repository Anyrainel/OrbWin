//go:build windows

package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

func TestRancherMigrationImageNative(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_OWNED") != "1" || os.Getenv("ORBWIN_QUALIFY_MIGRATION_SOURCE") != "rancher-desktop" {
		t.Skip("explicit owned and Rancher disposable fixture qualification required")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var initial migrationInventory
	data, err := os.ReadFile(filepath.Join(root, "artifacts/qualification/p2-rancher-source-inventory-20261025.json"))
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if err := json.Unmarshal(data, &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Source.Kind != "rancher-desktop" || initial.Source.WSL == nil || initial.Target == initial.Source.Daemon {
		t.Fatal("reviewed distinct Rancher/WSL source required")
	}
	token, err := transferToken()
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(root, "artifacts/qualification", "orbwin-rancher-image-"+token)
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(folder, "docker")
	if err := os.Mkdir(config, 0700); err != nil {
		t.Fatal(err)
	}
	transient := filepath.Join(folder, "transient")
	if err := os.Mkdir(transient, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORBWIN_TRANSIENT_DIR", transient)
	linuxDocker := filepath.Join(root, "artifacts/runtime-tools/linux/docker")
	cliAsset, err := os.ReadFile(linuxDocker)
	if err != nil {
		t.Fatal(err)
	}
	cliHash := sha256.Sum256(cliAsset)
	if hex.EncodeToString(cliHash[:]) != "2b3470ed501ae182223ca6d7e43fd92260eeec5cbfeb502418dc5153d25531f9" {
		t.Fatal("bundled Linux CLI changed")
	}
	t.Setenv("ORBWIN_MIGRATION_DOCKER", linuxDocker)
	t.Setenv("ORBWIN_MIGRATION_DOCKER_SHA256", hex.EncodeToString(cliHash[:]))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	source, session, closeSource, err := newRancherMigrationClient(ctx, initial.Source.WSL.Registration, initial.Source.WSL.Machine)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSource()
	if _, err := verifyRancherMigrationSource(ctx, source, initial.Target, initial.Source.Daemon); err != nil {
		t.Fatal(err)
	}
	owned, _, cancelOwned, err := transferClient(initial.Target)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelOwned()
	defer owned.Close()
	alias := "orbwin-rancher-image-" + token + ":source"
	proof := map[string]any{"schema": 1, "source": initial.Source, "target": initial.Target, "alias": alias, "started": time.Now().UTC(), "success": false}
	sdk := filepath.Join(root, "artifacts/backend/orbwin-session.exe")
	sdkBytes, err := os.ReadFile(sdk)
	if err != nil {
		t.Fatal(err)
	}
	sdkHash := sha256.Sum256(sdkBytes)
	proof["sdkSha256"] = hex.EncodeToString(sdkHash[:])
	steps := []map[string]any{}
	save := func() {
		proof["steps"] = steps
		encoded, _ := json.MarshalIndent(proof, "", "  ")
		_ = os.WriteFile(filepath.Join(folder, "proof.json"), encoded, 0600)
	}
	save()
	linuxPath := func(path string) string {
		t.Helper()
		absolute, err := filepath.Abs(path)
		if err != nil || len(absolute) < 3 || absolute[1] != ':' {
			t.Fatal("fixture requires an observed Windows drive path")
		}
		return "/mnt/" + strings.ToLower(absolute[:1]) + filepath.ToSlash(absolute[2:])
	}
	readCLI := func(args ...string) ([]byte, error) {
		t.Helper()
		command := exec.CommandContext(ctx, session.binary, append([]string{"--distribution-id", "{" + initial.Source.WSL.Registration + "}", "--user", "root", "--exec", "/usr/bin/env", "-i", "PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "HOME=/nonexistent", linuxPath(linuxDocker), "--config", linuxPath(config), "--host", "unix:///mnt/wsl/rancher-desktop/run/docker.sock"}, args...)...)
		output, err := command.CombinedOutput()
		steps = append(steps, map[string]any{"fixtureCLI": args, "output": string(output), "error": fmt.Sprint(err)})
		save()
		return output, err
	}
	refs := map[string]string{}
	sourceImage := ""
	cleanupErrors := []string{}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cleanupCancel()
		for ref, id := range refs {
			actual, err := owned.ImageInspect(cleanupCtx, ref)
			if err == nil && actual.ID == id {
				_, err = owned.ImageRemove(cleanupCtx, ref, client.ImageRemoveOptions{})
			} else if err == nil {
				err = fmt.Errorf("target fixture tag changed; retained")
			}
			if err != nil {
				cleanupErrors = append(cleanupErrors, err.Error())
			}
		}
		if sourceImage != "" {
			actual, err := source.ImageInspect(cleanupCtx, alias)
			if err == nil && actual.ID == sourceImage && actual.Config != nil && actual.Config.Labels["dev.orbwin.qualification"] == token {
				_, err = readCLI("image", "rm", alias)
			} else if err == nil {
				err = fmt.Errorf("source fixture ownership changed; retained")
			}
			if err != nil {
				cleanupErrors = append(cleanupErrors, err.Error())
			}
		}
		closeSource()
		proof["cleanupErrors"] = cleanupErrors
		proof["success"] = !t.Failed() && len(cleanupErrors) == 0
		proof["finished"] = time.Now().UTC()
		save()
		if len(cleanupErrors) > 0 {
			t.Errorf("cleanup %v; evidence %s", cleanupErrors, folder)
		}
	}()
	run := func(args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, sdk, args...)
		var diagnostic bytes.Buffer
		command.Stderr = &diagnostic
		output, err := command.Output()
		steps = append(steps, map[string]any{"sdk": args, "output": string(output), "stderr": diagnostic.String(), "error": fmt.Sprint(err)})
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
	var fixture bytes.Buffer
	writer := tar.NewWriter(&fixture)
	payload := []byte("Rancher disposable image payload 例\n")
	if err := writer.WriteHeader(&tar.Header{Name: "proof.txt", Mode: 0644, Size: int64(len(payload)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	writer.Write(payload)
	writer.Close()
	fixturePath := filepath.Join(folder, "fixture.tar")
	if err := os.WriteFile(fixturePath, fixture.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	proof["sourceFixtureIntent"] = alias
	save()
	if _, err := source.ImageInspect(ctx, alias); !cerrdefs.IsNotFound(err) {
		t.Fatal("source fixture tag not confirmed absent")
	}
	if _, err := owned.ImageInspect(ctx, alias); !cerrdefs.IsNotFound(err) {
		t.Fatal("target fixture tag not confirmed absent")
	}
	if _, err := readCLI("image", "import", "--platform", "linux/amd64", "--change", "LABEL dev.orbwin.qualification="+token, linuxPath(fixturePath), alias); err != nil {
		t.Fatal(err)
	}
	before, err := source.ImageInspect(ctx, alias)
	if err != nil {
		t.Fatal(err)
	}
	if before.Config == nil || before.Config.Labels["dev.orbwin.qualification"] != token {
		t.Fatal("fixture import collision")
	}
	sourceImage = before.ID
	proof["sourceImage"] = sourceImage
	save()
	archive := filepath.Join(folder, "selected 例.tar")
	args := []string{"migration-rancher-image-export", initial.Target, initial.Source.Daemon, initial.Source.WSL.Registration, initial.Source.WSL.Machine, sourceImage, alias, archive}
	wrong := append([]string{}, args...)
	wrong[4] = strings.Repeat("0", 32)
	if _, err := run(wrong...); err == nil {
		t.Fatal("changed WSL machine accepted")
	}
	if _, err := os.Stat(archive); !os.IsNotExist(err) {
		t.Fatal("wrong-machine review published an archive")
	}
	var exported map[string]any
	if err := json.Unmarshal(pass(args...), &exported); err != nil {
		t.Fatal(err)
	}
	proof["export"] = exported
	if _, err := run(args...); err == nil {
		t.Fatal("existing archive overwritten")
	}
	var review imageImportReview
	if err := json.Unmarshal(pass("image-import-review", initial.Target, archive), &review); err != nil {
		t.Fatal(err)
	}
	request := imageImportRequest{Source: archive, SHA256: review.SHA256}
	for _, tag := range review.Tags {
		if tag.CurrentID != "" || len(tag.ConfigIDs) != 1 {
			t.Fatal("fixture target collision")
		}
		request.Tags = append(request.Tags, imageImportChoice{Reference: tag.Reference, ConfigID: tag.ConfigIDs[0], Apply: true})
	}
	input, _ := json.Marshal(request)
	command := exec.CommandContext(ctx, sdk, "import-images", initial.Target)
	command.Stdin = bytes.NewReader(input)
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	output, err := command.Output()
	steps = append(steps, map[string]any{"import": string(output), "stderr": diagnostic.String(), "error": fmt.Sprint(err)})
	save()
	if err != nil {
		t.Fatal(err)
	}
	var imported struct {
		Transaction string             `json:"transaction"`
		Verified    map[string]string  `json:"verified"`
		Tagged      map[string]string  `json:"tagged"`
		Images      []imageImportStage `json:"images"`
	}
	if err := json.Unmarshal(output, &imported); err != nil {
		t.Fatal(err)
	}
	for ref, id := range imported.Tagged {
		refs[ref] = id
	}
	for _, image := range imported.Images {
		refs[image.Reference] = imported.Verified[image.ConfigID]
	}
	var history struct {
		Records []migrationImageHistoryEntry `json:"records"`
	}
	historyBytes := pass("migration-image-history", initial.Target)
	if err := json.Unmarshal(historyBytes, &history); err != nil {
		t.Fatal(err)
	}
	linked := false
	for _, row := range history.Records {
		if row.Transaction == exported["transaction"] && row.ExportAcknowledged && row.SHA256 == exported["sha256"] {
			for _, observed := range row.Imports {
				linked = linked || observed.Transaction == imported.Transaction && observed.Phase == "complete" && observed.VerifiedConfigs == len(imported.Images)
			}
		}
	}
	if !linked {
		t.Fatal("Rancher source-image export did not link to its completed import receipt")
	}
	proof["imageHistory"] = json.RawMessage(historyBytes)
	after, err := owned.ImageInspect(ctx, alias)
	if err != nil {
		t.Fatal(err)
	}
	left, _ := json.Marshal(before.RootFS)
	right, _ := json.Marshal(after.RootFS)
	if !bytes.Equal(left, right) {
		t.Fatal("source/target rootfs differs")
	}
	retained, err := source.ImageInspect(ctx, alias)
	if err != nil || retained.ID != sourceImage {
		t.Fatalf("source fixture changed: %v", err)
	}
	proof["sourcePreserved"] = true
	proof["targetRootfsMatched"] = true
	probeCtx, cancelProbe := context.WithCancel(ctx)
	probe, probeSession, closeProbe, err := newRancherMigrationClient(probeCtx, initial.Source.WSL.Registration, initial.Source.WSL.Machine)
	if err != nil {
		cancelProbe()
		t.Fatal(err)
	}
	if _, err := verifyRancherMigrationSource(probeCtx, probe, initial.Target, initial.Source.Daemon); err != nil {
		cancelProbe()
		closeProbe()
		t.Fatal(err)
	}
	cancelProbe()
	closeProbe()
	for _, connection := range probeSession.connections {
		select {
		case <-connection.done:
		case <-time.After(4 * time.Second):
			t.Fatal("cancelled WSL child was not reaped")
		}
	}
	proof["ownerCancellationReaped"] = true
	save()
}

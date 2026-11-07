//go:build windows

package localnet

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// This is a permanent-process journey, not an installed/trust-store test.
// Only unique qualification fixtures and the owned daemon are mutated.
func TestOwnedRuntimeProcessTrafficAndCrashRecovery(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_OWNED") != "1" {
		t.Skip("explicit owned daemon qualification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	const daemon = "d9c00194-ea9d-439a-8561-6a0ec580fbd5"
	cli, err := client.New(client.WithHost("npipe:////./pipe/docker-rancher-desktop-orbwin"), client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil || info.Info.ID != daemon || info.Info.OSType != "linux" {
		t.Fatal("owned daemon mismatch", err)
	}
	ports := ServicePorts{HTTP: 18080, HTTPS: 18443}
	if err := probeRelayHostPorts(ports); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("orbwin-p2-runtime-%d", time.Now().UnixNano())
	home := filepath.Join(root, "artifacts/qualification", name)
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	installation := fmt.Sprintf("%032x", time.Now().UnixNano())
	selectedNetwork, err := cli.NetworkCreate(ctx, name, client.NetworkCreateOptions{Driver: "bridge", Labels: map[string]string{"dev.orbwin.qualification": name}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, err := cli.NetworkRemove(clean, selectedNetwork.ID, client.NetworkRemoveOptions{}); err != nil {
			t.Error(err)
		}
	})
	fixture, err := os.ReadFile(filepath.Join(root, "artifacts/qualification/network-probe"))
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	if err := writer.WriteHeader(&tar.Header{Name: "probe", Mode: 0755, Size: int64(len(fixture))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(fixture); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	imported, err := cli.ImageImport(ctx, client.ImageImportSource{SourceName: "-", Source: &archive}, "", client.ImageImportOptions{Changes: []string{"LABEL dev.orbwin.qualification=" + name}})
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(io.LimitReader(imported, 65537))
	imageID := ""
	for {
		var message struct{ Status, Error string }
		err := decoder.Decode(&message)
		if err == io.EOF {
			break
		}
		if err != nil || message.Error != "" {
			imported.Close()
			t.Fatal("fixture import", err, message.Error)
		}
		if strings.HasPrefix(message.Status, "sha256:") {
			imageID = message.Status
		}
	}
	imported.Close()
	if !fullID(strings.TrimPrefix(imageID, "sha256:")) {
		t.Fatal("fixture image ID missing")
	}
	t.Cleanup(func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, err := cli.ImageRemove(clean, imageID, client.ImageRemoveOptions{}); err != nil {
			t.Error(err)
		}
	})
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: name, Config: &container.Config{Image: imageID, Cmd: []string{"/probe"}, Labels: map[string]string{"dev.orbwin.qualification": name}}, HostConfig: &container.HostConfig{ReadonlyRootfs: true, CapDrop: []string{"ALL"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, err := cli.ContainerRemove(clean, created.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
			t.Error(err)
		}
	})
	if _, err := cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	target, err := cli.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(root, "artifacts/backend/orbwin-services")
	assetBytes, err := os.ReadFile(asset)
	if err != nil {
		t.Fatal(err)
	}
	assetHash := sha256.Sum256(assetBytes)
	assetSHA := hex.EncodeToString(assetHash[:])
	binary := filepath.Join(root, "artifacts/backend/orbwin-session.exe")
	binaryBytes, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	binaryHash := sha256.Sum256(binaryBytes)
	var process *exec.Cmd
	var diagnostic bytes.Buffer
	callRequest := func(request ServiceRequest) ServiceResponse {
		t.Helper()
		request.Version = 1
		response, err := CallServiceIPC(ctx, home, installation, request)
		if err != nil || response.Error != "" || response.State == nil {
			t.Fatal(request.Operation, err, response.Error)
		}
		return response
	}
	call := func(operation string, revision uint64, settings *ServiceSettings) ServiceResponse {
		t.Helper()
		return callRequest(ServiceRequest{Operation: operation, Revision: revision, Settings: settings})
	}
	stop := func() {
		t.Helper()
		if process == nil {
			return
		}
		clean, done := context.WithTimeout(context.Background(), 45*time.Second)
		defer done()
		_, err := CallServiceIPC(clean, home, installation, ServiceRequest{Version: 1, Operation: "stop-process"})
		if err != nil {
			t.Error("stop owned process", err)
			_ = process.Process.Kill()
		}
		if err := process.Wait(); err != nil {
			t.Error("owned process cleanup", err, diagnostic.String())
		}
		process = nil
	}
	t.Cleanup(stop)
	start := func() RuntimeSnapshot {
		t.Helper()
		diagnostic.Reset()
		process = exec.Command(binary, "local-services-serve", home, installation, daemon, asset, assetSHA)
		process.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		process.Stderr = &diagnostic
		if err := process.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			response, err := CallServiceIPC(ctx, home, installation, ServiceRequest{Version: 1, Operation: "snapshot"})
			if err == nil && response.State != nil {
				if response.State.BinarySHA256 != hex.EncodeToString(binaryHash[:]) || response.State.ToolsSHA256 != assetSHA {
					t.Fatal("component identity differs")
				}
				return *response.State
			}
			if err != nil && !errors.Is(err, ErrServiceNotRunning) {
				t.Fatal(err)
			}
			if time.Now().After(deadline) {
				t.Fatal("process did not publish IPC")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	state := start()
	settings := state.Settings
	settings.Ports = ports
	settings.Networks = []string{selectedNetwork.ID}
	// Existing unrelated selections are explicitly disabled. This test creates no
	// routes/helpers for another workload and never changes its lifecycle.
	candidates, err := DiscoverServices(ctx, cli, daemon, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range candidates {
		settings.Preferences = append(settings.Preferences, ServicePreference{Selector: ServiceSelector{ContainerID: candidate.ContainerID}, Disabled: candidate.ContainerID != created.ID})
	}
	for index := range settings.Preferences {
		if settings.Preferences[index].Selector.ContainerID == created.ID {
			settings.Preferences[index].Name = "runtime-proof.orbwin.localhost"
			settings.Preferences[index].Port = 39081
		}
	}
	configured := call("configure", state.Revision, &settings)
	authorityResponse := call("create-authority", configured.State.Revision, nil)
	authority := authorityResponse.State.Authority
	if authority == nil {
		t.Fatal("reviewed authority missing")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(authority.PublicCertificatePEM) {
		t.Fatal("invalid public root")
	}
	httpClient := &http.Client{Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}, Timeout: 5 * time.Second}
	defer httpClient.CloseIdleConnections()
	settings = authorityResponse.State.Settings
	settings.Enabled = true
	call("configure", authorityResponse.State.Revision, &settings)
	awaitTraffic := func() RuntimeSnapshot {
		t.Helper()
		deadline := time.Now().Add(35 * time.Second)
		var last RuntimeSnapshot
		var trafficError error
		for {
			last = *call("snapshot", 0, nil).State
			if last.Phase == "running" {
				response, err := httpClient.Get(fmt.Sprintf("https://runtime-proof.orbwin.localhost:%d/", ports.HTTPS))
				trafficError = err
				if err == nil {
					data, readErr := io.ReadAll(io.LimitReader(response.Body, 1024))
					response.Body.Close()
					if readErr == nil && response.StatusCode == 200 && string(data) == "orbwin-p2-loopback-1792892200" {
						return last
					}
					trafficError = fmt.Errorf("status %d, body %q: %v", response.StatusCode, data, readErr)
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("traffic unavailable: phase=%s error=%s traffic=%v", last.Phase, last.Error, trafficError)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	running := awaitTraffic()
	captureWindowsPublicationScope(t, ctx, ports, filepath.Join(home, "windows-publication-scope-before-rotation.json"))
	toolsReceipt, err := (&NamespaceLedger{directory: filepath.Join(home, ".rdorbwin/local-services"), daemon: daemon}).readImageReceipt(assetSHA)
	if err != nil {
		t.Fatal(err)
	}
	listHelpers := func() []container.Summary {
		t.Helper()
		listed, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{"ancestor": {toolsReceipt.ID: true}}})
		if err != nil {
			t.Fatal(err)
		}
		return listed.Items
	}
	oldHelpers := listHelpers()
	if len(oldHelpers) != 2 {
		t.Fatal("unexpected active helpers", len(oldHelpers))
	}
	verifyContainerClient := func() {
		t.Helper()
		block, _ := pem.Decode(authority.PublicCertificatePEM)
		if block == nil {
			t.Fatal("invalid public root")
		}
		clientContainer, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: name + "-client", Config: &container.Config{Image: imageID, Cmd: []string{"/probe", "client", "https://runtime-proof.orbwin.test/", "runtime-proof.orbwin.test:443", base64.StdEncoding.EncodeToString(block.Bytes)}, Labels: map[string]string{"dev.orbwin.qualification": name, "dev.orbwin.services": "qualification-client"}}, HostConfig: &container.HostConfig{ReadonlyRootfs: true, CapDrop: []string{"ALL"}, NetworkMode: container.NetworkMode(selectedNetwork.ID)}, NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{selectedNetwork.ID: {NetworkID: selectedNetwork.ID}}}})
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			clean, done := context.WithTimeout(context.Background(), 10*time.Second)
			defer done()
			if _, err := cli.ContainerRemove(clean, clientContainer.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
				t.Error(err)
			}
		}()
		if _, err := cli.ContainerStart(ctx, clientContainer.ID, client.ContainerStartOptions{}); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			inspected, err := cli.ContainerInspect(ctx, clientContainer.ID, client.ContainerInspectOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !inspected.Container.State.Running {
				logs, err := cli.ContainerLogs(ctx, clientContainer.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
				if err != nil {
					t.Fatal(err)
				}
				var stdout, stderr bytes.Buffer
				_, err = stdcopy.StdCopy(&stdout, &stderr, io.LimitReader(logs, 8192))
				logs.Close()
				if err != nil || inspected.Container.State.ExitCode != 0 || stdout.String() != "orbwin-p2-loopback-1792892200" {
					t.Fatal("selected-network client", err, inspected.Container.State.ExitCode, stdout.String(), stderr.String())
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("selected-network client timed out")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	verifyContainerClient()
	// Deliberate loss of this exact disposable process; workload stays running.
	if err := process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = process.Wait()
	process = nil
	httpClient.CloseIdleConnections()
	restarted := start()
	if !restarted.Settings.Enabled || restarted.Settings.RootSHA256 != authority.RootSHA256 {
		t.Fatal("enabled intent/root changed after process loss")
	}
	recovered := awaitTraffic()
	verifyContainerClient()
	for _, helper := range oldHelpers {
		if _, err := cli.ContainerInspect(ctx, helper.ID, client.ContainerInspectOptions{}); !cerrdefs.IsNotFound(err) {
			t.Fatal("old helper retained or inspection failed", helper.ID, err)
		}
	}
	if helpers := listHelpers(); len(helpers) != 2 {
		t.Fatal("recovery duplicated helpers", len(helpers))
	}
	if recovered.Revision != running.Revision {
		t.Fatal("recovery rewrote reviewed intent")
	}
	settings = recovered.Settings
	settings.Enabled = false
	disabled := call("configure", recovered.Revision, &settings)
	awaitDisabled := func() {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for {
			state := call("snapshot", 0, nil).State
			if state.Phase == "disabled" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("disable did not finish", state.Error)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	awaitDisabled()
	oldRoot := authority.RootSHA256
	oldClient := httpClient
	oldPorts := ports
	ports = ServicePorts{HTTP: 28080, HTTPS: 28443}
	if err := probeRelayHostPorts(ports); err != nil {
		t.Fatal("rotation qualification pair unavailable", err)
	}
	rotated := callRequest(ServiceRequest{Operation: "rotate-authority", Revision: disabled.State.Revision, RootSHA256: oldRoot, Ports: &ports})
	if rotated.State.Rotation != nil || rotated.State.Settings.Enabled || rotated.State.Authority == nil || rotated.State.Settings.RootSHA256 == oldRoot || rotated.State.Settings.Ports != ports {
		t.Fatal("rotation did not settle disabled target state", rotated.State)
	}
	authority = rotated.State.Authority
	newPool := x509.NewCertPool()
	if !newPool.AppendCertsFromPEM(authority.PublicCertificatePEM) {
		t.Fatal("rotated public root invalid")
	}
	httpClient = &http.Client{Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: newPool}}, Timeout: 5 * time.Second}
	defer httpClient.CloseIdleConnections()
	settings = rotated.State.Settings
	settings.Enabled = true
	call("configure", rotated.State.Revision, &settings)
	rotatedRunning := awaitTraffic()
	captureWindowsPublicationScope(t, ctx, ports, filepath.Join(home, "windows-publication-scope-after-rotation.json"))
	verifyContainerClient()
	publicRootPath := filepath.Join(home, "rotated-root.pem")
	if err := os.WriteFile(publicRootPath, authority.PublicCertificatePEM, 0600); err != nil {
		t.Fatal(err)
	}
	curlBinary := filepath.Join(os.Getenv("SystemRoot"), "System32", "curl.exe")
	versionCommand := exec.CommandContext(ctx, curlBinary, "-q", "--version")
	versionCommand.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	curlVersion, err := versionCommand.CombinedOutput()
	if err != nil || !bytes.Contains(curlVersion, []byte("Schannel")) {
		t.Fatal("Windows Schannel curl required", err, string(curlVersion))
	}
	url := fmt.Sprintf("https://runtime-proof.orbwin.localhost:%d/", ports.HTTPS)
	verifiedCurl := func(binary string, args ...string) {
		t.Helper()
		command := exec.CommandContext(ctx, binary, args...)
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		output, err := command.CombinedOutput()
		if err != nil || string(output) != "orbwin-p2-loopback-1792892200" {
			t.Fatal("rotated verified client", err, string(output))
		}
	}
	// -q disables user curl configuration; there are no insecure or relaxed
	// revocation flags. Roots apply to these invocations, never a trust store.
	verifiedCurl(curlBinary, "-q", "--fail", "--silent", "--show-error", "--connect-timeout", "3", "--max-time", "10", "--noproxy", "*", "--cacert", publicRootPath, url)
	linuxRootPath := "/mnt/f/Codes/Orbwin/artifacts/qualification/" + name + "/rotated-root.pem"
	verifiedCurl("wsl.exe", "-d", "Ubuntu", "--exec", "curl", "-q", "--fail", "--silent", "--show-error", "--connect-timeout", "3", "--max-time", "10", "--noproxy", "*", "--cacert", linuxRootPath, url)
	rejected, oldClientErr := oldClient.Get(fmt.Sprintf("https://runtime-proof.orbwin.localhost:%d/", ports.HTTPS))
	if rejected != nil {
		rejected.Body.Close()
	}
	var unknownRoot x509.UnknownAuthorityError
	if !errors.As(oldClientErr, &unknownRoot) {
		t.Fatal("old-root-only client did not reject rotated chain", oldClientErr)
	}
	if err := probeRelayHostPorts(oldPorts); err != nil {
		t.Fatal("rotation retained old publications", err)
	}
	settings = rotatedRunning.Settings
	settings.Enabled = false
	disabled = call("configure", rotatedRunning.Revision, &settings)
	awaitDisabled()
	stop()
	if helpers := listHelpers(); len(helpers) != 0 {
		t.Fatal("disable left running/stopped helpers", len(helpers))
	}
	after, err := cli.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
	if err != nil || !after.Container.State.Running || after.Container.State.StartedAt != target.Container.State.StartedAt {
		t.Fatal("target incarnation changed", err)
	}
	ledger, err := OpenNamespaceLedger(filepath.Join(home, ".rdorbwin/local-services"), daemon)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	namespaces, err := ledger.records()
	if err != nil || len(namespaces) != 0 {
		t.Fatal("namespace cleanup incomplete", err, len(namespaces))
	}
	relays, err := ledger.relayRecords()
	if err != nil || len(relays) != 0 {
		t.Fatal("relay cleanup incomplete", err, len(relays))
	}
	if err := probeRelayHostPorts(ports); err != nil {
		t.Fatal("publication remained after disable", err)
	}
	image, err := ledger.readImageReceipt(assetSHA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cli.ImageRemove(ctx, image.ID, client.ImageRemoveOptions{}); err != nil {
		t.Fatal("exact disposable tools image cleanup", err)
	}
	report := map[string]any{"passed": true, "daemonId": daemon, "binarySha256": hex.EncodeToString(binaryHash[:]), "toolsSha256": assetSHA, "target": created.ID, "targetStartedAt": target.Container.State.StartedAt, "rootSha256": authority.RootSHA256, "oldRootSha256": oldRoot, "authorityRotated": true, "rotatedPorts": ports, "rotatedEnabledRevision": rotatedRunning.Revision, "oldRootClientRejectedNewChain": true, "windowsSchannelVerifiedAfterRotation": true, "windowsCurlVersion": string(curlVersion), "ubuntuCurlVerifiedAfterRotation": true, "relaxedTlsOrRevocationFlags": false, "enabledRevision": running.Revision, "disabledRevision": disabled.State.Revision, "processKilledAndRecovered": true, "verifiedHttpsBeforeAndAfter": true, "verifiedContainerAliasBeforeAndAfter": true, "verifiedBothClientsAfterRotation": true, "selectedNetwork": selectedNetwork.ID, "oldHelpersRetired": len(oldHelpers), "remainingHelpers": 0, "rendererRequired": false, "trustStoreChanged": false, "remainingNamespaceReceipts": len(namespaces), "remainingRelayReceipts": len(relays), "targetIncarnationPreserved": true, "completedAt": time.Now().UTC()}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "process-proof.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("enabled process, verified traffic, exact process loss/recovery, disable and target preservation passed; evidence", home)
}

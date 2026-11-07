//go:build windows

package localnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"orbwin.local/session/internal/wsltrust"
)

func qualifiedTrustAsset(t *testing.T, root, name, manifestName string) (string, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "artifacts/backend", manifestName))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		BinarySHA256 string            `json:"binarySha256"`
		Sources      map[string]string `json:"sources"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for name, digest := range manifest.Sources {
		if filepath.IsAbs(name) || strings.Contains(name, "..") {
			t.Fatal("source manifest path differs")
		}
		data, err := os.ReadFile(filepath.Join(root, "backend-tools/session", filepath.FromSlash(name)))
		if err != nil || wsltrust.Digest(data) != digest {
			t.Fatal("stale source asset", name, err)
		}
	}
	path := filepath.Join(root, "artifacts/backend", name)
	data, err = os.ReadFile(path)
	if err != nil || wsltrust.Digest(data) != manifest.BinarySHA256 {
		t.Fatal("stale binary asset", name, err)
	}
	return path, manifest.BinarySHA256
}

// Production SDK executable -> SID-only named pipe -> service actor -> selected
// registration GUID -> root-owned temporary ELF -> Linux read-only trust status.
// No engine objects, OS certificates, registrations or global defaults change.
func TestOwnedServiceProcessSelectedWSLTrustReadOnly(t *testing.T) {
	id := os.Getenv("ORBWIN_QUALIFY_WSL_TRUST_SERVICE")
	if id == "" {
		t.Skip("explicit owned-service and selected-registration read-only qualification")
	}
	if !wsltrust.ValidRegistrationID(id) {
		t.Fatal("canonical selected registration GUID required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	const daemon = "d9c00194-ea9d-439a-8561-6a0ec580fbd5"
	cli, err := client.New(client.WithHost("npipe:////./pipe/docker-rancher-desktop-orbwin"), client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil || info.Info.ID != daemon {
		t.Fatal("owned daemon differs", err)
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	binary, binaryHash := qualifiedTrustAsset(t, root, "orbwin-session.exe", "session-manifest.json")
	services, servicesHash := qualifiedTrustAsset(t, root, "orbwin-services", "services-manifest.json")
	trust, trustHash := qualifiedTrustAsset(t, root, "orbwin-trust", "trust-manifest.json")
	home := filepath.Join(root, "artifacts/qualification", fmt.Sprintf("orbwin-p2-wsl-trust-%d", time.Now().UnixNano()))
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	installation := fmt.Sprintf("%032x", time.Now().UnixNano())
	profile, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	globalHashes := func() map[string]string {
		values := map[string]string{}
		for key, path := range map[string]string{"docker": filepath.Join(profile, ".docker/config.json"), "kube": filepath.Join(profile, ".kube/config")} {
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				values[key] = "absent"
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			values[key] = wsltrust.Digest(data)
		}
		return values
	}
	beforeGlobals := globalHashes()
	probe := func() string {
		command := exec.CommandContext(ctx, filepath.Join(os.Getenv("SystemRoot"), "System32/wsl.exe"), "--distribution-id", "{"+id+"}", "--exec", "/usr/bin/sha256sum", "/usr/sbin/update-ca-certificates", "/etc/ssl/certs/ca-certificates.crt")
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		data, err := command.Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	beforeSystem := probe()
	log, err := os.OpenFile(filepath.Join(home, "service-stderr.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	process := exec.Command(binary, "local-services-serve", home, installation, daemon, services, servicesHash, trust, trustHash)
	process.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	process.Stderr = log
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- process.Wait() }()
	finished := false
	defer func() {
		if !finished {
			_ = process.Process.Kill()
			<-exited
		}
	}()
	var state *RuntimeSnapshot
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, err := CallServiceIPC(ctx, home, installation, ServiceRequest{Version: 1, Operation: "snapshot"})
		if err == nil && response.State != nil {
			state = response.State
			break
		}
		if err != nil && !errors.Is(err, ErrServiceNotRunning) {
			t.Fatal(err)
		}
		select {
		case err := <-exited:
			finished = true
			t.Fatal("service exited before readiness", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("service did not publish its pipe")
		}
		time.Sleep(25 * time.Millisecond)
	}
	if state.BinarySHA256 != binaryHash || state.ToolsSHA256 != servicesHash || state.WSLTrustSHA256 != trustHash || state.Settings.Enabled || state.WSLDistributionsRead || state.WSLTrust != nil {
		t.Fatal("service asset identity/startup observation differs")
	}
	call := func(request ServiceRequest) ServiceResponse {
		t.Helper()
		request.Version = 1
		response, err := CallServiceIPC(ctx, home, installation, request)
		if err != nil || response.Error != "" || response.State == nil {
			t.Fatal(request.Operation, err, response.Error)
		}
		return response
	}
	receipts := call(ServiceRequest{Operation: "wsl-trust-receipts"})
	if !receipts.State.WSLTrustReceiptsRead || len(receipts.State.WSLTrustReceipts) != 0 || receipts.State.WSLTrust != nil || receipts.State.WSLDistributionsRead {
		t.Fatal("receipt metadata read inspected a guest or required signing state")
	}
	created := call(ServiceRequest{Operation: "create-authority", Revision: state.Revision})
	if created.State.Authority == nil {
		t.Fatal("public authority review missing")
	}
	listed := call(ServiceRequest{Operation: "wsl-trust-distributions"})
	found := false
	for _, registration := range listed.State.WSLDistributions {
		if registration.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("selected ordinary registration missing")
	}
	observed := call(ServiceRequest{Operation: "wsl-trust-status", Revision: created.State.Revision, RootSHA256: created.State.Settings.RootSHA256, WSLReview: &WSLTrustReview{DistributionID: id}})
	status := observed.State.WSLTrust
	if status == nil || !status.Observed || status.Managed || status.OwnerSequence != 0 || status.Guest == nil || status.Guest.Sequence != 0 || status.Guest.FilePresent || status.Guest.BundlePresent {
		t.Fatal("expected absent ephemeral root", status)
	}
	rootStore, err := OpenCurrentUserRootStore(false)
	if err != nil {
		t.Fatal(err)
	}
	var authorityState authorityState
	data, err := os.ReadFile(filepath.Join(home, ".rdorbwin/local-services", authorityStateName))
	if err != nil {
		rootStore.Close()
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &authorityState); err != nil {
		rootStore.Close()
		t.Fatal(err)
	}
	present, err := rootStore.Contains(authorityState.RootCertificate)
	if err = errors.Join(err, rootStore.Close()); err != nil || present {
		t.Fatal("unexpected Windows trust for ephemeral root", err)
	}
	call(ServiceRequest{Operation: "stop-process"})
	select {
	case err := <-exited:
		finished = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("owned service did not stop")
	}
	if probe() != beforeSystem {
		t.Fatal("selected distro system bundle/updater changed")
	}
	afterGlobals := globalHashes()
	for name, digest := range beforeGlobals {
		if afterGlobals[name] != digest {
			t.Fatal("global client config changed", name)
		}
	}
	entries, err := os.ReadDir(filepath.Join(home, ".rdorbwin/local-services"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "wsl-trust-") {
			t.Fatal("read-only status wrote a host ownership receipt")
		}
	}
	info, err = cli.Info(ctx, client.InfoOptions{})
	if err != nil || info.Info.ID != daemon {
		t.Fatal("owned daemon identity changed", err)
	}
	proof := map[string]any{"scope": "production service IPC and selected WSL read-only status", "daemonId": daemon, "binarySha256": binaryHash, "servicesSha256": servicesHash, "trustSha256": trustHash, "status": status, "windowsRootAbsent": true, "systemBundleUpdaterBeforeAfter": beforeSystem, "globalClientConfigBeforeAfter": afterGlobals, "serviceStopped": true, "systemTrustMutated": false, "engineObjectsCreated": false, "phase2Acceptance": false}
	encoded, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "proof.json")
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("read-only production service proof:", path)
}

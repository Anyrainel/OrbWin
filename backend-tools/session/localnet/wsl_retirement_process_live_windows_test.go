//go:build windows

package localnet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// Controlled failure precondition: an owned host intent is persisted before a
// denied simulated guest dispatch. The production SDK then uses the real Windows
// registry to retire its never-registered GUID and rotate a private authority.
// This is not a claim that a real distro was unregistered or trust was removed.
func TestOwnedServiceRetiresMissingRegistrationIntentAndRotatesPrivateAuthority(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_WSL_TRUST_RECOVERY") != "1" {
		t.Skip("explicit host-only recovery qualification; no WSL registration/trust mutation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
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
	home := filepath.Join(root, "artifacts/qualification", fmt.Sprintf("orbwin-p2-wsl-retirement-%d", time.Now().UnixNano()))
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	installation := fmt.Sprintf("%032x", time.Now().UnixNano())
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	idText := hex.EncodeToString(nonce[:])
	id := idText[:8] + "-" + idText[8:12] + "-" + idText[12:16] + "-" + idText[16:20] + "-" + idText[20:]
	if present, err := wsltrust.RegistrationPresent(id); err != nil || present {
		t.Fatal("qualification GUID is not confirmed absent", err)
	}
	user, err := wsltrust.CurrentWindowsUser()
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := OpenNamespaceLedger(filepath.Join(home, ".rdorbwin/local-services"), daemon)
	if err != nil {
		t.Fatal(err)
	}
	// Close on all precondition failures; a second Close is harmless after the
	// explicit handoff of the exclusive lease to the child below.
	defer ledger.Close()
	authority, err := ledger.CreateAuthority(installation, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	settings.RootSHA256 = authority.Fingerprint()
	stored, err := ledger.StoreSettings(0, settings)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fixtureWSLTrustClient{user: user, fault: "denied", guest: wsltrust.Status{DistributionID: id, MachineID: strings.Repeat("a", 32), RootSHA256: authority.Fingerprint(), OSID: "ubuntu", OSVersion: "24.04", Observed: true, State: "absent"}}
	status, err := ledger.WSLTrust(ctx, fake, installation, authority.Fingerprint(), "install", WSLTrustReview{DistributionID: id, MachineID: fake.guest.MachineID}, time.Now())
	if err == nil || status.OwnerState != "install-intent" || status.OwnerSequence != 1 || fake.mutations != 0 {
		t.Fatal("controlled pre-dispatch failure did not retain intent", status, err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
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
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, err := CallServiceIPC(ctx, home, installation, ServiceRequest{Version: 1, Operation: "snapshot"})
		if err == nil && response.State != nil {
			if response.State.BinarySHA256 != binaryHash || response.State.Revision != stored.Revision {
				t.Fatal("source process or desired-state identity differs")
			}
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
	call := func(request ServiceRequest) ServiceResponse {
		t.Helper()
		request.Version = 1
		response, err := CallServiceIPC(ctx, home, installation, request)
		if err != nil || response.Error != "" || response.State == nil {
			t.Fatal(request.Operation, err, response.Error)
		}
		return response
	}
	listed := call(ServiceRequest{Operation: "wsl-trust-receipts"})
	if len(listed.State.WSLTrustReceipts) != 1 {
		t.Fatal("controlled intent missing")
	}
	row := listed.State.WSLTrustReceipts[0]
	if row.State != "install-intent" || row.RegistrationState != "absent" || row.GuestSequence != 0 {
		t.Fatal(row)
	}
	review := WSLTrustReview{DistributionID: row.DistributionID, MachineID: row.MachineID, OwnerSequence: row.OwnerSequence, GuestSequence: row.GuestSequence}
	retired := call(ServiceRequest{Operation: "retire-wsl-trust-receipt", Revision: stored.Revision, RootSHA256: row.RootSHA256, WSLReview: &review})
	if len(retired.State.WSLTrustReceipts) != 1 || retired.State.WSLTrustReceipts[0].State != "registration-retired" || retired.State.WSLTrustReceipts[0].GuestSequence != 0 {
		t.Fatal("retirement falsely became certificate removal", retired)
	}
	rotation := call(ServiceRequest{Operation: "rotate-authority", Revision: stored.Revision, RootSHA256: authority.Fingerprint(), Ports: &ServicePorts{HTTP: 18080, HTTPS: 18443}})
	if rotation.State.Revision != stored.Revision+1 || rotation.State.Settings.Enabled || rotation.State.Settings.RootSHA256 == authority.Fingerprint() || rotation.State.Rotation != nil {
		t.Fatal("retired missing GUID did not unblock private authority rotation", rotation)
	}
	store, err := OpenCurrentUserRootStore(false)
	if err != nil {
		t.Fatal(err)
	}
	present, err := store.Contains(authority.RootCertificate())
	var current authorityState
	currentData, readCurrentErr := os.ReadFile(filepath.Join(home, ".rdorbwin/local-services", authorityStateName))
	if readCurrentErr == nil {
		readCurrentErr = json.Unmarshal(currentData, &current)
	}
	newPresent := false
	if readCurrentErr == nil {
		newPresent, readCurrentErr = store.Contains(current.RootCertificate)
	}
	err = errors.Join(err, readCurrentErr, store.Close())
	if err != nil || present || newPresent {
		t.Fatal("unexpected Windows trust for private fixture root", err)
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
	retained, err := OpenNamespaceLedger(filepath.Join(home, ".rdorbwin/local-services"), daemon)
	if err != nil {
		t.Fatal(err)
	}
	record, readErr := retained.readWSLTrust(installation, user, id, authority.Fingerprint())
	closeErr := retained.Close()
	if err := errors.Join(readErr, closeErr); err != nil || record == nil || record.State != "registration-retired" || wsltrust.Digest(record.RootCertificate) != authority.Fingerprint() || record.GuestSequence != 0 {
		t.Fatal("public recovery history changed", err)
	}
	if present, err := wsltrust.RegistrationPresent(id); err != nil || present {
		t.Fatal("qualification registration presence changed", err)
	}
	info, err = cli.Info(ctx, client.InfoOptions{})
	if err != nil || info.Info.ID != daemon {
		t.Fatal("owned daemon changed", err)
	}
	proof := map[string]any{"scope": "production service host-only retirement and private authority rotation with controlled pre-dispatch failure", "precondition": "owned intent persisted through the ledger API before denied simulated guest dispatch; GUID never registered; no guest trust installation", "daemonId": daemon, "binarySha256": binaryHash, "registrationId": id, "oldRootSha256": authority.Fingerprint(), "newRootSha256": rotation.State.Settings.RootSHA256, "receipt": retired.State.WSLTrustReceipts[0], "publicHistoryRetained": true, "oldWindowsRootAbsent": true, "registrationStillAbsent": true, "serviceStopped": true, "systemTrustMutated": false, "registrationsMutated": false, "engineObjectsCreated": false, "phase2Acceptance": false}
	encoded, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "proof.json")
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("controlled native retirement/rotation proof:", path)
}

//go:build windows

package localnet

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/wsltrust"
)

func TestRuntimeWSLTrustRequiresReviewAndKeepsListingAndStartupObservational(t *testing.T) {
	client := &fixtureWSLTrustClient{user: "S-1-5-21-1", guest: wsltrust.Status{DistributionID: "00000000-0000-4000-8000-000000000002", MachineID: strings.Repeat("a", 32), OSID: "ubuntu", OSVersion: "24.04", Observed: true, State: "absent"}}
	opens, listings := 0, 0
	runtime, _ := runtimeFixtureWithOptions(t, nil, func(options *RuntimeOptions) {
		options.WSLTrustFactory = func() (WSLTrustClient, error) { opens++; return client, nil }
		options.WSLRegistrations = func() ([]wsltrust.Registration, error) {
			listings++
			return []wsltrust.Registration{{ID: client.guest.DistributionID, Name: "Ubuntu"}}, nil
		}
	})
	state := awaitRuntimePhase(t, runtime, "disabled")
	if opens != 0 || listings != 0 {
		t.Fatal("startup inspected/started selected distributions")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runtime.SelectedWSLTrust(ctx, "wsl-trust-distributions", 0, "", WSLTrustReview{}); err != nil {
		t.Fatal(err)
	}
	if opens != 0 || listings != 1 || client.statuses != 0 {
		t.Fatal("listing invoked a Linux helper")
	}
	if _, err := runtime.CreateAuthority(ctx, state.Revision); err != nil {
		t.Fatal(err)
	}
	state, _ = runtime.Snapshot()
	root := state.Settings.RootSHA256
	client.guest.RootSHA256 = root
	for _, fault := range []string{"stale", "wrong-root"} {
		revision, reviewedRoot := state.Revision, root
		if fault == "stale" {
			revision--
		} else {
			reviewedRoot = strings.Repeat("0", 64)
		}
		if err := runtime.SelectedWSLTrust(ctx, "wsl-trust-install", revision, reviewedRoot, WSLTrustReview{DistributionID: client.guest.DistributionID}); err == nil || opens != 0 {
			t.Fatal("unreviewed request opened guest transport", fault, err)
		}
	}
	request := ServiceRequest{Version: 1, Operation: "wsl-trust-status", Revision: state.Revision, RootSHA256: root, WSLReview: &WSLTrustReview{DistributionID: client.guest.DistributionID}}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response := dispatchServiceRequest(ctx, runtime, raw)
	if response.Error != "" || response.State.WSLTrust == nil || !response.State.WSLTrust.Guest.Observed || opens != 1 || client.statuses != 1 {
		t.Fatal(response, opens, client.statuses)
	}
	response.State.WSLTrust.Guest.MachineID = "mutated"
	response.State.WSLDistributions[0].Name = "mutated"
	state, _ = runtime.Snapshot()
	if state.WSLTrust.Guest.MachineID != client.guest.MachineID || state.WSLDistributions[0].Name != "Ubuntu" {
		t.Fatal("snapshot aliases runtime state")
	}
	request.Operation = "wsl-trust-install"
	review := wslReview(*state.WSLTrust)
	request.WSLReview = &review
	raw, err = json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response = dispatchServiceRequest(ctx, runtime, raw)
	if response.Error != "" || response.State.WSLTrust.OwnerState != "installed" || !response.State.WSLTrust.Managed || client.mutations != 1 {
		t.Fatal(response, client.mutations)
	}
	if err := os.Remove(filepath.Join(runtime.opts.Ledger.directory, authorityStateName)); err != nil {
		t.Fatal(err)
	}
	request.Operation = "wsl-trust-remove"
	review = wslReview(*response.State.WSLTrust)
	request.WSLReview = &review
	raw, err = json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response = dispatchServiceRequest(ctx, runtime, raw)
	if response.Error != "" || response.State.WSLTrust.OwnerState != "removed" || response.State.WSLTrust.Managed || client.mutations != 2 || response.State.Revision != state.Revision {
		t.Fatal(response, client.mutations)
	}
}

func TestWSLServiceIPCRejectsUntypedOrCrossOperationReview(t *testing.T) {
	runtime, _ := runtimeFixture(t)
	for _, raw := range []string{
		`{"version":1,"operation":"snapshot","wslReview":{"distributionId":"00000000-0000-4000-8000-000000000002"}}`,
		`{"version":1,"operation":"wsl-trust-status"}`,
		`{"version":1,"operation":"wsl-trust-status","wslReview":{"distributionId":"Ubuntu","command":"sh"}}`,
		`{"version":1,"operation":"wsl-trust-distributions","rootSha256":"unreviewed"}`,
		`{"version":1,"operation":"retire-wsl-trust-receipt"}`,
		`{"version":1,"operation":"wsl-trust-receipts","wslReview":{"distributionId":"Ubuntu"}}`,
	} {
		if response := dispatchServiceRequest(context.Background(), runtime, []byte(raw)); response.Error == "" {
			t.Fatal("untyped review accepted", raw)
		}
	}
	if ServiceOperationTimeout(ServiceRequest{Operation: "wsl-trust-install"}) <= 2*65*time.Second || ServiceOperationTimeout(ServiceRequest{Operation: "snapshot"}) != 40*time.Second {
		t.Fatal("WSL observation/mutation budget or existing operation budget differs")
	}
}

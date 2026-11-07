//go:build windows

package localnet

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/wsltrust"
)

func TestRuntimeWSLReceiptRecoveryUsesRawRegistryAndNeverStartsGuest(t *testing.T) {
	client := &fixtureWSLTrustClient{user: "S-1-5-21-1", guest: wsltrust.Status{DistributionID: "00000000-0000-4000-8000-000000000002", MachineID: strings.Repeat("a", 32), OSID: "ubuntu", OSVersion: "24.04", Observed: true, State: "absent"}}
	registry := &fixtureWSLRegistry{user: client.user, present: true}
	opens := 0
	runtime, _ := runtimeFixtureWithOptions(t, nil, func(options *RuntimeOptions) {
		options.WSLTrustFactory = func() (WSLTrustClient, error) { return client, nil }
		options.WSLRegistrations = func() ([]wsltrust.Registration, error) { return nil, nil } // A filtered-out registration is still raw-present.
		options.WSLRegistryFactory = func() (WSLTrustRegistry, error) { opens++; return registry, nil }
	})
	state := awaitRuntimePhase(t, runtime, "disabled")
	if opens != 0 || registry.calls != 0 || state.WSLTrustReceiptsRead {
		t.Fatal("startup inspected receipt registrations")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := runtime.CreateAuthority(ctx, state.Revision); err != nil {
		t.Fatal(err)
	}
	state, _ = runtime.Snapshot()
	root := state.Settings.RootSHA256
	client.guest.RootSHA256 = root
	if err := runtime.SelectedWSLTrust(ctx, "wsl-trust-status", state.Revision, root, WSLTrustReview{DistributionID: client.guest.DistributionID}); err != nil {
		t.Fatal(err)
	}
	state, _ = runtime.Snapshot()
	if err := runtime.SelectedWSLTrust(ctx, "wsl-trust-install", state.Revision, root, wslReview(*state.WSLTrust)); err != nil {
		t.Fatal(err)
	}
	statuses, mutations := client.statuses, client.mutations
	call := func(request ServiceRequest) ServiceResponse {
		t.Helper()
		request.Version = 1
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		return dispatchServiceRequest(ctx, runtime, raw)
	}
	filtered := call(ServiceRequest{Operation: "wsl-trust-distributions"})
	if filtered.Error != "" || !filtered.State.WSLDistributionsRead || len(filtered.State.WSLDistributions) != 0 {
		t.Fatal(filtered)
	}
	listed := call(ServiceRequest{Operation: "wsl-trust-receipts"})
	if listed.Error != "" || len(listed.State.WSLTrustReceipts) != 1 || listed.State.WSLTrustReceipts[0].RegistrationState != "present" {
		t.Fatal(listed)
	}
	row := listed.State.WSLTrustReceipts[0]
	review := WSLTrustReview{DistributionID: row.DistributionID, MachineID: row.MachineID, OwnerSequence: row.OwnerSequence, GuestSequence: row.GuestSequence}
	refused := call(ServiceRequest{Operation: "retire-wsl-trust-receipt", Revision: state.Revision, RootSHA256: root, WSLReview: &review})
	if refused.Error == "" || refused.State.WSLTrustReceipts[0].State != "installed" {
		t.Fatal("filtered registry list became retirement permission", refused)
	}
	registry.present = false
	before := registry.calls
	stale := call(ServiceRequest{Operation: "retire-wsl-trust-receipt", Revision: state.Revision - 1, RootSHA256: root, WSLReview: &review})
	if stale.Error == "" || registry.calls != before {
		t.Fatal("stale settings revision read or changed receipt", stale)
	}
	retired := call(ServiceRequest{Operation: "retire-wsl-trust-receipt", Revision: state.Revision, RootSHA256: root, WSLReview: &review})
	if retired.Error != "" || retired.State.WSLTrustReceipts[0].State != "registration-retired" || retired.State.WSLTrustReceipts[0].RetiredAt == nil || retired.State.WSLTrust != nil || retired.State.Revision != state.Revision {
		t.Fatal(retired)
	}
	retired.State.WSLTrustReceipts[0].MachineID = "mutated"
	snapshot, _ := runtime.Snapshot()
	if snapshot.WSLTrustReceipts[0].MachineID != client.guest.MachineID {
		t.Fatal("receipt snapshot aliases runtime state")
	}
	if client.statuses != statuses || client.mutations != mutations {
		t.Fatal("receipt recovery started a guest helper")
	}
}

//go:build windows

package localnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/wsltrust"
)

type fixtureWSLTrustClient struct {
	user                string
	guest               wsltrust.Status
	fault               string
	statuses, mutations int
	beforeMutation      func(wsltrust.Request)
}

func (client *fixtureWSLTrustClient) UserID() string { return client.user }
func (client *fixtureWSLTrustClient) Call(ctx context.Context, request wsltrust.Request) (*wsltrust.Status, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Operation == "status" {
		client.statuses++
		copy := client.guest
		return &copy, nil
	}
	if client.beforeMutation != nil {
		client.beforeMutation(request)
	}
	if request.ExpectedSequence != client.guest.Sequence || request.ExpectedMachineID != client.guest.MachineID {
		return nil, errors.New("guest sequence or machine changed")
	}
	if client.fault == "denied" {
		return nil, errors.New("guest operation denied")
	}
	if request.Operation == "install" {
		client.mutations++
		client.guest.Sequence += 2
		client.guest.State, client.guest.Managed, client.guest.FilePresent, client.guest.BundlePresent = "installed", true, true, true
	} else if request.Operation == "remove" && client.guest.State != "removed" {
		client.mutations++
		client.guest.Sequence += 2
		client.guest.State, client.guest.Managed, client.guest.FilePresent, client.guest.BundlePresent = "removed", false, false, false
	}
	if client.fault == "accepted" {
		return nil, context.DeadlineExceeded
	}
	copy := client.guest
	return &copy, nil
}
func fixtureWSLTrust(t *testing.T) (*NamespaceLedger, *Authority, string, *fixtureWSLTrustClient) {
	t.Helper()
	ledger, authority, installation, _ := authorityStoreFixture(t)
	client := &fixtureWSLTrustClient{user: "S-1-5-21-1", guest: wsltrust.Status{DistributionID: "00000000-0000-4000-8000-000000000002", MachineID: strings.Repeat("a", 32), OSID: "ubuntu", OSVersion: "24.04", RootSHA256: authority.Fingerprint(), Observed: true, State: "absent"}}
	client.beforeMutation = func(request wsltrust.Request) {
		record, err := ledger.readWSLTrust(installation, client.user, request.DistributionID, request.RootSHA256)
		if err != nil || record == nil || record.State != request.Operation+"-intent" || record.MachineID != request.ExpectedMachineID || record.GuestSequence != request.ExpectedSequence || !bytes.Equal(record.RootCertificate, request.RootCertificate) {
			t.Fatal("guest mutation preceded exact durable host intent", record, err)
		}
	}
	return ledger, authority, installation, client
}
func wslReview(status WSLTrustStatus) WSLTrustReview {
	return WSLTrustReview{DistributionID: status.DistributionID, MachineID: status.Guest.MachineID, GuestSequence: status.Guest.Sequence, OwnerSequence: status.OwnerSequence}
}
func wslRead(t *testing.T, ledger *NamespaceLedger, installation, root string, client *fixtureWSLTrustClient) WSLTrustStatus {
	t.Helper()
	status, err := ledger.WSLTrust(context.Background(), client, installation, root, "status", WSLTrustReview{DistributionID: client.guest.DistributionID}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return status
}
func TestWSLHostTrustOwnsBeforeMutationAndRemovesWithoutSigningState(t *testing.T) {
	ledger, authority, installation, client := fixtureWSLTrust(t)
	root := authority.Fingerprint()
	status := wslRead(t, ledger, installation, root, client)
	if status.OwnerState != "unmanaged" || status.OwnerSequence != 0 || status.Managed {
		t.Fatal(status)
	}
	status, err := ledger.WSLTrust(context.Background(), client, installation, root, "install", wslReview(status), time.Now())
	if err != nil || status.OwnerState != "installed" || !status.Managed || client.mutations != 1 {
		t.Fatal(status, err)
	}
	status, err = ledger.WSLTrust(context.Background(), client, installation, root, "install", wslReview(status), time.Now())
	if err != nil || client.mutations != 1 {
		t.Fatal("install replayed", status, err)
	}
	if err := os.Remove(filepath.Join(ledger.directory, authorityStateName)); err != nil {
		t.Fatal(err)
	}
	status = wslRead(t, ledger, installation, root, client)
	status, err = ledger.WSLTrust(context.Background(), client, installation, root, "remove", wslReview(status), time.Now().AddDate(3, 0, 0))
	if err != nil || status.OwnerState != "removed" || status.Managed || status.Guest.BundlePresent || client.mutations != 2 {
		t.Fatal(status, err)
	}
	status, err = ledger.WSLTrust(context.Background(), client, installation, root, "remove", wslReview(status), time.Now())
	if err != nil || client.mutations != 2 {
		t.Fatal("removal replayed", status, err)
	}
	client.guest.State, client.guest.FilePresent, client.guest.BundlePresent = "present-unmanaged", true, true
	status, err = ledger.WSLTrust(context.Background(), client, installation, root, "remove", wslReview(status), time.Now())
	if err == nil || status.Managed || client.mutations != 2 {
		t.Fatal("external re-add adopted", status, err)
	}
}
func TestWSLHostTrustPreservesUnownedGuestCertificatesAndReceipts(t *testing.T) {
	for _, managed := range []bool{false, true} {
		t.Run(map[bool]string{false: "preexisting", true: "guest-receipt-only"}[managed], func(t *testing.T) {
			ledger, authority, installation, client := fixtureWSLTrust(t)
			client.guest.Managed, client.guest.FilePresent, client.guest.BundlePresent = managed, true, true
			client.guest.State = "present-unmanaged"
			if managed {
				client.guest.State = "installed"
				client.guest.Sequence = 2
			}
			status := wslRead(t, ledger, installation, authority.Fingerprint(), client)
			status, err := ledger.WSLTrust(context.Background(), client, installation, authority.Fingerprint(), "install", wslReview(status), time.Now())
			if managed && err == nil {
				t.Fatal("unowned guest receipt adopted")
			}
			if !managed && err != nil {
				t.Fatal(err)
			}
			if status.Managed || client.mutations != 0 {
				t.Fatal(status)
			}
			if _, err := os.Lstat(ledger.wslTrustPath(client.guest.DistributionID, authority.Fingerprint())); !os.IsNotExist(err) {
				t.Fatal("unowned state gained host receipt", err)
			}
			if _, err := ledger.WSLTrust(context.Background(), client, installation, authority.Fingerprint(), "remove", wslReview(status), time.Now()); err == nil {
				t.Fatal("unowned guest root removed")
			}
		})
	}
}
func TestWSLHostTrustUnknownResultRequiresReadAndExplicitSettlement(t *testing.T) {
	for _, fault := range []string{"denied", "accepted"} {
		t.Run(fault, func(t *testing.T) {
			ledger, authority, installation, client := fixtureWSLTrust(t)
			root := authority.Fingerprint()
			status := wslRead(t, ledger, installation, root, client)
			client.fault = fault
			status, err := ledger.WSLTrust(context.Background(), client, installation, root, "install", wslReview(status), time.Now())
			if err == nil || status.OwnerState != "install-intent" {
				t.Fatal(status, err)
			}
			path := ledger.wslTrustPath(client.guest.DistributionID, root)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			status = wslRead(t, ledger, installation, root, client)
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) || status.OwnerState != "install-intent" {
				t.Fatal("status settled/replayed intent", status, err)
			}
			client.fault = ""
			if fault == "accepted" {
				status, err = ledger.WSLTrust(context.Background(), client, installation, root, "install", wslReview(status), time.Now())
				if err != nil || status.OwnerState != "installed" || client.mutations != 1 {
					t.Fatal("explicit settlement repeated install", status, err)
				}
			} else {
				status, err = ledger.WSLTrust(context.Background(), client, installation, root, "remove", wslReview(status), time.Now())
				if err != nil || status.OwnerState != "removed" || status.Guest.Sequence == 0 || client.mutations != 1 {
					t.Fatal("denied first install did not settle through guest tombstone", status, err)
				}
			}
		})
	}
}
func TestWSLHostTrustRefusesStaleForeignAndRegressedState(t *testing.T) {
	for _, fault := range []string{"owner-sequence", "guest-sequence", "machine", "windows-user", "record-root", "record-version", "regressed-guest", "foreign-observation", "unobserved"} {
		t.Run(fault, func(t *testing.T) {
			ledger, authority, installation, client := fixtureWSLTrust(t)
			root := authority.Fingerprint()
			status := wslRead(t, ledger, installation, root, client)
			status, err := ledger.WSLTrust(context.Background(), client, installation, root, "install", wslReview(status), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			review := wslReview(status)
			switch fault {
			case "owner-sequence":
				review.OwnerSequence--
			case "guest-sequence":
				review.GuestSequence--
			case "machine":
				client.guest.MachineID = strings.Repeat("b", 32)
			case "windows-user":
				client.user = "S-1-5-21-2"
			case "regressed-guest":
				client.guest.Sequence = 0
				client.guest.Managed = false
				client.guest.FilePresent = false
				client.guest.BundlePresent = false
				client.guest.State = "absent"
				review.GuestSequence = 0
			case "foreign-observation":
				client.guest.RootSHA256 = strings.Repeat("0", 64)
			case "unobserved":
				client.guest.Observed = false
			case "record-root", "record-version":
				path := ledger.wslTrustPath(client.guest.DistributionID, root)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var record wslTrustRecord
				if err := json.Unmarshal(data, &record); err != nil {
					t.Fatal(err)
				}
				if fault == "record-root" {
					record.RootCertificate[0] ^= 1
				} else {
					record.Version++
				}
				data, err = json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := ledger.WSLTrust(context.Background(), client, installation, root, "remove", review, time.Now()); err == nil || client.mutations != 1 {
				t.Fatal("unsafe removal accepted", fault, err)
			}
		})
	}
}
func TestWSLHostTrustLostFileAcknowledgementDoesNotReplayGuestMutation(t *testing.T) {
	for _, boundary := range []string{"before-intent", "after-intent", "before-settle", "after-settle"} {
		t.Run(boundary, func(t *testing.T) {
			ledger, authority, installation, client := fixtureWSLTrust(t)
			root := authority.Fingerprint()
			status := wslRead(t, ledger, installation, root, client)
			writes := 0
			ledger.trustReplace = func(source, destination string) error {
				writes++
				trigger := (writes == 1 && strings.HasSuffix(boundary, "intent")) || (writes == 2 && strings.HasSuffix(boundary, "settle"))
				if trigger && strings.HasPrefix(boundary, "before") {
					return errors.New("replace refused")
				}
				if err := replaceSettings(source, destination); err != nil {
					return err
				}
				if trigger {
					return errors.New("accepted replace acknowledgement lost")
				}
				return nil
			}
			status, err := ledger.WSLTrust(context.Background(), client, installation, root, "install", wslReview(status), time.Now())
			if err == nil {
				t.Fatal("file fault swallowed")
			}
			if strings.HasSuffix(boundary, "intent") && client.mutations != 0 {
				t.Fatal("guest dispatched after uncertain host intent flush")
			}
			ledger.trustReplace = nil
			status = wslRead(t, ledger, installation, root, client)
			status, err = ledger.WSLTrust(context.Background(), client, installation, root, "install", wslReview(status), time.Now())
			if err != nil || status.OwnerState != "installed" || client.mutations != 1 {
				t.Fatal("explicit recovery did not preserve single guest installation", status, err)
			}
		})
	}
}
func TestAuthorityRotationRequiresOwnedWSLTrustRemovalWithoutGuestCalls(t *testing.T) {
	ledger, authority, installation, store, settings := rotationFixture(t)
	client := &fixtureWSLTrustClient{user: "S-1-5-21-1", guest: wsltrust.Status{DistributionID: "00000000-0000-4000-8000-000000000002", MachineID: strings.Repeat("a", 32), OSID: "ubuntu", OSVersion: "24.04", RootSHA256: authority.Fingerprint(), Observed: true, State: "absent"}}
	store.user = client.user
	status := wslRead(t, ledger, installation, authority.Fingerprint(), client)
	client.fault = "denied"
	status, err := ledger.WSLTrust(context.Background(), client, installation, authority.Fingerprint(), "install", wslReview(status), time.Now())
	if err == nil || status.OwnerState != "install-intent" {
		t.Fatal(status, err)
	}
	calls := client.statuses
	if _, err := ledger.BeginAuthorityRotation(store, installation, authority.Fingerprint(), settings.Revision, ServicePorts{HTTP: 18080, HTTPS: 18443}, time.Now()); err == nil || client.statuses != calls {
		t.Fatal("pending WSL intent bypassed or started guest during rotation", err)
	}
	if _, err := os.Lstat(ledger.rotationPath()); !os.IsNotExist(err) {
		t.Fatal("rotation created preparations before WSL removal", err)
	}
	client.fault = ""
	status = wslRead(t, ledger, installation, authority.Fingerprint(), client)
	status, err = ledger.WSLTrust(context.Background(), client, installation, authority.Fingerprint(), "remove", wslReview(status), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	calls = client.statuses
	info, err := ledger.BeginAuthorityRotation(store, installation, authority.Fingerprint(), settings.Revision, ServicePorts{HTTP: 18080, HTTPS: 18443}, time.Now())
	if err != nil || info == nil || client.statuses != calls {
		t.Fatal("settled WSL removal prevented rotation or started guest", info, err)
	}
	if _, err := ledger.WSLTrust(context.Background(), client, installation, authority.Fingerprint(), "install", wslReview(status), time.Now()); err == nil {
		t.Fatal("trust installation accepted during pending rotation")
	}
}

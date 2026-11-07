//go:build windows

package localnet

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/wsltrust"
)

type fixtureWSLRegistry struct {
	user    string
	present bool
	err     error
	calls   int
	read    func(int) (bool, error)
}

func (registry *fixtureWSLRegistry) UserID() string { return registry.user }
func (registry *fixtureWSLRegistry) Present(string) (bool, error) {
	registry.calls++
	if registry.read != nil {
		return registry.read(registry.calls)
	}
	return registry.present, registry.err
}
func retirementFixture(t *testing.T) (*NamespaceLedger, *Authority, string, *fixtureWSLTrustClient, *fixtureWSLRegistry, WSLTrustReview) {
	t.Helper()
	ledger, authority, installation, client := fixtureWSLTrust(t)
	status := wslRead(t, ledger, installation, authority.Fingerprint(), client)
	status, err := ledger.WSLTrust(context.Background(), client, installation, authority.Fingerprint(), "install", wslReview(status), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	registry := &fixtureWSLRegistry{user: client.user}
	ledger.wslRegistryFactory = func() (WSLTrustRegistry, error) { return registry, nil }
	return ledger, authority, installation, client, registry, wslReview(status)
}
func TestWSLRetirementPreservesPublicHistoryWithoutGuestOrSigningState(t *testing.T) {
	ledger, authority, installation, client, registry, review := retirementFixture(t)
	root := authority.Fingerprint()
	path := ledger.wslTrustPath(review.DistributionID, root)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := ledger.ListWSLTrustReceipts(installation)
	if err != nil || len(infos) != 1 || infos[0].RegistrationState != "absent" || infos[0].State != "installed" {
		t.Fatal(infos, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("listing rewrote intent", err)
	}
	if err := os.Remove(filepath.Join(ledger.directory, authorityStateName)); err != nil {
		t.Fatal(err)
	}
	statuses, mutations := client.statuses, client.mutations
	now := time.Now()
	info, err := ledger.RetireWSLTrustReceipt(context.Background(), installation, root, review, now)
	if err != nil || info.State != "registration-retired" || info.OwnerSequence != review.OwnerSequence+2 || info.RetiredAt == nil || !info.RetiredAt.Equal(now) || registry.calls != 4 {
		t.Fatal(info, err, registry.calls)
	}
	if client.statuses != statuses || client.mutations != mutations {
		t.Fatal("retirement invoked a guest helper")
	}
	record, err := ledger.readWSLTrust(installation, client.user, review.DistributionID, root)
	if err != nil || record == nil || !bytes.Equal(record.RootCertificate, authority.RootCertificate()) || record.GuestSequence != review.GuestSequence {
		t.Fatal("public trust history changed", record, err)
	}
	if err := ledger.rotationWSLTrustSettled(installation, client.user); err != nil {
		t.Fatal(err)
	}
	registry.present = true
	if err := ledger.rotationWSLTrustSettled(installation, client.user); err == nil {
		t.Fatal("returned GUID bypassed rotation guard")
	}
	registry.present = false
	registry.err = errors.New("registry unreadable")
	if err := ledger.rotationWSLTrustSettled(installation, client.user); err == nil {
		t.Fatal("registry failure became absence")
	}
}
func TestWSLRetirementRefusesPresentUnreadableStaleAndForeignReceipts(t *testing.T) {
	for _, fault := range []string{"present", "unreadable", "owner-sequence", "guest-sequence", "machine", "windows-user", "root", "canceled"} {
		t.Run(fault, func(t *testing.T) {
			ledger, authority, installation, client, registry, review := retirementFixture(t)
			root := authority.Fingerprint()
			path := ledger.wslTrustPath(review.DistributionID, root)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch fault {
			case "present":
				registry.present = true // Even if excluded from the ordinary-distro list.
			case "unreadable":
				registry.err = errors.New("access denied")
			case "owner-sequence":
				review.OwnerSequence--
			case "guest-sequence":
				review.GuestSequence--
			case "machine":
				review.MachineID = "foreign"
			case "windows-user":
				registry.user = "S-1-5-21-2"
			case "root":
				root = string(bytes.Repeat([]byte{'0'}, 64))
			case "canceled":
				cancel()
			}
			if _, err := ledger.RetireWSLTrustReceipt(ctx, installation, root, review, time.Now()); err == nil {
				t.Fatal("unsafe retirement accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("refused retirement changed public record", err)
			}
			if client.mutations != 1 {
				t.Fatal("retirement changed guest trust")
			}
			if fault == "unreadable" {
				infos, err := ledger.ListWSLTrustReceipts(installation)
				if err != nil || infos[0].RegistrationState != "unknown" || infos[0].RegistrationError == "" {
					t.Fatal("unreadable registry reported absent", infos, err)
				}
			}
		})
	}
}
func TestWSLRetirementRetainsUncertaintyWhenRegistrationReturns(t *testing.T) {
	for _, boundary := range []int{2, 3} {
		t.Run(map[int]string{2: "before-final", 3: "after-final"}[boundary], func(t *testing.T) {
			ledger, authority, installation, client, registry, review := retirementFixture(t)
			registry.read = func(call int) (bool, error) { return call >= boundary, nil }
			info, err := ledger.RetireWSLTrustReceipt(context.Background(), installation, authority.Fingerprint(), review, time.Now())
			if err == nil || info.State != "registration-retiring" || info.RetiredAt != nil || info.RegistrationState != "present" {
				t.Fatal(info, err)
			}
			if err := ledger.rotationWSLTrustSettled(installation, client.user); err == nil {
				t.Fatal("uncertain retirement allowed rotation")
			}
			// The same registration can still use reviewed trust removal while its
			// retirement remains pending; no restored certificate is newly adopted.
			status := wslRead(t, ledger, installation, authority.Fingerprint(), client)
			status, err = ledger.WSLTrust(context.Background(), client, installation, authority.Fingerprint(), "remove", wslReview(status), time.Now())
			if err != nil || status.OwnerState != "removed" || client.mutations != 2 {
				t.Fatal(status, err)
			}
		})
	}
}
func TestWSLRetirementLostFileAcknowledgementsNeedFreshReviewWithoutGuestCalls(t *testing.T) {
	for _, boundary := range []string{"before-intent", "after-intent", "before-final", "after-final"} {
		t.Run(boundary, func(t *testing.T) {
			ledger, authority, installation, client, _, review := retirementFixture(t)
			writes := 0
			ledger.trustReplace = func(source, destination string) error {
				writes++
				trigger := (writes == 1 && (boundary == "before-intent" || boundary == "after-intent")) || (writes == 2 && (boundary == "before-final" || boundary == "after-final"))
				if trigger && (boundary == "before-intent" || boundary == "before-final") {
					return errors.New("replace refused")
				}
				if err := replaceSettings(source, destination); err != nil {
					return err
				}
				if trigger {
					return errors.New("accepted acknowledgement lost")
				}
				return nil
			}
			info, err := ledger.RetireWSLTrustReceipt(context.Background(), installation, authority.Fingerprint(), review, time.Now())
			if err == nil || info.RegistrationState != "unknown" || client.mutations != 1 {
				t.Fatal(info, err)
			}
			ledger.trustReplace = nil
			infos, err := ledger.ListWSLTrustReceipts(installation)
			if err != nil {
				t.Fatal(err)
			}
			updated := infos[0]
			review = WSLTrustReview{DistributionID: updated.DistributionID, MachineID: updated.MachineID, OwnerSequence: updated.OwnerSequence, GuestSequence: updated.GuestSequence}
			info, err = ledger.RetireWSLTrustReceipt(context.Background(), installation, authority.Fingerprint(), review, time.Now())
			if err != nil || info.State != "registration-retired" || client.mutations != 1 {
				t.Fatal(info, err)
			}
			if boundary == "after-final" && info.OwnerSequence != updated.OwnerSequence {
				t.Fatal("committed retirement was replayed")
			}
		})
	}
}
func TestAuthorityRotationRechecksRetiredGUIDAtBeginAndResume(t *testing.T) {
	ledger, authority, installation, store, settings := rotationFixture(t)
	client := &fixtureWSLTrustClient{user: "S-1-5-21-1"}
	client.guest = wsltrust.Status{DistributionID: "00000000-0000-4000-8000-000000000002", MachineID: strings.Repeat("a", 32), RootSHA256: authority.Fingerprint(), OSID: "ubuntu", OSVersion: "24.04", Observed: true, State: "absent"}
	store.user = client.user
	registry := &fixtureWSLRegistry{user: client.user}
	ledger.wslRegistryFactory = func() (WSLTrustRegistry, error) { return registry, nil }
	status := wslRead(t, ledger, installation, authority.Fingerprint(), client)
	status, err := ledger.WSLTrust(context.Background(), client, installation, authority.Fingerprint(), "install", wslReview(status), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	review := wslReview(status)
	if _, err := ledger.RetireWSLTrustReceipt(context.Background(), installation, authority.Fingerprint(), review, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.WSLTrust(context.Background(), client, installation, authority.Fingerprint(), "remove", review, time.Now()); err == nil {
		t.Fatal("completed retirement adopted restored trust")
	}
	calls := client.statuses
	info, err := ledger.BeginAuthorityRotation(store, installation, authority.Fingerprint(), settings.Revision, ServicePorts{HTTP: 18080, HTTPS: 18443}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"present", "unreadable"} {
		registry.present = fault == "present"
		if fault == "unreadable" {
			registry.err = errors.New("registry unavailable")
		}
		if _, err := ledger.ResumeAuthorityRotation(store, installation, info.OldRootSHA256, info.NewRootSHA256, time.Now()); err == nil {
			t.Fatal("rotation resumed with unconfirmed retired registration", fault)
		}
		current, err := ledger.LoadSettings()
		if err != nil || current.Revision != settings.Revision {
			t.Fatal("refused rotation changed settings", current, err)
		}
	}
	registry.present, registry.err = false, nil
	if _, err := ledger.ResumeAuthorityRotation(store, installation, info.OldRootSHA256, info.NewRootSHA256, time.Now()); err != nil {
		t.Fatal(err)
	}
	if client.statuses != calls || client.mutations != 1 || store.adds != 0 || store.removes != 0 {
		t.Fatal("retirement/rotation changed a guest or root store")
	}
}

func TestWSLRetirementFailedCompensatingWriteStillBlocksReturnedGUID(t *testing.T) {
	ledger, authority, installation, client, registry, review := retirementFixture(t)
	registry.read = func(call int) (bool, error) { return call >= 3, nil }
	writes := 0
	ledger.trustReplace = func(source, destination string) error {
		writes++
		if writes == 3 {
			return errors.New("compensating receipt replacement failed")
		}
		return replaceSettings(source, destination)
	}
	info, err := ledger.RetireWSLTrustReceipt(context.Background(), installation, authority.Fingerprint(), review, time.Now())
	if err == nil || info.State != "registration-retired" || info.RegistrationState != "present" {
		t.Fatal("uncertain committed retirement misreported", info, err)
	}
	if err := ledger.rotationWSLTrustSettled(installation, client.user); err == nil {
		t.Fatal("failed compensating write allowed rotation despite returned GUID")
	}
	if client.mutations != 1 {
		t.Fatal("compensation changed guest trust")
	}
}

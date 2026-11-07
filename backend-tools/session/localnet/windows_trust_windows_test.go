//go:build windows

package localnet

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fixtureRootStore struct {
	user           string
	roots          map[string]bool
	fault          string
	adds, removes  int
	unreadable     bool
	beforeMutation func(string)
}

func (store *fixtureRootStore) UserID() string { return store.user }
func (store *fixtureRootStore) Close() error   { return nil }
func (store *fixtureRootStore) Contains(der []byte) (bool, error) {
	if store.unreadable {
		return false, errors.New("store read failed")
	}
	return store.roots[string(der)], nil
}
func (store *fixtureRootStore) Add(der []byte) error {
	store.adds++
	if store.beforeMutation != nil {
		store.beforeMutation("install-intent")
	}
	if store.fault == "deny-add" {
		return errors.New("trust denied")
	}
	store.roots[string(der)] = true
	if store.fault == "accepted-add" {
		return errors.New("add acknowledgement lost")
	}
	if store.fault == "inspect-add" {
		store.unreadable = true
	}
	return nil
}
func (store *fixtureRootStore) Remove(der []byte) error {
	store.removes++
	if store.beforeMutation != nil {
		store.beforeMutation("remove-intent")
	}
	if store.fault == "deny-remove" {
		return errors.New("remove denied")
	}
	delete(store.roots, string(der))
	if store.fault == "accepted-remove" {
		return errors.New("remove acknowledgement lost")
	}
	return nil
}
func trustFixture(t *testing.T) (*NamespaceLedger, *Authority, string, *fixtureRootStore) {
	t.Helper()
	ledger, authority, installation, _ := authorityStoreFixture(t)
	store := &fixtureRootStore{user: "fixture-user", roots: map[string]bool{}}
	store.beforeMutation = func(want string) {
		record, err := ledger.readWindowsTrust(installation, store.user, authority.Fingerprint())
		if err != nil || record == nil || record.State != want {
			t.Fatal("store mutation preceded durable intent", want, record, err)
		}
	}
	return ledger, authority, installation, store
}
func TestWindowsTrustRecordsOwnershipBeforeMutationAndRemovesWithoutPrivateState(t *testing.T) {
	ledger, authority, installation, store := trustFixture(t)
	root := authority.Fingerprint()
	status, err := ledger.InstallWindowsTrust(store, installation, root, authority.root.NotBefore.AddDate(0, 0, 1))
	if err != nil || !status.Present || !status.Managed || status.State != "installed" {
		t.Fatal(status, err)
	}
	if _, err := ledger.InstallWindowsTrust(store, installation, root, authority.root.NotBefore.AddDate(0, 0, 1)); err != nil || store.adds != 1 {
		t.Fatal("install replayed", err, store.adds)
	}
	if err := os.Remove(filepath.Join(ledger.directory, authorityStateName)); err != nil {
		t.Fatal(err)
	}
	status, err = ledger.RemoveWindowsTrust(store, installation, root)
	if err != nil || status.Present || status.Managed || status.State != "removed" || store.removes != 1 {
		t.Fatal(status, err, store.removes)
	}
	if _, err := ledger.RemoveWindowsTrust(store, installation, root); err != nil || store.removes != 1 {
		t.Fatal("removal replayed", err)
	}
	store.roots[string(authority.RootCertificate())] = true
	if status, err := ledger.RemoveWindowsTrust(store, installation, root); err == nil || status.Managed || store.removes != 1 {
		t.Fatal("external re-add adopted", status, err)
	}
}
func TestWindowsTrustPreservesPreexistingRootAndUnrelatedCertificates(t *testing.T) {
	ledger, authority, installation, store := trustFixture(t)
	store.roots[string(authority.RootCertificate())] = true
	store.roots["unrelated"] = true
	status, err := ledger.InstallWindowsTrust(store, installation, authority.Fingerprint(), authority.root.NotBefore.AddDate(0, 0, 1))
	if err != nil || !status.Present || status.Managed || status.State != "present-unmanaged" || store.adds != 0 {
		t.Fatal(status, err)
	}
	if _, err := os.Stat(ledger.trustPath(authority.Fingerprint())); !os.IsNotExist(err) {
		t.Fatal("preexisting root gained ownership receipt", err)
	}
	if _, err := ledger.RemoveWindowsTrust(store, installation, authority.Fingerprint()); err == nil {
		t.Fatal("preexisting root removed")
	}
	if len(store.roots) != 2 || store.removes != 0 {
		t.Fatal("preexisting or unrelated root changed")
	}
}
func TestWindowsTrustDenialAndUnknownAddRequireExplicitRetry(t *testing.T) {
	for _, fault := range []string{"deny-add", "accepted-add", "inspect-add"} {
		t.Run(fault, func(t *testing.T) {
			ledger, authority, installation, store := trustFixture(t)
			store.fault = fault
			root := authority.Fingerprint()
			status, err := ledger.InstallWindowsTrust(store, installation, root, authority.root.NotBefore.AddDate(0, 0, 1))
			if err == nil || status.State != "install-intent" || !status.Managed {
				t.Fatal(status, err)
			}
			if fault == "accepted-add" && (!status.Observed || !status.Present) {
				t.Fatal("unknown acceptance reported absent", status)
			}
			if fault == "inspect-add" && status.Observed {
				t.Fatal("failed observation claimed certainty", status)
			}
			store.fault = ""
			store.unreadable = false
			before, err := os.ReadFile(ledger.trustPath(root))
			if err != nil {
				t.Fatal(err)
			}
			observed, err := ledger.WindowsTrustStatus(store, installation, root)
			if err != nil || observed.State != "install-intent" {
				t.Fatal("read completed a mutation", observed, err)
			}
			after, err := os.ReadFile(ledger.trustPath(root))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("status rewrote receipt", err)
			}
			status, err = ledger.InstallWindowsTrust(store, installation, root, authority.root.NotBefore.AddDate(0, 0, 1))
			if err != nil || status.State != "installed" || !status.Present {
				t.Fatal(status, err)
			}
			wantAdds := 1
			if fault == "deny-add" {
				wantAdds = 2
			}
			if store.adds != wantAdds {
				t.Fatal("accepted install replayed", store.adds)
			}
		})
	}
}
func TestWindowsTrustRemovalDenialAndUnknownAcceptance(t *testing.T) {
	for _, fault := range []string{"deny-remove", "accepted-remove"} {
		t.Run(fault, func(t *testing.T) {
			ledger, authority, installation, store := trustFixture(t)
			root := authority.Fingerprint()
			if _, err := ledger.InstallWindowsTrust(store, installation, root, authority.root.NotBefore.AddDate(0, 0, 1)); err != nil {
				t.Fatal(err)
			}
			store.fault = fault
			status, err := ledger.RemoveWindowsTrust(store, installation, root)
			if err == nil {
				t.Fatal("failed acknowledgement reported success")
			}
			if fault == "deny-remove" && (!status.Present || status.State != "remove-intent") {
				t.Fatal(status)
			}
			if fault == "deny-remove" {
				if _, err := ledger.InstallWindowsTrust(store, installation, root, authority.root.NotBefore.AddDate(0, 0, 1)); err == nil || store.adds != 1 {
					t.Fatal("pending removal was overwritten by install", err)
				}
			}
			if fault == "accepted-remove" && (status.Present || status.State != "removed") {
				t.Fatal(status)
			}
			store.fault = ""
			if _, err := ledger.RemoveWindowsTrust(store, installation, root); err != nil {
				t.Fatal(err)
			}
			wantRemoves := 1
			if fault == "deny-remove" {
				wantRemoves = 2
			}
			if store.removes != wantRemoves {
				t.Fatal("accepted removal replayed", store.removes)
			}
		})
	}
}
func TestWindowsTrustRefusesForeignAndPartialReceiptsWithoutStoreMutation(t *testing.T) {
	ledger, authority, installation, store := trustFixture(t)
	root := authority.Fingerprint()
	if _, err := ledger.InstallWindowsTrust(store, installation, root, authority.root.NotBefore.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	path := ledger.trustPath(root)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store.user = "another-user"
	if _, err := ledger.RemoveWindowsTrust(store, installation, root); err == nil {
		t.Fatal("foreign user receipt accepted")
	}
	store.user = "fixture-user"
	if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.RemoveWindowsTrust(store, installation, root); err == nil {
		t.Fatal("partial receipt accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != "partial" || store.removes != 0 || store.adds != 1 {
		t.Fatal("refusal changed root or evidence", err)
	}
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsTrustReceiptFailureNeverReplaysAcceptedMutation(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		failAt    int
		committed bool
	}{
		{"intent-committed-with-error", 1, true},
		{"installed-not-committed", 2, false},
		{"installed-committed-with-error", 2, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ledger, authority, installation, store := trustFixture(t)
			writes := 0
			ledger.trustReplace = func(source, destination string) error {
				writes++
				if writes != testCase.failAt {
					return replaceSettings(source, destination)
				}
				if testCase.committed {
					if err := replaceSettings(source, destination); err != nil {
						return err
					}
				}
				return errors.New("receipt write acknowledgement lost")
			}
			root := authority.Fingerprint()
			status, err := ledger.InstallWindowsTrust(store, installation, root, authority.root.NotBefore.AddDate(0, 0, 1))
			if err == nil || !status.Observed {
				t.Fatal("uncertain receipt write was hidden", status, err)
			}
			if testCase.failAt == 1 && store.adds != 0 {
				t.Fatal("root changed after unconfirmed intent flush")
			}
			if testCase.failAt == 2 && (!status.Present || store.adds != 1) {
				t.Fatal("accepted root was reported absent", status, store.adds)
			}
			status, err = ledger.InstallWindowsTrust(store, installation, root, authority.root.NotBefore.AddDate(0, 0, 1))
			if err != nil || !status.Present || status.State != "installed" || store.adds != 1 {
				t.Fatal("explicit recovery replayed an accepted add", status, err, store.adds)
			}
		})
	}
}

func TestWindowsTrustRemovalSettlementFailurePreservesKnownAbsence(t *testing.T) {
	ledger, authority, installation, store := trustFixture(t)
	root := authority.Fingerprint()
	if _, err := ledger.InstallWindowsTrust(store, installation, root, authority.root.NotBefore.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	writes := 0
	ledger.trustReplace = func(source, destination string) error {
		writes++
		if writes == 2 {
			return errors.New("removal settlement write failed")
		}
		return replaceSettings(source, destination)
	}
	status, err := ledger.RemoveWindowsTrust(store, installation, root)
	if err == nil || !status.Observed || status.Present || status.State != "remove-intent" {
		t.Fatal(status, err)
	}
	status, err = ledger.RemoveWindowsTrust(store, installation, root)
	if err != nil || status.Present || status.State != "removed" || store.removes != 1 {
		t.Fatal("known absence replayed a deletion", status, err, store.removes)
	}
}

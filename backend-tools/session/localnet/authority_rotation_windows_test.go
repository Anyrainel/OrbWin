//go:build windows

package localnet

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func rotationFixture(t *testing.T) (*NamespaceLedger, *Authority, string, *fixtureRootStore, SettingsSnapshot) {
	t.Helper()
	ledger, authority, installation, store := trustFixture(t)
	settings := DefaultSettings()
	settings.RootSHA256 = authority.Fingerprint()
	settings.Networks = []string{strings.Repeat("d", 64)}
	settings.Preferences = []ServicePreference{{Selector: ServiceSelector{ContainerID: strings.Repeat("a", 64)}, Disabled: true}}
	stored, err := ledger.StoreSettings(0, settings)
	if err != nil {
		t.Fatal(err)
	}
	return ledger, authority, installation, store, stored
}
func TestAuthorityRotationChangesProtectedAuthorityAndPortsWithoutTrustMutation(t *testing.T) {
	ledger, authority, installation, store, old := rotationFixture(t)
	// Old signer expiry must not force deletion/reset of the public state.
	now := time.Now().AddDate(3, 0, 0)
	ports := ServicePorts{HTTP: 18080, HTTPS: 18443}
	info, err := ledger.BeginAuthorityRotation(store, installation, authority.Fingerprint(), old.Revision, ports, now)
	if err != nil {
		t.Fatal(err)
	}
	if info.NewRootSHA256 == info.OldRootSHA256 || info.OldRootSHA256 != authority.Fingerprint() {
		t.Fatal(info)
	}
	committed, err := ledger.ResumeAuthorityRotation(store, installation, info.OldRootSHA256, info.NewRootSHA256, now)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Revision != old.Revision+1 || committed.Settings.Enabled || committed.Settings.RootSHA256 != info.NewRootSHA256 || committed.Settings.Ports != ports || !reflect.DeepEqual(committed.Settings.Preferences, old.Settings.Preferences) || !reflect.DeepEqual(committed.Settings.Networks, old.Settings.Networks) {
		t.Fatal(committed)
	}
	loaded, err := ledger.LoadAuthority(installation, info.NewRootSHA256, now)
	if err != nil || loaded.RevocationHTTPPort() != ports.HTTP {
		t.Fatal(err)
	}
	if pending, err := ledger.AuthorityRotationInfo(installation); err != nil || pending != nil {
		t.Fatal("settled rotation retained journal", pending, err)
	}
	for _, suffix := range []string{"old", "new"} {
		if _, err := os.Stat(ledger.rotationArtifact(info.NewRootSHA256, suffix)); !os.IsNotExist(err) {
			t.Fatal("protected recovery artifact retained", suffix, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(ledger.directory, "authority-retired-"+info.OldRootSHA256+".pem")); err != nil || !bytes.Equal(data, info.OldCertificatePEM) {
		t.Fatal("public cleanup certificate not retained", err)
	}
	if store.adds != 0 || store.removes != 0 {
		t.Fatal("rotation changed trust stores")
	}
}
func TestAuthorityRotationPreflightPreservesActiveState(t *testing.T) {
	for _, fault := range []string{"trusted", "owned-trust-pending", "stale", "wrong-root", "enabled", "invalid-ports"} {
		t.Run(fault, func(t *testing.T) {
			ledger, authority, installation, store, old := rotationFixture(t)
			root, revision := authority.Fingerprint(), old.Revision
			ports := ServicePorts{HTTP: 18080, HTTPS: 18443}
			switch fault {
			case "trusted":
				store.roots[string(authority.RootCertificate())] = true
			case "owned-trust-pending":
				if _, err := ledger.InstallWindowsTrust(store, installation, root, time.Now()); err != nil {
					t.Fatal(err)
				}
				delete(store.roots, string(authority.RootCertificate()))
			case "stale":
				revision++
			case "wrong-root":
				root = strings.Repeat("0", 64)
			case "enabled":
				settings := old.Settings
				settings.Enabled = true
				stored, err := ledger.StoreSettings(revision, settings)
				if err != nil {
					t.Fatal(err)
				}
				revision = stored.Revision
			case "invalid-ports":
				ports.HTTP = 80
			}
			path := filepath.Join(ledger.directory, authorityStateName)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ledger.BeginAuthorityRotation(store, installation, root, revision, ports, time.Now()); err == nil {
				t.Fatal("unsafe rotation preflight accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("preflight changed active authority", err)
			}
			if info, err := ledger.AuthorityRotationInfo(installation); err != nil || info != nil {
				t.Fatal("refused preflight published a rotation", info, err)
			}
		})
	}
}
func TestAuthorityRotationResumesUnknownAcceptedReplacementWithoutMintingAnotherRoot(t *testing.T) {
	ledger, authority, installation, store, old := rotationFixture(t)
	info, err := ledger.BeginAuthorityRotation(store, installation, authority.Fingerprint(), old.Revision, ServicePorts{HTTP: 18080, HTTPS: 18443}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ledger.rotationReplace = func(source, destination string) error {
		if err := replaceSettings(source, destination); err != nil {
			return err
		}
		return errors.New("authority switch acknowledgement lost")
	}
	if _, err := ledger.ResumeAuthorityRotation(store, installation, info.OldRootSHA256, info.NewRootSHA256, time.Now()); err == nil {
		t.Fatal("uncertain switch reported success")
	}
	selected, err := ledger.AuthorityInfo(installation)
	if err != nil || selected.RootSHA256 != info.NewRootSHA256 {
		t.Fatal(selected, err)
	}
	if err := ledger.CancelAuthorityRotation(installation, info.OldRootSHA256, info.NewRootSHA256); err == nil {
		t.Fatal("accepted switch silently rolled back")
	}
	if _, err := ledger.BeginAuthorityRotation(store, installation, authority.Fingerprint(), old.Revision, ServicePorts{HTTP: 28080, HTTPS: 28443}, time.Now()); err == nil {
		t.Fatal("pending transaction minted another root")
	}
	ledger.Close()
	reopened, err := OpenNamespaceLedger(ledger.directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	stored, err := reopened.ResumeAuthorityRotation(store, installation, info.OldRootSHA256, info.NewRootSHA256, time.Now())
	if err != nil || stored.Settings.RootSHA256 != info.NewRootSHA256 || stored.Revision != old.Revision+1 {
		t.Fatal(stored, err)
	}
}
func TestAuthorityRotationResumesAfterRealWindowsCleanupSharingFailure(t *testing.T) {
	ledger, authority, installation, store, old := rotationFixture(t)
	info, err := ledger.BeginAuthorityRotation(store, installation, authority.Fingerprint(), old.Revision, ServicePorts{HTTP: 18080, HTTPS: 18443}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	backup, err := windows.UTF16PtrFromString(ledger.rotationArtifact(info.NewRootSHA256, "old"))
	if err != nil {
		t.Fatal(err)
	}
	// This private test file can be read but cannot be unlinked until the handle
	// closes. The new authority/settings should commit before cleanup fails.
	handle, err := windows.CreateFile(backup, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
	}()
	committed, err := ledger.ResumeAuthorityRotation(store, installation, info.OldRootSHA256, info.NewRootSHA256, time.Now())
	if err == nil || committed.Revision != info.TargetRevision {
		t.Fatal("sharing failure was hidden or commit lost", committed, err)
	}
	if pending, err := ledger.AuthorityRotationInfo(installation); err != nil || pending == nil {
		t.Fatal("cleanup failure lost recovery journal", pending, err)
	}
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	handle = 0
	settled, err := ledger.ResumeAuthorityRotation(store, installation, info.OldRootSHA256, info.NewRootSHA256, time.Now())
	if err != nil || !reflect.DeepEqual(committed, settled) {
		t.Fatal("settlement replayed settings", settled, err)
	}
}
func TestAuthorityRotationCancellationAndTamperRefusal(t *testing.T) {
	for _, fault := range []string{"cancel", "new-stage", "old-backup", "active-authority", "settings", "journal"} {
		t.Run(fault, func(t *testing.T) {
			ledger, authority, installation, store, old := rotationFixture(t)
			path := filepath.Join(ledger.directory, authorityStateName)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			info, err := ledger.BeginAuthorityRotation(store, installation, authority.Fingerprint(), old.Revision, ServicePorts{HTTP: 18080, HTTPS: 18443}, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if fault == "cancel" {
				if err := ledger.CancelAuthorityRotation(installation, info.OldRootSHA256, info.NewRootSHA256); err != nil {
					t.Fatal(err)
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("cancel changed authority", err)
				}
				settings, err := ledger.LoadSettings()
				if err != nil || !reflect.DeepEqual(settings, old) {
					t.Fatal("cancel changed settings", err)
				}
				return
			}
			switch fault {
			case "new-stage":
				err = os.WriteFile(ledger.rotationArtifact(info.NewRootSHA256, "new"), []byte("tampered"), 0600)
			case "old-backup":
				err = os.WriteFile(ledger.rotationArtifact(info.NewRootSHA256, "old"), []byte("tampered"), 0600)
			case "active-authority":
				err = os.WriteFile(path, []byte("tampered"), 0600)
			case "journal":
				err = os.WriteFile(ledger.rotationPath(), []byte("partial"), 0600)
			case "settings":
				changed := old.Settings
				changed.Ports.HTTP = 24080
				changed.Ports.HTTPS = 24443
				_, err = ledger.StoreSettings(old.Revision, changed)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ledger.ResumeAuthorityRotation(store, installation, info.OldRootSHA256, info.NewRootSHA256, time.Now()); err == nil {
				t.Fatal("unexpected state accepted")
			}
			if err := ledger.CancelAuthorityRotation(installation, info.OldRootSHA256, info.NewRootSHA256); err == nil {
				t.Fatal("unexpected state deleted during cancel")
			}
			if _, err := os.Stat(ledger.rotationPath()); err != nil {
				t.Fatal("failed resume/cancel discarded journal", err)
			}
		})
	}
}

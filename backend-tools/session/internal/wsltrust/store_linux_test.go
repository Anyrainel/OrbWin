//go:build linux

package wsltrust

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type updaterFixture struct {
	calls                          int
	failBefore, failAfter, warning bool
}

func TestReviewedMachineIdentityRequiredBeforeCreatingTrustState(t *testing.T) {
	store, request, updater := fixtureStore(t)
	request.ExpectedMachineID = strings.Repeat("b", 32)
	status, err := store.Apply(context.Background(), request)
	if err == nil || status.MachineID != strings.Repeat("a", 32) || status.Observed || updater.calls != 0 {
		t.Fatalf("changed machine accepted: %+v %v", status, err)
	}
	if _, err := store.root.Lstat(store.paths.state); !os.IsNotExist(err) {
		t.Fatalf("stale first-install identity created trust state: %v", err)
	}
}

func TestAbsentRootRemovalPreventsDelayedFirstInstall(t *testing.T) {
	store, delayedInstall, updater := fixtureStore(t)
	removal := delayedInstall
	removal.Operation = "remove"
	status, err := store.Apply(context.Background(), removal)
	if err != nil || !status.Observed || status.State != "removed" || status.Sequence == 0 || status.FilePresent || status.BundlePresent || status.Managed || updater.calls != 0 {
		t.Fatalf("absent-root removal not durably settled: %+v %v", status, err)
	}
	status, err = store.Apply(context.Background(), delayedInstall)
	if err == nil || status.State != "removed" || status.FilePresent || status.BundlePresent || updater.calls != 0 {
		t.Fatalf("delayed sequence-zero install accepted after removal: %+v %v", status, err)
	}
	if _, err := store.root.Lstat(store.anchor(delayedInstall)); !os.IsNotExist(err) {
		t.Fatalf("delayed install created anchor: %v", err)
	}
}

func fixtureStore(t *testing.T) (*Store, Request, *updaterFixture) {
	t.Helper()
	boundary := t.TempDir()
	paths := storePaths{anchors: "usr/local/share/ca-certificates", bundle: "etc/ssl/certs/ca-certificates.crt", certs: "etc/ssl/certs", state: "var/lib/orbwin/trust", osRelease: "etc/os-release", machine: "etc/machine-id", updater: "usr/sbin/update-ca-certificates", config: "etc/ca-certificates.conf", owner: uint32(os.Geteuid())}
	for _, path := range []string{paths.anchors, paths.certs, "usr/sbin", "usr/share/ca-certificates", "etc/ca-certificates/update.d", "var/lib"} {
		if err := os.MkdirAll(filepath.Join(boundary, path), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for path, data := range map[string]string{paths.bundle: "", paths.osRelease: "ID=ubuntu\nVERSION_ID=\"24.04\"\n", paths.machine: strings.Repeat("a", 32) + "\n", paths.config: "# private fixture\n", paths.updater: "#!/bin/sh\nexit 0\n"} {
		if err := os.WriteFile(filepath.Join(boundary, path), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(boundary, paths.updater), 0755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(boundary)
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{root: root, paths: paths, now: time.Now}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	request := fixtureRequest(t)
	updater := &updaterFixture{}
	store.update = func(_ context.Context, lock *os.File) (bool, error) {
		updater.calls++
		if lock == nil {
			return false, errors.New("updater ran without inherited operation lock")
		}
		record, err := store.load(request, strings.Repeat("a", 32))
		if err != nil || record == nil || (record.State != "install-intent" && record.State != "remove-intent") {
			return false, errors.New("updater ran before durable intent")
		}
		if updater.failBefore {
			return false, errors.New("updater refused")
		}
		entries, err := os.ReadDir(filepath.Join(boundary, paths.anchors))
		if err != nil {
			return false, err
		}
		var bundle []byte
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".crt") {
				data, err := os.ReadFile(filepath.Join(boundary, paths.anchors, entry.Name()))
				if err != nil {
					return false, err
				}
				bundle = append(bundle, data...)
			}
		}
		if err := os.WriteFile(filepath.Join(boundary, paths.bundle), bundle, 0644); err != nil {
			return false, err
		}
		if updater.failAfter {
			return false, errors.New("accepted updater acknowledgement lost")
		}
		return updater.warning, nil
	}
	return store, request, updater
}
func nextRequest(request Request, status Status, operation string) Request {
	request.Operation = operation
	request.ExpectedSequence = status.Sequence
	return request
}
func TestOwnedDistroTrustInstallRemoveAndExternalReaddition(t *testing.T) {
	store, request, updater := fixtureStore(t)
	status, err := store.Apply(context.Background(), request)
	if err != nil || !status.Observed || !status.FilePresent || !status.BundlePresent || !status.Managed || status.State != "installed" {
		t.Fatal(status, err)
	}
	request = nextRequest(request, status, "install")
	if _, err := store.Apply(context.Background(), request); err != nil || updater.calls != 1 {
		t.Fatal("accepted install replayed", err, updater.calls)
	}
	status, err = store.Apply(context.Background(), nextRequest(request, status, "remove"))
	if err != nil || status.FilePresent || status.BundlePresent || status.Managed || status.State != "removed" {
		t.Fatal(status, err)
	}
	if updater.calls != 2 {
		t.Fatal(updater.calls)
	}
	if err := store.root.WriteFile(store.anchor(request), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: request.RootCertificate}), 0644); err != nil {
		t.Fatal(err)
	}
	if observed, err := store.Apply(context.Background(), nextRequest(request, status, "remove")); err == nil || observed.Managed || updater.calls != 2 {
		t.Fatal("external root re-addition was adopted", observed, err)
	}
}
func TestDistroTrustPreservesPreexistingRootAndStatusDoesNotWrite(t *testing.T) {
	store, request, updater := fixtureStore(t)
	request.Operation = "status"
	if _, err := store.Apply(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := store.root.Lstat(store.paths.state); !os.IsNotExist(err) {
		t.Fatal("status created state directory", err)
	}
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: request.RootCertificate})
	if err := store.root.WriteFile(store.anchor(request), cert, 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.root.WriteFile(store.paths.bundle, cert, 0644); err != nil {
		t.Fatal(err)
	}
	request.Operation = "install"
	status, err := store.Apply(context.Background(), request)
	if err != nil || status.Managed || status.State != "present-unmanaged" || updater.calls != 0 {
		t.Fatal(status, err)
	}
	if _, err := store.Apply(context.Background(), nextRequest(request, status, "remove")); err == nil {
		t.Fatal("unmanaged root removed")
	}
}
func TestDistroTrustUnknownUpdaterResultUsesObservedBundleWithoutReplay(t *testing.T) {
	for _, fault := range []string{"before", "after", "hook-warning"} {
		t.Run(fault, func(t *testing.T) {
			store, request, updater := fixtureStore(t)
			updater.failBefore = fault == "before"
			updater.failAfter = fault == "after"
			updater.warning = fault == "hook-warning"
			status, err := store.Apply(context.Background(), request)
			if fault != "hook-warning" && err == nil {
				t.Fatal("updater failure hidden")
			}
			if !status.Observed || !status.FilePresent || !status.UpdaterWarning {
				t.Fatal(status, err)
			}
			updater.failBefore = false
			updater.failAfter = false
			status, err = store.Apply(context.Background(), nextRequest(request, status, "install"))
			if err != nil || !status.BundlePresent || status.State != "installed" {
				t.Fatal(status, err)
			}
			want := 1
			if fault == "before" {
				want = 2
			}
			if updater.calls != want {
				t.Fatal("accepted updater replayed", updater.calls, want)
			}
		})
	}
}
func TestDistroTrustRefusesStaleForeignAndChangedFiles(t *testing.T) {
	for _, fault := range []string{"stale", "registration", "user", "machine", "anchor", "receipt"} {
		t.Run(fault, func(t *testing.T) {
			store, request, updater := fixtureStore(t)
			status, err := store.Apply(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			request = nextRequest(request, status, "remove")
			switch fault {
			case "stale":
				request.ExpectedSequence = 0
			case "registration":
				request.DistributionID = "00000000-0000-4000-8000-000000000003"
			case "user":
				request.WindowsUser = "S-1-5-21-2"
			case "machine":
				err = store.root.WriteFile(store.paths.machine, []byte(strings.Repeat("b", 32)), 0644)
			case "anchor":
				err = store.root.WriteFile(store.anchor(request), []byte("changed"), 0644)
			case "receipt":
				err = store.root.WriteFile(store.recordPath(request), []byte("partial"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Apply(context.Background(), request); err == nil {
				t.Fatal("unsafe removal accepted")
			}
			if updater.calls != 1 {
				t.Fatal("refusal invoked updater")
			}
			if _, err := store.root.Lstat(store.anchor(request)); err != nil {
				t.Fatal("refusal removed anchor", err)
			}
		})
	}
}
func TestDistroTrustRefusesUnsafePathsBeforeCreatingState(t *testing.T) {
	for _, fault := range []string{"writable", "symlink", "empty-config", "unsupported"} {
		t.Run(fault, func(t *testing.T) {
			store, request, updater := fixtureStore(t)
			var err error
			switch fault {
			case "writable":
				err = store.root.Chmod(store.paths.anchors, 0777)
			case "symlink":
				err = store.root.Rename(store.paths.anchors, store.paths.anchors+"-original")
				if err == nil {
					err = store.root.Symlink("ca-certificates-original", store.paths.anchors)
				}
			case "empty-config":
				err = store.root.WriteFile(store.paths.config, nil, 0644)
			case "unsupported":
				err = store.root.WriteFile(store.paths.osRelease, []byte("ID=other\nVERSION_ID=1\n"), 0644)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Apply(context.Background(), request); err == nil {
				t.Fatal("unsafe trust layout accepted")
			}
			if updater.calls != 0 {
				t.Fatal("unsafe layout invoked updater")
			}
			if _, err := store.root.Lstat(store.paths.state); !os.IsNotExist(err) {
				t.Fatal("unsafe layout created state", err)
			}
		})
	}
}
func TestDistroTrustRemovesOnlyExactGeneratedLinksAndRetainsDrafts(t *testing.T) {
	store, request, _ := fixtureStore(t)
	status, err := store.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	pemName := store.base(request) + ".pem"
	for name, target := range map[string]string{pemName: filepath.Join(store.root.Name(), store.anchor(request)), "01234567.0": pemName, "other-link": "other.pem"} {
		if err := store.root.Symlink(target, filepath.Join(store.paths.certs, name)); err != nil {
			t.Fatal(err)
		}
	}
	draft := filepath.Join(store.paths.state, ".receipt-abandoned.partial")
	if err := store.root.WriteFile(draft, []byte("retain"), 0600); err != nil {
		t.Fatal(err)
	}
	status, err = store.Apply(context.Background(), nextRequest(request, status, "remove"))
	if err != nil {
		t.Fatal(status, err)
	}
	for _, name := range []string{pemName, "01234567.0"} {
		if _, err := store.root.Lstat(filepath.Join(store.paths.certs, name)); !os.IsNotExist(err) {
			t.Fatal("owned link retained", name, err)
		}
	}
	if _, err := store.root.Lstat(filepath.Join(store.paths.certs, "other-link")); err != nil {
		t.Fatal("unrelated link removed", err)
	}
	if data, err := store.root.ReadFile(draft); err != nil || string(data) != "retain" {
		t.Fatal("unconfirmed draft discarded", err)
	}
}
func TestBundleScanLimitIsUnknownRatherThanFalseAbsence(t *testing.T) {
	store, request, _ := fixtureStore(t)
	other := fixtureRequest(t)
	block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: other.RootCertificate})
	bundle := append(bytes.Repeat(block, 4096), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: request.RootCertificate})...)
	if err := store.root.WriteFile(store.paths.bundle, bundle, 0644); err != nil {
		t.Fatal(err)
	}
	request.Operation = "status"
	if status, err := store.Apply(context.Background(), request); err == nil || status.Observed {
		t.Fatal("oversized scan reported absent", status, err)
	}
}

func TestDistroTrustLockSerializesMutationAndBoundsStatusCancellation(t *testing.T) {
	store, request, updater := fixtureStore(t)
	original := store.update
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	store.update = func(ctx context.Context, lock *os.File) (bool, error) {
		close(entered)
		<-release
		return original(ctx, lock)
	}
	finished := make(chan error, 1)
	go func() { _, err := store.Apply(context.Background(), request); finished <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("update did not reach held transaction")
	}
	read := request
	read.Operation = "status"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := store.Apply(ctx, read); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("status crossed an active update or failed to cancel", err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if status, err := store.Apply(context.Background(), request); err == nil || !status.Managed {
		t.Fatal("stale concurrent mutation was accepted", status, err)
	}
	if updater.calls != 1 {
		t.Fatal("stale mutation replayed updater", updater.calls)
	}
}

func TestNativeUbuntuUpdaterInPrivateDirectories(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_WSL_TRUST_SANDBOX") != "1" {
		t.Skip("explicit private-directory native updater qualification")
	}
	const updater = "/usr/sbin/update-ca-certificates"
	const systemBundle = "/etc/ssl/certs/ca-certificates.crt"
	beforeUpdater, err := os.ReadFile(updater)
	if err != nil {
		t.Fatal(err)
	}
	beforeBundle, err := os.ReadFile(systemBundle)
	if err != nil {
		t.Fatal(err)
	}
	store, request, _ := fixtureStore(t)
	other := fixtureRequest(t)
	otherPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: other.RootCertificate})
	if err := store.root.WriteFile(filepath.Join(store.paths.anchors, "other.crt"), otherPem, 0644); err != nil {
		t.Fatal(err)
	}
	full := func(path string) string { return filepath.Join(store.root.Name(), path) }
	args := []string{"--certsconf", full(store.paths.config), "--certsdir", full("usr/share/ca-certificates"), "--localcertsdir", full(store.paths.anchors), "--etccertsdir", full(store.paths.certs), "--hooksdir", full("etc/ca-certificates/update.d")}
	store.update = func(_ context.Context, lock *os.File) (bool, error) { return runUpdater(updater, args, lock) }
	status, err := store.Apply(context.Background(), request)
	if err != nil || !status.BundlePresent {
		t.Fatal(status, err)
	}
	verify := exec.Command("/usr/bin/openssl", "verify", "-CAfile", full(store.paths.bundle), full(store.anchor(request)))
	if output, err := verify.CombinedOutput(); err != nil {
		t.Fatal("native bundle did not verify root", err, string(output))
	}
	status, err = store.Apply(context.Background(), nextRequest(request, status, "remove"))
	if err != nil || status.BundlePresent || status.FilePresent {
		t.Fatal(status, err)
	}
	bundle, err := store.root.ReadFile(store.paths.bundle)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := containsRoot(bundle, other.RootCertificate); err != nil || !found {
		t.Fatal("unrelated root lost", found, err)
	}
	afterUpdater, err := os.ReadFile(updater)
	if err != nil {
		t.Fatal(err)
	}
	afterBundle, err := os.ReadFile(systemBundle)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeUpdater, afterUpdater) || !bytes.Equal(beforeBundle, afterBundle) {
		t.Fatal("native qualification modified a system updater/bundle")
	}
	t.Logf("private updater install/verify/remove passed; system updater %s and bundle %s unchanged; Linux uid %d", Digest(afterUpdater), Digest(afterBundle), os.Geteuid())
}

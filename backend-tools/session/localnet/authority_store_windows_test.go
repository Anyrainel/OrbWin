//go:build windows

package localnet

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func authorityStoreFixture(t *testing.T) (*NamespaceLedger, *Authority, string, time.Time) {
	t.Helper()
	ledger, err := OpenNamespaceLedger(t.TempDir(), "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ledger.Close() })
	installation := strings.Repeat("c", 32)
	now := time.Now().UTC()
	authority, err := ledger.CreateAuthority(installation, now)
	if err != nil {
		t.Fatal(err)
	}
	return ledger, authority, installation, now
}
func TestAuthorityStoreProtectsAndReopensExactSigningState(t *testing.T) {
	ledger, authority, installation, now := authorityStoreFixture(t)
	path := filepath.Join(ledger.directory, authorityStateName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state authorityState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(state.ProtectedSigning, []byte(protectedKeyHeader)) {
		t.Fatal("signing state was not OS-protected")
	}
	plain, err := authority.PrivateBundle()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(plain)
	key, err := x509.MarshalPKCS8PrivateKey(authority.signer)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	if bytes.Contains(data, plain) || bytes.Contains(data, []byte(base64.StdEncoding.EncodeToString(plain))) || bytes.Contains(data, []byte(base64.StdEncoding.EncodeToString(key))) {
		t.Fatal("private key/bundle appeared in persisted plaintext")
	}
	info, err := ledger.AuthorityInfo(installation)
	if err != nil {
		t.Fatal(err)
	}
	block, suffix := pem.Decode(info.PublicCertificatePEM)
	if block == nil || block.Type != "CERTIFICATE" || len(suffix) != 0 || !bytes.Equal(block.Bytes, authority.RootCertificate()) || info.RootSHA256 != authority.Fingerprint() {
		t.Fatal("public review/export contains a wrong certificate or private material")
	}
	loaded, err := ledger.LoadAuthority(installation, info.RootSHA256, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loaded.Leaf("stored.orbwin.localhost", now); err != nil {
		t.Fatal(err)
	}
	if !loaded.SigningExpiresAt().Before(info.RootNotAfter) {
		t.Fatal("signer/root expiry distinction was lost")
	}
	if _, err := ledger.CreateAuthority(installation, now); err == nil {
		t.Fatal("existing authority was silently replaced")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(data, after) {
		t.Fatal("duplicate setup changed committed authority state")
	}
	entries, _ := os.ReadDir(ledger.directory)
	if len(entries) != 2 {
		t.Fatal("successful setup left an encrypted draft")
	}
}
func TestAuthorityStoreRefusesForeignModifiedExpiredAndPartialState(t *testing.T) {
	for _, fault := range []string{"foreign-installation", "wrong-reviewed-root", "ciphertext", "public-root", "public-and-fingerprint", "missing-key", "version", "partial", "suffix", "oversized", "expired"} {
		t.Run(fault, func(t *testing.T) {
			ledger, authority, installation, now := authorityStoreFixture(t)
			path := filepath.Join(ledger.directory, authorityStateName)
			data, _ := os.ReadFile(path)
			var state authorityState
			_ = json.Unmarshal(data, &state)
			expected := authority.Fingerprint()
			loadInstallation := installation
			stamp := now
			switch fault {
			case "foreign-installation":
				loadInstallation = strings.Repeat("d", 32)
			case "wrong-reviewed-root":
				expected = strings.Repeat("0", 64)
			case "ciphertext":
				state.ProtectedSigning[len(state.ProtectedSigning)-1] ^= 1
			case "public-root":
				state.RootCertificate[len(state.RootCertificate)-1] ^= 1
			case "public-and-fingerprint":
				other, err := NewAuthority(installation, now)
				if err != nil {
					t.Fatal(err)
				}
				state.RootCertificate = other.RootCertificate()
				state.RootSHA256 = other.Fingerprint()
				expected = other.Fingerprint()
			case "missing-key":
				state.ProtectedSigning = nil
			case "version":
				state.Version = 2
			case "expired":
				stamp = authority.SigningExpiresAt().Add(time.Hour)
			}
			modified, _ := json.Marshal(state)
			switch fault {
			case "partial":
				modified = []byte(`{"version":1`)
			case "suffix":
				modified = append(modified, []byte(" {}")...)
			case "oversized":
				modified = bytes.Repeat([]byte{'x'}, authorityStateBudget+1)
			}
			if err := os.WriteFile(path, modified, 0600); err != nil {
				t.Fatal(err)
			}
			if loaded, err := ledger.LoadAuthority(loadInstallation, expected, stamp); err == nil || loaded != nil {
				t.Fatal("foreign/modified/expired signing authority was accepted")
			}
			if _, err := ledger.CreateAuthority(installation, now); err == nil {
				t.Fatal("failed load regenerated an authority over existing state")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(after, modified) {
				t.Fatal("refusal modified the authority evidence")
			}
		})
	}
}
func TestAuthorityStoreRefusesLinkedOrNonFileState(t *testing.T) {
	ledger, authority, installation, now := authorityStoreFixture(t)
	path := filepath.Join(ledger.directory, authorityStateName)
	saved := path + ".saved"
	if err := os.Rename(path, saved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.LoadAuthority(installation, authority.Fingerprint(), now); err == nil {
		t.Fatal("directory was treated as signing state")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(saved, path); err != nil {
		t.Log("file-symlink case unavailable without OS permission; directory refusal was tested")
		return
	}
	if _, err := ledger.LoadAuthority(installation, authority.Fingerprint(), now); err == nil {
		t.Fatal("linked signing-state path was followed")
	}
}
func TestAuthorityStoreCrashChild(t *testing.T) {
	directory := os.Getenv("ORBWIN_AUTHORITY_CRASH_TEST")
	if directory == "" {
		return
	}
	ledger, err := OpenNamespaceLedger(directory, "owned-daemon")
	if err != nil {
		os.Exit(92)
	}
	if _, err := ledger.CreateAuthority(strings.Repeat("c", 32), time.Now().UTC()); err != nil {
		os.Exit(93)
	}
	os.Exit(91)
}
func TestAuthorityStoreCommittedRootSurvivesProcessLossWithoutRegeneration(t *testing.T) {
	directory := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestAuthorityStoreCrashChild$")
	command.Env = append(os.Environ(), "ORBWIN_AUTHORITY_CRASH_TEST="+directory)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err := command.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 91 {
		t.Fatalf("authority crash fixture failed before commit: %v", err)
	}
	ledger, err := OpenNamespaceLedger(directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	installation := strings.Repeat("c", 32)
	info, err := ledger.AuthorityInfo(installation)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := ledger.LoadAuthority(installation, info.RootSHA256, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Fingerprint() != info.RootSHA256 {
		t.Fatal("process-loss recovery adopted a different public root")
	}
	if _, err := ledger.CreateAuthority(installation, time.Now().UTC()); err == nil {
		t.Fatal("process-loss recovery silently replaced a committed root")
	}
}

//go:build windows

package localnet

import (
	"bytes"
	"testing"
)

func TestUserProtectionRoundTripRefusesTamperingAndForeignOwnershipContext(t *testing.T) {
	authority, now := authorityFixture(t)
	bundle, err := authority.PrivateBundle()
	if err != nil {
		t.Fatal(err)
	}
	installation := "00000000-0000-4000-8000-000000000001"
	protected, err := ProtectPrivateBundle(bundle, installation, authority.Fingerprint())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(protected, bundle) {
		t.Fatal("protection returned a plaintext authority bundle")
	}
	plain, err := UnprotectPrivateBundle(protected, installation, authority.Fingerprint())
	if err != nil || !bytes.Equal(plain, bundle) {
		t.Fatalf("user protection changed signing state: %v", err)
	}
	if _, err = OpenAuthority(plain, authority.Fingerprint(), now); err != nil {
		t.Fatal(err)
	}
	if _, err = UnprotectPrivateBundle(protected, "00000000-0000-4000-8000-000000000002", authority.Fingerprint()); err == nil {
		t.Fatal("protected signing state adopted by a different installation")
	}
	changed := append([]byte(nil), protected...)
	changed[len(changed)-1] ^= 1
	if _, err = UnprotectPrivateBundle(changed, installation, authority.Fingerprint()); err == nil {
		t.Fatal("tampered protected key was accepted")
	}
	if _, err = ProtectPrivateBundle(bundle, installation, "not-a-fingerprint"); err == nil {
		t.Fatal("unbound protected key was created")
	}
}

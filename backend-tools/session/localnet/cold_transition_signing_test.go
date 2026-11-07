package localnet

import (
	"encoding/json"
	"errors"
	"orbwin.local/session/internal/sdkstate"
	"testing"
	"time"
)

// Pure semantic tests: the opener is injected; no DPAPI/native-state operation.
func TestColdSigningAdmissionRequiresLiveMatchingSignerAndFixedLegacyDomain(t *testing.T) {
	scope, files, now := coldStateFixture(t, false, false)
	delete(files, rotationStateName)
	ca, err := NewAuthority(scope.Installation, now)
	if err != nil {
		t.Fatal(err)
	}
	state := authorityState{Version: 1, Installation: scope.Installation, RootSHA256: ca.Fingerprint(), RootCertificate: ca.RootCertificate(), ProtectedSigning: []byte("opaque original ciphertext")}
	files[authorityStateName], _ = json.Marshal(state)
	var settings settingsRecord
	if err := json.Unmarshal(files[settingsName], &settings); err != nil {
		t.Fatal(err)
	}
	settings.Settings.RootSHA256 = ca.Fingerprint()
	files[settingsName], _ = json.Marshal(settings)
	review, err := validateColdStateRecords(scope, files, now)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	open := func(got authorityState, installation, root string, at time.Time) (*Authority, error) {
		calls++
		if installation != scope.Installation || root != ca.Fingerprint() || !at.Equal(now) || string(got.ProtectedSigning) != "opaque original ciphertext" {
			t.Fatal("legacy protection domain or opaque bytes changed")
		}
		bundle, err := ca.PrivateBundle()
		if err != nil {
			return nil, err
		}
		defer clear(bundle)
		return OpenAuthority(bundle, root, at)
	}
	got, err := admitColdSigner(scope, review, files[authorityStateName], now, open)
	if err != nil || got == nil || got.Fingerprint() != ca.Fingerprint() || calls != 1 {
		t.Fatal("matching signer refused", err)
	}
	for _, kind := range []string{"public-only", "expired", "rotation", "opaque-only", "absent"} {
		t.Run(kind, func(t *testing.T) {
			bad := review
			switch kind {
			case "public-only":
				bad.Authority = "public-only"
			case "expired":
				bad.Validity = "expired"
			case "rotation":
				bad.Rotation = "retained-coherent-recovery-required"
			case "opaque-only":
				bad.Activation = "preserved"
			case "absent":
				bad.Authority = "none"
			}
			before := calls
			if _, err := admitColdSigner(scope, bad, files[authorityStateName], now, open); err == nil || calls != before {
				t.Fatal("nonactivating checkpoint reached native opener")
			}
		})
	}
	for _, kind := range []string{"unavailable", "nil", "different-ca", "expired-intermediate"} {
		t.Run(kind, func(t *testing.T) {
			badOpen := func(authorityState, string, string, time.Time) (*Authority, error) {
				switch kind {
				case "unavailable":
					return nil, errors.New("injected decryption failure")
				case "nil":
					return nil, nil
				case "different-ca":
					return NewAuthority(scope.Installation, now)
				default:
					return NewAuthority(scope.Installation, now.AddDate(-1, -1, 0))
				}
			}
			if _, err := admitColdSigner(scope, review, files[authorityStateName], now, badOpen); err == nil {
				t.Fatal("unusable/mismatched signer accepted")
			}
		})
	}
}

func TestColdSigningValidatorCannotExposeAuthorityWithoutActualTransition(t *testing.T) {
	v := NewColdSigningValidator()
	if v.ValidateColdSDK(nil) == nil {
		t.Fatal("missing held snapshot accepted")
	}
	if _, _, err := v.CandidateAuthority(nil, &sdkstate.ColdTransition{}); err == nil {
		t.Fatal("unvalidated signer exposed")
	}
}

package localnet

import (
	"bytes"
	"encoding/json"
	"orbwin.local/session/internal/sdkstate"
	"strings"
	"testing"
	"time"
)

func coldStateFixture(t *testing.T, expired, missing bool) (sdkstate.TransitionScope, map[string][]byte, time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	born := now
	if expired {
		born = now.AddDate(-3, 0, 0)
	}
	scope := sdkstate.TransitionScope{Transaction: strings.Repeat("a", 32), Installation: strings.Repeat("b", 32), Daemon: "actual-daemon", SID: "S-1-5-21-1", ManagementRetirement: [32]byte{1}, SDKImage: [32]byte{2}}
	ca, err := NewAuthority(scope.Installation, born)
	if err != nil {
		t.Fatal(err)
	}
	der := ca.RootCertificate()
	state := authorityState{Version: 1, Installation: scope.Installation, RootSHA256: fingerprint(der), RootCertificate: der, ProtectedSigning: []byte{0, 255, 0, 128, 42}}
	if missing {
		state.ProtectedSigning = nil
	}
	authority, _ := json.MarshalIndent(state, "", "  ")
	settings := DefaultSettings()
	settings.RootSHA256 = state.RootSHA256
	data, _ := json.Marshal(settingsRecord{Version: 1, Daemon: scope.Daemon, SettingsSnapshot: SettingsSnapshot{Revision: 42, Settings: settings}})
	return scope, map[string][]byte{authorityStateName: authority, settingsName: data, "windows-trust-" + strings.Repeat("c", 64) + ".json": []byte(`{"state":"owned","remove":"foreign"}`), "lan-identity.json": []byte(`{"schema":1}`), rotationStateName: []byte("unverified old rotation operation")}, now
}
func TestColdStatePreservesOpaqueExpiredAndMissingSignerWithoutDPAPIOrReplacement(t *testing.T) {
	for _, expired := range []bool{false, true} {
		for _, missing := range []bool{false, true} {
			scope, files, now := coldStateFixture(t, expired, missing)
			original := append([]byte(nil), files[authorityStateName]...)
			review, err := validateColdStateRecords(scope, files, now)
			if err != nil {
				t.Fatal(err)
			}
			want := "opaque-key"
			if missing {
				want = "public-only"
			}
			validity := "current"
			if expired {
				validity = "expired"
			}
			if review.Authority != want || review.Validity != validity || review.SettingsRevision != 42 || review.HistoricalRecords != 3 || review.Rotation != "retained-recovery-required" || review.Activation != "rotation-recovery-required" || review.DataSHA256 == [32]byte{} || !bytes.Equal(original, files[authorityStateName]) {
				t.Fatal("opaque preservation or non-authorizing review changed")
			}
			// Historical forged removal bytes change only the preservation digest;
			// they never become a signer, revision, Root removal or active rotation.
			files["windows-trust-"+strings.Repeat("c", 64)+".json"] = []byte("tampered historical receipt")
			again, err := validateColdStateRecords(scope, files, now)
			if err != nil || again.Authority != want || again.RootSHA256 != review.RootSHA256 || again.DataSHA256 == review.DataSHA256 {
				t.Fatal("historical bytes granted authority or were discarded")
			}
		}
	}
}
func TestColdStateRejectsAmbiguousForeignOrMismatchedActiveData(t *testing.T) {
	for _, kind := range []string{"duplicate", "foreign-installation", "foreign-daemon", "revision", "root", "suffix"} {
		t.Run(kind, func(t *testing.T) {
			scope, files, now := coldStateFixture(t, false, false)
			switch kind {
			case "duplicate":
				files[authorityStateName] = bytes.Replace(files[authorityStateName], []byte(`"version": 1`), []byte(`"version": 1,"Version":1`), 1)
			case "foreign-installation":
				scope.Installation = strings.Repeat("f", 32)
			case "foreign-daemon":
				scope.Daemon = "other-engine"
			case "revision":
				files[settingsName] = bytes.Replace(files[settingsName], []byte(`"revision":42`), []byte(`"revision":0`), 1)
			case "root":
				files[settingsName] = bytes.Replace(files[settingsName], []byte(`"rootSha256":"`), []byte(`"rootSha256":"f`), 1)
			case "suffix":
				files[authorityStateName] = append(files[authorityStateName], []byte(`{}`)...)
			}
			if _, err := validateColdStateRecords(scope, files, now); err == nil {
				t.Fatal("unsupported active data accepted")
			}
		})
	}
}

func TestColdStatePreservationNeverAdmitsSigningOrDropsPendingRotation(t *testing.T) {
	for _, expired := range []bool{false, true} {
		for _, missing := range []bool{false, true} {
			scope, files, now := coldStateFixture(t, expired, missing)
			delete(files, rotationStateName)
			review, err := validateColdStateRecords(scope, files, now)
			if err != nil || review.Activation != "native-signing-admission-required" || review.Rotation != "none" {
				t.Fatal("preservation was treated as usable signing", err)
			}
			for _, rotation := range [][]byte{nil, []byte(`{}`), []byte(`{"stage":"completed"}`), []byte(`{"stage":"pending"}`)} {
				files[rotationStateName] = rotation
				review, err = validateColdStateRecords(scope, files, now)
				if err != nil || review.Activation != "rotation-recovery-required" || review.Rotation != "retained-recovery-required" || !bytes.Equal(files[rotationStateName], rotation) {
					t.Fatal("unknown/pending historical rotation silently omitted", err)
				}
			}
		}
	}
}

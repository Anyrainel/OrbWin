package localnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/sdkstate"
)

// Inert record-capability model only: no native root, store or DPAPI call.
type stateOriginFake struct {
	records                                          map[string][]byte
	reads                                            map[string][]byte
	active                                           bool
	beginErr, readErr, checkErr, finishErr, writeErr error
	failAfterIntent                                  bool
	events                                           []string
}

func (f *stateOriginFake) Begin(context.Context) error {
	if f.beginErr != nil {
		return f.beginErr
	}
	if f.active {
		return errors.New("busy")
	}
	f.active = true
	f.reads = map[string][]byte{}
	f.events = append(f.events, "begin")
	return nil
}
func (f *stateOriginFake) Finish() error {
	f.events = append(f.events, "finish")
	f.active = false
	if f.finishErr != nil {
		f.beginErr = f.finishErr
	}
	return f.finishErr
}
func (f *stateOriginFake) Check() error { return f.checkErr }
func (f *stateOriginFake) Read(name string, budget int64) ([]byte, error) {
	if !f.active {
		return nil, errors.New("read without origin guard")
	}
	if f.readErr != nil {
		return nil, f.readErr
	}
	data, ok := f.records[name]
	if !ok {
		f.reads[name] = nil
		return nil, os.ErrNotExist
	}
	if int64(len(data)) > budget {
		return nil, errors.New("budget")
	}
	f.reads[name] = append([]byte(nil), data...)
	return append([]byte(nil), data...), nil
}
func (f *stateOriginFake) change(name string, before, data []byte, kind string) error {
	if !f.active {
		return errors.New("write without guard")
	}
	if f.writeErr != nil {
		f.beginErr = f.writeErr
		return f.writeErr
	}
	read, seen := f.reads[name]
	if !seen || !bytes.Equal(read, before) || !bytes.Equal(f.records[name], before) {
		return errors.New("missing same-guard expected object")
	}
	if kind == "create" && before != nil {
		return os.ErrExist
	}
	f.records[name] = append([]byte(nil), data...)
	f.reads[name] = append([]byte(nil), data...)
	f.events = append(f.events, kind+":"+name)
	if f.failAfterIntent && bytes.Contains(data, []byte(`"state":"remove-intent"`)) {
		f.checkErr = errors.New("origin changed after intent")
	}
	return nil
}
func (f *stateOriginFake) Create(n string, data []byte) error {
	return f.change(n, nil, data, "create")
}
func (f *stateOriginFake) Replace(n string, old, data []byte) error {
	return f.change(n, old, data, "replace")
}
func (f *stateOriginFake) Append(n string, old, data []byte) error {
	return f.change(n, old, append(append([]byte(nil), old...), data...), "append")
}
func (f *stateOriginFake) Delete(n string, old []byte) error {
	if err := f.change(n, old, nil, "delete"); err != nil {
		return err
	}
	delete(f.records, n)
	return nil
}
func (f *stateOriginFake) Names() ([]string, error) {
	if !f.active {
		return nil, errors.New("list without guard")
	}
	var names []string
	for n := range f.records {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}
func (f *stateOriginFake) Close() error { return f.finishErr }

type originRootStore struct {
	state   *stateOriginFake
	present bool
	removes int
	guarded bool
	user    string
}

func (s *originRootStore) UserID() string                { return s.user }
func (s *originRootStore) Contains([]byte) (bool, error) { return s.present, nil }
func (s *originRootStore) Add([]byte) error {
	return errors.New("unexpected Add in removal-only fixture")
}
func (s *originRootStore) Remove([]byte) error {
	s.removes++
	s.guarded = s.state.active
	s.state.events = append(s.state.events, "external-remove")
	s.present = false
	return nil
}
func (s *originRootStore) Close() error { return nil }

func stateOriginFixture(t *testing.T) (*NamespaceLedger, *stateOriginFake) {
	t.Helper()
	f := &stateOriginFake{records: map[string][]byte{}}
	return &NamespaceLedger{directory: managedStateLabel, daemon: "owned", stateStore: f}, f
}
func installPublicTrustFixture(t *testing.T, ledger *NamespaceLedger, state *stateOriginFake, times ...time.Time) (string, *originRootStore) {
	t.Helper()
	installation := strings.Repeat("a", 32)
	issued := time.Now()
	if len(times) > 0 {
		issued = times[0]
	}
	authority, err := NewAuthority(installation, issued)
	if err != nil {
		t.Fatal(err)
	}
	root := authority.Fingerprint()
	record := windowsTrustRecord{Version: 1, Daemon: ledger.daemon, Installation: installation, UserID: "native-user", Scope: windowsTrustScope, RootSHA256: root, RootCertificate: authority.RootCertificate(), State: "installed", Sequence: 1}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	state.records[filepath.Base(ledger.trustPath(root))] = data
	return root, &originRootStore{state: state, present: true, user: record.UserID}
}

func TestManagedStateOriginRefusesForgedPlaintextTrustRemovalBeforeStoreMutation(t *testing.T) {
	for _, failure := range []string{"begin", "read", "intent-check"} {
		t.Run(failure, func(t *testing.T) {
			ledger, state := stateOriginFixture(t)
			root, store := installPublicTrustFixture(t, ledger, state)
			switch failure {
			case "begin":
				state.beginErr = errors.New("private root origin refused")
			case "read":
				state.readErr = errors.New("receipt native provenance refused")
			case "intent-check":
				state.failAfterIntent = true
			}
			if _, err := ledger.RemoveWindowsTrust(store, strings.Repeat("a", 32), root); err == nil || store.removes != 0 {
				t.Fatal("unproven receipt authorized CurrentUserROOT removal", err)
			}
		})
	}
}
func TestManagedStateMissingSignerRemovalRetainsGuardThroughExactStoreAndReceipt(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid-public-root", true: "expired-public-root"}[expired], func(t *testing.T) {
			ledger, state := stateOriginFixture(t)
			issued := time.Now()
			if expired {
				issued = issued.AddDate(-20, 0, 0)
			}
			root, store := installPublicTrustFixture(t, ledger, state, issued)
			// There is deliberately no authority-v1/signing-state record. Public DER
			// removal remains available only through the authenticated receipt guard.
			status, err := ledger.RemoveWindowsTrust(store, strings.Repeat("a", 32), root)
			if err != nil || status.State != "removed" || store.removes != 1 || !store.guarded || state.active {
				t.Fatal("guard or missing-signer cleanup semantics lost", status, err, state.events)
			}
			if state.events[len(state.events)-1] != "finish" {
				t.Fatal("origin released before operation completed")
			}
			state.finishErr = errors.New("native guard close unknown")
			if _, err = ledger.WindowsTrustStatus(store, strings.Repeat("a", 32), root); err == nil {
				t.Fatal("guard close uncertainty reported success")
			}
			if _, err = ledger.WindowsTrustStatus(store, strings.Repeat("a", 32), root); err == nil {
				t.Fatal("unknown close owner silently reacquired native authority")
			}
		})
	}
}

func TestManagedStateSettingsCASAndUnknownWriteDoNotFallBackToPlainFiles(t *testing.T) {
	ledger, state := stateOriginFixture(t)
	snapshot, err := ledger.StoreSettings(0, DefaultSettings())
	if err != nil || snapshot.Revision != 1 {
		t.Fatal("private initial settings failed", err)
	}
	before := append([]byte(nil), state.records[settingsName]...)
	if _, err = ledger.StoreSettings(0, DefaultSettings()); err == nil || !bytes.Equal(before, state.records[settingsName]) {
		t.Fatal("stale settings revision overwritten")
	}
	state.writeErr = errors.New("private replacement completion unknown")
	changed := DefaultSettings()
	changed.Ports = ServicePorts{HTTP: 18080, HTTPS: 18443}
	if _, err = ledger.StoreSettings(1, changed); err == nil || !bytes.Equal(before, state.records[settingsName]) {
		t.Fatal("uncertain settings write reported success")
	}
	state.writeErr = nil
	if _, err = ledger.StoreSettings(1, changed); err == nil {
		t.Fatal("unknown private owner silently retried through plain path")
	}
}

func TestManagedStateRotationCancellationUsesPrivateArtifactsWithoutOpeningCiphertext(t *testing.T) {
	ledger, state := stateOriginFixture(t)
	installation := strings.Repeat("a", 32)
	old, err := NewAuthority(installation, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	next, err := NewAuthority(installation, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	oldState := authorityState{Version: 1, Installation: installation, RootSHA256: old.Fingerprint(), RootCertificate: old.RootCertificate(), ProtectedSigning: []byte("unopened-ciphertext-fixture")}
	newState := authorityState{Version: 1, Installation: installation, RootSHA256: next.Fingerprint(), RootCertificate: next.RootCertificate(), ProtectedSigning: []byte("unopened-new-ciphertext-fixture")}
	oldData, _ := json.Marshal(oldState)
	newData, _ := json.Marshal(newState)
	settings := DefaultSettings()
	settings.RootSHA256 = old.Fingerprint()
	oldSettings := SettingsSnapshot{Revision: 1, Settings: settings}
	target := oldSettings
	target.Revision = 2
	target.Settings.RootSHA256 = next.Fingerprint()
	rotation := authorityRotation{Version: 1, Daemon: ledger.daemon, Installation: installation, Old: oldSettings, Target: target, OldCertificate: old.RootCertificate(), NewCertificate: next.RootCertificate(), OldStateSHA256: rotationDigest(oldData), NewStateSHA256: rotationDigest(newData)}
	state.records[authorityStateName] = oldData
	state.records[settingsName], _ = marshalSettings(settingsRecord{Version: 1, Daemon: ledger.daemon, SettingsSnapshot: oldSettings})
	state.records[rotationStateName], _ = json.Marshal(rotation)
	state.records[filepath.Base(ledger.rotationArtifact(next.Fingerprint(), "old"))] = oldData
	state.records[filepath.Base(ledger.rotationArtifact(next.Fingerprint(), "new"))] = newData
	if err = ledger.CancelAuthorityRotation(installation, old.Fingerprint(), next.Fingerprint()); err != nil {
		t.Fatal("private cancellation failed", err)
	}
	if _, ok := state.records[rotationStateName]; ok || !bytes.Equal(oldData, state.records[authorityStateName]) {
		t.Fatal("rotation cancellation changed active authority or retained settled intent")
	}
	if len(state.records["authority-abandoned-"+next.Fingerprint()+".pem"]) == 0 || state.active {
		t.Fatal("private public archive/guard lifetime lost")
	}
}
func TestManagedStateNamespaceIntentAcceptedIDAndUnknownAppendAreNotReplayed(t *testing.T) {
	ledger, state := stateOriginFixture(t)
	intent := namespaceFixture()
	if err := ledger.Begin(intent); err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(ledger.path(intent.Session))
	before := append([]byte(nil), state.records[name]...)
	state.writeErr = errors.New("accepted ID append outcome unknown")
	if ledger.Created(intent, strings.Repeat("b", 64)) == nil || !bytes.Equal(before, state.records[name]) {
		t.Fatal("unknown append changed immutable intent")
	}
	if ledger.Created(intent, strings.Repeat("b", 64)) == nil {
		t.Fatal("unknown native append owner replayed")
	}
	ledger, state = stateOriginFixture(t)
	if err := ledger.Begin(intent); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Created(intent, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if ledger.Created(intent, strings.Repeat("c", 64)) == nil {
		t.Fatal("accepted helper identity replayed")
	}
	records, err := ledger.records()
	if err != nil || len(records) != 1 || records[0].ID != strings.Repeat("b", 64) {
		t.Fatal("guarded namespace record lost", err)
	}
}
func TestManagedLANIdentityUsesProductAndSIDAcrossStorageMovesAndRefusesDaemonDrift(t *testing.T) {
	first, state := stateOriginFixture(t)
	sid := "S-1-5-21-1-2-3-1001"
	installation := "00000000-0000-4000-8000-000000000001"
	id, err := first.LANInstallationID(installation, sid)
	if err != nil {
		t.Fatal(err)
	}
	second := &NamespaceLedger{directory: "different-logical-storage-label", daemon: first.daemon, stateStore: state}
	if got, err := second.LANInstallationID(installation, sid); err != nil || got != id {
		t.Fatal("storage move changed LAN product identity", err)
	}
	second.daemon = "other-engine"
	if _, err := second.LANInstallationID(installation, sid); err == nil {
		t.Fatal("different daemon adopted managed identity")
	}
	second.daemon = first.daemon
	if _, err := second.LANInstallationID(installation, "S-1-5-21-1-2-3-1002"); err == nil {
		t.Fatal("different SID adopted managed identity")
	}
	if _, err := OpenManagedNamespaceLedger(new(sdkstate.Root), "owned"); err == nil {
		t.Fatal("zero/copied root value minted native storage authority")
	}
}

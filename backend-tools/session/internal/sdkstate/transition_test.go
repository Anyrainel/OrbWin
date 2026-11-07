package sdkstate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
)

type fakeColdAuthority struct{ backend *fakeColdBackend }

func (a *fakeColdAuthority) check(context.Context) error                { return nil }
func (a *fakeColdAuthority) begin(context.Context) (coldBackend, error) { return a.backend, nil }
func (a *fakeColdAuthority) close() error                               { return nil }

type fakeColdValidator struct {
	backend *fakeColdBackend
	fail    bool
}

func (v fakeColdValidator) ValidateColdSDK(s *ColdSnapshot) error {
	v.backend.events = append(v.backend.events, "validate")
	if v.fail {
		return ErrState
	}
	data, err := s.Read("authority-v1.json")
	if err != nil || !bytes.Equal(data, []byte{0, 255, 128, 42}) {
		return ErrState
	}
	return nil
}

type fakeColdBackend struct {
	scope  TransitionScope
	root   *Root
	events []string
	fail   string
	tamper string
	files  []TransitionFile
	bodies map[string][]byte
}

type fakeColdRuntime struct {
	root    *Root
	joined  bool
	factory *LANObservationFactory
}

func (r *fakeColdRuntime) RootBacking() *Root                         { return r.root }
func (r *fakeColdRuntime) LeafFactoryBacking() *LANObservationFactory { return r.factory }
func (r *fakeColdRuntime) VerifyJoined(context.Context) error {
	if !r.joined {
		return ErrRecoveryRequired
	}
	return nil
}
func (b *fakeColdBackend) bindRuntime(root *Root, participant ColdRuntimeParticipant) error {
	if root != b.root || participant.RootBacking() != root {
		return ErrState
	}
	return nil
}
func registerFakeColdRuntime(t *testing.T, transition *ColdTransition, ctx context.Context) *fakeColdRuntime {
	t.Helper()
	r := &fakeColdRuntime{root: transition.root, joined: true, factory: testColdFactory(transition.root, transition)}
	if err := transition.RegisterRuntime(ctx, r); err != nil {
		t.Fatal(err)
	}
	return r
}

func testColdFactory(root *Root, transition *ColdTransition) *LANObservationFactory {
	f := &LANObservationFactory{root: root, transition: transition}
	f.identity = f
	return f
}

func (b *fakeColdBackend) prepareCommittedClose(receipt TransitionReceipt) error {
	if !receipt.valid() || receipt.Scope != b.scope {
		return ErrState
	}
	return b.step("transfer-final-owner")
}

func (b *fakeColdBackend) step(name string) error {
	b.events = append(b.events, name)
	if b.fail == name {
		return ErrState
	}
	return nil
}
func (b *fakeColdBackend) check(context.Context) error { return nil }
func (b *fakeColdBackend) capture(context.Context) (*ColdSnapshot, error) {
	if b.step("capture") != nil {
		return nil, ErrState
	}
	records := map[string][]byte{}
	for n, d := range b.bodies {
		records[n] = append([]byte(nil), d...)
	}
	return &ColdSnapshot{scope: b.scope, records: records}, nil
}
func (b *fakeColdBackend) admit(context.Context) error  { return b.step("admit") }
func (b *fakeColdBackend) seal(context.Context) error   { return b.step("seal") }
func (b *fakeColdBackend) create(context.Context) error { return b.step("create") }
func (b *fakeColdBackend) copy(_ context.Context, name string, data []byte, role string) (TransitionFile, error) {
	if b.step("copy") != nil {
		return TransitionFile{}, ErrState
	}
	if !bytes.Equal(data, b.bodies[name]) {
		return TransitionFile{}, ErrState
	}
	f := TransitionFile{Name: name, Source: "source-" + name, Target: "target-" + name, Disposition: role, Size: uint64(len(data)), SHA256: sha256.Sum256(data)}
	b.files = append(b.files, f)
	return f, nil
}
func (b *fakeColdBackend) finish(_ context.Context, files []TransitionFile) (TransitionReceipt, *Root, error) {
	if b.step("finish") != nil {
		return TransitionReceipt{}, nil, ErrState
	}
	r := TransitionReceipt{Schema: 1, Scope: b.scope, LegacyRoot: "old-root", LegacyController: "old-native-lock", Root: "new-root", Init: "init", Fence: "pending-fence", Quarantine: "historical", Files: files}
	r.Inventory, r.Active, r.Historical = transitionHashes(files)
	if b.tamper == "omit" {
		r.Files = append([]TransitionFile(nil), files[:len(files)-1]...)
		r.Inventory, r.Active, r.Historical = transitionHashes(r.Files)
	}
	if b.tamper == "identity" {
		r.Files = append([]TransitionFile(nil), files...)
		r.Files[0].Target = "different-native-object"
		r.Inventory, r.Active, r.Historical = transitionHashes(r.Files)
	}
	return r, b.root, nil
}

func TestSDKColdReceiptRequiresExactCanonicalInventoryAndDigests(t *testing.T) {
	retired, b, ctx := coldTransitionFixture(t)
	transition, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b})
	if err != nil {
		t.Fatal(err)
	}
	_, good, err := transition.CandidateRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := good.Digest()
	if err != nil || digest == [32]byte{} {
		t.Fatal("missing complete receipt digest")
	}
	for _, kind := range []string{"hash", "omit", "reorder", "alias", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			bad := good
			bad.Files = append([]TransitionFile(nil), good.Files...)
			switch kind {
			case "hash":
				bad.Active[0] ^= 1
			case "omit":
				bad.Files = bad.Files[1:]
			case "reorder":
				bad.Files[0], bad.Files[1] = bad.Files[1], bad.Files[0]
			case "alias":
				bad.Files[1].Target = bad.Files[0].Source
			case "duplicate":
				bad.Files[1] = bad.Files[0]
			}
			if bad.valid() {
				t.Fatal("ambiguous or altered inventory accepted")
			}
			if _, err := bad.Digest(); err == nil {
				t.Fatal("invalid receipt obtained correlation digest")
			}
		})
	}
	for _, kind := range []string{"omit", "identity"} {
		t.Run("backend-"+kind, func(t *testing.T) {
			retired, b, ctx := coldTransitionFixture(t)
			b.tamper = kind
			transition, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b})
			if err == nil || transition == nil || !transition.unknown || transition.Close() == nil {
				t.Fatal("backend substituted self-consistent receipt after copy")
			}
		})
	}
}
func (b *fakeColdBackend) close() error                                    { return b.step("close") }
func (b *fakeColdBackend) commit(context.Context, TransitionReceipt) error { return b.step("commit") }

func TestSDKColdCommitIsOneUseAndUnknownReplyRetainsEveryOwner(t *testing.T) {
	for _, fail := range []bool{false, true} {
		retired, b, ctx := coldTransitionFixture(t)
		transition, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b})
		if err != nil {
			t.Fatal(err)
		}
		if fail {
			b.fail = "commit"
		}
		err = transition.Commit(ctx)
		if (err != nil) != fail || transition.committed == fail || transition.unknown != fail {
			t.Fatal("commit outcome/latch differs", err)
		}
		before := len(b.events)
		if transition.Commit(ctx) == nil || len(b.events) != before {
			t.Fatal("final slot replayed")
		}
		if fail && (transition.Close() == nil || len(b.events) != before) {
			t.Fatal("lost marker reply released owner")
		}
	}
	var forged ColdTransition
	_ = json.Unmarshal([]byte(`{"committed":true}`), &forged)
	_, _, ctx := coldTransitionFixture(t)
	if forged.Commit(ctx) == nil {
		t.Fatal("decoded state manufactured native final capability")
	}
}

func TestSDKColdCommittedCloseUnknownTransferRetainsRootAndNeverRetries(t *testing.T) {
	retired, b, ctx := coldTransitionFixture(t)
	transition, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b})
	if err != nil {
		t.Fatal(err)
	}
	registerFakeColdRuntime(t, transition, ctx)
	if transition.Commit(ctx) != nil {
		t.Fatal("commit", err)
	}
	root := transition.root
	b.fail = "transfer-final-owner"
	if transition.Close() == nil || !transition.unknown || root.Check() != nil {
		t.Fatal("unknown final transfer closed the root")
	}
	before := len(b.events)
	b.fail = ""
	if transition.Close() == nil || len(b.events) != before || root.Check() != nil {
		t.Fatal("unknown final transfer retried or released ownership")
	}
	for _, event := range b.events {
		if event == "close" {
			t.Fatal("native exclusion released before positive transfer")
		}
	}
}

func TestSDKColdCommittedCloseWaitsForGuardBeforeTransferringOwner(t *testing.T) {
	retired, b, ctx := coldTransitionFixture(t)
	transition, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b})
	if err != nil {
		t.Fatal(err)
	}
	registerFakeColdRuntime(t, transition, ctx)
	if transition.Commit(ctx) != nil {
		t.Fatal("commit", err)
	}
	guard, err := transition.root.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before := len(b.events)
	if transition.Close() != ErrBusy || transition.unknown || len(b.events) != before {
		t.Fatal("active record guard transferred final owner")
	}
	if guard.Close() != nil || transition.Close() != nil {
		t.Fatal("positive guarded close")
	}
}

func TestSDKColdRuntimeRegistrationRequiresExactRootAndPositiveJoinedHolder(t *testing.T) {
	retired, b, ctx := coldTransitionFixture(t)
	transition, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b})
	if err != nil {
		t.Fatal(err)
	}
	foreign := &fakeColdRuntime{root: &Root{s: transition.root.s}, joined: true}
	if transition.RegisterRuntime(ctx, foreign) == nil {
		t.Fatal("different opaque root wrapper adopted")
	}
	missing := &fakeColdRuntime{root: transition.root, joined: true}
	if transition.RegisterRuntime(ctx, missing) == nil {
		t.Fatal("actual runtime root without actual source factory adopted")
	}
	borrowed := &fakeColdRuntime{root: transition.root, factory: testColdFactory(transition.root, &ColdTransition{})}
	if transition.RegisterRuntime(ctx, borrowed) == nil {
		t.Fatal("factory from another transition borrowed actual root")
	}
	runtime := &fakeColdRuntime{root: transition.root, factory: testColdFactory(transition.root, transition)}
	if transition.RegisterRuntime(ctx, runtime) != nil || transition.RegisterRuntime(ctx, runtime) == nil {
		t.Fatal("runtime rebind accepted or initial registration failed")
	}
	if transition.Commit(ctx) != nil {
		t.Fatal("commit")
	}
	before := len(b.events)
	if transition.Close() == nil || transition.unknown || len(b.events) != before || transition.root.Check() != nil {
		t.Fatal("unjoined runtime released source or candidate")
	}
	runtime.joined = true
	original := runtime.factory
	runtime.factory = testColdFactory(transition.root, transition)
	if transition.Close() == nil || transition.unknown || len(b.events) != before {
		t.Fatal("copied source shape replaced the actual registered factory")
	}
	runtime.factory = original
	if transition.Close() != nil {
		t.Fatal("positive actual holder refused")
	}
}

func TestSDKColdFinalSourceCloseUnknownNeverRepeatsNativeClosure(t *testing.T) {
	retired, b, ctx := coldTransitionFixture(t)
	transition, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b})
	if err != nil {
		t.Fatal(err)
	}
	registerFakeColdRuntime(t, transition, ctx)
	if transition.Commit(ctx) != nil {
		t.Fatal("commit")
	}
	b.fail = "close"
	if transition.Close() == nil || !transition.unknown || transition.closed {
		t.Fatal("unknown source-owner close reported complete")
	}
	before := len(b.events)
	b.fail = ""
	if transition.Close() == nil || len(b.events) != before {
		t.Fatal("unknown source owner close replayed")
	}
}

func TestSDKColdCommitWaitsForExistingGuardWithoutConsumingFinalSlot(t *testing.T) {
	retired, b, ctx := coldTransitionFixture(t)
	transition, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b})
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := transition.CandidateRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := root.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before := len(b.events)
	if transition.Commit(ctx) != ErrBusy || len(b.events) != before || transition.commitAttempted {
		t.Fatal("commit raced active SDK transaction or consumed slot")
	}
	if guard.Close() != nil || transition.Commit(ctx) != nil {
		t.Fatal("closed SDK transaction did not permit exact final consumption")
	}
}

func TestSDKColdFinalWireRejectsRebindingAndPartialOrNoncanonicalRecords(t *testing.T) {
	scope := TransitionScope{Transaction: strings.Repeat("a", 32)}
	parent := transitionProcess{PID: 17, Creation: 23, Session: 2, Image: [32]byte{7}}
	digest, nonce := [32]byte{8}, [32]byte{9}
	good := transitionFinalRecord{2, scope.Transaction, nonce, digest, [32]byte{10}, 12, parent.PID, parent.Creation, parent.Session, parent.Image, 29, 31, parent.Session, parent.Image}
	body, _ := json.Marshal(good)
	if _, err := parseTransitionFinal(body, scope, digest, nonce, parent); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"scope", "nonce", "digest", "journal", "sequence", "generation", "image", "serving-absent", "serving-parent", "serving-reused", "serving-session", "serving-image", "v1", "suffix", "partial", "unknown", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			bad := good
			switch kind {
			case "scope":
				bad.Transaction = strings.Repeat("b", 32)
			case "nonce":
				bad.Nonce[0] ^= 1
			case "digest":
				bad.SDKDigest[0] ^= 1
			case "journal":
				bad.ManagementRecord = [32]byte{}
			case "sequence":
				bad.Sequence = 65
			case "generation":
				bad.ParentCreation++
			case "image":
				bad.ParentImage[0] ^= 1
			case "serving-absent":
				bad.ServingPID = 0
			case "serving-parent":
				bad.ServingPID = parent.PID
			case "serving-reused":
				bad.ServingCreation = parent.Creation
			case "serving-session":
				bad.ServingSession++
			case "serving-image":
				bad.ServingImage[0] ^= 1
			case "v1":
				bad.Schema = 1
			}
			data, _ := json.Marshal(bad)
			switch kind {
			case "suffix":
				data = append(data, ' ')
			case "partial":
				data = data[:len(data)-1]
			case "unknown":
				data = append(data[:len(data)-1], []byte(`,"Other":true}`)...)
			case "duplicate":
				data = append(data[:len(data)-1], []byte(`,"Schema":1}`)...)
			}
			if _, err := parseTransitionFinal(data, scope, digest, nonce, parent); err == nil {
				t.Fatal("final grant rebound or torn/ambiguous bytes accepted")
			}
		})
	}
}

func TestSDKColdDenialReceiptRequiresExactNativeObjectsAndPersistentReadOnlyAttribute(t *testing.T) {
	scope := TransitionScope{Transaction: strings.Repeat("a", 32)}
	root, controller := transitionDenialObject{1, 2, 3}, transitionDenialObject{1, 2, 4}
	good := transitionDenialRecord{Schema: 1, Transaction: scope.Transaction, Root: root, Controller: controller, Before: 0x20, After: 0x21, RootBeforeACL: [32]byte{1}, RootAfterACL: [32]byte{2}, ControllerBeforeACL: [32]byte{3}, ControllerAfterACL: [32]byte{4}, Stage: "readonly-owner-acquired"}
	body, _ := json.Marshal(good)
	if _, err := parseTransitionDenial(body, scope, root, controller, 0x21); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"scope", "root", "controller", "not-admitted", "readonly-cleared", "before-unsupported", "after-rewritten", "alias", "missing-SD", "partial", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			record := good
			attrs := uint32(0x21)
			switch kind {
			case "scope":
				record.Transaction = strings.Repeat("b", 32)
			case "root":
				record.Root.Low++
			case "controller":
				record.Controller.Low++
			case "not-admitted":
				record.Stage = "attribute-change-admitted"
			case "readonly-cleared":
				attrs = 0x20
			case "before-unsupported":
				record.Before |= 0x100
				record.After |= 0x100
			case "after-rewritten":
				record.After = 1
			case "alias":
				record.Controller = record.Root
			case "missing-SD":
				record.ControllerAfterACL = [32]byte{}
			}
			data, _ := json.Marshal(record)
			if kind == "partial" {
				data = data[:len(data)-1]
			}
			if kind == "duplicate" {
				data = append(data[:len(data)-1], []byte(`,"Schema":1}`)...)
			}
			if _, err := parseTransitionDenial(data, scope, root, controller, attrs); err == nil {
				t.Fatal("denial-only receipt accepted ambiguous/rebound/unsealed native state")
			}
		})
	}
}
func coldTransitionFixture(t *testing.T) (*ColdRetirement, *fakeColdBackend, context.Context) {
	t.Helper()
	root, _, ctx := fixture(t)
	scope := TransitionScope{Transaction: strings.Repeat("a", 32), Installation: strings.Repeat("b", 32), Daemon: "daemon", SID: "S-1-5-21-1", ManagementRetirement: [32]byte{1}, SDKImage: [32]byte{2}}
	b := &fakeColdBackend{scope: scope, root: root, bodies: map[string][]byte{"authority-v1.json": {0, 255, 128, 42}, "settings-v1.json": []byte("unchanged settings"), "lan-identity.json": []byte("old v1 pin"), "windows-trust-" + strings.Repeat("c", 64) + ".json": []byte("untrusted removal receipt"), "authority-rotation-v1.json": []byte("old rotation intent")}}
	return &ColdRetirement{&coldRetirementState{scope: scope, native: &fakeColdAuthority{b}}}, b, ctx
}

func TestSDKColdTransitionPreservesOpaqueBytesAndQuarantinesMutationAuthority(t *testing.T) {
	retired, b, ctx := coldTransitionFixture(t)
	transition, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b})
	if err != nil {
		t.Fatal(err)
	}
	root, r, err := transition.CandidateRoot(ctx)
	if err != nil || root == nil || !r.valid() || len(r.Files) != len(b.bodies) {
		t.Fatal("missing retained source/target receipt")
	}
	for _, f := range r.Files {
		if f.Disposition != transitionDisposition(f.Name) || f.SHA256 != sha256.Sum256(b.bodies[f.Name]) {
			t.Fatal("opaque bytes changed or old mutation receipt activated")
		}
	}
	if strings.Join(b.events[:5], ",") != "capture,validate,admit,seal,create" {
		t.Fatal("target created before native retirement/validation/fence/seal", b.events)
	}
	if _, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b}); err == nil {
		t.Fatal("retirement capability replayed")
	}
	before := len(b.events)
	if transition.Close() == nil || len(b.events) != before || root.Check() != nil {
		t.Fatal("precommit close released candidate ownership")
	}
	registerFakeColdRuntime(t, transition, ctx)
	if transition.Commit(ctx) != nil || transition.Close() != nil {
		t.Fatal("known fake closure")
	}
	if _, _, err := transition.CandidateRoot(ctx); err == nil {
		t.Fatal("closed root returned as usable")
	}
}
func TestSDKColdTransitionEveryAdmittedFaultRetainsOwnerAndNeverResets(t *testing.T) {
	for _, failure := range []string{"capture", "validate", "admit", "seal", "create", "copy", "finish"} {
		t.Run(failure, func(t *testing.T) {
			retired, b, ctx := coldTransitionFixture(t)
			b.fail = failure
			transition, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b, fail: failure == "validate"})
			if err == nil || transition == nil || !transition.unknown {
				t.Fatal("partial cold import accepted")
			}
			before := len(b.events)
			if transition.Close() == nil || len(b.events) != before {
				t.Fatal("unknown owner cleaned up/reset")
			}
			if _, err := BeginColdTransition(ctx, retired, fakeColdValidator{backend: b}); err == nil || len(b.events) != before {
				t.Fatal("admitted copy replayed")
			}
			if failure == "validate" {
				for _, event := range b.events {
					if event == "admit" || event == "create" {
						t.Fatal("unvalidated bytes became active state")
					}
				}
			}
		})
	}
}
func TestSDKColdRetirementHasNoJSONOrBytesAuthorityConstructor(t *testing.T) {
	var forged ColdRetirement
	_ = json.Unmarshal([]byte(`{"scope":{"Transaction":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"retired":true}`), &forged)
	_, b, ctx := coldTransitionFixture(t)
	if _, err := BeginColdTransition(ctx, &forged, fakeColdValidator{backend: b}); err == nil || len(b.events) != 0 {
		t.Fatal("serialized booleans minted SDK retirement")
	}
}

package lanprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
)

// All tests below use an in-memory backend. No provider, file, token or process
// call occurs; positive fake facts test ordering/refusal, never native support.
type ownedTestJournal struct {
	events []string
	fail   string
}

func (j *ownedTestJournal) FlushOwnedRuleEvent(e OwnedRuleEvent) error {
	if _, err := e.ProtectedBytes(); err != nil {
		return err
	}
	j.events = append(j.events, e.Kind())
	if e.Kind() == j.fail {
		return errors.New("flush failed")
	}
	return nil
}

type ownedTestBackend struct {
	definition                          ownedDefinition
	completion                          ownedCompletion
	createCalls, deleteCalls, readCalls int
	absent                              bool
	readErr, deleteErr                  error
	journal                             *ownedTestJournal
}

func (b *ownedTestBackend) Create(ctx context.Context, p ownedPolicy, ready func() error) (ownedCompletion, error) {
	if err := ready(); err != nil {
		return ownedCompletion{}, err
	}
	b.createCalls++
	return b.completion, nil
}
func (b *ownedTestBackend) Read(context.Context, observedRuleKeys) (ownedDefinition, error) {
	b.readCalls++
	return b.definition, b.readErr
}
func (b *ownedTestBackend) Delete(ctx context.Context, k observedRuleKeys, ready func() error) error {
	if err := ready(); err != nil {
		return err
	}
	b.deleteCalls++
	return b.deleteErr
}
func (b *ownedTestBackend) Absent(context.Context, string) (bool, error) { return b.absent, nil }
func ownedTestPolicy(t *testing.T) ownedPolicy {
	t.Helper()
	now := time.Now().UTC()
	r := lanfirewall.Request{Version: 1, Operation: "create", Installation: strings.Repeat("a", 32), UserSID: "S-1-5-21-1-2-3-1001", Transaction: strings.Repeat("b", 32), Nonce: strings.Repeat("c", 64), ReviewSHA256: strings.Repeat("d", 64), CallerPID: 1234, CallerImageSHA256: strings.Repeat("e", 64), CallerCreationTime: "123456789", IssuedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano), Scope: lanfirewall.Scope{Protocol: "tcp", LocalAddress: "192.168.4.10", LocalPort: 58080, InterfaceGUID: "{11111111-1111-1111-1111-111111111111}", InterfaceLUID: "1234", Profile: "private", ClientNetworks: []string{"192.168.4.0/24", "192.168.5.0/24"}, WorkerSHA256: strings.Repeat("e", 64)}}
	receipt := lanfirewall.Receipt{Version: 1, Request: r, Sequence: 1, State: "create-intent", CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano), RuleName: r.RuleName(), ScopeSHA256: r.Scope.Digest(), Worker: lanfirewall.WorkerBinding{Path: `C:\Users\fixture\AppData\Local\OrbWin\runtime\backend\orbwin-session.exe`, FileID: strings.Repeat("f", 32), VolumeSerial: "1234", SHA256: r.Scope.WorkerSHA256, InterfaceAlias: "Ethernet"}}
	binding := lanfirewall.InterfaceBinding{GUID: r.Scope.InterfaceGUID, LUID: r.Scope.InterfaceLUID, Alias: "Ethernet", Address: r.Scope.LocalAddress, Profile: "private", Index: 3, PrefixLength: 24}
	p, err := prepareOwnedPolicy(receipt, binding, strings.Repeat("1", 64))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func ownedTestDefinition(p ownedPolicy) ownedDefinition {
	k := observedRuleKeys{p.receipt.RuleName, "C", "P", "S", "H"}
	d := ownedDefinition{keys: k, rule: typedInstance{class: ClassName, values: map[string]typedValue{}}}
	for n, c := range propertyContracts {
		d.rule.values[n] = typedValue{kind: c.kind}
	}
	for i, n := range []string{"InstanceID", "CreationClassName", "PolicyRuleName", "SystemCreationClassName", "SystemName"} {
		d.rule.values[n] = typedValue{kind: 13, text: ownedKeysWire(k)[i]}
	}
	for n, w := range map[string]uint32{"Enabled": 1, "Action": 2, "Direction": 1, "Profiles": uint32(p.spec.Properties.Profiles), "EdgeTraversalPolicy": 0} {
		d.rule.values[n] = typedValue{kind: 3, scalar: w}
	}
	for n, w := range map[string]string{"Description": p.spec.Properties.Description, "ElementName": p.spec.Properties.ElementName, "RuleGroup": p.spec.Properties.RuleGroup} {
		d.rule.values[n] = typedValue{kind: 13, text: w}
	}
	for i := range d.filters {
		f := typedInstance{class: filterFamilies[i].class, values: map[string]typedValue{}}
		for n, c := range commonFilterProperties {
			f.values[n] = typedValue{kind: c.kind}
		}
		for n, k := range familyFilterProperties[i] {
			f.values[n] = typedValue{kind: k}
		}
		for _, n := range []string{"Name", "CreationClassName", "SystemCreationClassName", "SystemName"} {
			f.values[n] = typedValue{kind: 13, text: "retained-key"}
		}
		d.filters[i] = f
	}
	d.filters[0].values["LocalAddress"] = typedValue{kind: 29, texts: p.spec.Options.LocalAddress}
	d.filters[0].values["RemoteAddress"] = typedValue{kind: 29, texts: p.spec.Options.RemoteAddress}
	d.filters[1].values["AppPath"] = typedValue{kind: 13, text: p.spec.Options.Program}
	d.filters[2].values["InterfaceAlias"] = typedValue{kind: 29, texts: p.spec.Options.InterfaceAlias}
	d.filters[4].values["LocalPort"] = typedValue{kind: 29, texts: p.spec.Options.LocalPort}
	d.filters[4].values["Protocol"] = typedValue{kind: 13, text: p.spec.Options.Protocol}
	return d
}
func ownedTestSetup(t *testing.T) (ownedPolicy, *ownedTestJournal, *ownedTestBackend) {
	p := ownedTestPolicy(t)
	d := ownedTestDefinition(p)
	j := &ownedTestJournal{}
	return p, j, &ownedTestBackend{definition: d, completion: ownedCompletion{dispatched: true, terminal: true, closed: true, returnedFullKeys: true, keys: d.keys}, absent: true, journal: j}
}
func TestOwnedRuleFlushesBeforeCreateAndBeforeGrant(t *testing.T) {
	for _, phase := range []string{"create-intent", "create-returned", "created", ""} {
		t.Run(phase, func(t *testing.T) {
			p, j, b := ownedTestSetup(t)
			j.fail = phase
			o, err := createOwned(context.Background(), p, j, b)
			if phase != "" {
				if err == nil || o != nil {
					t.Fatal("failed flush granted owner")
				}
				if phase == "create-intent" && b.createCalls != 0 {
					t.Fatal("dispatched before intent flush")
				}
				return
			}
			if err != nil || o == nil {
				t.Fatal(err)
			}
			ack, err := o.Acknowledgement()
			if err != nil || !ack.AcknowledgementFlushed || ack.NativeKeysSHA == "" || ack.TypedSnapshotSHA == "" || ack.ReadAt.IsZero() {
				t.Fatal("invalid positive ack")
			}
			if !reflect.DeepEqual(j.events, []string{"create-intent", "create-returned", "created"}) {
				t.Fatal(j.events)
			}
			data, _ := json.Marshal(o)
			if string(data) != "{}" {
				t.Fatal("owner serialized")
			}
		})
	}
}
func TestOwnedRuleUnknownOrForeignCreateNeverGrantsOwnership(t *testing.T) {
	for _, mutate := range []func(*ownedTestBackend){
		func(b *ownedTestBackend) { b.completion.terminal = false }, func(b *ownedTestBackend) { b.completion.closed = false }, func(b *ownedTestBackend) { b.completion.status = 11 }, func(b *ownedTestBackend) { b.completion.returnedFullKeys = false }, func(b *ownedTestBackend) { b.completion.keys.policyRuleName = "" }, func(b *ownedTestBackend) { b.completion.keys.instanceID = "foreign" }, func(b *ownedTestBackend) { b.readErr = errors.New("read failed") }, func(b *ownedTestBackend) {
			v := b.definition.filters[0].values["RemoteAddress"]
			v.texts = []string{"Any"}
			b.definition.filters[0].values["RemoteAddress"] = v
		},
	} {
		p, j, b := ownedTestSetup(t)
		mutate(b)
		o, err := createOwned(context.Background(), p, j, b)
		if o != nil || !errors.Is(err, ErrOwnedRuleUnknown) {
			t.Fatal("unknown/foreign create became owned")
		}
		if len(j.events) > 2 {
			t.Fatal("unknown result recorded positive")
		}
	}
}
func ownedTestClosure(p ownedPolicy) lanlease.CloseAcknowledgement {
	return lanlease.CloseAcknowledgement{LeaseSHA: p.leaseSHA, Worker: lanfirewall.CallerIdentity{UserSID: p.receipt.Request.UserSID, PID: 2345, CreationTime: "23456789", ImageSHA256: p.receipt.Worker.SHA256}, IngressClosed: true, FlowsJoined: true, ProcessExited: true, NoUnknownChild: true}
}
func TestOwnedRuleRetirementRequiresClosureBaselineAndDurableAbsence(t *testing.T) {
	for _, mode := range []string{"closure", "changed", "intent-flush", "delete-unknown", "still-present", "result-flush", "success"} {
		t.Run(mode, func(t *testing.T) {
			p, j, b := ownedTestSetup(t)
			o, err := createOwned(context.Background(), p, j, b)
			if err != nil {
				t.Fatal(err)
			}
			c := ownedTestClosure(p)
			switch mode {
			case "closure":
				c.FlowsJoined = false
			case "changed":
				b.definition = ownedTestDefinition(p)
				v := b.definition.rule.values["Enabled"]
				v.scalar = 2
				b.definition.rule.values["Enabled"] = v
			case "intent-flush":
				j.fail = "remove-intent"
			case "delete-unknown":
				b.deleteErr = errors.New("native unknown")
			case "still-present":
				b.absent = false
			case "result-flush":
				j.fail = "removed"
			}
			ack, err := RetireOwnedRule(context.Background(), o, c, j)
			if mode == "success" {
				if err != nil || !ack.ObservedAbsent || !ack.AcknowledgementFlushed {
					t.Fatal(err)
				}
				if _, e := o.Acknowledgement(); e == nil {
					t.Fatal("retired owner remained usable")
				}
				return
			}
			if err == nil || ack.ObservedAbsent {
				t.Fatal("unsafe retirement acknowledged")
			}
			if mode == "closure" || mode == "changed" || mode == "intent-flush" {
				if b.deleteCalls != 0 {
					t.Fatal("delete before guards/flush")
				}
				return
			}
			calls := b.deleteCalls
			if _, err = RetireOwnedRule(context.Background(), o, c, j); !errors.Is(err, ErrOwnedRuleUnknown) || b.deleteCalls != calls {
				t.Fatal("unknown mutation replayed")
			}
		})
	}
}
func TestOwnedRuleFreshReadDoesNotInventFreshnessOrReplayCreate(t *testing.T) {
	p, j, b := ownedTestSetup(t)
	o, err := createOwned(context.Background(), p, j, b)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := o.Acknowledgement()
	b.readErr = errors.New("unavailable")
	if ack, err := RefreshOwnedRule(context.Background(), o, j); err == nil || !ack.ReadAt.IsZero() {
		t.Fatal("failed read refreshed ack")
	}
	b.readErr = nil
	count := len(j.events)
	for i := 0; i < 20; i++ {
		ack, err := RefreshOwnedRule(context.Background(), o, nil)
		if err != nil || ack.ReadAt.Before(old.ReadAt) {
			t.Fatal("fresh read did not advance observation")
		}
		ack.ReadAt = old.ReadAt
		if ack != old || len(j.events) != count || b.createCalls != 1 {
			t.Fatal("observation changed ownership or grew journal")
		}
	}
	removed, err := RetireOwnedRule(context.Background(), o, ownedTestClosure(p), j)
	if err != nil || removed.Sequence != old.Sequence+2 {
		t.Fatal("refresh changed removal sequence")
	}
}

func TestOwnedRuleCopiesShareUnknownRemovalFence(t *testing.T) {
	p, j, b := ownedTestSetup(t)
	o, err := createOwned(context.Background(), p, j, b)
	if err != nil {
		t.Fatal(err)
	}
	copyOfOwner := *o
	b.deleteErr = errors.New("unknown Delete")
	if _, err = RetireOwnedRule(context.Background(), o, ownedTestClosure(p), j); !errors.Is(err, ErrOwnedRuleUnknown) {
		t.Fatal(err)
	}
	if _, err = RetireOwnedRule(context.Background(), &copyOfOwner, ownedTestClosure(p), j); !errors.Is(err, ErrOwnedRuleUnknown) || b.deleteCalls != 1 {
		t.Fatal("copied capability replayed Delete")
	}
}

func TestOwnedRuleProtectedEncodingRetainsPrivateBaselineWithoutExposingCapability(t *testing.T) {
	p, _, b := ownedTestSetup(t)
	e := ownedEvent(p, b.definition, "created", 3)
	public, _ := json.Marshal(e)
	if string(public) != "{}" {
		t.Fatal("opaque event serialized publicly")
	}
	data, err := e.ProtectedBytes()
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{p.leaseSHA, p.receipt.RuleName, "CreationClassName", "InterfaceAlias", "RemoteAddress"} {
		if !strings.Contains(string(data), required) {
			t.Fatal("protected recovery baseline incomplete")
		}
	}
	large := strings.Repeat("x", OwnedRuleEventBudget)
	e.definition = ownedTestDefinition(p)
	e.definition.rule.values["Description"] = typedValue{kind: 13, text: large}
	if data, err = e.ProtectedBytes(); err == nil || data != nil {
		t.Fatal("unbounded protected event accepted")
	}
}

func TestOwnedRuleRedactionNilContextsAndPrivilegeShapes(t *testing.T) {
	p, j, b := ownedTestSetup(t)
	o, err := createOwned(context.Background(), p, j, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{o, *o, ownedEvent(p, b.definition, "created", 3)} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			text := fmt.Sprintf(format, v)
			for _, private := range []string{p.leaseSHA, p.receipt.RuleName, p.receipt.Worker.Path, p.receipt.Request.UserSID} {
				if strings.Contains(text, private) {
					t.Fatal("native authority leaked through fmt")
				}
			}
		}
	}
	if _, err := CreateOwnedRule(nil, lanfirewall.Receipt{}, lanfirewall.InterfaceBinding{}, "", nil); err == nil {
		t.Fatal("nil create context")
	}
	reads, deletes := b.readCalls, b.deleteCalls
	if _, err := RefreshOwnedRule(nil, o, nil); err == nil {
		t.Fatal("nil refresh context")
	}
	if _, err := RetireOwnedRule(nil, o, ownedTestClosure(p), j); err == nil {
		t.Fatal("nil removal context")
	}
	if b.readCalls != reads || b.deleteCalls != deletes {
		t.Fatal("nil context entered backend")
	}
	if !ownedElevationAllowed(1, 4, true) {
		t.Fatal("positive native elevation shape rejected")
	}
	for _, v := range []struct {
		value, size uint32
		absent      bool
	}{{0, 4, true}, {2, 4, true}, {1, 0, true}, {1, 8, true}, {1, 4, false}} {
		if ownedElevationAllowed(v.value, v.size, v.absent) {
			t.Fatal("unknown/non-elevated/delegated token shape admitted")
		}
	}
}

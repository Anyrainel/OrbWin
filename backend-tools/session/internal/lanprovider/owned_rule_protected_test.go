package lanprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/lanlease"
)

func ownedProtectedFixture(t *testing.T) ([]byte, []byte, ownedPolicy, traversalDeclarations) {
	t.Helper()
	p := ownedTestPolicy(t)
	now := time.Now().UTC()
	r := p.receipt.Request
	spec := lanlease.Spec{Request: r, LeaseID: strings.Repeat("2", 32), WorkerRoot: `C:\ProgramData\OrbWin\LAN`, Worker: p.receipt.Worker, Publication: lanlease.Publication{OwnerInstance: strings.Repeat("3", 32), Daemon: "owned-daemon", ContainerID: strings.Repeat("4", 64), StartedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), Generation: 1, HostPort: r.Scope.LocalPort, ContainerPort: 80, Protocol: r.Scope.Protocol}}
	spec.Worker.Path = spec.WorkerRoot + `\` + r.Installation + `\` + spec.LeaseID + `\orbwin-session.exe`
	manager := ownedTestClosure(p).Worker
	manager.PID = r.CallerPID
	manager.CreationTime = r.CallerCreationTime
	manager.ImageSHA256 = r.CallerImageSHA256
	issued, _ := time.Parse(time.RFC3339Nano, r.IssuedAt)
	lease, err := lanlease.NewOwner(r.Installation, manager).Prepare(spec, issued)
	if err != nil {
		t.Fatal(err)
	}
	p.leaseSHA = lease.Snapshot(issued).LeaseSHA
	p.receipt.Worker = spec.Worker
	p.spec.Options.Program = spec.Worker.Path
	meta := ownedProtectedMetadata{Version: 2, ReviewID: strings.Repeat("5", 32), ReviewSHA256: r.ReviewSHA256, OperationID: strings.Repeat("6", 32), RuntimeInstance: strings.Repeat("7", 32), Child: manager}
	meta.Namespace.ContainerID = strings.Repeat("8", 64)
	meta.Namespace.DaemonID = spec.Publication.Daemon
	meta.Child.PID = 2345
	meta.Child.CreationTime = "234567890"
	meta.Prepared.Spec = spec
	meta.Prepared.LeaseSHA = p.leaseSHA
	meta.Prepared.Sequence = 2
	definition := ownedTestDefinition(p)
	for name, v := range definition.rule.values {
		c := propertyContracts[name]
		v.flags = 4 | instanceNotModifiedFlag
		if c.readOnly {
			v.flags |= 1 << 21
		}
		if c.key {
			v.flags |= 1 << 12
		}
		definition.rule.values[name] = v
	}
	for i := range definition.filters {
		for name, v := range definition.filters[i].values {
			flags := filterMutableMetadataFlags
			if c, ok := commonFilterProperties[name]; ok {
				flags = filterReadOnlyMetadataFlags
				if c.key {
					flags = filterKeyMetadataFlags
				}
			}
			v.flags = (flags &^ instanceNullFlag) | instanceNotModifiedFlag
			if name == "StatusDescriptions" {
				v.flags |= instanceNullFlag
			} // legitimate typed NULL array state.
			definition.filters[i].values[name] = v
		}
	}
	wire := ownedDefinitionWire(definition)
	decl, err := ownedProtectedDeclarations(wire)
	if err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(meta)
	var lines []byte
	for i, phase := range []string{"create-intent", "create-returned", "created"} {
		d := ownedDefinition{}
		if i == 1 {
			d.keys = definition.keys
		}
		if i == 2 {
			d = definition
		}
		event := ownedEvent(p, d, phase, uint64(i+1))
		data, e := event.ProtectedBytes()
		if e != nil {
			t.Fatal(e)
		}
		lines = append(lines, data...)
		lines = append(lines, '\n')
	}
	return metadata, lines, p, decl
}
func TestOwnedRuleProtectedParserPreservesTypedNullsAndExactBaseline(t *testing.T) {
	meta, events, p, decl := ownedProtectedFixture(t)
	a, err := parseOwnedProtected(meta, events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID, traversalDeclarations{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := parseOwnedProtected(meta, events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID, decl)
	if err != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("native declaration recheck changed protected baseline", err)
	}
	for _, f := range a.definition.filters {
		v := f.values["StatusDescriptions"]
		if v.flags&instanceNullFlag == 0 || v.texts != nil {
			t.Fatal("native NULL array state collapsed")
		}
	}
	if a.sequence != 3 || a.policy.leaseSHA != p.leaseSHA {
		t.Fatal("ownership correlation changed")
	}
}
func TestOwnedRuleProtectedParserRejectsTornDuplicateForeignAndPendingRecords(t *testing.T) {
	meta, events, p, _ := ownedProtectedFixture(t)
	for _, tc := range []struct {
		name                   string
		meta, events           []byte
		installation, sha, sid string
	}{
		{"torn", meta, events[:len(events)-1], p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID},
		{"duplicate", bytes.Replace(meta, []byte(`"version":2`), []byte(`"version":2,"version":2`), 1), events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID},
		{"case-alias", bytes.Replace(meta, []byte(`"version":2`), []byte(`"Version":2`), 1), events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID},
		{"missing", bytes.Replace(meta, []byte(`"version":2,`), nil, 1), events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID},
		{"unknown", bytes.Replace(meta, []byte(`"version":2`), []byte(`"version":2,"extra":0`), 1), events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID},
		{"foreign-sid", meta, events, p.receipt.Request.Installation, p.leaseSHA, "S-1-5-21-1-2-3-9999"},
		{"foreign-lease", meta, events, p.receipt.Request.Installation, strings.Repeat("e", 64), p.receipt.Request.UserSID},
		{"trailing", append(append([]byte(nil), meta...), []byte(` {}`)...), events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID},
		{"candidate-only", meta, events[:bytes.LastIndex(events[:len(events)-1], []byte{'\n'})+1], p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID},
		{"remove-pending", meta, append(append([]byte(nil), events...), []byte("{}\n")...), p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID},
		{"changed-review", bytes.Replace(meta, []byte(p.receipt.Request.ReviewSHA256), []byte(strings.Repeat("a", 64)), 1), events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID},
		{"null-metadata", bytes.Replace(meta, []byte(`"prepared":{`), []byte(`"prepared":null,"discard":{`), 1), events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseOwnedProtected(tc.meta, tc.events, tc.installation, tc.sha, tc.sid, traversalDeclarations{}); err == nil {
				t.Fatal("malformed/unproven record accepted")
			}
		})
	}
}
func TestOwnedRuleProtectedStrictStorageSchemaBudgets(t *testing.T) {
	for _, raw := range [][]byte{[]byte(`{"Kind":29,"Flags":0,"Scalar":0,"Text":"","Shorts":null,"Texts":null,"Timestamp":false,"Parts":[0,0,0,0,0,0,0],"UTC":0}`)} {
		var value ownedWireValue
		if decodeOwnedProtected(raw, &value) != nil {
			t.Fatal("legitimate typed null state rejected")
		}
		for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"Kind":29`), []byte(`"Kind":29,"Kind":29`), 1), bytes.Replace(raw, []byte(`"Kind":29`), []byte(`"Kind":null`), 1), bytes.Replace(raw, []byte(`"Parts":[0,0,0,0,0,0,0]`), []byte(`"Parts":[0]`), 1), bytes.Repeat([]byte{'x'}, OwnedRuleEventBudget+1), []byte{0xff}} {
			if decodeOwnedProtected(bad, &value) == nil {
				t.Fatal("malformed typed storage shape accepted")
			}
		}
	}
}

func TestOwnedRuleProtectedAckNeedsCurrentExactTypedObservationAndCannotRestoreOwner(t *testing.T) {
	meta, events, p, _ := ownedProtectedFixture(t)
	observed, err := parseOwnedProtected(meta, events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID, traversalDeclarations{})
	if err != nil {
		t.Fatal(err)
	}
	readAt := time.Now().UTC()
	ack, err := acknowledgeProtectedRule(observed, observed.definition, readAt)
	if err != nil || ack.Sequence != 3 || ack.ReadAt != readAt || !ack.AcknowledgementFlushed {
		t.Fatal(err)
	}
	if _, err := acknowledgeProtectedRule(observed, observed.definition, time.Time{}); err == nil {
		t.Fatal("missing native observation time admitted")
	}
	changed := observed.definition
	changed.keys.policyRuleName = "different"
	if _, err := acknowledgeProtectedRule(observed, changed, readAt); err == nil {
		t.Fatal("changed full-key native state acknowledged")
	}
	var owner OwnedRule
	if err = json.Unmarshal(meta, &owner); err != nil {
		t.Fatal(err)
	}
	if _, err = RetireOwnedRule(context.Background(), &owner, ownedTestClosure(p), &ownedTestJournal{}); err == nil {
		t.Fatal("protected JSON constructed mutation authority")
	}
}

func TestOwnedRuleProtectedV2NamespaceAndRemovalPhases(t *testing.T) {
	metadata, events, p, d := ownedProtectedFixture(t)
	var m ownedProtectedMetadata
	if decodeOwnedProtected(metadata, &m) != nil {
		t.Fatal("fixture")
	}
	for _, kind := range []string{"v1", "zero", "workload", "daemon"} {
		t.Run(kind, func(t *testing.T) {
			bad := m
			switch kind {
			case "v1":
				bad.Version = 1
			case "zero":
				bad.Namespace.ContainerID = strings.Repeat("0", 64)
			case "workload":
				bad.Namespace.ContainerID = bad.Prepared.Spec.Publication.ContainerID
			case "daemon":
				bad.Namespace.DaemonID = "other"
			}
			b, _ := json.Marshal(bad)
			if _, e := parseOwnedProtectedMode(b, events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID, d, true); e == nil {
				t.Fatal("invalid namespace accepted")
			}
		})
	}
	original, e := parseOwnedProtectedMode(metadata, events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID, d, true)
	if e != nil {
		t.Fatal(e)
	}
	for i, phase := range []string{"created", "remove-intent", "removed"} {
		if i > 0 {
			v := ownedEvent(p, original.definition, phase, uint64(i+3))
			b, e := v.ProtectedBytes()
			if e != nil {
				t.Fatal(e)
			}
			events = append(events, append(b, '\n')...)
		}
		observed, e := parseOwnedProtectedMode(metadata, events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID, d, true)
		if e != nil || observed.journalPhase != phase {
			t.Fatal(phase, e)
		}
		if i > 0 {
			if _, e := parseOwnedProtected(metadata, events, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID, d); e == nil {
				t.Fatal("removed record granted created owner")
			}
		}
		for _, absent := range []bool{false, true} {
			a, e := acknowledgeProtectedRemoval(observed, absent)
			want := phase == "removed" && absent
			if (e == nil) != want || want && !a.ObservedAbsent {
				t.Fatal(phase, absent, a, e)
			}
		}
	}
}

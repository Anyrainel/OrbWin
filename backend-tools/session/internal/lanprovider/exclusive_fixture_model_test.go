package lanprovider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFixtureDuplicateUnknownAndMissingReturnedKeysNeverConferOwnership(t *testing.T) {
	intent := fixtureIntent{name: "OrbWin-LAN-proof-0123456789abcdef0123456789abcdef-i", actor: "A"}
	keys := observedRuleKeys{instanceID: intent.name, creationClassName: "private-created", policyRuleName: "private-key", systemCreationClassName: "private-system", systemName: "private-host"}
	good := fixtureCompletion{recorded: true, dispatched: true, terminal: true, closed: true, returnedFullKeys: true, keys: keys}
	if _, e := fixtureGrantOwnership(intent, good); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*fixtureIntent, *fixtureCompletion){
		func(i *fixtureIntent, c *fixtureCompletion) { i.incumbent = true },
		func(i *fixtureIntent, c *fixtureCompletion) { c.terminal = false },
		func(i *fixtureIntent, c *fixtureCompletion) { c.closed = false },
		func(i *fixtureIntent, c *fixtureCompletion) { c.recorded = false },
		func(i *fixtureIntent, c *fixtureCompletion) { c.dispatched = false },
		func(i *fixtureIntent, c *fixtureCompletion) { c.returnedFullKeys = false },
		func(i *fixtureIntent, c *fixtureCompletion) { c.status = 11; c.returnedFullKeys = false },
		func(i *fixtureIntent, c *fixtureCompletion) { c.keys.instanceID = "private-foreign" },
		func(i *fixtureIntent, c *fixtureCompletion) { c.keys.policyRuleName = "" },
	} {
		i, c := intent, good
		change(&i, &c)
		if owner, e := fixtureGrantOwnership(i, c); e == nil || owner.intent.name != "" {
			t.Fatal("collision/unknown/foreign create adopted")
		}
	}
	if fixturePrivilegePrecondition(false) == nil {
		t.Fatal("non-elevated fixture accepted")
	}
}
func TestFixtureRaceRequiresOnePositiveCreateAndIndependentDefiniteRefusal(t *testing.T) {
	a := fixtureIntent{name: "OrbWin-LAN-proof-0123456789abcdef0123456789abcdef-r", actor: "C"}
	b := a
	b.actor = "D"
	success := fixtureCompletion{recorded: true, dispatched: true, terminal: true, closed: true, returnedFullKeys: true, keys: observedRuleKeys{instanceID: a.name, creationClassName: "C", policyRuleName: "P", systemCreationClassName: "S", systemName: "H"}}
	refused := fixtureCompletion{recorded: true, dispatched: true, terminal: true, closed: true, status: 11}
	if owner, e := fixtureRaceOwner(a, b, success, refused); e != nil || owner.intent.actor != "C" {
		t.Fatal("single acknowledged winner refused")
	}
	for _, other := range []fixtureCompletion{success, {dispatched: true}, {dispatched: true, terminal: true, closed: true, status: 1}, {dispatched: true, terminal: true, status: 11}} {
		if _, e := fixtureRaceOwner(a, b, success, other); e == nil {
			t.Fatal("ambiguous success/transport/generic failure became owned")
		}
	}
}
func TestFixtureRetirementHasNoNameOnlyUnknownOrChangedStateFallback(t *testing.T) {
	i := fixtureIntent{name: "OrbWin-LAN-proof-0123456789abcdef0123456789abcdef-i", actor: "A"}
	keys := observedRuleKeys{instanceID: i.name, creationClassName: "C", policyRuleName: "P", systemCreationClassName: "S", systemName: "H"}
	owner, e := fixtureGrantOwnership(i, fixtureCompletion{recorded: true, dispatched: true, terminal: true, closed: true, returnedFullKeys: true, keys: keys})
	if e != nil {
		t.Fatal(e)
	}
	baseline := fixtureDefinition{keys: keys, rule: typedInstance{class: ClassName, values: map[string]typedValue{"Enabled": {kind: 3, scalar: 2}}}}
	if fixtureRetireGuard(owner, baseline, baseline, true, true) != nil {
		t.Fatal("exact live fixture retirement refused")
	}
	changed := baseline
	changed.rule = typedInstance{class: ClassName, values: map[string]typedValue{"Enabled": {kind: 3, scalar: 1}}}
	for _, c := range []struct {
		o            fixtureOwnership
		d            fixtureDefinition
		live, synced bool
	}{{fixtureOwnership{}, baseline, true, true}, {owner, changed, true, true}, {owner, baseline, false, true}, {owner, baseline, true, false}} {
		if fixtureRetireGuard(c.o, baseline, c.d, c.live, c.synced) == nil {
			t.Fatal("unowned/changed/unsynced retirement accepted")
		}
	}
	data, _ := json.Marshal(owner)
	if strings.Contains(string(data), "private-") || string(data) != "{}" {
		t.Fatal("fixture capability became a serializable receipt")
	}
}

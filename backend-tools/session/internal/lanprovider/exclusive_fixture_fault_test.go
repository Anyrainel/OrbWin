package lanprovider

import (
	"context"
	"errors"
	"testing"
)

type faultFixtureJournal struct {
	events []string
	fail   string
	after  func(string)
}

func (j *faultFixtureJournal) record(stage string, _ fixtureIntent, _ fixtureCompletion) error {
	j.events = append(j.events, stage)
	if stage == j.fail {
		return errors.New("simulated durable flush failure")
	}
	if j.after != nil {
		j.after(stage)
	}
	return nil
}
func TestFixtureCancelledRetirementAndUnknownDeleteDoNotRetryOrFallback(t *testing.T) {
	i := fixtureIntent{name: "OrbWin-LAN-proof-0123456789abcdef0123456789abcdef-i", actor: "A"}
	owner, _ := fixtureGrantOwnership(i, fixtureTestSuccess(i))
	state := fixtureDefinition{keys: owner.keys}
	ctx, cancel := context.WithCancel(context.Background())
	j := &faultFixtureJournal{after: func(stage string) {
		if stage == "remove-intent" {
			cancel()
		}
	}}
	calls := 0
	if fixtureDeleteTransaction(ctx, j, owner, state, state, func() error { calls++; return nil }) == nil || calls != 0 {
		t.Fatal("cancelled removal dispatched after intent")
	}
	j = &faultFixtureJournal{}
	calls = 0
	if fixtureDeleteTransaction(context.Background(), j, owner, state, state, func() error { calls++; return errors.New("unknown transport after dispatch") }) == nil || calls != 1 || len(j.events) != 1 || j.events[0] != "remove-intent" {
		t.Fatal("unknown delete retried, gained terminal receipt or used fallback")
	}
}
func fixtureTestSuccess(i fixtureIntent) fixtureCompletion {
	return fixtureCompletion{recorded: true, dispatched: true, terminal: true, closed: true, returnedFullKeys: true, keys: observedRuleKeys{instanceID: i.name, creationClassName: "C", policyRuleName: "P", systemCreationClassName: "S", systemName: "H"}}
}
func TestFixtureDurableIntentAndAcknowledgementFailuresDoNotGrantAuthority(t *testing.T) {
	i := fixtureIntent{name: "OrbWin-LAN-proof-0123456789abcdef0123456789abcdef-i", actor: "A"}
	for _, stage := range []string{"create-intent", "create-result"} {
		j := &faultFixtureJournal{fail: stage}
		calls := 0
		_, owner, e := fixtureCreateTransaction(context.Background(), j, i, func() fixtureCompletion {
			calls++
			if len(j.events) != 1 || j.events[0] != "create-intent" {
				t.Fatal("create dispatched without durable intent")
			}
			return fixtureTestSuccess(i)
		})
		if e == nil || owner.intent.name != "" || calls != map[string]int{"create-intent": 0, "create-result": 1}[stage] {
			t.Fatal("failed flush granted authority or dispatched without intent")
		}
	}
}
func TestFixtureCancellationAndUnknownCreateNeverReachDelete(t *testing.T) {
	i := fixtureIntent{name: "OrbWin-LAN-proof-0123456789abcdef0123456789abcdef-i", actor: "A"}
	for _, unknown := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		j := &faultFixtureJournal{}
		_, owner, e := fixtureCreateTransaction(ctx, j, i, func() fixtureCompletion {
			if unknown {
				return fixtureCompletion{dispatched: true}
			}
			cancel()
			return fixtureTestSuccess(i)
		})
		cancel()
		if e == nil || owner.intent.name != "" {
			t.Fatal("unknown/cancelled completion adopted")
		}
		deletes := 0
		if fixtureDeleteTransaction(context.Background(), j, owner, fixtureDefinition{}, fixtureDefinition{}, func() error { deletes++; return nil }) == nil || deletes != 0 {
			t.Fatal("unknown created resource deleted by fallback")
		}
	}
}
func TestFixtureRetirementFlushFailureAndDriftDoNotDispatchDelete(t *testing.T) {
	i := fixtureIntent{name: "OrbWin-LAN-proof-0123456789abcdef0123456789abcdef-i", actor: "A"}
	owner, _ := fixtureGrantOwnership(i, fixtureTestSuccess(i))
	retained := fixtureDefinition{keys: owner.keys}
	changed := retained
	changed.keys.policyRuleName = "foreign-replacement"
	for _, c := range []struct {
		fail    string
		current fixtureDefinition
	}{{"remove-intent", retained}, {"", changed}} {
		j := &faultFixtureJournal{fail: c.fail}
		calls := 0
		if fixtureDeleteTransaction(context.Background(), j, owner, retained, c.current, func() error { calls++; return nil }) == nil || calls != 0 {
			t.Fatal("retirement without exact state and flushed intent dispatched")
		}
	}
}

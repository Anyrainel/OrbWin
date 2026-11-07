package lanprovider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func fakeTypedRule(t *testing.T) typedInstance {
	d := qualifiedRuleDeclaration(t)
	out := typedInstance{class: ClassName, values: map[string]typedValue{}}
	for name, p := range d.properties {
		out.values[name] = typedValue{kind: p.Type, flags: p.Flags | instanceNotModifiedFlag}
	}
	for name, text := range map[string]string{"InstanceID": exactRuleSelector, "CreationClassName": "private-rule-class", "PolicyRuleName": "private-rule-key", "SystemCreationClassName": "private-system-class", "SystemName": "private-host"} {
		p := d.properties[name]
		out.values[name] = typedValue{kind: 13, flags: (p.Flags &^ instanceNullFlag) | instanceNotModifiedFlag, text: text}
	}
	p := d.properties["Enabled"]
	out.values["Enabled"] = typedValue{kind: 3, flags: (p.Flags &^ instanceNullFlag) | instanceNotModifiedFlag, scalar: 1}
	return out
}
func fakeTypedFilter(t *testing.T, index int) typedInstance {
	v := observedFilterMetadata(t)[index]
	out := typedInstance{class: v.Class.Name, values: map[string]typedValue{}}
	for _, p := range v.Class.Properties {
		out.values[p.Name] = typedValue{kind: p.Type, flags: p.Flags | instanceNotModifiedFlag}
	}
	for _, name := range []string{"Name", "CreationClassName", "SystemCreationClassName", "SystemName"} {
		value := out.values[name]
		value.flags &^= instanceNullFlag
		value.text = "private-filter-" + name
		out.values[name] = value
	}
	return out
}

type fakeTraversalSession struct {
	t                             *testing.T
	keys                          observedRuleKeys
	rule, filter                  typedInstance
	variant                       string
	forwards, reverses, ruleReads int
	closed                        bool
	cancel                        context.CancelFunc
}

func (s *fakeTraversalSession) Select(context.Context) (readCursor, error) {
	return oneCursor(func() error { s.rule = fakeTypedRule(s.t); s.keys, _ = s.rule.ruleKeys(); return nil }), nil
}
func (s *fakeTraversalSession) ReadRule(context.Context, observedRuleKeys) (readCursor, error) {
	s.ruleReads++
	return oneCursor(func() error {
		s.rule = fakeTypedRule(s.t)
		if s.variant == "rule-drift" && s.ruleReads == 2 {
			v := s.rule.values["Enabled"]
			v.scalar = 2
			s.rule.values["Enabled"] = v
		}
		s.keys, _ = s.rule.ruleKeys()
		return nil
	}), nil
}
func (s *fakeTraversalSession) Forward(_ context.Context, _ observedRuleKeys, index int) (readCursor, error) {
	s.forwards++
	if s.variant == "missing-filter" && index == 2 {
		return &fakeReadCursor{steps: []cursorStep{{settled: true}}}, nil
	}
	c := oneCursor(func() error {
		s.filter = fakeTypedFilter(s.t, index)
		if s.variant == "filter-drift" && s.forwards == 8 {
			v := s.filter.values["IsNegated"]
			v.flags &^= instanceNullFlag
			v.scalar = 1
			s.filter.values["IsNegated"] = v
		}
		if s.cancel != nil {
			s.cancel()
		}
		return nil
	})
	if s.variant == "ambiguous-filter" && index == 2 {
		c.steps = []cursorStep{{hasValue: true}, {hasValue: true}, {settled: true}}
	}
	return c, nil
}
func (s *fakeTraversalSession) Reverse(_ context.Context, _ observedFilterKeys, index int) (readCursor, error) {
	s.reverses++
	c := oneCursor(func() error {
		s.keys, _ = fakeTypedRule(s.t).ruleKeys()
		if s.variant == "reverse-mismatch" && index == 2 {
			s.keys.systemName = "private-other-host"
		}
		return nil
	})
	if s.variant == "ambiguous-reverse" && index == 2 {
		c.steps = []cursorStep{{hasValue: true}, {hasValue: true}, {settled: true}}
	}
	return c, nil
}
func (s *fakeTraversalSession) CurrentRuleKeys() observedRuleKeys { return s.keys }
func (s *fakeTraversalSession) CurrentRule() typedInstance        { return s.rule }
func (s *fakeTraversalSession) CurrentFilter() typedInstance      { return s.filter }
func (s *fakeTraversalSession) Close() error {
	s.closed = true
	if s.variant == "close-failure" {
		return errors.New("cleanup failure")
	}
	return nil
}

func TestTraversalRequiresAllSevenReverseLinksAndFullTypedStability(t *testing.T) {
	for _, variant := range []string{"stable", "missing-filter", "ambiguous-filter", "ambiguous-reverse", "reverse-mismatch", "filter-drift", "rule-drift", "close-failure"} {
		t.Run(variant, func(t *testing.T) {
			s := &fakeTraversalSession{t: t, variant: variant}
			out, e := observeFilterTraversal(context.Background(), func(context.Context) (filterTraversalSession, error) { return s, nil })
			if variant == "stable" {
				if e != nil || !out.Complete || out.Passes != 2 || out.Families != 7 || out.ForwardReads != 14 || out.ReverseReads != 14 || out.RuleFullReads != 3 || !out.TypedValuesStable || !out.ReverseIdentityMatched {
					t.Fatal("complete typed traversal refused")
				}
			} else if e == nil || out.Complete || out.Passes != 0 || out.TypedValuesStable || out.ReverseIdentityMatched {
				t.Fatal("incomplete/drifted/ambiguous traversal accepted")
			}
			data, _ := json.Marshal(out)
			if strings.Contains(string(data), "private-") || out.Owned || out.ActivationAvailable || out.AtomicSnapshot || out.MutationAuthorized || !s.closed {
				t.Fatal("private values/authority escaped or session not closed")
			}
			if e != nil && strings.Contains(e.Error(), "private-") {
				t.Fatal("private value in error")
			}
		})
	}
}
func TestTraversalCancellationCannotReportPartialSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &fakeTraversalSession{t: t, cancel: cancel}
	out, e := observeFilterTraversal(ctx, func(context.Context) (filterTraversalSession, error) { return s, nil })
	if !errors.Is(e, context.Canceled) || out.Complete || out.Passes != 0 || !s.closed || s.reverses != 0 {
		t.Fatal("cancelled traversal continued or reported success")
	}
}
func TestTypedComparisonDoesNotNormalizeNullEmptyArrayOrderOrTokens(t *testing.T) {
	base := func(v typedValue) typedInstance {
		return typedInstance{class: "fixed", values: map[string]typedValue{"fixed": v}}
	}
	for _, pair := range [][2]typedValue{
		{{kind: 13, flags: instanceNullFlag}, {kind: 13, text: ""}},
		{{kind: 29, texts: []string{}}, {kind: 29, flags: instanceNullFlag}},
		{{kind: 29, texts: []string{"Any", "LocalSubnet"}}, {kind: 29, texts: []string{"LocalSubnet", "Any"}}},
		{{kind: 13, text: "Any"}, {kind: 13, text: "*"}},
		{{kind: 12, date: typedDatetime{timestamp: true, parts: [7]uint32{2026, 1, 1}}}, {kind: 12, date: typedDatetime{timestamp: true, parts: [7]uint32{2026, 1, 2}}}},
		{{kind: 12, date: typedDatetime{timestamp: true, utc: -60}}, {kind: 12, date: typedDatetime{timestamp: true, utc: 60}}},
		{{kind: 3, scalar: 1}, {kind: 3, scalar: 1, flags: instanceNotModifiedFlag}},
	} {
		if sameTypedInstance(base(pair[0]), base(pair[1])) {
			t.Fatal("distinct typed observations normalized")
		}
	}
}
func TestTypedPayloadBoundsFailBeforeAdvancingCounters(t *testing.T) {
	b := &typedCopyBudget{}
	for n := 0; n < 4; n++ {
		b.beginField()
		if b.reserve(maxTypedFieldBytes) != nil {
			t.Fatal("exact field budget refused")
		}
	}
	b.beginField()
	if b.used != maxTypedPayloadBytes || b.reserve(1) == nil || b.used != maxTypedPayloadBytes {
		t.Fatal("cumulative payload overflow accepted")
	}
	b = &typedCopyBudget{}
	if b.reserve(maxTypedFieldBytes+1) == nil || b.used != 0 || b.reserve(-1) == nil {
		t.Fatal("oversized/negative reservation changed budget")
	}
}

func TestPrivateTypedRuleAndFilterValuesCannotBeJSONReceipts(t *testing.T) {
	for _, value := range []typedInstance{fakeTypedRule(t), fakeTypedFilter(t, 0)} {
		data, e := json.Marshal(value)
		if e != nil || string(data) != "{}" {
			t.Fatal("private observed values became serializable")
		}
	}
}

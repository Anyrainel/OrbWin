package lanprovider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeReadCursor struct {
	steps                   []cursorStep
	next                    int
	copies, cancels, closes int
	copy                    func() error
	closeErr                error
}

func (c *fakeReadCursor) Next() cursorStep {
	if c.next >= len(c.steps) {
		panic("cursor read beyond terminal")
	}
	v := c.steps[c.next]
	c.next++
	return v
}
func (c *fakeReadCursor) CopyCurrent(context.Context) error {
	c.copies++
	if c.copy != nil {
		return c.copy()
	}
	return nil
}
func (c *fakeReadCursor) Cancel() error { c.cancels++; return nil }
func (c *fakeReadCursor) Close() error  { c.closes++; return c.closeErr }
func oneCursor(copy func() error) *fakeReadCursor {
	return &fakeReadCursor{steps: []cursorStep{{hasValue: true}, {settled: true}}, copy: copy}
}

func TestInstanceReadFailureDrainsWithoutCopyingBorrowedResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &fakeReadCursor{steps: []cursorStep{{hasValue: true}, {hasValue: true}, {settled: true}}, copy: func() error { cancel(); return nil }}
	found, e := consumeOne(ctx, c)
	if !errors.Is(e, context.Canceled) || found || c.copies != 1 || c.cancels != 1 || c.closes != 1 || c.next != 3 {
		t.Fatal("cancellation lost positive-terminal ownership")
	}
}
func TestInstanceReadRejectsAmbiguityAndCleanupFailure(t *testing.T) {
	for _, c := range []*fakeReadCursor{
		{steps: []cursorStep{{hasValue: true}, {hasValue: true}, {settled: true}}},
		{steps: []cursorStep{{hasValue: true, settled: true}}, closeErr: errors.New("close failure")},
		{steps: []cursorStep{{}, {}, {}, {}, {}, {settled: true}}},
	} {
		found, e := consumeOne(context.Background(), c)
		if e == nil || found || c.closes != 1 {
			t.Fatal("ambiguous, unbounded or cleanup-failed read accepted")
		}
	}
}

type fakeKeySession struct {
	current  observedRuleKeys
	query    *fakeReadCursor
	reads    []*fakeReadCursor
	at       int
	closed   bool
	closeErr error
}

func (s *fakeKeySession) Query(context.Context) (readCursor, error) { return s.query, nil }
func (s *fakeKeySession) Read(context.Context, observedRuleKeys) (readCursor, error) {
	if s.at >= len(s.reads) {
		return nil, errors.New("unexpected extra read")
	}
	c := s.reads[s.at]
	s.at++
	return c, nil
}
func (s *fakeKeySession) CurrentKeys() observedRuleKeys { return s.current }
func (s *fakeKeySession) Close() error                  { s.closed = true; return s.closeErr }
func TestFullKeysRoundTripRequiresTwoStableReadsAndKeepsValuesPrivate(t *testing.T) {
	original := observedRuleKeys{exactRuleSelector, "private-creation", "private-policy", "private-system-class", "private-host"}
	for _, variant := range []string{"stable", "changed", "missing", "close-failed"} {
		t.Run(variant, func(t *testing.T) {
			s := &fakeKeySession{current: original}
			s.query = oneCursor(nil)
			s.reads = []*fakeReadCursor{oneCursor(nil), oneCursor(nil)}
			if variant == "changed" {
				s.reads[1].copy = func() error { s.current.systemName = "other-private-host"; return nil }
			}
			if variant == "missing" {
				s.reads[1] = &fakeReadCursor{steps: []cursorStep{{settled: true}}}
			}
			if variant == "close-failed" {
				s.closeErr = errors.New("cleanup failed")
			}
			out, e := keyRoundTrip(context.Background(), func(context.Context) (keySession, error) { return s, nil })
			if variant == "stable" {
				if e != nil || !out.Complete || !out.ExactIDAndKeysMatched || out.RoundTrips != 2 || out.KeyCount != 4 {
					t.Fatal("stable round-trip refused")
				}
			} else if e == nil || out.Complete || out.Found || out.RoundTrips != 0 {
				t.Fatal("partial or changed observation accepted")
			}
			data, _ := json.Marshal(out)
			if strings.Contains(string(data), "private-") || out.Owned || out.ActivationAvailable || out.AtomicSnapshot || out.MutationAuthorized || !s.closed {
				t.Fatal("private keys or mutation authority escaped")
			}
		})
	}
}
func TestExactSelectorAbsenceAndCancellationAreNotPositiveGates(t *testing.T) {
	s := &fakeKeySession{query: &fakeReadCursor{steps: []cursorStep{{settled: true}}}}
	out, e := keyRoundTrip(context.Background(), func(context.Context) (keySession, error) { return s, nil })
	if e != nil || !out.Complete || out.Status != "not-found" || out.Found || out.ExactIDAndKeysMatched || s.at != 0 {
		t.Fatal("absent selection became positive gate")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opened := false
	out, e = keyRoundTrip(ctx, func(context.Context) (keySession, error) { opened = true; return s, nil })
	if !errors.Is(e, context.Canceled) || opened || out.Complete {
		t.Fatal("cancelled selection opened provider")
	}
}
func associationFixture() FilterSchema {
	return FilterSchema{Class: Class{Name: filterFamilies[0].association, Namespace: Namespace, Provider: "wfascim", Properties: []Property{{Name: "GroupComponent", Type: 14, Flags: 4096}, {Name: "PartComponent", Type: 14, Flags: 4096}}}, References: []ReferenceMetadata{{"GroupComponent", ClassName}, {"PartComponent", filterFamilies[0].class}}}
}
func TestAssociationContractRejectsReversedMissingAndUnkeyedEnds(t *testing.T) {
	if e := validateFilterSchema(associationFixture(), 7); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*FilterSchema){
		func(v *FilterSchema) {
			v.References[0].Class = filterFamilies[0].class
			v.References[1].Class = ClassName
		},
		func(v *FilterSchema) { v.References = v.References[:1] },
		func(v *FilterSchema) { v.Class.Properties[0].Flags = 0 },
		func(v *FilterSchema) { v.Class.Properties = append(v.Class.Properties, v.Class.Properties[0]) },
		func(v *FilterSchema) { v.Class.Name = "MSFT_Unexpected" },
	} {
		v := associationFixture()
		mutate(&v)
		if validateFilterSchema(v, 7) == nil {
			t.Fatal("association drift accepted")
		}
	}
}

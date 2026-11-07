//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func successorFixture(t *testing.T) (writerRecoveryV2, []byte, string) {
	t.Helper()
	i, ib, _, _ := recoveryFixture(t)
	profile := `C:\Users\owner`
	selector := func(b string) string {
		return profile + `\.orbwin-private\runtime\docker-` + strings.Repeat(b, 64) + `\docker-cli-plugins`
	}
	baseline := []byte(`{ "auths":{}, "credsStore":"orbwin", "cliPluginsExtraDirs":[` + freshQuote(selector("a"), false) + `], "HttpHeaders": {"X":"kept <raw>"} }`)
	target, e := recoveryTarget(baseline, selector("c"), profile)
	if e != nil {
		t.Fatal(e)
	}
	v := recoveryV2From(writerRecoveryRecord{Schema: 1, Kind: "restore-original", Binding: i.Binding, CurrentVolume: 1, CurrentHigh: 2, CurrentLow: 3, CurrentSD: i.BaseSD, CurrentSHA: digestBytes(baseline), RestoreSHA: digestBytes(target), IntentBody: string(ib)}, selector("c"))
	v.PreviousVolume, v.PreviousHigh, v.PreviousLow = 1, 2, 4
	v.PreviousSD = i.BaseSD
	v.PreviousSHA = digestBytes([]byte("actual old journal"))
	v.PreviousRestoreSHA = digestBytes([]byte("actual B target"))
	v.PublishedBaselineSHA = v.PreviousRestoreSHA
	v.StagePresent = true
	v.StageVolume, v.StageHigh, v.StageLow = 1, 2, 5
	v.StageSD = i.BaseSD
	v.StageSHA = digestBytes([]byte("captured partial stage"))
	return v, baseline, profile
}
func TestRecoverySuccessorEveryProducerPrefixAndStaleFullTarget(t *testing.T) {
	v, baseline, profile := successorFixture(t)
	body, e := recoveryV2Bytes(v)
	if e != nil {
		t.Fatal(e)
	}
	for cut := 0; cut < len(body); cut++ {
		partial, e := validateNextBody(body[:cut], v, baseline, profile)
		if e != nil || !partial {
			t.Fatalf("producer cut %d refused: %v", cut, e)
		}
	}
	partial, e := validateNextBody(body, v, baseline, profile)
	if e != nil || partial {
		t.Fatal("full next not admitted")
	}
	oldTarget := v
	oldTarget.TargetSelector = strings.Replace(v.TargetSelector, strings.Repeat("c", 64), strings.Repeat("b", 64), 1)
	b, e := recoveryTarget(baseline, oldTarget.TargetSelector, profile)
	if e != nil {
		t.Fatal(e)
	}
	oldTarget.RestoreSHA = digestBytes(b)
	oldBytes, _ := recoveryV2Bytes(oldTarget)
	if partial, e := validateNextBody(oldBytes, v, baseline, profile); e != nil || partial {
		t.Fatal("valid full old-selector next cannot be explicitly superseded")
	}
	for cut := 0; cut < len(oldBytes); cut++ {
		if !recoveryNextPrefix(oldBytes[:cut], v, profile) {
			t.Fatalf("prior target producer prefix cut %d refused", cut)
		}
	}
	for name, mutate := range map[string]func(writerRecoveryV2) writerRecoveryV2{
		"different binding":        func(x writerRecoveryV2) writerRecoveryV2 { x.Binding = digestBytes([]byte("foreign")); return x },
		"old journal generation":   func(x writerRecoveryV2) writerRecoveryV2 { x.PreviousLow++; return x },
		"old journal descriptor":   func(x writerRecoveryV2) writerRecoveryV2 { x.PreviousSD = digestBytes([]byte("different")); return x },
		"current generation":       func(x writerRecoveryV2) writerRecoveryV2 { x.CurrentLow++; return x },
		"current bytes":            func(x writerRecoveryV2) writerRecoveryV2 { x.CurrentSHA = digestBytes([]byte("changed")); return x },
		"stage generation":         func(x writerRecoveryV2) writerRecoveryV2 { x.StageLow++; return x },
		"target hash not preimage": func(x writerRecoveryV2) writerRecoveryV2 { x.RestoreSHA = digestBytes([]byte("guess")); return x },
		"selector wrong profile": func(x writerRecoveryV2) writerRecoveryV2 {
			x.TargetSelector = strings.Replace(x.TargetSelector, "owner", "someone", 1)
			return x
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := mutate(v)
			b, _ := json.Marshal(changed)
			if _, e := validateNextBody(b, v, baseline, profile); e == nil {
				t.Fatal("unbound next accepted")
			}
		})
	}
	// Invalid JSON must not become a reusable interrupted NEXT, even when an old
	// record exists. A valid but complete unrelated object is also not a prefix.
	for _, bad := range [][]byte{[]byte(`{"schema":2,"bogus":`), []byte(`{"schema":2,"kind":"restore-original","binding":"wrong`), []byte(`{"schema":2}`), []byte(`{"schema":2,"x":!}`), []byte(`[`)} {
		if _, e := validateNextBody(bad, v, baseline, profile); e == nil {
			t.Fatal("unknown partial or complete record accepted")
		}
	}
}
func TestRecoverySuccessorDurabilityRecheckAndPublicationOrder(t *testing.T) {
	for _, failure := range []string{"create", "write", "flush", "readback", "close", "open", "reopen-drift", "recheck", "rename", ""} {
		t.Run(failure, func(t *testing.T) {
			storage := &memoryPrivateRecord{failure: failure}
			oldIntact := true
			published := false
			sourceChecked := false
			e := durableSuccessorTransition(func() error { return durablePrivateRecord([]byte("full successor metadata"), storage) }, func() error {
				if !oldIntact || storage.writer || !storage.held {
					t.Fatal("old changed before durable readonly NEXT")
				}
				sourceChecked = true
				if failure == "recheck" {
					return errors.New("source drift")
				}
				return nil
			}, func() error {
				if !sourceChecked || !oldIntact {
					t.Fatal("replacement before actual source recheck")
				}
				if failure == "rename" {
					return errors.New("native rename result unknown")
				}
				oldIntact = false
				published = true
				return nil
			})
			if (e == nil) != (failure == "") || published != (failure == "") {
				t.Fatalf("failure=%s published=%v err=%v", failure, published, e)
			}
			if failure != "" && !oldIntact {
				t.Fatal("failed transition erased old authority")
			}
		})
	}
}
func TestRecoverySuccessorManagedRewritesPreserveOriginalRawFields(t *testing.T) {
	v, baseline, profile := successorFixture(t)
	bsel := strings.Replace(v.TargetSelector, strings.Repeat("c", 64), strings.Repeat("b", 64), 1)
	b, e := recoveryTarget(baseline, bsel, profile)
	if e != nil {
		t.Fatal(e)
	}
	c, e := recoveryTarget(baseline, v.TargetSelector, profile)
	if e != nil {
		t.Fatal(e)
	}
	cFromPublishedB, e := recoveryTarget(b, v.TargetSelector, profile)
	if e != nil || !bytes.Equal(c, cFromPublishedB) {
		t.Fatal("actual owner-published B cleanup changes baseline user fields")
	}
	if !bytes.Contains(c, []byte(`"HttpHeaders":{"X":"kept <raw>"}`)) {
		t.Fatal("raw baseline field value rewritten")
	}
	// Both Go and Rust's existing producer preserve raw field values and use the
	// Go quote codec for field names/current selector (including HTML/U+2028).
	htmlProfile := "C:\\Users\\<owner>\u2028"
	htmlSelector := htmlProfile + `\.orbwin-private\runtime\docker-` + strings.Repeat("a", 64) + `\docker-cli-plugins`
	quoted := freshQuote(htmlSelector, false)
	if !strings.Contains(quoted, `\u003c`) || !strings.Contains(quoted, `\u2028`) {
		t.Fatal("Go cross-language quote contract drifted")
	}
}
func TestRecoverySuccessorMaximumWireBudgetAndClosedSchema(t *testing.T) {
	v, _, _ := successorFixture(t)
	// Worst producer metadata: raw intent at its 4096 bound with heavily escaped
	// admitted digest strings, plus the 1024-UTF16 selector's six-byte Go escapes.
	intent := v.IntentBody
	for _, field := range []string{"binding", "keysSha256", "baseSha256", "baseSdSha256", "originalSdSha256", "originalSha256"} {
		var parsed map[string]any
		_ = json.Unmarshal([]byte(intent), &parsed)
		value := parsed[field].(string)
		var escaped strings.Builder
		for _, b := range []byte(value) {
			escaped.WriteString(`\u00`)
			const hex = "0123456789abcdef"
			escaped.WriteByte(hex[b>>4])
			escaped.WriteByte(hex[b&15])
		}
		old := freshQuote(field, false) + ":" + freshQuote(value, false)
		replacement := freshQuote(field, false) + `:"` + escaped.String() + `"`
		if len(intent)+len(replacement)-len(old) <= 4096 {
			intent = strings.Replace(intent, old, replacement, 1)
		}
	}
	if len(intent) > 4096 {
		t.Fatal("test intent exceeds source bound")
	}
	intent += strings.Repeat("\t", 4096-len(intent))
	if _, e := parseWriterIntent([]byte(intent)); e != nil {
		t.Fatal(e)
	}
	v.IntentBody = intent
	suffix := `\.orbwin-private\runtime\docker-` + strings.Repeat("c", 64) + `\docker-cli-plugins`
	profile := `C:\` + strings.Repeat("\u2028", 1024-3-len(suffix))
	v.TargetSelector = profile + suffix
	body, e := recoveryV2Bytes(v)
	if e != nil {
		t.Fatalf("valid worst-case record exceeds 16KiB: bytes=%d err=%v", len(body), e)
	}
	t.Logf("maximum-bound producer record: %d / %d bytes", len(body), recoveryV2Budget)
	if len(body) <= 8192 {
		t.Fatal("long record did not exercise enlarged bound")
	}
	// Closed writer-intent values are ASCII enum/digest/number fields. Valid
	// raw whitespace or escape spelling expands at most 2x when quoted, unlike
	// arbitrary secret/config strings. A selector UTF16 unit expands at most
	// six Go-JSON bytes. Include maximal native numeric fields and all digests.
	maxV := v
	maxV.CurrentVolume, maxV.CurrentHigh, maxV.CurrentLow = ^uint32(0), ^uint32(0), ^uint32(0)
	maxV.PreviousVolume, maxV.PreviousHigh, maxV.PreviousLow = ^uint32(0), ^uint32(0), ^uint32(0)
	maxV.StageVolume, maxV.StageHigh, maxV.StageLow = ^uint32(0), ^uint32(0), ^uint32(0)
	full, _ := json.Marshal(maxV)
	iq, _ := json.Marshal(maxV.IntentBody)
	sq, _ := json.Marshal(maxV.TargetSelector)
	worst := len(full) - len(iq) - len(sq) + (2*4096 + 2) + (6*1024 + 2)
	if worst > recoveryV2Budget {
		t.Fatalf("conservative closed-schema upper bound %d exceeds %d", worst, recoveryV2Budget)
	}
	t.Logf("conservative exact-schema upper bound %d / %d", worst, recoveryV2Budget)

	for _, cut := range []int{8192, 8193, len(body) - 1} {
		if !incompleteRecoveryRecord(body[:cut]) || !recoveryNextPrefix(body[:cut], v, profile) {
			t.Fatalf("long producer prefix %d refused", cut)
		}
	}
	for _, bad := range [][]byte{append(append([]byte(nil), body[:len(body)-1]...), []byte(`,"extra":1}`)...), bytes.Replace(body, []byte(`"stagePresent":true`), []byte(`"stagePresent":null`), 1), bytes.Replace(body, []byte(`"schema":2`), []byte(`"Schema":2`), 1), append(body, ' ')} {
		if len(bad) == len(body)+1 {
			continue
		} // trailing JSON whitespace is allowed for a full closed journal.
		if _, _, e := parseRecoveryAny(bad); e == nil {
			t.Fatal("open/aliased/null schema accepted")
		}
	}
}

func TestRecoverySuccessorMissingOriginalNeverReusesPrestateOrForeignBinding(t *testing.T) {
	v, _, _ := successorFixture(t)
	base := v.base()
	for _, tt := range []struct {
		name          string
		v2            *writerRecoveryV2
		prior, strict bool
		hash          string
		want          bool
	}{
		{"schema1 prestate is not original", nil, true, true, base.CurrentSHA, false},
		{"schema1 owner target", nil, false, true, base.RestoreSHA, true},
		{"writer descriptor is not publication", nil, false, false, base.RestoreSHA, false},
		{"successor actual captured B", &v, true, true, v.PublishedBaselineSHA, true},
		{"B wrong native generation", &v, false, true, v.PublishedBaselineSHA, false},
		{"current C committed", &v, false, true, v.RestoreSHA, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if recoveryPublishedBaseline(base, tt.v2, tt.prior, tt.strict, tt.hash) != tt.want {
				t.Fatal("missing baseline classification wrong")
			}
		})
	}
	foreign := v
	foreign.Binding = digestBytes([]byte("different writer generation"))
	if recoveryPublishedBaseline(base, &foreign, true, true, v.PreviousRestoreSHA) {
		t.Fatal("foreign no-birth binding reused")
	}
	foreign = v
	foreign.IntentBody = " " + foreign.IntentBody
	if recoveryPublishedBaseline(base, &foreign, true, true, v.PreviousRestoreSHA) {
		t.Fatal("different original authority reused")
	}
	if refusePendingPrivateRecords(func(name string) (bool, error) { return name == "recovery.next", nil }) == nil {
		t.Fatal("NEXT alone permits ordinary admission")
	}
}
func TestRecoverySuccessorBirthOnlyCleanupThenFreshManagedTransaction(t *testing.T) {
	v, baseline, profile := successorFixture(t)
	bSelector := strings.Replace(v.TargetSelector, strings.Repeat("c", 64), strings.Repeat("b", 64), 1)
	b, e := recoveryTarget(baseline, bSelector, profile)
	if e != nil {
		t.Fatal(e)
	}
	b = bytes.Replace(b, []byte(`"auths":{}`), []byte(`"auths":{"registry.example":{"auth":"synthetic-existing-value"}}`), 1)
	c, e := recoveryTarget(b, v.TargetSelector, profile)
	if e != nil {
		t.Fatal(e)
	}
	for _, failure := range []string{"check", "prepare-delete", "birth-delete", "between", "new-local-intent", "new-publication", ""} {
		t.Run(failure, func(t *testing.T) {
			current := append([]byte(nil), b...)
			birthPending, preparePending, localIntent := true, true, false
			e := completedWriterRefresh(func() error {
				if failure == "check" {
					return errors.New("actual source drift")
				}
				return nil
			}, func() error {
				if failure == "prepare-delete" {
					return errors.New("old prepare closure unknown")
				}
				preparePending = false
				if failure == "birth-delete" {
					return errors.New("old birth closure unknown")
				}
				birthPending = false
				return nil
			}, func() error {
				if birthPending || preparePending {
					t.Fatal("new transaction reused old writer authority")
				}
				if failure == "between" {
					return errors.New("canceled between completed cleanup and fresh transaction")
				}
				localIntent = true
				if failure == "new-local-intent" {
					return errors.New("fresh local journal interrupted")
				}
				if failure == "new-publication" {
					return errors.New("fresh local publication unknown")
				}
				current = append([]byte(nil), c...)
				localIntent = false
				return nil
			})
			if (e == nil) != (failure == "") {
				t.Fatal("transition status wrong")
			}
			if failure == "between" && (birthPending || localIntent || !bytes.Equal(current, b)) {
				t.Fatal("completed cleanup/cancel did not leave healthy unchanged B")
			}
			if failure == "new-local-intent" || failure == "new-publication" {
				if birthPending || !localIntent || !bytes.Equal(current, b) {
					t.Fatal("new local failure lost its own journal or resurrected old birth")
				}
			}
			if failure == "" && (!bytes.Equal(current, c) || !bytes.Contains(current, []byte(`"auth":"synthetic-existing-value"`)) || !bytes.Contains(current, []byte(`{"X":"kept <raw>"}`))) {
				t.Fatal("ordinary refresh changed existing auth/raw values")
			}
		})
	}
}

func TestRecoverySuccessorRepeatedUpgradeBeforePublicationKeepsActualBaseline(t *testing.T) {
	v, _, _ := successorFixture(t)
	actualB := v.PublishedBaselineSHA
	// B is actual owner-published bytes. C was merely the first successor target;
	// D is a later successor. The flat published witness remains B, never C.
	third := v
	third.PreviousRestoreSHA = v.RestoreSHA
	third.RestoreSHA = digestBytes([]byte("uncommitted target D"))
	third.PreviousSHA = digestBytes([]byte("actual complete C journal"))
	if !recoveryPublishedBaseline(third.base(), &third, true, true, actualB) {
		t.Fatal("repeated prepublication upgrade lost actual published B")
	}
	if recoveryPublishedBaseline(third.base(), &third, true, true, v.RestoreSHA) {
		t.Fatal("uncommitted C target was guessed as baseline")
	}
	third.PublishedBaselineSHA = ""
	if recoveryPublishedBaseline(third.base(), &third, true, true, actualB) {
		t.Fatal("lost B witness inferred from predecessor target")
	}
}
func TestRecoverySuccessorProductionTargetCoordinatorMissingOriginalBToCAndD(t *testing.T) {
	v, originalA, profile := successorFixture(t)
	selector := func(pin string) string {
		return profile + `\.orbwin-private\runtime\docker-` + strings.Repeat(pin, 64) + `\docker-cli-plugins`
	}
	b, e := recoveryTarget(originalA, selector("b"), profile)
	if e != nil {
		t.Fatal(e)
	}
	c, e := recoveryTarget(originalA, selector("c"), profile)
	if e != nil {
		t.Fatal(e)
	}
	d, e := recoveryTarget(originalA, selector("d"), profile)
	if e != nil {
		t.Fatal(e)
	}
	// This is the same data coordinator called by prepareHeldWriterRecovery BEFORE
	// target construction. Native observations remain data inputs; these tests do
	// not construct or return native guards/process/resource grants.
	old1 := v.base()
	old1.Schema = 1
	old1.CurrentSHA = digestBytes([]byte("recorded writer output before restoration"))
	old1.RestoreSHA = digestBytes(b)
	called := 0
	plan, e := prepareRecoveryDataPlan(nil, b, profile, selector("c"), false, &old1, nil, false, true, func() error { called++; return nil })
	if e != nil || called != 1 || !plan.needsPublish || !bytes.Equal(plan.baseline, b) || !bytes.Equal(plan.target, c) {
		t.Fatalf("published B -> C failed actual coordinator: calls=%d err=%v", called, e)
	}
	// First successor has planned C but current is still actual B. An upgrade D
	// must carry B's published witness, not demand current already routes to D or
	// guess the uncommitted C hash as a current baseline.
	pendingC := v
	pendingC.CurrentSHA = digestBytes(b)
	pendingC.RestoreSHA = digestBytes(c)
	pendingC.TargetSelector = selector("c")
	pendingC.PublishedBaselineSHA = digestBytes(b)
	pendingC.PreviousRestoreSHA = digestBytes(b)
	baseC := pendingC.base()
	called = 0
	plan, e = prepareRecoveryDataPlan(nil, b, profile, selector("d"), false, &baseC, &pendingC, true, true, func() error { called++; return nil })
	if e != nil || called != 1 || !plan.needsPublish || !bytes.Equal(plan.baseline, b) || !bytes.Equal(plan.target, d) {
		t.Fatalf("published B / uncommitted C -> D failed actual coordinator: calls=%d err=%v", called, e)
	}
	for _, tt := range []struct {
		name          string
		body          []byte
		prior, strict bool
		gateErr       error
	}{
		{"wrong native current generation", b, false, true, nil},
		{"nonprivate current", b, true, false, nil},
		{"not actually owner published", originalA, true, true, nil},
		{"actual stopped/root/source check fails", b, true, true, errors.New("stopped native root is unknown")},
		{"unknown current bytes", []byte(`{"auths":{}}`), true, true, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, e := prepareRecoveryDataPlan(nil, tt.body, profile, selector("d"), false, &baseC, &pendingC, tt.prior, tt.strict, func() error { return tt.gateErr }); e == nil {
				t.Fatal("production target coordinator bypassed source/publication gate")
			}
		})
	}
	// Original-present rollback remains independent of current routing/descriptor:
	// actual OLD current identity and original native A are checked by the native
	// callback. A corrupt writer output may be restored from independently held A.
	called = 0
	plan, e = prepareRecoveryDataPlan(originalA, []byte("corrupt writer output"), profile, selector("c"), true, &old1, nil, true, false, func() error { called++; return nil })
	if e != nil || called != 1 || !bytes.Equal(plan.baseline, originalA) || !bytes.Equal(plan.target, c) {
		t.Fatalf("original-present recorded-current rollback changed: %v", e)
	}
	if _, e := prepareRecoveryDataPlan(nil, b, profile, selector("c"), false, &old1, nil, false, true, nil); e == nil {
		t.Fatal("missing actual authority callback accepted")
	}
}

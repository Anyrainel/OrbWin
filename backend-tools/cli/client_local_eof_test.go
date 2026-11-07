//go:build windows

package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestLocalEOFExactActualProducerPrefixes(t *testing.T) {
	current := []byte("actual independently held strict current")
	base := digestBytes(current)
	for _, next := range []string{"", strings.Repeat("b", 64)} {
		encoded, e := localReplaceProducerBytes(base, next)
		if e != nil {
			t.Fatal(e)
		}
		// Current Go map order and pinned Rust BTreeMap order, not struct order.
		want := `{"baseSha256":"` + base + `","kind":"config-replace",`
		if next != "" {
			want += `"nextSha256":"` + next + `",`
		}
		want += `"schema":1}`
		if string(encoded) != want {
			t.Fatal("current compact producer order changed")
		}
		for cut := 0; cut < len(encoded); cut++ {
			if !localReplaceEOFPrefix(encoded[:cut], base) {
				t.Fatalf("producer next=%q cut=%d refused", next, cut)
			}
		}
		if localReplaceEOFPrefix(encoded, base) || localReplaceEOFPrefix(append(encoded, ' '), base) {
			t.Fatal("complete record admitted as EOF")
		}
		for _, bad := range [][]byte{
			append([]byte(" "), encoded[:len(encoded)-1]...),
			bytes.Replace(encoded[:len(encoded)-1], []byte("baseSha256"), []byte("BaseSha256"), 1),
			bytes.Replace(encoded[:len(encoded)-1], []byte(base), []byte(strings.Repeat("c", 64)), 1),
			bytes.Replace(encoded[:len(encoded)-1], []byte("config-replace"), []byte("docker-writer"), 1),
			[]byte(`{"schema":1,"kind":"config-replace","baseSha256":"`),
			[]byte(`{"baseSha256":null,"kind":"config-replace",`),
			[]byte(`{"baseSha256":"` + base + `","unknown":`),
			[]byte(`{"baseSha256":"` + base + `","kind":"config-replace","nextSha256":"ABC`),
		} {
			if localReplaceEOFPrefix(bad, base) {
				t.Fatalf("non-producer EOF admitted: %q", bad)
			}
		}
	}
	if localReplaceEOFPrefix(make([]byte, 4097), base) || localReplaceEOFPrefix(nil, "bad digest") {
		t.Fatal("unbounded or unknown-current input admitted")
	}
}

func TestLocalEOFActualPlanKeepsBThenRefreshesC(t *testing.T) {
	profile := `C:\Users\<owner>`
	selector := func(pin string) string {
		return profile + `\` + freshPrivateDirectory + `\runtime\docker-` + strings.Repeat(pin, 64) + `\docker-cli-plugins`
	}
	current := []byte(`{ "auths":{}, "credsStore":"orbwin", "cliPluginsExtraDirs":[` + freshQuote(selector("b"), false) + `],"X":{"opaque":1e9999} }`)
	encoded, _ := localReplaceProducerBytes(digestBytes(current), strings.Repeat("a", 64))
	partial := encoded[:len(encoded)-1]
	calls := 0
	plan, e := prepareLocalEOFPlan(partial, current, profile, selector("c"), true, func() error { calls++; return nil })
	if e != nil || calls != 1 || !plan.changed || !bytes.Contains(plan.target, []byte(`"opaque":1e9999`)) || privateClientRouting(plan.target, selector("c")) != nil {
		t.Fatalf("actual plan refused native-gated B→C: %v", e)
	}
	for _, failure := range []string{"no-native-check", "native-source", "not-strict", "unknown-current", "foreign-selector", "wrong-base", "complete-intent"} {
		t.Run(failure, func(t *testing.T) {
			body, config, strict, check := partial, current, true, func() error { return nil }
			switch failure {
			case "no-native-check":
				check = nil
			case "native-source":
				check = func() error { return errors.New("native witness/refusal") }
			case "not-strict":
				strict = false
			case "unknown-current":
				config = []byte(`{"auths":{}}`)
			case "foreign-selector":
				config = bytes.Replace(current, []byte(freshPrivateDirectory), []byte(freshPrivateDirectory+"-foreign"), 1)
			case "wrong-base":
				body = bytes.Replace(partial, []byte(digestBytes(current)), []byte(strings.Repeat("f", 64)), 1)
			case "complete-intent":
				body = encoded
			}
			if _, e := prepareLocalEOFPlan(body, config, profile, selector("c"), strict, check); e == nil {
				t.Fatal("actual coordinator bypassed required source/prefix gate")
			}
		})
	}
}

func TestLocalEOFApplyFaultBoundariesNeverReplayOrLoseCurrent(t *testing.T) {
	for _, failure := range []string{"source-recheck", "intent-dispose", "cancel-between", "new-intent", "new-publication", ""} {
		t.Run(failure, func(t *testing.T) {
			oldIntent, newIntent, current := true, false, "B"
			refreshes := 0
			e := applyLocalEOF(func() error {
				if failure == "source-recheck" {
					return errors.New("actual source/absence changed")
				}
				return nil
			}, func() error {
				if failure == "intent-dispose" {
					return errors.New("native close/delete unknown; fence")
				}
				oldIntent = false
				return nil
			}, func() error {
				if oldIntent {
					t.Fatal("new transaction before old intent disposal")
				}
				if failure == "cancel-between" {
					return errors.New("canceled with healthy B")
				}
				refreshes++
				newIntent = true
				if failure == "new-intent" || failure == "new-publication" {
					return errors.New("separate new transaction pending")
				}
				current = "C"
				newIntent = false
				return nil
			})
			if (e == nil) != (failure == "") {
				t.Fatalf("wrong outcome: %v", e)
			}
			if failure == "source-recheck" || failure == "intent-dispose" {
				if !oldIntent || refreshes != 0 || current != "B" {
					t.Fatal("failed old transaction changed current or refreshed")
				}
			} else if failure == "cancel-between" {
				if oldIntent || newIntent || current != "B" {
					t.Fatal("cancel did not leave healthy B")
				}
			} else if failure != "" && (oldIntent || !newIntent || current != "B") {
				t.Fatal("new failure lost its own journal")
			}
		})
	}
}

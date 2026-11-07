package main

import (
	"strings"
	"testing"
)

func TestLocalReplaceClosedProducerShapesAndCurrentClassification(t *testing.T) {
	base, next, other := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	goRecord := `{"schema":1,"kind":"config-replace","baseSha256":"` + base + `"}`
	rustRecord := strings.TrimSuffix(goRecord, "}") + `,"nextSha256":"` + next + `"}`
	for _, body := range []string{goRecord, rustRecord} {
		r, target, e := parseLocalReplaceIntent([]byte(body))
		if e != nil || r.BaseSHA != base || !localReplaceCurrentMatches(r, target, base) || !localReplaceCurrentMatches(r, target, next) {
			t.Fatal("exact local producer record refused")
		}
		if localReplaceCurrentMatches(r, target, other) != (target == "") {
			t.Fatal("target-bound Rust record admitted unrelated current bytes")
		}
	}
	for _, body := range []string{
		"", "{}", "[]", goRecord[:len(goRecord)-1], goRecord + "{}",
		strings.Replace(goRecord, `"schema":1`, `"schema":3`, 1),
		strings.Replace(goRecord, `"schema":1`, `"schema":null`, 1),
		strings.Replace(goRecord, `"schema":1`, `"Schema":1`, 1),
		strings.Replace(goRecord, `"schema":1`, `"schema":1,"schema":1`, 1),
		strings.Replace(goRecord, "config-replace", "docker-writer", 1),
		strings.Replace(goRecord, base, strings.ToUpper(base), 1),
		strings.TrimSuffix(goRecord, "}") + `,"unknown":true}`,
		strings.TrimSuffix(goRecord, "}") + `,"nextSha256":null}`,
		strings.TrimSuffix(goRecord, "}") + `,"nextSha256":""}`,
		strings.TrimSuffix(goRecord, "}") + `,"NextSha256":"` + next + `"}`,
	} {
		if _, _, e := parseLocalReplaceIntent([]byte(body)); e == nil {
			t.Fatalf("accepted unbound or non-producer record %q", body)
		}
	}
	r := &clientWriterRecovery{localReplace: true}
	if !strings.Contains(r.summary(), "No saved original is restored") || strings.Contains(r.summary(), "exact durable") {
		t.Fatal("local recovery claimed a saved original")
	}
}

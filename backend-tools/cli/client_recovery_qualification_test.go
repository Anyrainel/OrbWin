package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecoveryQualificationClosedEventCodes(t *testing.T) {
	events := []recoveryQualificationFault{recoveryHistoricalJournal,
		recoveryPublishedOriginalRemoved, recoveryCutNextPrefix, recoveryNextDurable,
		recoveryNextBeforeRename, recoveryNextPublished,
		recoveryHistoricalStagePrepared, recoveryStageRetired, recoveryLocalEmpty, recoveryLocalEarly, recoveryLocalBase, recoveryLocalLate, recoveryWriterIntentPrefix, recoveryLostMetadataRename, recoveryLostTargetRename, recoveryLostStageDisposition, recoveryLostSnapshotConfigClose}
	seen := make(map[recoveryQualificationFault]bool)
	for _, f := range events {
		if f < 1 || f > 17 || seen[f] {
			t.Fatal("closed checkpoint code collision")
		}
		seen[f] = true
	}
	if len(seen) != 17 {
		t.Fatal("closed checkpoint count")
	}
	// Never arm a native checkpoint or manufacture a native owner in pure tests.
}
func TestRecoveryQualificationActualPrefixSelection(t *testing.T) {
	h := strings.Repeat("a", 64)
	for _, next := range []bool{false, true} {
		v := map[string]any{"schema": 1, "kind": "config-replace", "baseSha256": h}
		if next {
			v["nextSha256"] = strings.Repeat("b", 64)
		}
		body, _ := json.Marshal(v)
		for _, f := range []recoveryQualificationFault{recoveryLocalEmpty, recoveryLocalEarly, recoveryLocalBase, recoveryLocalLate} {
			n, ok := recoveryQualificationPrefixCount(f, "writer.intent", body)
			if !ok || n >= len(body) || !localReplaceEOFPrefix(body[:n], h) {
				t.Fatal("actual closed prefix", f)
			}
			if _, ok := recoveryQualificationPrefixCount(f, "writer.original", body); ok {
				t.Fatal("wrong private object")
			}
		}
		if _, ok := recoveryQualificationPrefixCount(recoveryWriterIntentPrefix, "writer.intent", body); ok {
			t.Fatal("local shape cannot become writer scope")
		}
	}
	if _, ok := recoveryQualificationPrefixCount(recoveryLocalLate, "writer.intent", []byte(`{}`)); ok {
		t.Fatal("unbound JSON")
	}
}

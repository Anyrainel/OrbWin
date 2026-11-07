//go:build windows

package main

import (
	"encoding/json"
	"testing"
)

func TestWriterRecoveryDispatchPreservesSchema3PartialIntent(t *testing.T) {
	_, intentBytes, _, _ := recoveryFixture(t)
	var fields map[string]any
	if e := json.Unmarshal(intentBytes, &fields); e != nil {
		t.Fatal(e)
	}
	// Actual Go writer producer is a sorted map, unlike the parsing struct.
	producer, e := json.Marshal(fields)
	if e != nil {
		t.Fatal(e)
	}
	for cut := 0; cut < len(producer); cut++ {
		for _, siblings := range []recoverySiblingPresence{
			{prepare: true, original: true},
			{prepare: true, original: true, birth: true},
			{prepare: true, original: true, recovery: true, next: true},
		} {
			dispatch, e := classifyRecoveryDispatch(true, producer[:cut], siblings, digestBytes([]byte("actual retained current")))
			if e != nil || dispatch.route != writerRecoveryRoute {
				t.Fatalf("existing independent schema3 preIntent route lost at %d: %v", cut, e)
			}
		}
	}
	if dispatch, e := classifyRecoveryDispatch(true, producer, recoverySiblingPresence{prepare: true, original: true}, digestBytes(nil)); e != nil || dispatch.route != writerRecoveryRoute {
		t.Fatal("complete schema3 writer route changed")
	}
}

func TestWriterRecoveryDispatchLocalEOFCompleteUnknownAndOrphanNext(t *testing.T) {
	currentSHA := digestBytes([]byte("strict held current"))
	local, _ := localReplaceProducerBytes(currentSHA, "")
	for cut := 0; cut < len(local); cut++ {
		dispatch, e := classifyRecoveryDispatch(true, local[:cut], recoverySiblingPresence{}, currentSHA)
		if e != nil || dispatch.route != localEOFRoute {
			t.Fatalf("no-sibling exact local cut %d lost: %v", cut, e)
		}
		// Independent records take priority even for a common zero/early prefix.
		for _, sibling := range []recoverySiblingPresence{{prepare: true}, {original: true}, {birth: true}, {recovery: true}} {
			dispatch, e = classifyRecoveryDispatch(true, local[:cut], sibling, currentSHA)
			if e != nil || dispatch.route != writerRecoveryRoute {
				t.Fatal("partial writer evidence diverted to local EOF")
			}
		}
	}
	dispatch, e := classifyRecoveryDispatch(true, local, recoverySiblingPresence{}, currentSHA)
	if e != nil || dispatch.route != localCompleteRoute || dispatch.local.BaseSHA != currentSHA {
		t.Fatal("existing complete local provider not selected")
	}
	for _, body := range [][]byte{[]byte(`{"unknown":`), []byte(`{"schema":3,"kind":"docker-writer",`), []byte(`{"baseSha256":"wrong`)} {
		if _, e := classifyRecoveryDispatch(true, body, recoverySiblingPresence{}, currentSHA); e == nil {
			t.Fatal("unknown EOF without independent records admitted")
		}
	}
	for _, present := range []bool{false, true} {
		if _, e := classifyRecoveryDispatch(present, local[:5], recoverySiblingPresence{next: true}, currentSHA); e == nil {
			t.Fatal("orphan NEXT admitted by actual dispatch classifier")
		}
	}
	if _, e := classifyRecoveryDispatch(true, local, recoverySiblingPresence{original: true}, currentSHA); e == nil {
		t.Fatal("full local record mixed with writer evidence admitted")
	}
	if dispatch, e := classifyRecoveryDispatch(false, nil, recoverySiblingPresence{prepare: true}, currentSHA); e != nil || dispatch.route != writerRecoveryRoute {
		t.Fatal("existing no-intent prepare route changed")
	}
}

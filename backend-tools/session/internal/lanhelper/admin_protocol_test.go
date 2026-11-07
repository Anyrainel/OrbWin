package lanhelper

import (
	"bytes"
	"encoding/json"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
	"strings"
	"testing"
)

func TestLANAdminFramesAreClosedBoundedAndCorrelated(t *testing.T) {
	record, prepared := protectedRecordFixture(t)
	page := uint32(0)
	more := false
	stones := []lanlease.Tombstone{}
	frames := []AdminFrame{{Version: 2, Kind: "review", Sequence: 1, Review: &record}, {Version: 2, Kind: "review-accepted", Sequence: 1, RequestSequence: 1}, {Version: 2, Kind: "read-tombstones", Sequence: 2, Page: &page}, {Version: 2, Kind: "tombstones", Sequence: 2, RequestSequence: 2, Page: &page, Tombstones: &stones, More: &more}, {Version: 2, Kind: "stage", Sequence: 3}, {Version: 2, Kind: "prepared", Sequence: 3, RequestSequence: 3, Prepared: &prepared}, {Version: 2, Kind: "commit", Sequence: 4, NamespaceID: strings.Repeat("a", 64), Child: &lanfirewall.CallerIdentity{PID: 101}, Observation: &lanlease.Observation{Scope: record.Stage.Base.Scope}}, {Version: 2, Kind: "pulse", Sequence: 5}, {Version: 2, Kind: "error", Sequence: 4, RequestSequence: 4, ErrorCode: "unknown"}, {Version: 2, Kind: "close", Sequence: 6}}
	for _, frame := range frames {
		var buffer bytes.Buffer
		if err := WriteAdmin(&buffer, frame); err != nil {
			t.Fatal(frame.Kind, err)
		}
		decoded, err := ReadAdmin(&buffer)
		if err != nil || decoded.Kind != frame.Kind {
			t.Fatal(frame.Kind, err)
		}
	}
	good, _ := json.Marshal(AdminFrame{Version: 2, Kind: "pulse", Sequence: 2})
	for _, bad := range []string{strings.Replace(string(good), `"kind"`, `"Kind"`, 1), strings.Replace(string(good), `"sequence":2`, `"sequence":2,"sequence":2`, 1), strings.Replace(string(good), `"sequence":2`, `"sequence":2,"errorCode":""`, 1), strings.Replace(string(good), `"sequence":2`, `"sequence":2,"requestSequence":0`, 1), strings.Replace(string(good), `"sequence":2`, `"sequence":2,"review":null`, 1), string(good) + `{}`} {
		if _, err := DecodeAdmin([]byte(bad)); err == nil {
			t.Fatal("admin shape bypass", bad)
		}
	}
}
func TestLANAdminConversationConsumesBeforeMutationAndDoesNotRetry(t *testing.T) {
	record, _ := protectedRecordFixture(t)
	var session adminConversation
	if err := session.accept(AdminFrame{Version: 2, Kind: "review", Sequence: 1, Review: &record}); err != nil {
		t.Fatal(err)
	}
	if err := session.accept(AdminFrame{Version: 2, Kind: "pulse", Sequence: 2}); err != nil {
		t.Fatal(err)
	}
	if err := session.reviewed(); err != nil {
		t.Fatal(err)
	}
	if err := session.accept(AdminFrame{Version: 2, Kind: "stage", Sequence: 3}); err != nil {
		t.Fatal(err)
	}
	if session.accept(AdminFrame{Version: 2, Kind: "stage", Sequence: 4}) == nil {
		t.Fatal("stage replayed after external dispatch")
	}
	if err := session.prepared(); err != nil {
		t.Fatal(err)
	}
	commit := AdminFrame{Version: 2, Kind: "commit", Sequence: 5, NamespaceID: strings.Repeat("a", 64), Child: &lanfirewall.CallerIdentity{PID: 101}, Observation: &lanlease.Observation{Scope: record.Stage.Base.Scope}}
	if err := session.accept(commit); err != nil {
		t.Fatal(err)
	}
	commit.Sequence = 6
	if session.accept(commit) == nil {
		t.Fatal("commit replayed after unknown completion")
	}
	if err := session.accept(AdminFrame{Version: 2, Kind: "pulse", Sequence: 7}); err != nil {
		t.Fatal(err)
	}
	session.close()
	if session.accept(AdminFrame{Version: 2, Kind: "pulse", Sequence: 8}) == nil {
		t.Fatal("closed conversation revived")
	}
}

func TestLANAdminSettlementReviewCannotAuthorizeDeleteOrReplay(t *testing.T) {
	for _, purpose := range []string{"remove", "settle-remove"} {
		record, prepared := protectedRecordFixture(t)
		record.Purpose, record.LeaseSHA256 = purpose, prepared.LeaseSHA
		var session adminConversation
		if session.accept(AdminFrame{Version: 2, Kind: "review", Sequence: 1, Review: &record}) != nil || session.reviewed() != nil {
			t.Fatal("cleanup review refused")
		}
		other := "remove"
		if purpose == "remove" {
			other = "settle-remove"
		}
		if session.accept(AdminFrame{Version: 2, Kind: other, Sequence: 2}) == nil {
			t.Fatal("review purpose authorized the other cleanup capability")
		}
		frame := AdminFrame{Version: 2, Kind: purpose, Sequence: 3}
		var encoded bytes.Buffer
		if WriteAdmin(&encoded, frame) != nil {
			t.Fatal("closed cleanup frame refused")
		}
		decoded, err := ReadAdmin(&encoded)
		if err != nil || session.accept(decoded) != nil {
			t.Fatal("exact cleanup capability refused", err)
		}
		frame.Sequence++
		if session.accept(frame) == nil {
			t.Fatal("cleanup command replayed after dispatch")
		}
	}
}

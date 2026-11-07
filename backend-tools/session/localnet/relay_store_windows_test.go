//go:build windows

package localnet

import (
	"os"
	"strings"
	"testing"
)

func TestRelayReceiptsPreserveUnknownAcceptanceAndRejectForeignOrPartialState(t *testing.T) {
	intent, _, id := relayFixture(t)
	directory := t.TempDir()
	ledger, err := OpenNamespaceLedger(directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	if err = ledger.BeginRelay(intent); err != nil {
		t.Fatal(err)
	}
	if err = ledger.BeginRelay(intent); err == nil {
		t.Fatal("replayed pending intent")
	}
	records, err := ledger.relayRecords()
	if err != nil || len(records) != 1 {
		t.Fatal(records, err)
	}
	if err = ledger.settleRelay(records[0]); err == nil {
		t.Fatal("forgot unknown create")
	}
	changed := intent
	changed.Gateway.Port++
	if err = ledger.CreatedRelay(changed, id); err == nil {
		t.Fatal("accepted different gateway")
	}
	if err = ledger.CreatedRelay(intent, id); err != nil {
		t.Fatal(err)
	}
	if err = ledger.CreatedRelay(intent, id); err == nil {
		t.Fatal("replayed create acceptance")
	}
	ledger.Close()
	ledger, err = OpenNamespaceLedger(directory, "foreign-daemon")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ledger.relayRecords(); err == nil {
		t.Fatal("foreign daemon accepted")
	}
	ledger.Close()
	ledger, err = OpenNamespaceLedger(directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	records, err = ledger.relayRecords()
	if err != nil || len(records) != 1 || records[0].ID != id {
		t.Fatal(records, err)
	}
	path := ledger.relayPath(intent.Session)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("{\"id\":\"")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err = ledger.relayRecords(); err == nil {
		t.Fatal("partial receipt accepted")
	}
	if err = ledger.settleRelay(records[0]); err == nil {
		t.Fatal("settled changed evidence")
	}
	if err = os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	wrong := records[0]
	wrong.ID = strings.Repeat("1", 64)
	if err = ledger.settleRelay(wrong); err == nil {
		t.Fatal("settled different ID")
	}
	if err = ledger.settleRelay(records[0]); err != nil {
		t.Fatal(err)
	}
	ledger.Close()
	if err = ledger.BeginRelay(intent); err == nil {
		t.Fatal("closed owner wrote intent")
	}
}

//go:build windows

package localnet

import (
	"context"
	"errors"
	"github.com/moby/moby/client"
	"testing"
	"time"
)

func TestColdConstructionReceiptComesFromActualCalleeAndRetainsPartialLedgerFailure(t *testing.T) {
	var construction coldRuntimeConstruction
	if construction.failedBeforeActor() {
		t.Fatal("empty/in-flight constructor counted as completed no-start")
	}
	cli, err := client.New(client.WithHost("http://127.0.0.1:1"), client.WithAPIVersion("1.56"))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	asset, sha := testToolsAsset()
	origin := &stateOriginFake{records: map[string][]byte{settingsName: []byte("invalid")}, finishErr: errors.New("guard finish unknown")}
	store := &coldManagedServiceState{serviceStateStore: origin}
	ledger := &NamespaceLedger{directory: managedStateLabel, daemon: "owned", stateStore: store}
	runtime, err := construction.construct(RuntimeOptions{Client: cli, Ledger: ledger, Installation: "owned", ToolsAsset: asset, ToolsSHA256: sha})
	if runtime != nil || err == nil || !construction.failedBeforeActor() {
		t.Fatal("actual pre-allocation callee failure lost its completion", err)
	}
	if ledger.Close() == nil || store.closed || ledger.closeErr == nil {
		t.Fatal("actual partial ledger owner was treated as settled")
	}
	if _, err := construction.construct(RuntimeOptions{}); err == nil {
		t.Fatal("actual constructor replayed")
	}
}

func TestColdConstructionStartedActorCannotBecomeUnstartedAfterJoin(t *testing.T) {
	var construction coldRuntimeConstruction
	cli, err := client.New(client.WithHost("http://127.0.0.1:1"), client.WithAPIVersion("1.56"))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	asset, sha := testToolsAsset()
	store := &coldManagedServiceState{serviceStateStore: &stateOriginFake{records: map[string][]byte{}}}
	ledger := &NamespaceLedger{directory: managedStateLabel, daemon: "owned", stateStore: store}
	runtime, err := construction.construct(RuntimeOptions{Client: cli, Ledger: ledger, Installation: "owned", ToolsAsset: asset, ToolsSHA256: sha})
	if err != nil || runtime == nil || construction.failedBeforeActor() {
		t.Fatal("actual started actor mislabeled unstarted", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if runtime.Close(ctx) != nil || construction.failedBeforeActor() {
		t.Fatal("joined actor became a no-start receipt")
	}
}

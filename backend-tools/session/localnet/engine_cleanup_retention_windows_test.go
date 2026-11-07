//go:build windows

package localnet

import (
	"context"
	"testing"
)

type imageImportOrigin struct {
	*stateOriginFake
	closes int
}

func (o *imageImportOrigin) Close() error { o.closes++; return o.stateOriginFake.Close() }

func TestNamespaceUnknownCreateCannotReportSuccessfulCloseOrForgetScope(t *testing.T) {
	daemon, cli, directory := managerFixture(t, "uncertain-create")
	manager := openManagerFixture(t, daemon, cli, directory)
	if _, err := manager.Acquire(context.Background(), daemon.intent.ContainerID, daemon.intent.StartedAt); err == nil {
		t.Fatal("fixture failed to produce ambiguous native API acceptance")
	}
	if manager.Close(context.Background()) == nil || manager.Close(context.Background()) == nil || len(manager.pending) != 1 {
		t.Fatal("unknown create vanished from completed Close")
	}
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	if daemon.creates != 1 || daemon.removes != 0 {
		t.Fatal("shutdown replayed unconfirmed create/removal")
	}
}

func TestNativeImageImportUnknownCannotReleaseLedgerOrResolveByLookup(t *testing.T) {
	ledger, daemon, cli, binary, asset := imageStoreFixture(t, "uncertain-missing")
	// Only the native state adapter is modeled. The real ledger/import code and
	// fake HTTP daemon edge execute; no actual engine/helper/process is called.
	if err := ledger.lease.Close(); err != nil {
		t.Fatal(err)
	}
	ledger.lease = nil
	origin := &imageImportOrigin{stateOriginFake: &stateOriginFake{records: map[string][]byte{}}}
	ledger.directory = managedStateLabel
	ledger.stateStore = origin
	if _, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset); err == nil {
		t.Fatal("fixture did not produce ambiguous import")
	}
	if ledger.pendingImport == nil || ledger.Close() == nil || origin.closes != 0 || ledger.closed {
		t.Fatal("unknown import released native owner")
	}
	if _, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset); err == nil {
		t.Fatal("same issuer resolved unknown import from metadata")
	}
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	if daemon.imports != 1 || daemon.lists != 0 {
		t.Fatal("unknown import was replayed or accepted by label lookup")
	}
}

//go:build windows

package localnet

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOwnedBackendFatalHostFailureSettlesRelayBeforeReleasingCommonOwner(t *testing.T) {
	relay, daemon, intent := managedRelayFixture(t, "")
	ctx := context.Background()
	namespaces, err := OpenNamespaceManagerWithLedger(ctx, relay.cli, relay.ledger, relay.image)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := OwnServiceBackend(namespaces, relay)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := NewAuthority(strings.Repeat("8", 32), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	host, err := StartServiceHost(backend, authority, intent.Session)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = relay.Start(ctx, host.Gateway(), intent.Networks); err != nil {
		t.Fatal(err)
	}
	// Losing the gateway listener is a fatal independent service event, without
	// any renderer request. The route host must consume backend cleanup itself.
	_ = host.mux.Close()
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err = host.Wait(bounded); err == nil {
		t.Fatal("fatal listener failure reported success")
	}
	if err = backend.Close(bounded); err != nil {
		t.Fatal(err)
	}
	if daemon.removes != 1 || daemon.exists {
		t.Fatal("fatal shutdown did not remove exact relay")
	}
	if _, err = relay.ledger.relayRecords(); err == nil {
		t.Fatal("namespace owner was not closed")
	}
	owner, err := OpenNamespaceLedger(relay.ledger.directory, "owned-daemon")
	if err != nil {
		t.Fatal("lease retained after shutdown", err)
	}
	defer owner.Close()
	if records, e := owner.relayRecords(); e != nil || len(records) != 0 {
		t.Fatal("relay cleanup ran after owner release", records, e)
	}
	if _, err = backend.Target(ctx, strings.Repeat("1", 64)); err == nil {
		t.Fatal("closed backend admitted new work")
	}
	if err = backend.Close(bounded); err != nil || daemon.removes != 1 {
		t.Fatal("shutdown was not idempotent", err)
	}
}

func TestOwnedBackendCallerCancellationKeepsOwnerUntilCleanupActuallyCompletes(t *testing.T) {
	relay, daemon, intent := managedRelayFixture(t, "")
	ctx := context.Background()
	namespaces, err := OpenNamespaceManagerWithLedger(ctx, relay.cli, relay.ledger, relay.image)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := OwnServiceBackend(namespaces, relay)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = relay.Start(ctx, intent.Gateway, intent.Networks); err != nil {
		t.Fatal(err)
	}
	daemon.mu.Lock()
	daemon.fault = "blocked-remove"
	daemon.removeEntered = make(chan struct{})
	daemon.removeGate = make(chan struct{})
	daemon.mu.Unlock()
	gateClosed := false
	defer func() {
		if !gateClosed {
			close(daemon.removeGate)
		}
	}()
	short, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	if err = backend.Close(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("cancelled caller reported stopped", err)
	}
	select {
	case <-daemon.removeEntered:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not start")
	}
	if duplicate, e := OpenNamespaceLedger(relay.ledger.directory, "owned-daemon"); e == nil {
		duplicate.Close()
		t.Fatal("owner released while relay cleanup was pending")
	}
	if _, err = backend.Target(ctx, strings.Repeat("1", 64)); err == nil {
		t.Fatal("stopping backend admitted work")
	}
	close(daemon.removeGate)
	gateClosed = true
	bounded, done := context.WithTimeout(ctx, time.Second)
	defer done()
	if err = backend.Close(bounded); err != nil {
		t.Fatal("cleanup did not finish independently", err)
	}
	if daemon.removes != 1 || daemon.exists {
		t.Fatal("exact relay remained after completion")
	}
	if err = backend.Close(short); err != nil {
		t.Fatal("completed cleanup result was lost to expired caller", err)
	}
}

func TestBorrowedGenerationClosesWithoutReleasingPermanentSettingsOwner(t *testing.T) {
	relay, _, _ := managedRelayFixture(t, "")
	namespaces, err := OpenNamespaceManagerBorrowingLedger(context.Background(), relay.cli, relay.ledger, relay.image)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := OwnServiceBackend(namespaces, relay)
	if err != nil {
		t.Fatal(err)
	}
	if err = backend.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = relay.ledger.StoreSettings(0, DefaultSettings()); err != nil {
		t.Fatal("generation released permanent owner", err)
	}
	if duplicate, e := OpenNamespaceLedger(relay.ledger.directory, "owned-daemon"); e == nil {
		duplicate.Close()
		t.Fatal("generation opened an ownership gap")
	}
}

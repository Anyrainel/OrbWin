//go:build windows && amd64

package localnet

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/moby/moby/client"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/sdkself"
	"orbwin.local/session/internal/sdkstate"
)

type registrationClientFake struct {
	LANAdministrativeHelper
	manager              lanfirewall.CallerIdentity
	installation, worker string
	closeErr             error
	closes               int
	partial              bool
}

func (f *registrationClientFake) Close(context.Context) error { f.closes++; return f.closeErr }
func (f *registrationClientFake) ManagerIdentity() lanfirewall.CallerIdentity {
	if f.partial {
		panic("partial manager getter")
	}
	return f.manager
}
func (f *registrationClientFake) Installation() string {
	if f.partial {
		panic("partial installation getter")
	}
	return f.installation
}
func (f *registrationClientFake) WorkerSHA256() string {
	if f.partial {
		panic("partial worker getter")
	}
	return f.worker
}

func registrationFixture() (RuntimeOptions, lanfirewall.CallerIdentity, string, *registrationClientFake) {
	self := &sdkself.Identity{Schema: 1, PID: 11, CreationTime: "100", Session: 1, ImageSHA256: strings.Repeat("a", 64), BundleSHA256: strings.Repeat("b", 64), LeaseHeld: true}
	manager := lanfirewall.CallerIdentity{PID: self.PID, CreationTime: self.CreationTime, ImageSHA256: self.ImageSHA256, UserSID: "S-1-5-21-1-2-3-1001"}
	id := strings.Repeat("c", 32)
	return RuntimeOptions{SelfIdentity: self, BinarySHA256: self.ImageSHA256, Installation: "00000000-0000-4000-8000-000000000001"}, manager, id, &registrationClientFake{manager: manager, installation: id, worker: strings.Repeat("d", 64)}
}

func TestNativeLANRegistrationSettlesOnlyExactHeldIdentityAndSignedWorker(t *testing.T) {
	options, manager, id, helper := registrationFixture()
	adapters, err := settleNativeLANRegistration(options, manager, id, helper, nil)
	if err != nil || adapters == nil || adapters.Helper != helper || adapters.Installation != id || adapters.WorkerSHA256 != helper.worker || helper.closes != 0 {
		t.Fatal("exact native facts not retained", err)
	}
	for _, field := range []string{"manager", "installation", "worker"} {
		options, manager, id, helper := registrationFixture()
		switch field {
		case "manager":
			helper.manager.PID++
		case "installation":
			helper.installation = strings.Repeat("e", 32)
		case "worker":
			helper.worker = "invalid"
		}
		adapters, err := settleNativeLANRegistration(options, manager, id, helper, nil)
		if err == nil || adapters != nil || helper.closes != 1 {
			t.Fatal("mismatch retained/admitted registration without positive cleanup", field, err)
		}
	}
}

func TestNativeLANRegistrationUnknownConstructionAndCloseRetainExactHelper(t *testing.T) {
	options, manager, id, helper := registrationFixture()
	failure := errors.New("native construction cleanup unknown")
	helper.partial = true
	adapters, err := settleNativeLANRegistration(options, manager, id, helper, failure)
	if !errors.Is(err, failure) || adapters == nil || adapters.Helper != helper || helper.closes != 0 {
		t.Fatal("partial helper queried, reclosed or lost", err)
	}
	options, manager, id, helper = registrationFixture()
	helper.worker = "invalid"
	helper.closeErr = failure
	adapters, err = settleNativeLANRegistration(options, manager, id, helper, nil)
	if !errors.Is(err, failure) || adapters == nil || adapters.Helper != helper || helper.closes != 1 {
		t.Fatal("failed positive-result cleanup claimed no owner", err)
	}
}

func TestNativeLANRegistrationRejectsPlainOrLookalikeManagedStorageBeforeEffects(t *testing.T) {
	options, _, _, _ := registrationFixture()
	for _, store := range []serviceStateStore{nil, &stateOriginFake{}, &managedServiceState{}, &coldManagedServiceState{serviceStateStore: &stateOriginFake{}}} {
		options.Ledger = &NamespaceLedger{directory: managedStateLabel, daemon: "owned", stateStore: store}
		if _, err := nativeLANManagedRoot(options); err == nil {
			t.Fatal("plain or fake store became native authority")
		}
	}
	options.Ledger = &NamespaceLedger{directory: managedStateLabel, daemon: "owned", stateStore: &managedServiceState{root: &sdkstate.Root{}}}
	if err := validateNativeLANRegistration(options); err == nil {
		t.Fatal("inert root passed actual Root.Check")
	}
	options.LAN = &LANAdapters{}
	if err := validateNativeLANRegistration(options); err == nil {
		t.Fatal("native selector accepted caller adapters")
	}
}

func TestNativeLANRegistrationConstructorRefusesPlainLedgerBeforeSettingsOrNativeFactory(t *testing.T) {
	options, _, _, _ := registrationFixture()
	cli, err := client.New(client.WithHost("http://127.0.0.1:1"), client.WithAPIVersion("1.56"))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	origin := &stateOriginFake{records: map[string][]byte{}}
	options.Client, options.Installation, options.NativeLAN = cli, "00000000-0000-4000-8000-000000000001", true
	options.Ledger = &NamespaceLedger{directory: managedStateLabel, daemon: "owned", stateStore: origin}
	got, err := NewServiceRuntime(options)
	if err == nil || got != nil || len(origin.events) != 0 {
		t.Fatal("plain ledger reached settings/native construction", err)
	}
}

func TestNativeLANRegistrationPartialRuntimeRetainsNoActorAndRejectsUnstartedWitness(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	helper := &registrationClientFake{partial: true}
	ledger := &NamespaceLedger{}
	r := &ServiceRuntime{ctx: ctx, cancel: cancel, done: make(chan struct{}), cleanupResult: make(chan struct{}), opts: RuntimeOptions{Ledger: ledger}}
	failure := errors.New("unknown registration owner")
	got, err := retainFailedLANRegistration(r, &LANAdapters{Helper: helper}, failure)
	if got != r || !errors.Is(err, failure) || r.opts.Ledger != ledger || r.opts.LAN.Helper != helper || r.state.Phase != "cleanup-pending" {
		t.Fatal("partial graph changed")
	}
	if ctx.Err() == nil {
		t.Fatal("partial runtime remained dispatchable")
	}
	select {
	case <-r.cleanupResult:
	default:
		t.Fatal("construction error was not published")
	}
	select {
	case <-r.done:
		t.Fatal("partial runtime claimed resource closure")
	default:
	}
	if !errors.Is(r.Close(context.Background()), failure) || helper.closes != 0 {
		t.Fatal("partial Close retried cleanup or lost failure")
	}
	wait, stop := context.WithCancel(context.Background())
	stop()
	if !errors.Is(r.Wait(wait), context.Canceled) {
		t.Fatal("partial Wait released owners")
	}
	// Contract check only, not a manufactured native construction capability.
	c := &coldRuntimeConstruction{attempted: true, completed: true, runtime: got, err: err}
	if c.failedBeforeActor() {
		t.Fatal("nonnil failed result became no-start")
	}
}

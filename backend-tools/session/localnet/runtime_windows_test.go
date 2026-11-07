//go:build windows

package localnet

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeResumesPublishedAuthorityOnlyWithReviewedFingerprint(t *testing.T) {
	runtime, _ := runtimeFixture(t)
	awaitRuntimePhase(t, runtime, "disabled")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Model a process loss after exclusive protected-authority publication but
	// before its fingerprint was committed to desired settings.
	authority, err := runtime.opts.Ledger.CreateAuthorityWithCRLPort(runtime.opts.Installation, time.Now(), 18080)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runtime.opts.Ledger.directory, authorityStateName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateAuthority(ctx, 0); err == nil {
		t.Fatal("setup replaced a published authority")
	}
	if review, _ := runtime.Snapshot(); review.Authority == nil || review.Authority.RootSHA256 != authority.Fingerprint() {
		t.Fatal("incomplete setup did not expose saved public review metadata")
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	options := runtime.opts
	options.Ledger, err = OpenNamespaceLedger(options.Ledger.directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServiceRuntime(options)
	if err != nil {
		options.Ledger.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		if err := restarted.Close(clean); err != nil {
			t.Error(err)
		}
	})
	state := awaitRuntimePhase(t, restarted, "disabled")
	if state.Authority == nil || state.Authority.RootSHA256 != authority.Fingerprint() || state.Settings.RootSHA256 != "" {
		t.Fatal("interrupted authority was hidden or implicitly adopted")
	}
	if _, err := restarted.AdoptAuthority(ctx, state.Revision, strings.Repeat("0", 64)); err == nil {
		t.Fatal("wrong reviewed fingerprint accepted")
	}
	if _, err := restarted.AdoptAuthority(ctx, state.Revision, authority.Fingerprint()); err == nil {
		t.Fatal("incompatible HTTP port silently accepted")
	}
	settings := state.Settings
	settings.Ports = ServicePorts{HTTP: 18080, HTTPS: 18443}
	stored, err := restarted.Configure(ctx, state.Revision, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.AdoptAuthority(ctx, state.Revision, authority.Fingerprint()); err == nil {
		t.Fatal("stale adoption revision accepted")
	}
	info, err := restarted.AdoptAuthority(ctx, stored.Revision, authority.Fingerprint())
	if err != nil || info.RootSHA256 != authority.Fingerprint() {
		t.Fatal(info, err)
	}
	selected, _ := restarted.Snapshot()
	if selected.Settings.Enabled || selected.Settings.RootSHA256 != authority.Fingerprint() || selected.Revision != stored.Revision+1 {
		t.Fatal("reviewed authority adoption did not persist disabled selection", selected)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("adoption rewrote protected authority", err)
	}
}

func runtimeFixture(t *testing.T) (*ServiceRuntime, *relayDaemon) {
	return runtimeFixtureWithRootStore(t, nil)
}
func runtimeFixtureWithRootStore(t *testing.T, factory func(bool) (RootStore, error)) (*ServiceRuntime, *relayDaemon) {
	return runtimeFixtureWithOptions(t, factory, nil)
}
func runtimeFixtureWithOptions(t *testing.T, factory func(bool) (RootStore, error), configure func(*RuntimeOptions)) (*ServiceRuntime, *relayDaemon) {
	t.Helper()
	relay, daemon, _ := managedRelayFixture(t, "")
	asset, sha := testToolsAsset()
	options := RuntimeOptions{Client: relay.cli, Ledger: relay.ledger, Installation: "00000000-0000-4000-8000-000000000001", ToolsAsset: asset, ToolsSHA256: sha, RootStoreFactory: factory, portProbe: func(ServicePorts) error { return nil }}
	if configure != nil {
		configure(&options)
	}
	runtime, err := NewServiceRuntime(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := runtime.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return runtime, daemon
}

func TestRuntimeWindowsTrustRequiresReviewAndNeverOpensStoreOnStartup(t *testing.T) {
	store := &fixtureRootStore{user: "runtime-user", roots: map[string]bool{}}
	opens := 0
	runtime, _ := runtimeFixtureWithRootStore(t, func(bool) (RootStore, error) { opens++; return store, nil })
	state := awaitRuntimePhase(t, runtime, "disabled")
	if opens != 0 {
		t.Fatal("startup opened a trust store")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := runtime.CreateAuthority(ctx, state.Revision); err != nil {
		t.Fatal(err)
	}
	state, _ = runtime.Snapshot()
	root := state.Settings.RootSHA256
	if err := runtime.WindowsTrust(ctx, "trust-install", state.Revision-1, root); err == nil {
		t.Fatal("stale trust review accepted")
	}
	if err := runtime.WindowsTrust(ctx, "trust-install", state.Revision, strings.Repeat("0", 64)); err == nil {
		t.Fatal("unselected root accepted")
	}
	if opens != 0 {
		t.Fatal("unreviewed install opened a trust store")
	}
	if err := runtime.WindowsTrust(ctx, "trust-status", state.Revision, root); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WindowsTrust(ctx, "trust-install", state.Revision, root); err != nil {
		t.Fatal(err)
	}
	installed, _ := runtime.Snapshot()
	if installed.WindowsTrust == nil || !installed.WindowsTrust.Present || !installed.WindowsTrust.Managed || installed.Revision != state.Revision || installed.Sequence <= state.Sequence {
		t.Fatal("trust observation or settings ownership incorrect", installed)
	}
	if err := os.Remove(filepath.Join(runtime.opts.Ledger.directory, authorityStateName)); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WindowsTrust(ctx, "trust-remove", state.Revision, root); err != nil {
		t.Fatal(err)
	}
	removed, _ := runtime.Snapshot()
	if removed.WindowsTrust == nil || removed.WindowsTrust.Present || removed.WindowsTrust.State != "removed" || store.adds != 1 || store.removes != 1 {
		t.Fatal(removed)
	}
}
func awaitRuntimePhase(t *testing.T, runtime *ServiceRuntime, phase string) RuntimeSnapshot {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		state, err := runtime.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if state.Phase == phase {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime phase %s, wanted %s: %s", state.Phase, phase, state.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func TestRuntimePersistsIntentAndKeepsGenerationOwnerAcrossDisable(t *testing.T) {
	runtime, daemon := runtimeFixture(t)
	awaitRuntimePhase(t, runtime, "disabled")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	settings := DefaultSettings()
	settings.Ports = ServicePorts{HTTP: 18080, HTTPS: 18443}
	stored, err := runtime.Configure(ctx, 0, settings)
	if err != nil {
		t.Fatal(err)
	}
	info, err := runtime.CreateAuthority(ctx, stored.Revision)
	if err != nil {
		t.Fatal(err)
	}
	state, err := runtime.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	settings = state.Settings
	settings.Enabled = true
	if _, err = runtime.Configure(ctx, state.Revision, settings); err != nil {
		t.Fatal(err)
	}
	running := awaitRuntimePhase(t, runtime, "running")
	if running.Settings.RootSHA256 != info.RootSHA256 {
		t.Fatal("runtime changed reviewed root")
	}
	settings = running.Settings
	settings.Enabled = false
	if _, err = runtime.Configure(ctx, running.Revision, settings); err != nil {
		t.Fatal(err)
	}
	disabled := awaitRuntimePhase(t, runtime, "disabled")
	if disabled.Settings.Enabled || disabled.Revision <= running.Revision {
		t.Fatal("disabled intent was not committed")
	}
	daemon.mu.Lock()
	creates, removes := daemon.creates, daemon.removes
	daemon.mu.Unlock()
	if creates != 1 || removes != 1 {
		t.Fatal("generation lifecycle was replayed or leaked", creates, removes)
	}
	if duplicate, e := OpenNamespaceLedger(runtime.opts.Ledger.directory, "owned-daemon"); e == nil {
		duplicate.Close()
		t.Fatal("disable released process ownership")
	}
	if _, err = runtime.CreateAuthority(ctx, disabled.Revision); err == nil {
		t.Fatal("existing authority silently replaced")
	}
	settings = disabled.Settings
	settings.Enabled = true
	settings.Ports = ServicePorts{HTTP: 28080, HTTPS: 28443}
	if _, err = runtime.Configure(ctx, disabled.Revision, settings); err == nil {
		t.Fatal("enabled a port pair incompatible with the reviewed authority")
	}
	refused, _ := runtime.Snapshot()
	if refused.Revision != disabled.Revision || refused.Settings.Enabled {
		t.Fatal("rejected port pair changed durable intent")
	}
}
func TestRuntimeRejectsStaleSettingsAndNeverGeneratesAuthorityImplicitly(t *testing.T) {
	runtime, daemon := runtimeFixture(t)
	awaitRuntimePhase(t, runtime, "disabled")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	settings := DefaultSettings()
	settings.Enabled = true
	if _, err := runtime.Configure(ctx, 0, settings); err == nil {
		t.Fatal("enabled without reviewed authority")
	}
	if _, err := runtime.Configure(ctx, 1, DefaultSettings()); err == nil {
		t.Fatal("stale revision accepted")
	}
	state, _ := runtime.Snapshot()
	if state.Revision != 0 || state.Settings.RootSHA256 != "" {
		t.Fatal("failed request generated or changed state")
	}
	daemon.mu.Lock()
	creates := daemon.creates
	daemon.mu.Unlock()
	if creates != 0 {
		t.Fatal("disabled runtime created a helper")
	}
}

func TestRuntimeRetainsSuccessfulEmptyInventoryAcrossDisabledTicks(t *testing.T) {
	runtime, _ := runtimeFixture(t)
	initial := awaitRuntimePhase(t, runtime, "disabled")
	if initial.InventoryReadAt != nil {
		t.Fatal("disabled startup claimed to have read inventory")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runtime.Preview(ctx); err != nil {
		t.Fatal(err)
	}
	read, err := runtime.Snapshot()
	if err != nil || read.InventoryReadAt == nil || len(read.Candidates) != 0 {
		t.Fatal("successful empty inventory not recorded", read, err)
	}
	// The actor publishes disabled state every two seconds. Discovery must remain
	// distinguishable from the initial, unread state after that publication.
	deadline := time.Now().Add(3 * time.Second)
	for {
		after, err := runtime.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if after.Sequence > read.Sequence {
			if after.InventoryReadAt == nil || !after.InventoryReadAt.Equal(*read.InventoryReadAt) {
				t.Fatal("disabled publication erased or fabricated discovery", after)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("disabled actor did not publish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRuntimeReloadsCommittedSettingsAfterUncertainWrite(t *testing.T) {
	runtime, _ := runtimeFixture(t)
	state := awaitRuntimePhase(t, runtime, "disabled")
	settings := state.Settings
	settings.Ports = ServicePorts{HTTP: 18080, HTTPS: 18443}
	// Model the on-disk state after replace committed but confirmation was lost:
	// the actor still has the old revision. CAS must refuse the stale retry and
	// the actor must publish the actual committed revision without replacing it.
	committed, err := runtime.opts.Ledger.StoreSettings(state.Revision, settings)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := runtime.Configure(ctx, state.Revision, settings); err == nil {
		t.Fatal("uncertain acknowledgement was reported as a new successful write")
	}
	actual, err := runtime.Snapshot()
	if err != nil || actual.Revision != committed.Revision || actual.Settings.Ports != settings.Ports {
		t.Fatal("actor retained stale intent", actual, err)
	}
	stored, err := runtime.opts.Ledger.LoadSettings()
	if err != nil || stored.Revision != committed.Revision {
		t.Fatal("recovery replayed configuration", stored, err)
	}
}

func TestRuntimeRetiresIngressWhenSettingsCannotBeRecovered(t *testing.T) {
	for _, fault := range []string{"corrupt", "missing", "regressed"} {
		t.Run(fault, func(t *testing.T) {
			runtime, daemon := runtimeFixture(t)
			awaitRuntimePhase(t, runtime, "disabled")
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			settings := DefaultSettings()
			settings.Ports = ServicePorts{HTTP: 18080, HTTPS: 18443}
			stored, err := runtime.Configure(ctx, 0, settings)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(runtime.opts.Ledger.directory, settingsName)
			older, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.CreateAuthority(ctx, stored.Revision); err != nil {
				t.Fatal(err)
			}
			state, _ := runtime.Snapshot()
			settings = state.Settings
			settings.Enabled = true
			if _, err := runtime.Configure(ctx, state.Revision, settings); err != nil {
				t.Fatal(err)
			}
			running := awaitRuntimePhase(t, runtime, "running")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "corrupt":
				err = os.WriteFile(path, []byte("incomplete"), 0600)
			case "missing":
				err = os.Remove(path)
			case "regressed":
				err = os.WriteFile(path, older, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			settings.Enabled = false
			if _, err := runtime.Configure(ctx, running.Revision, settings); err == nil {
				t.Fatal("unreadable/regressed settings reported a successful disable")
			}
			unavailable := awaitRuntimePhase(t, runtime, "unavailable")
			daemon.mu.Lock()
			exists, creates, removes := daemon.exists, daemon.creates, daemon.removes
			daemon.mu.Unlock()
			if exists || creates != 1 || removes != 1 {
				t.Fatal("uncertain intent retained ingress", exists, creates, removes)
			}
			if err := runtime.Retry(ctx); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			for {
				state, _ := runtime.Snapshot()
				if state.Sequence > unavailable.Sequence {
					if state.Phase != "unavailable" {
						t.Fatal("retry accepted uncertain intent", state)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("retry did not reread settings: prior=%v current=%+v", unavailable.UpdatedAt, state)
				}
				time.Sleep(5 * time.Millisecond)
			}
			daemon.mu.Lock()
			creates = daemon.creates
			daemon.mu.Unlock()
			if creates != 1 {
				t.Fatal("retry started ingress before settings recovery")
			}
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Retry(ctx); err != nil {
				t.Fatal(err)
			}
			recovered := awaitRuntimePhase(t, runtime, "running")
			if recovered.Revision != running.Revision || recovered.Settings.RootSHA256 != running.Settings.RootSHA256 {
				t.Fatal("recovery changed intent", recovered)
			}
		})
	}
}

func TestRuntimePendingRotationSurvivesRestartAndBlocksIngressUntilExplicitResume(t *testing.T) {
	store := &fixtureRootStore{user: "rotation-user", roots: map[string]bool{}}
	opens := 0
	runtime, daemon := runtimeFixtureWithRootStore(t, func(write bool) (RootStore, error) {
		if write {
			t.Error("rotation opened a writable root store")
		}
		opens++
		return store, nil
	})
	awaitRuntimePhase(t, runtime, "disabled")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := runtime.CreateAuthority(ctx, 0); err != nil {
		t.Fatal(err)
	}
	old, _ := runtime.Snapshot()
	info, err := runtime.opts.Ledger.BeginAuthorityRotation(store, runtime.opts.Installation, old.Settings.RootSHA256, old.Revision, ServicePorts{HTTP: 18080, HTTPS: 18443}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	settings := old.Settings
	settings.Enabled = true
	if _, err := runtime.Configure(ctx, old.Revision, settings); err == nil {
		t.Fatal("pending rotation enabled ingress")
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	options := runtime.opts
	options.Ledger, err = OpenNamespaceLedger(options.Ledger.directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServiceRuntime(options)
	if err != nil {
		options.Ledger.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clean, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		if err := restarted.Close(clean); err != nil {
			t.Error(err)
		}
	})
	pending := awaitRuntimePhase(t, restarted, "rotation-pending")
	if pending.Rotation == nil || pending.Rotation.NewRootSHA256 != info.NewRootSHA256 || opens != 0 {
		t.Fatal("startup changed trust or lost pending identity", pending, opens)
	}
	daemon.mu.Lock()
	creates := daemon.creates
	daemon.mu.Unlock()
	if creates != 0 {
		t.Fatal("pending rotation created ingress helpers")
	}
	if err := restarted.RotateAuthority(ctx, "resume-authority-rotation", pending.Revision, info.OldRootSHA256, info.NewRootSHA256, ServicePorts{}); err != nil {
		t.Fatal(err)
	}
	settled := awaitRuntimePhase(t, restarted, "disabled")
	if settled.Rotation != nil || settled.Settings.RootSHA256 != info.NewRootSHA256 || settled.Revision != old.Revision+1 || store.adds != 0 || store.removes != 0 {
		t.Fatal(settled)
	}
}

func TestRuntimeDoesNotApplyQueuedMutationsAfterShutdownStarts(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	store := &fixtureRootStore{user: "shutdown-user", roots: map[string]bool{}}
	runtime, _ := runtimeFixtureWithRootStore(t, func(bool) (RootStore, error) { close(entered); <-release; return store, nil })
	awaitRuntimePhase(t, runtime, "disabled")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := runtime.CreateAuthority(ctx, 0); err != nil {
		t.Fatal(err)
	}
	state, _ := runtime.Snapshot()
	reading := make(chan error, 1)
	go func() {
		reading <- runtime.WindowsTrust(ctx, "trust-status", state.Revision, state.Settings.RootSHA256)
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	settings := state.Settings
	settings.Ports = ServicePorts{HTTP: 18080, HTTPS: 18443}
	queued := make(chan error, 1)
	go func() { _, err := runtime.Configure(ctx, state.Revision, settings); queued <- err }()
	for len(runtime.commands) == 0 {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	closed := make(chan error, 1)
	go func() { closed <- runtime.Close(ctx) }()
	select {
	case <-runtime.ctx.Done():
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := runtime.Configure(ctx, state.Revision, settings); err == nil {
		t.Fatal("closing process accepted another mutation")
	}
	close(release)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if err := <-queued; err == nil {
		t.Fatal("queued mutation applied after shutdown")
	}
	<-reading
	ledger, err := OpenNamespaceLedger(runtime.opts.Ledger.directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	stored, err := ledger.LoadSettings()
	if err != nil || stored.Revision != state.Revision || stored.Settings.Ports != state.Settings.Ports {
		t.Fatal("shutdown changed desired settings", stored, err)
	}
}

package localnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
)

type controlledLANHelper struct {
	LANAdministrativeHelper
	stage  func(context.Context, lanhelper.StageRequest) (lanhelper.Prepared, error)
	commit func(context.Context, lanhelper.Prepared, lanfirewall.CallerIdentity, lanlease.Observation) (lanlease.RuleAcknowledgement, error)
}

func (h *controlledLANHelper) Stage(ctx context.Context, s lanhelper.StageRequest) (lanhelper.Prepared, error) {
	if h.stage != nil {
		return h.stage(ctx, s)
	}
	return h.LANAdministrativeHelper.Stage(ctx, s)
}
func (h *controlledLANHelper) Commit(ctx context.Context, p lanhelper.Prepared, w lanfirewall.CallerIdentity, o lanlease.Observation) (lanlease.RuleAcknowledgement, error) {
	if h.commit != nil {
		return h.commit(ctx, p, w, o)
	}
	return h.LANAdministrativeHelper.Commit(ctx, p, w, o)
}

func TestLANSetupConsentWaitDoesNotHoldCommittedRefreshLock(t *testing.T) {
	f := makeLANActor(t)
	if _, e := f.controller.start(context.Background(), f.stage); e != nil {
		t.Fatal(e)
	}
	entered := make(chan struct{})
	f.controller.adapters.Helper = &controlledLANHelper{LANAdministrativeHelper: f.helper, stage: func(ctx context.Context, _ lanhelper.StageRequest) (lanhelper.Prepared, error) {
		close(entered)
		<-ctx.Done()
		return lanhelper.Prepared{}, ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := f.controller.start(ctx, f.stage); done <- e }()
	<-entered
	bounded, release := context.WithTimeout(context.Background(), time.Second)
	defer release()
	if e := f.controller.refresh(bounded); e != nil || f.worker.sends.Load() != 2 {
		t.Fatal("consent wait blocked live lease refresh", e)
	}
	cancel()
	if <-done == nil {
		t.Fatal("cancelled consent reported ready")
	}
	close(f.worker.joined)
	if e := f.controller.Close(bounded); e != nil {
		t.Fatal(e)
	}
	if f.helper.retires.Load() != 0 {
		t.Fatal("automatic cancellation prompted rule retirement")
	}
}

func TestLANSetupCloseContainsLateCreatedChildBeforeReferenceRelease(t *testing.T) {
	f := makeLANActor(t)
	entered, release := make(chan struct{}), make(chan struct{})
	f.controller.adapters.Launch = func(ctx context.Context, _ lanhelper.Prepared, _ func(context.Context, lanhelper.Stream) (net.Conn, error)) (LANWorker, error) {
		close(entered)
		<-release
		return f.worker, ctx.Err()
	}
	started := make(chan error, 1)
	go func() { _, e := f.controller.start(context.Background(), f.stage); started <- e }()
	<-entered
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if f.controller.Close(short) == nil || f.base.releases.Load() != 0 {
		t.Fatal("late child was abandoned")
	}
	cancel()
	close(release)
	if <-started == nil {
		t.Fatal("closed generation accepted late child")
	}
	close(f.worker.joined)
	bounded, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if e := f.controller.Wait(bounded); e != nil || f.base.releases.Load() != 1 || f.helper.commits.Load() != 0 || f.worker.stops.Load() != 1 {
		t.Fatal("late child/reference did not positively settle", e)
	}
}

func TestLANSetupLatePositiveCreateAfterCancelRetainsOnlyDormantOwnership(t *testing.T) {
	f := makeLANActor(t)
	entered, release := make(chan struct{}), make(chan struct{})
	f.controller.adapters.Helper = &controlledLANHelper{LANAdministrativeHelper: f.helper, commit: func(context.Context, lanhelper.Prepared, lanfirewall.CallerIdentity, lanlease.Observation) (lanlease.RuleAcknowledgement, error) {
		close(entered)
		<-release
		return f.helper.rule, nil
	}}
	started := make(chan error, 1)
	go func() { _, e := f.controller.start(context.Background(), f.stage); started <- e }()
	<-entered
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if f.controller.Close(short) == nil {
		t.Fatal("unsettled provider/child was reported stopped")
	}
	cancel()
	close(release)
	if <-started == nil {
		t.Fatal("late positive create reactivated cancelled setup")
	}
	close(f.worker.joined)
	bounded, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if e := f.controller.Wait(bounded); e != nil || f.base.releases.Load() != 1 || f.helper.retires.Load() != 0 {
		t.Fatal("dormant ownership trapped resources or prompted administration", e)
	}
	statuses := f.controller.statuses()
	if len(statuses) != 1 || statuses[0].Ingress != "closed" || statuses[0].Firewall != "created" {
		t.Fatal("positive late rule receipt was lost", statuses)
	}
}

func TestLANSetupRefreshRuleDriftClosesWithoutAdoptingOrPrompting(t *testing.T) {
	f := makeLANActor(t)
	if _, e := f.controller.start(context.Background(), f.stage); e != nil {
		t.Fatal(e)
	}
	f.helper.rule.TypedSnapshotSHA = strings.Repeat("3", 64)
	if f.controller.refresh(context.Background()) == nil || f.worker.stops.Load() != 1 || f.helper.retires.Load() != 0 {
		t.Fatal("changed rule was adopted or triggered unattended retirement")
	}
	close(f.worker.joined)
	bounded, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := f.controller.Close(bounded); e != nil || f.base.releases.Load() != 1 {
		t.Fatal("known child join did not release namespace", e)
	}
	statuses := f.controller.statuses()
	if len(statuses) != 1 || statuses[0].Ingress != "closed" || statuses[0].Phase != "cleanup-pending" {
		t.Fatal("dormant altered rule was hidden", statuses)
	}
}

func TestLANSetupMoreThanSixteenHealthyCyclesKeepOneOwnerAndDenialHistory(t *testing.T) {
	f := makeLANActor(t)
	for i := 1; i <= 24; i++ {
		spec := f.helper.prepared.Spec
		spec.LeaseID = fmt.Sprintf("%032x", i+100)
		spec.Request.Transaction = fmt.Sprintf("%032x", i+200)
		spec.Request.Nonce = fmt.Sprintf("%064x", i+300)
		spec.Request.ReviewSHA256 = fmt.Sprintf("%064x", i+400)
		spec.Worker.Path = spec.WorkerRoot + `\` + spec.Request.Installation + `\` + spec.LeaseID + `\orbwin-session.exe`
		manager := f.observation.Manager
		lease, e := lanlease.NewOwner(spec.Request.Installation, manager).Prepare(spec, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		sha := lease.Snapshot(time.Now()).LeaseSHA
		f.helper.prepared = lanhelper.Prepared{Spec: spec, LeaseSHA: sha, Sequence: 2}
		f.helper.rule.LeaseSHA = sha
		f.helper.rule.NativeKeysSHA = fmt.Sprintf("%064x", i+500)
		f.helper.rule.ReadAt = time.Now()
		f.observation.WorkerFile = spec.Worker
		f.worker = &lanActorWorker{identity: f.observation.Worker, joined: make(chan struct{})}
		f.stage = lanhelper.StageRequest{Base: spec.Request, Publication: spec.Publication}
		status, e := f.controller.start(context.Background(), f.stage)
		if e != nil || status.Phase != "ready" {
			t.Fatalf("cycle %d admission: %v", i, e)
		}
		close(f.worker.joined)
		bounded, cancel := context.WithTimeout(context.Background(), time.Second)
		if e = f.controller.stop(bounded, sha); e == nil {
			e = f.controller.retire(bounded, sha, nil)
		}
		cancel()
		if e != nil {
			t.Fatalf("cycle %d retirement: %v", i, e)
		}
	}
	f.controller.mu.Lock()
	active, rules := len(f.controller.sessions), len(f.controller.rules)
	f.controller.mu.Unlock()
	if active != 0 || rules != 0 || f.base.acquires.Load() != 24 || f.base.releases.Load() != 24 || f.helper.retires.Load() != 24 {
		t.Fatal("healthy owners leaked into permanent capacity", active, rules)
	}
	if _, e := f.controller.start(context.Background(), f.stage); e == nil {
		t.Fatal("compaction erased denial history and replayed a spent lease")
	}
	if e := f.controller.Close(context.Background()); e != nil && !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

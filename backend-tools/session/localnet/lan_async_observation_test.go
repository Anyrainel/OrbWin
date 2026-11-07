package localnet

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
)

type parallelRuleHelper struct {
	*lanActorHelper
	prepared         map[string]lanhelper.Prepared
	rules            map[string]lanlease.RuleAcknowledgement
	blocked          string
	entered, release chan struct{}
	reads            atomic.Int32
}

func (helper *parallelRuleHelper) Stage(_ context.Context, stage lanhelper.StageRequest) (lanhelper.Prepared, error) {
	return helper.prepared[stage.Base.Digest()], nil
}
func (helper *parallelRuleHelper) Commit(_ context.Context, prepared lanhelper.Prepared, _ lanfirewall.CallerIdentity, _ lanlease.Observation) (lanlease.RuleAcknowledgement, error) {
	return helper.rules[prepared.LeaseSHA], nil
}
func (helper *parallelRuleHelper) ReadRule(_ context.Context, prepared lanhelper.Prepared) (lanlease.RuleAcknowledgement, error) {
	if prepared.LeaseSHA == helper.blocked {
		if helper.reads.Add(1) == 1 {
			close(helper.entered)
		}
		<-helper.release
	}
	ack := helper.rules[prepared.LeaseSHA]
	ack.ReadAt = time.Now()
	return ack, nil
}
func TestLANAsyncObservationDoesNotQueueOrBlockHealthyPeerAndOwnsLateJoin(t *testing.T) {
	f := makeLANActor(t)
	first := f.helper.prepared
	secondSpec := first.Spec
	secondSpec.Request.Transaction = strings.Repeat("5", 32)
	secondSpec.Request.Nonce = strings.Repeat("6", 64)
	secondSpec.Request.ReviewSHA256 = strings.Repeat("8", 64)
	secondSpec.Request.Scope.LocalPort++
	secondSpec.Publication.HostPort++
	secondSpec.Publication.Generation++
	secondSpec.LeaseID = strings.Repeat("9", 32)
	secondSpec.Worker.Path = secondSpec.WorkerRoot + `\` + secondSpec.Request.Installation + `\` + secondSpec.LeaseID + `\orbwin-session.exe`
	lease, err := lanlease.NewOwner(f.stage.Base.Installation, f.observation.Manager).Prepare(secondSpec, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second := lanhelper.Prepared{Spec: secondSpec, LeaseSHA: lease.Snapshot(time.Now()).LeaseSHA, Sequence: 2}
	secondRule := f.helper.rule
	secondRule.LeaseSHA = second.LeaseSHA
	secondRule.ReviewedScopeSHA = secondSpec.Request.Scope.Digest()
	helper := &parallelRuleHelper{lanActorHelper: f.helper, prepared: map[string]lanhelper.Prepared{first.Spec.Request.Digest(): first, secondSpec.Request.Digest(): second}, rules: map[string]lanlease.RuleAcknowledgement{first.LeaseSHA: f.helper.rule, second.LeaseSHA: secondRule}, blocked: first.LeaseSHA, entered: make(chan struct{}), release: make(chan struct{})}
	otherWorker := &lanActorWorker{identity: f.worker.identity, joined: make(chan struct{})}
	otherWorker.identity.PID++
	otherWorker.identity.CreationTime = "201"
	f.controller.adapters.Helper = helper
	f.controller.adapters.Launch = func(_ context.Context, prepared lanhelper.Prepared, _ func(context.Context, lanhelper.Stream) (net.Conn, error)) (LANWorker, error) {
		if prepared.LeaseSHA == first.LeaseSHA {
			return f.worker, nil
		}
		return otherWorker, nil
	}
	f.controller.adapters.Observe = func(_ context.Context, prepared lanhelper.Prepared, child lanfirewall.CallerIdentity) (lanlease.Observation, error) {
		observation := f.observation
		observation.ReadAt = time.Now()
		observation.Scope = prepared.Spec.Request.Scope
		observation.Publication = prepared.Spec.Publication
		observation.WorkerFile = prepared.Spec.Worker
		observation.Worker = child
		observation.WorkerPipePID = child.PID
		return observation, nil
	}
	if _, err = f.controller.start(context.Background(), f.stage); err != nil {
		t.Fatal(err)
	}
	if _, err = f.controller.start(context.Background(), lanhelper.StageRequest{Base: secondSpec.Request, Publication: secondSpec.Publication}); err != nil {
		t.Fatal(err)
	}
	updates := make(chan struct{}, 8)
	f.controller.onChange = func() {
		select {
		case updates <- struct{}{}:
		default:
		}
	}
	f.controller.kickRefresh()
	<-helper.entered
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("blocked native rule read held healthy peer observation")
	}
	if otherWorker.sends.Load() < 2 || len(f.controller.statuses()) != 2 {
		t.Fatal("healthy peer/read-state did not advance")
	}
	f.controller.kickRefresh()
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("healthy peer second observation blocked")
	}
	if helper.reads.Load() != 1 {
		t.Fatal("stuck observer gained another queued setup/read")
	}
	close(f.worker.joined)
	close(otherWorker.joined)
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if f.controller.Close(short) == nil {
		t.Fatal("generation forgot still-unjoined native observation child")
	}
	cancel()
	close(helper.release)
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err = f.controller.Wait(ctx); err != nil || f.helper.retires.Load() != 0 || f.base.releases.Load() != 2 {
		t.Fatal("late read/child/reference join failed or auto-retired rule", err)
	}
}

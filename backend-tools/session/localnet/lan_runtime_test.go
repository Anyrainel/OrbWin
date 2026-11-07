package localnet

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
)

type lanActorWorker struct {
	identity     lanfirewall.CallerIdentity
	joined       chan struct{}
	stops, sends atomic.Int32
}

func (w *lanActorWorker) Identity() lanfirewall.CallerIdentity        { return w.identity }
func (w *lanActorWorker) WaitHello(context.Context) error             { return nil }
func (w *lanActorWorker) Send(context.Context, lanhelper.Frame) error { w.sends.Add(1); return nil }
func (w *lanActorWorker) Stop()                                       { w.stops.Add(1) }
func (w *lanActorWorker) Wait(ctx context.Context) error {
	select {
	case <-w.joined:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type lanActorHelper struct {
	prepared                        lanhelper.Prepared
	rule                            lanlease.RuleAcknowledgement
	stages, commits, reads, retires atomic.Int32
	commitErr, retireErr            error
}

func (h *lanActorHelper) Tombstones(context.Context) ([]lanlease.Tombstone, error) { return nil, nil }
func (h *lanActorHelper) Close(context.Context) error                              { return nil }
func (h *lanActorHelper) Stage(context.Context, lanhelper.StageRequest) (lanhelper.Prepared, error) {
	h.stages.Add(1)
	return h.prepared, nil
}
func (h *lanActorHelper) Commit(context.Context, lanhelper.Prepared, lanfirewall.CallerIdentity, lanlease.Observation) (lanlease.RuleAcknowledgement, error) {
	h.commits.Add(1)
	return h.rule, h.commitErr
}
func (h *lanActorHelper) ReadRule(context.Context, lanhelper.Prepared) (lanlease.RuleAcknowledgement, error) {
	h.reads.Add(1)
	r := h.rule
	r.ReadAt = time.Now()
	return r, nil
}
func (h *lanActorHelper) Retire(context.Context, lanhelper.Prepared, lanlease.RuleAcknowledgement) (lanlease.RemovalAcknowledgement, error) {
	h.retires.Add(1)
	return lanlease.RemovalAcknowledgement{LeaseSHA: h.prepared.LeaseSHA, NativeKeysSHA: h.rule.NativeKeysSHA, Sequence: h.rule.Sequence + 2, TerminalSuccess: true, OperationClosed: true, ObservedAbsent: true, ProtectedReceipt: true, AcknowledgementFlushed: true}, h.retireErr
}

type lanActorFixture struct {
	controller  *lanController
	worker      *lanActorWorker
	helper      *lanActorHelper
	base        *namespaceReferenceFake
	stage       lanhelper.StageRequest
	observation lanlease.Observation
}

func makeLANActor(t *testing.T) *lanActorFixture {
	t.Helper()
	now := time.Now().UTC()
	manager := lanfirewall.CallerIdentity{PID: 100, CreationTime: "100", UserSID: "S-1-5-21-1-2-3-1001", ImageSHA256: strings.Repeat("f", 64)}
	child := manager
	child.PID = 101
	child.CreationTime = "200"
	child.ImageSHA256 = strings.Repeat("e", 64)
	request := lanfirewall.Request{Version: 1, Operation: "create", Installation: strings.Repeat("a", 32), UserSID: manager.UserSID, Transaction: strings.Repeat("b", 32), Nonce: strings.Repeat("d", 64), ReviewSHA256: strings.Repeat("9", 64), CallerPID: manager.PID, CallerCreationTime: manager.CreationTime, CallerImageSHA256: manager.ImageSHA256, IssuedAt: now.Add(-time.Second).Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano), Scope: lanfirewall.Scope{Protocol: "tcp", LocalAddress: "192.168.4.10", LocalPort: 58080, InterfaceGUID: "{11111111-1111-1111-1111-111111111111}", InterfaceLUID: "42", Profile: "private", ClientNetworks: []string{"192.168.4.0/24"}, WorkerSHA256: child.ImageSHA256}}
	spec := lanlease.Spec{Request: request, Publication: lanlease.Publication{OwnerInstance: strings.Repeat("7", 32), Daemon: "owned", ContainerID: strings.Repeat("a", 64), StartedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), Generation: 7, HostPort: 58080, ContainerPort: 8080, Protocol: "tcp"}, LeaseID: strings.Repeat("c", 32), WorkerRoot: `C:\ProgramData\OrbWin\LAN`}
	spec.Worker = lanfirewall.WorkerBinding{Path: spec.WorkerRoot + `\` + request.Installation + `\` + spec.LeaseID + `\orbwin-session.exe`, FileID: strings.Repeat("b", 32), VolumeSerial: "17", SHA256: child.ImageSHA256, InterfaceAlias: "Ethernet"}
	lease, e := lanlease.NewOwner(request.Installation, manager).Prepare(spec, now)
	if e != nil {
		t.Fatal(e)
	}
	sha := lease.Snapshot(now).LeaseSHA
	f := &lanActorFixture{worker: &lanActorWorker{identity: child, joined: make(chan struct{})}, helper: &lanActorHelper{}, base: &namespaceReferenceFake{}, stage: lanhelper.StageRequest{Base: request, Publication: spec.Publication}}
	f.helper.prepared = lanhelper.Prepared{Spec: spec, LeaseSHA: sha, Sequence: 2}
	f.helper.rule = lanlease.RuleAcknowledgement{ReadAt: now, LeaseSHA: sha, ReviewedScopeSHA: request.Scope.Digest(), NativeKeysSHA: strings.Repeat("1", 64), TypedSnapshotSHA: strings.Repeat("2", 64), Sequence: 2, Outcome: "created", TerminalSuccess: true, OperationClosed: true, ProtectedReceipt: true, AcknowledgementFlushed: true, RuleProperties: 39, FilterFamilies: 7, ReverseIdentities: 7}
	f.observation = lanlease.Observation{ReadAt: now, Manager: manager, Worker: child, WorkerFile: spec.Worker, Publication: spec.Publication, Scope: request.Scope, ManagerPipePID: manager.PID, WorkerPipePID: child.PID, OwnerLeaseHeld: true, ManagerAlive: true, WorkerAlive: true, WorkerFileHeld: true, ProtectedPath: true, PublicationActive: true, InterfacePresent: true}
	adapters := &LANAdapters{Manager: func(context.Context) (lanfirewall.CallerIdentity, error) { return manager, nil }, ValidatePublication: func(context.Context, lanhelper.StageRequest) (time.Time, error) { return time.Now(), nil }, Observe: func(context.Context, lanhelper.Prepared, lanfirewall.CallerIdentity) (lanlease.Observation, error) {
		o := f.observation
		o.ReadAt = time.Now()
		return o, nil
	}, Helper: f.helper, Launch: func(context.Context, lanhelper.Prepared, func(context.Context, lanhelper.Stream) (net.Conn, error)) (LANWorker, error) {
		return f.worker, nil
	}}
	f.controller = newLANController(request.Installation, newSharedNamespaces(f.base, nil, nil), adapters)
	return f
}
func TestLANActorMissingPublicationRefusesBeforeHelperOrChild(t *testing.T) {
	f := makeLANActor(t)
	f.controller.adapters.ValidatePublication = func(context.Context, lanhelper.StageRequest) (time.Time, error) {
		return time.Time{}, errors.New("publication-v2 endpoint unavailable")
	}
	if _, e := f.controller.start(context.Background(), f.stage); e == nil || f.helper.stages.Load() != 0 || f.base.acquires.Load() != 0 || f.worker.sends.Load() != 0 {
		t.Fatal("missing generation source reached mutation/launch")
	}
	if e := newLANController(f.stage.Base.Installation, f.controller.shared, nil).Close(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func TestLANActorShutdownJoinsChildAndReleasesNamespaceWithoutAdministrativePrompt(t *testing.T) {
	f := makeLANActor(t)
	status, e := f.controller.start(context.Background(), f.stage)
	if e != nil || status.Phase != "ready" {
		t.Fatalf("start: %+v %v", status, e)
	}
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if f.controller.Close(short) == nil || f.helper.reads.Load() != 0 || f.helper.retires.Load() != 0 || f.base.releases.Load() != 0 {
		t.Fatal("timeout/Stop became positive closure")
	}
	close(f.worker.joined)
	bounded, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if e = f.controller.Wait(bounded); e != nil {
		t.Fatal(e)
	}
	if f.helper.retires.Load() != 0 || f.helper.reads.Load() != 0 || f.base.releases.Load() != 1 || f.worker.stops.Load() != 1 {
		t.Fatal("automatic shutdown prompted administration or skipped exact child/reference join")
	}
	if e = f.controller.retire(bounded, status.LeaseSHA, nil); e != nil {
		t.Fatal("explicit reviewed retirement failed", e)
	}
	if f.helper.retires.Load() != 1 || f.helper.reads.Load() != 1 || f.controller.Close(bounded) != nil {
		t.Fatal("explicit removal was not one exact read/retirement")
	}
	if f.controller.retire(bounded, status.LeaseSHA, nil) == nil || f.helper.retires.Load() != 1 {
		t.Fatal("retired rule was replayed")
	}
}
func TestLANActorUnknownCommitRetainsDormantReceiptWithoutHoldingClosedNamespace(t *testing.T) {
	f := makeLANActor(t)
	f.helper.commitErr = errors.New("lost create transport")
	status, e := f.controller.start(context.Background(), f.stage)
	if e == nil || status.Phase == "ready" || f.helper.commits.Load() != 1 {
		t.Fatal("unknown creation became ingress authority")
	}
	close(f.worker.joined)
	short, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if f.controller.Close(short) != nil || f.helper.reads.Load() != 0 || f.helper.retires.Load() != 0 || f.base.releases.Load() != 1 {
		t.Fatal("unknown creation adopted/removed, or positive namespace join was trapped by dormant rule")
	}
	statuses := f.controller.statuses()
	if len(statuses) != 1 || statuses[0].Ingress != "closed" || statuses[0].Firewall != "create-unknown" || statuses[0].Phase != "cleanup-pending" {
		t.Fatal("unknown rule truth was discarded", statuses)
	}
	if _, e = f.controller.start(context.Background(), f.stage); e == nil || f.helper.commits.Load() != 1 {
		t.Fatal("retried uncertain create")
	}
}
func TestLANActorPartialLaunchFailureStillWaitsItsOwnedChild(t *testing.T) {
	f := makeLANActor(t)
	f.controller.adapters.Launch = func(context.Context, lanhelper.Prepared, func(context.Context, lanhelper.Stream) (net.Conn, error)) (LANWorker, error) {
		return f.worker, errors.New("post-Create bind failed")
	}
	if _, e := f.controller.start(context.Background(), f.stage); e == nil {
		t.Fatal("launch became success")
	}
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if f.controller.Close(short) == nil || f.base.releases.Load() != 0 {
		t.Fatal("unbound partial child escaped positive join")
	}
	close(f.worker.joined)
	bounded, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if f.controller.Wait(bounded) != nil || f.helper.commits.Load() != 0 || f.helper.retires.Load() != 0 || f.base.releases.Load() != 1 {
		t.Fatal("pre-commit partial child was not joined/settled without provider action")
	}
}

func TestLANActorComposedObservationDoesNotRefreshOldSourceTime(t *testing.T) {
	f := makeLANActor(t)
	if _, e := f.controller.start(context.Background(), f.stage); e != nil {
		t.Fatal(e)
	}
	old := time.Now().Add(-6 * time.Second)
	f.controller.adapters.ValidatePublication = func(context.Context, lanhelper.StageRequest) (time.Time, error) { return old, nil }
	if f.controller.refresh(context.Background()) == nil || f.worker.sends.Load() != 1 {
		t.Fatal("fresh native read refreshed expired publication evidence")
	}
	close(f.worker.joined)
	bounded, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if f.controller.Close(bounded) != nil {
		t.Fatal("failed source refresh did not retire contained lease")
	}
}

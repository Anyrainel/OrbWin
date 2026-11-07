package lanlease

import (
	"net/netip"
	"orbwin.local/session/internal/lanfirewall"
	"strconv"
	"strings"
	"testing"
	"time"
)

func leaseFixture(t *testing.T) (*Owner, *Lease, Spec, Observation, RuleAcknowledgement, time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	manager := lanfirewall.CallerIdentity{UserSID: "S-1-5-21-1-2-3-1001", PID: 100, CreationTime: "100", ImageSHA256: strings.Repeat("f", 64)}
	s := Spec{LeaseID: strings.Repeat("c", 32), WorkerRoot: `C:\ProgramData\OrbWin\LAN`, Publication: Publication{OwnerInstance: strings.Repeat("7", 32), Daemon: "owned-daemon", ContainerID: strings.Repeat("a", 64), StartedAt: now.Add(-time.Hour).Format(time.RFC3339Nano), Generation: 7, HostPort: 58080, ContainerPort: 8080, Protocol: "tcp"}}
	s.Request = lanfirewall.Request{Version: 1, Operation: "create", Installation: strings.Repeat("a", 32), UserSID: manager.UserSID, Transaction: strings.Repeat("b", 32), Nonce: strings.Repeat("d", 64), ReviewSHA256: strings.Repeat("9", 64), CallerPID: manager.PID, CallerCreationTime: manager.CreationTime, CallerImageSHA256: manager.ImageSHA256, IssuedAt: now.Add(-time.Second).Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano), Scope: lanfirewall.Scope{Protocol: "tcp", LocalAddress: "192.168.4.10", LocalPort: 58080, InterfaceGUID: "{11111111-1111-1111-1111-111111111111}", InterfaceLUID: "42", Profile: "private", ClientNetworks: []string{"192.168.4.0/24"}, WorkerSHA256: strings.Repeat("e", 64)}}
	p, e := workerPath(s.WorkerRoot, s.Request.Installation, s.LeaseID)
	if e != nil {
		t.Fatal(e)
	}
	s.Worker = lanfirewall.WorkerBinding{Path: p, FileID: strings.Repeat("b", 32), VolumeSerial: "17", SHA256: s.Request.Scope.WorkerSHA256, InterfaceAlias: "Ethernet"}
	owner := NewOwner(s.Request.Installation, manager)
	lease, e := owner.Prepare(s, now)
	if e != nil {
		t.Fatal(e)
	}
	child := lanfirewall.CallerIdentity{UserSID: manager.UserSID, PID: 101, CreationTime: "200", ImageSHA256: s.Worker.SHA256}
	observed := Observation{ReadAt: now, Manager: manager, Worker: child, WorkerFile: s.Worker, Publication: s.Publication, Scope: s.Request.Scope, ManagerPipePID: manager.PID, WorkerPipePID: child.PID, OwnerLeaseHeld: true, ManagerAlive: true, WorkerAlive: true, WorkerFileHeld: true, ProtectedPath: true, PublicationActive: true, InterfacePresent: true}
	observed.Scope.ClientNetworks = append([]string(nil), s.Request.Scope.ClientNetworks...)
	ack := RuleAcknowledgement{ReadAt: now, LeaseSHA: lease.Snapshot(now).LeaseSHA, ReviewedScopeSHA: s.Request.Scope.Digest(), NativeKeysSHA: strings.Repeat("1", 64), TypedSnapshotSHA: strings.Repeat("2", 64), Sequence: 2, Outcome: "created", TerminalSuccess: true, OperationClosed: true, AcknowledgementFlushed: true, ProtectedReceipt: true, RuleProperties: 39, FilterFamilies: 7, ReverseIdentities: 7}
	return owner, lease, s, observed, ack, now
}
func readyLease(t *testing.T) (*Lease, Observation, RuleAcknowledgement, time.Time) {
	t.Helper()
	_, l, _, o, a, now := leaseFixture(t)
	if e := l.BindChild(1, o.Worker, o, now); e != nil {
		t.Fatal(e)
	}
	if e := l.AcknowledgeRule(2, a, o, now); e != nil {
		t.Fatal(e)
	}
	if e := l.ReadyForIngress(3, o, now); e != nil {
		t.Fatal(e)
	}
	return l, o, a, now
}
func closeLease(t *testing.T, l *Lease, o Observation, now time.Time) {
	t.Helper()
	if e := l.BeginClose(l.Snapshot(now).Revision); e != nil {
		t.Fatal(e)
	}
	if e := l.AcknowledgeClosed(l.Snapshot(now).Revision, CloseAcknowledgement{LeaseSHA: l.sha, Worker: o.Worker, IngressClosed: true, FlowsJoined: true, ProcessExited: true, NoUnknownChild: true}); e != nil {
		t.Fatal(e)
	}
}

func TestLeaseCannotStartBeforeExactChildAndAcknowledgedRuleAndCannotRetireBeforeClosure(t *testing.T) {
	_, l, _, o, a, now := leaseFixture(t)
	if l.ReadyForIngress(1, o, now) == nil || l.Snapshot(now).IngressEligible {
		t.Fatal("prepared lease granted ingress")
	}
	if e := l.BindChild(1, o.Worker, o, now); e != nil {
		t.Fatal(e)
	}
	if l.ReadyForIngress(2, o, now) == nil {
		t.Fatal("child alone granted ingress")
	}
	if e := l.AcknowledgeRule(2, a, o, now); e != nil {
		t.Fatal(e)
	}
	if e := l.ReadyForIngress(3, o, now); e != nil {
		t.Fatal(e)
	}
	if !l.AllowsPeer(netip.MustParseAddr("192.168.4.50"), now) || l.AllowsPeer(netip.MustParseAddr("192.168.5.50"), now) || l.AllowsPeer(netip.MustParseAddr("::ffff:192.168.4.50"), now) {
		t.Fatal("listener peer scope widened")
	}
	if l.BeginRetirement(4, a, now) == nil {
		t.Fatal("active ingress allowed rule retirement")
	}
	closeLease(t, l, o, now)
	if !l.Snapshot(now).Contained || l.Snapshot(now).RuleRetired {
		t.Fatal("closure confused with rule deletion")
	}
	if e := l.BeginRetirement(l.Snapshot(now).Revision, a, now); e != nil {
		t.Fatal(e)
	}
	if e := l.AcknowledgeRemoved(l.Snapshot(now).Revision, RemovalAcknowledgement{LeaseSHA: l.sha, NativeKeysSHA: a.NativeKeysSHA, Sequence: a.Sequence + 2, TerminalSuccess: true, OperationClosed: true, ObservedAbsent: true, AcknowledgementFlushed: true, ProtectedReceipt: true}); e != nil {
		t.Fatal(e)
	}
	if !l.Snapshot(now).RuleRetired || l.AllowsPeer(netip.MustParseAddr("192.168.4.50"), now) {
		t.Fatal("retired lease regained ingress")
	}
}
func TestLeaseNativeParentChildFileBindingsRejectPIDReuseAndSameSIDLookalikes(t *testing.T) {
	changes := map[string]func(*Observation){"manager-pid": func(o *Observation) { o.Manager.PID++ }, "manager-creation": func(o *Observation) { o.Manager.CreationTime = "101" }, "manager-sid": func(o *Observation) { o.Manager.UserSID = "S-1-5-21-1-2-3-1002" }, "manager-image": func(o *Observation) { o.Manager.ImageSHA256 = strings.Repeat("0", 64) }, "child-creation": func(o *Observation) { o.Worker.CreationTime = "201" }, "pipe-parent": func(o *Observation) { o.ManagerPipePID++ }, "pipe-child": func(o *Observation) { o.WorkerPipePID++ }, "file-id": func(o *Observation) { o.WorkerFile.FileID = strings.Repeat("0", 32) }, "file-path": func(o *Observation) { o.WorkerFile.Path += ".other" }, "file-hash": func(o *Observation) { o.WorkerFile.SHA256 = strings.Repeat("0", 64) }, "file-volume": func(o *Observation) { o.WorkerFile.VolumeSerial = "18" }, "unprotected": func(o *Observation) { o.ProtectedPath = false }, "no-held-file": func(o *Observation) { o.WorkerFileHeld = false }}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			_, l, _, o, _, now := leaseFixture(t)
			spawned := o.Worker
			change(&o)
			if l.BindChild(1, spawned, o, now) == nil || l.Snapshot(now).IngressEligible {
				t.Fatal("unbound native identity accepted")
			}
		})
	}
}
func TestDuplicateUnknownIncompleteAndUnprotectedRuleEvidenceNeverConfersOwnership(t *testing.T) {
	changes := map[string]func(*RuleAcknowledgement){"duplicate": func(a *RuleAcknowledgement) { a.Outcome = "duplicate" }, "unknown": func(a *RuleAcknowledgement) { a.Outcome = "unknown" }, "wrong-lease": func(a *RuleAcknowledgement) { a.LeaseSHA = strings.Repeat("0", 64) }, "no-terminal": func(a *RuleAcknowledgement) { a.TerminalSuccess = false }, "no-close": func(a *RuleAcknowledgement) { a.OperationClosed = false }, "user-receipt": func(a *RuleAcknowledgement) { a.ProtectedReceipt = false }, "missing-keys": func(a *RuleAcknowledgement) { a.NativeKeysSHA = "" }, "partial-rule": func(a *RuleAcknowledgement) { a.RuleProperties = 38 }, "partial-filter": func(a *RuleAcknowledgement) { a.FilterFamilies = 6 }, "partial-reverse": func(a *RuleAcknowledgement) { a.ReverseIdentities = 6 }}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			_, l, _, o, a, now := leaseFixture(t)
			if e := l.BindChild(1, o.Worker, o, now); e != nil {
				t.Fatal(e)
			}
			original := a
			change(&a)
			if l.AcknowledgeRule(2, a, o, now) == nil || l.ReadyForIngress(l.Snapshot(now).Revision, o, now) == nil {
				t.Fatal("uncertain rule enabled ingress")
			}
			closeLease(t, l, o, now)
			if l.BeginRetirement(l.Snapshot(now).Revision, original, now) == nil {
				t.Fatal("unknown/duplicate rule adopted for removal")
			}
		})
	}
}
func TestPublicationInterfaceProfileOrOwnerDriftClosesAndFreshnessExpiresEvenWithoutAnotherPoll(t *testing.T) {
	changes := map[string]func(*Observation, time.Time){"publication-owner-restarted": func(o *Observation, _ time.Time) { o.Publication.OwnerInstance = strings.Repeat("8", 32) }, "publication-generation": func(o *Observation, _ time.Time) { o.Publication.Generation++ }, "target-incarnation": func(o *Observation, now time.Time) { o.Publication.StartedAt = now.Format(time.RFC3339Nano) }, "container-id": func(o *Observation, _ time.Time) { o.Publication.ContainerID = strings.Repeat("0", 64) }, "local-address": func(o *Observation, _ time.Time) { o.Scope.LocalAddress = "192.168.4.11" }, "interface": func(o *Observation, _ time.Time) { o.Scope.InterfaceLUID = "43" }, "public-profile": func(o *Observation, _ time.Time) { o.Scope.Profile = "public" }, "stale": func(o *Observation, now time.Time) { o.ReadAt = now.Add(-6 * time.Second) }, "future": func(o *Observation, now time.Time) { o.ReadAt = now.Add(time.Second) }, "owner-lost": func(o *Observation, _ time.Time) { o.OwnerLeaseHeld = false }, "parent-dead": func(o *Observation, _ time.Time) { o.ManagerAlive = false }, "worker-dead": func(o *Observation, _ time.Time) { o.WorkerAlive = false }, "source-unknown": func(o *Observation, _ time.Time) { o.PublicationActive = false }}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			l, o, _, now := readyLease(t)
			change(&o, now)
			if l.Observe(4, o, now) == nil || l.Snapshot(now).Phase != "closing" || l.Snapshot(now).Contained || l.AllowsPeer(netip.MustParseAddr("192.168.4.20"), now) {
				t.Fatal("drift stayed open or requested closure falsely claimed containment")
			}
		})
	}
	l, _, _, now := readyLease(t)
	later := now.Add(5*time.Second + time.Nanosecond)
	if l.Snapshot(later).IngressEligible || l.AllowsPeer(netip.MustParseAddr("192.168.4.20"), later) {
		t.Fatal("stalled observation retained packet admission")
	}
}
func TestCloseTimeoutUnjoinedFlowsOrUnknownChildCannotAuthorizeConservativeRetirement(t *testing.T) {
	for _, field := range []string{"ingress", "flows", "process", "unknown-child", "pid-reuse"} {
		t.Run(field, func(t *testing.T) {
			l, o, a, now := readyLease(t)
			if e := l.BeginClose(4); e != nil {
				t.Fatal(e)
			}
			closed := CloseAcknowledgement{LeaseSHA: l.sha, Worker: o.Worker, IngressClosed: true, FlowsJoined: true, ProcessExited: true, NoUnknownChild: true}
			switch field {
			case "ingress":
				closed.IngressClosed = false
			case "flows":
				closed.FlowsJoined = false
			case "process":
				closed.ProcessExited = false
			case "unknown-child":
				closed.NoUnknownChild = false
			case "pid-reuse":
				closed.Worker.CreationTime = "201"
			}
			if l.AcknowledgeClosed(5, closed) == nil || l.Snapshot(now).Contained || l.BeginRetirement(5, a, now) == nil {
				t.Fatal("requested/incomplete closure became containment or deletion authority")
			}
		})
	}
}
func TestRetirementRejectsChangedNativeStateAndLostAcknowledgementWithoutRetrying(t *testing.T) {
	l, o, a, now := readyLease(t)
	closeLease(t, l, o, now)
	changed := a
	changed.TypedSnapshotSHA = strings.Repeat("3", 64)
	if l.BeginRetirement(l.Snapshot(now).Revision, changed, now) == nil {
		t.Fatal("changed rule state adopted")
	}
	if e := l.BeginRetirement(l.Snapshot(now).Revision, a, now); e != nil {
		t.Fatal(e)
	}
	revision := l.Snapshot(now).Revision
	for _, bad := range []RemovalAcknowledgement{{LeaseSHA: l.sha, NativeKeysSHA: a.NativeKeysSHA, Sequence: a.Sequence + 2, ObservedAbsent: true, AcknowledgementFlushed: true, ProtectedReceipt: true}, {LeaseSHA: l.sha, NativeKeysSHA: a.NativeKeysSHA, Sequence: a.Sequence + 1, TerminalSuccess: true, OperationClosed: true, ObservedAbsent: true, AcknowledgementFlushed: true, ProtectedReceipt: true}} {
		if l.AcknowledgeRemoved(revision, bad) == nil || l.Snapshot(now).Phase != "retire-pending" || !l.Snapshot(now).Contained || l.Snapshot(now).RuleRetired {
			t.Fatal("absence/lost ack rewrote pending retirement")
		}
	}
	if l.ReadyForIngress(revision, o, now) == nil || l.BeginRetirement(revision, a, now) == nil {
		t.Fatal("pending removal replayed activation/retirement")
	}
}
func TestPreparedFailedAndRetiredLeasePathsRemainTombstonedAcrossOwnerRestart(t *testing.T) {
	owner, l, s, o, a, now := leaseFixture(t)
	if _, e := owner.Prepare(s, now); e == nil {
		t.Fatal("concurrent duplicate reused prepared lease")
	}
	recovered := NewOwner(s.Request.Installation, o.Manager)
	if e := recovered.RestoreTombstone(l.Tombstone()); e != nil {
		t.Fatal(e)
	}
	if _, e := recovered.Prepare(s, now); e == nil {
		t.Fatal("owner restart reauthorized old lease/path")
	}
	s.LeaseID = strings.Repeat("d", 32)
	s.Request.Transaction = strings.Repeat("c", 32)
	s.Request.Nonce = strings.Repeat("e", 64)
	s.Worker.Path, _ = workerPath(s.WorkerRoot, s.Request.Installation, s.LeaseID)
	fresh, e := owner.Prepare(s, now)
	if e != nil {
		t.Fatal(e)
	}
	// A new lease cannot accept the old fixed v1 review's native acknowledgement,
	// even though the frozen Request.RuleName happens to be unchanged.
	if e = fresh.BindChild(1, o.Worker, Observation{ReadAt: now, Manager: o.Manager, Worker: o.Worker, WorkerFile: s.Worker, Publication: s.Publication, Scope: s.Request.Scope, ManagerPipePID: o.Manager.PID, WorkerPipePID: o.Worker.PID, OwnerLeaseHeld: true, ManagerAlive: true, WorkerAlive: true, WorkerFileHeld: true, ProtectedPath: true, PublicationActive: true, InterfacePresent: true}, now); e != nil {
		t.Fatal(e)
	}
	if fresh.AcknowledgeRule(2, a, o, now) == nil {
		t.Fatal("old acknowledgement adopted for fresh lease")
	}
}
func TestScopeInputIsFrozenAndStaleActorRevisionsCannotAmendLease(t *testing.T) {
	_, l, s, o, a, now := leaseFixture(t)
	originalSHA := l.sha
	s.Request.Scope.ClientNetworks[0] = "10.0.0.0/8"
	if e := l.BindChild(1, o.Worker, o, now); e != nil {
		t.Fatal(e)
	}
	if l.sha != originalSHA || l.AcknowledgeRule(1, a, o, now) == nil {
		t.Fatal("mutable input/stale revision changed lease")
	}
	if e := l.AcknowledgeRule(2, a, o, now); e != nil {
		t.Fatal(e)
	}
	if e := l.ReadyForIngress(3, o, now); e != nil {
		t.Fatal(e)
	}
	if l.AllowsPeer(netip.MustParseAddr("10.0.0.1"), now) || !l.AllowsPeer(netip.MustParseAddr("192.168.4.20"), now) {
		t.Fatal("caller slice mutation widened retained client scope")
	}
}
func TestUnflushedRuleOrRemovalAcknowledgementDoesNotBecomeOwnershipOrRetirement(t *testing.T) {
	_, l, _, o, a, now := leaseFixture(t)
	if e := l.BindChild(1, o.Worker, o, now); e != nil {
		t.Fatal(e)
	}
	a.AcknowledgementFlushed = false
	if l.AcknowledgeRule(2, a, o, now) == nil || l.Snapshot(now).IngressEligible {
		t.Fatal("unflushed provider success became ownership")
	}
	l, o, a, now = readyLease(t)
	closeLease(t, l, o, now)
	if e := l.BeginRetirement(l.Snapshot(now).Revision, a, now); e != nil {
		t.Fatal(e)
	}
	if l.AcknowledgeRemoved(l.Snapshot(now).Revision, RemovalAcknowledgement{LeaseSHA: l.sha, NativeKeysSHA: a.NativeKeysSHA, Sequence: a.Sequence + 2, TerminalSuccess: true, OperationClosed: true, ObservedAbsent: true, ProtectedReceipt: true}) == nil || l.Snapshot(now).RuleRetired {
		t.Fatal("unflushed removal became retired")
	}
}
func TestExpiredSetupCannotStartButHealthyEstablishedLeaseUsesFreshObservation(t *testing.T) {
	_, l, _, o, _, now := leaseFixture(t)
	later := now.Add(2 * time.Minute)
	o.ReadAt = later
	if l.BindChild(1, o.Worker, o, later) == nil || l.Snapshot(later).Phase != "closing" {
		t.Fatal("expired setup bound a runnable child")
	}
	l, o, _, now = readyLease(t)
	o.ReadAt = later
	if e := l.Observe(l.Snapshot(later).Revision, o, later); e != nil {
		t.Fatal("transaction expiry revoked established healthy consent", e)
	}
	if !l.AllowsPeer(netip.MustParseAddr("192.168.4.20"), later) {
		t.Fatal("healthy lease did not use refreshed native observation")
	}
}
func TestUnsafeRootsReboundTombstoneAndFullIndexRefuseWithoutEviction(t *testing.T) {
	for _, root := range []string{`C:\ProgramData\..\OrbWin\LAN`, `C:\ProgramData\CON\OrbWin\LAN`, `C:\ProgramData\COM1.tmp\OrbWin\LAN`, `C:\ProgramData\OrbWin\LAN.`, `\\host\ProgramData\OrbWin\LAN`, `C:\ProgramData\*\OrbWin\LAN`} {
		if _, e := workerPath(root, strings.Repeat("a", 32), strings.Repeat("b", 32)); e == nil {
			t.Fatal("unsafe native worker root accepted")
		}
	}
	owner, l, s, _, _, now := leaseFixture(t)
	old := l.Tombstone()
	rebound := old
	rebound.WorkerPath = strings.Replace(rebound.WorkerPath, `C:\ProgramData`, `D:\ProgramData`, 1)
	if owner.RestoreTombstone(rebound) == nil {
		t.Fatal("retained lease identity rebound to different root")
	}
	if e := owner.RestoreTombstone(old); e != nil {
		t.Fatal("same tombstone was not idempotent")
	}
	for n := len(owner.usedIDs); n < MaxRetainedLeaseIDs; n++ {
		owner.usedIDs[strconv.Itoa(n)] = "retained"
	}
	s.LeaseID = strings.Repeat("d", 32)
	s.Request.Transaction = strings.Repeat("c", 32)
	s.Request.Nonce = strings.Repeat("e", 64)
	s.Worker.Path, _ = workerPath(s.WorkerRoot, s.Request.Installation, s.LeaseID)
	if _, e := owner.Prepare(s, now); e == nil || owner.usedIDs[old.LeaseID] == "" {
		t.Fatal("full retained index evicted evidence or accepted unbounded lease")
	}
}
func TestFreshPathCannotReplaySpentTransactionOrNonceIncludingRestoredTombstones(t *testing.T) {
	owner, l, s, o, _, now := leaseFixture(t)
	recovered := NewOwner(s.Request.Installation, o.Manager)
	if e := recovered.RestoreTombstone(l.Tombstone()); e != nil {
		t.Fatal(e)
	}
	for _, candidateOwner := range []*Owner{owner, recovered} {
		for _, replay := range []string{"transaction", "nonce"} {
			candidate := s
			candidate.LeaseID = strings.Repeat("d", 32)
			candidate.Worker.Path, _ = workerPath(s.WorkerRoot, s.Request.Installation, candidate.LeaseID)
			if replay == "transaction" {
				candidate.Request.Nonce = strings.Repeat("e", 64)
			} else {
				candidate.Request.Transaction = strings.Repeat("c", 32)
			}
			if _, e := candidateOwner.Prepare(candidate, now); e == nil {
				t.Fatal("fresh path replayed a spent request identity")
			}
		}
	}
}
func TestRetirementNeedsFreshNativeReadbackWhileRetainedOwnershipRemainsImmutable(t *testing.T) {
	l, o, a, now := readyLease(t)
	closeLease(t, l, o, now)
	later := now.Add(6 * time.Second)
	if l.BeginRetirement(l.Snapshot(later).Revision, a, later) == nil {
		t.Fatal("retained snapshot substituted for fresh native retirement observation")
	}
	a.ReadAt = later
	if e := l.BeginRetirement(l.Snapshot(later).Revision, a, later); e != nil {
		t.Fatal("fresh identical full native readback refused", e)
	}
}

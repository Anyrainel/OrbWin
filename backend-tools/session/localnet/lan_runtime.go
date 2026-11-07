package localnet

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"

	"orbwin.local/session/internal/landatagram"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
)

var ErrLANAdapterUnavailable = errors.New("LAN activation requires qualified publication-v2 source, protected administrative helper and ordinary worker adapters")

// Internal actor API only. No existing user-only IPC command or renderer field
// can register adapters or bypass the native helper qualification gate.
func (runtime *ServiceRuntime) StartLAN(ctx context.Context, revision uint64, stage lanhelper.StageRequest) (LANStatus, error) {
	data, e := json.Marshal(lanhelper.Frame{Version: 2, Kind: "stage-request", Sequence: 1, Stage: &stage})
	if e != nil {
		return LANStatus{}, e
	}
	frame, e := lanhelper.Decode(data)
	if e != nil {
		return LANStatus{}, e
	}
	reply, e := runtime.request(ctx, runtimeCommand{kind: "lan-start", revision: revision, lanStage: frame.Stage})
	return reply.lan, e
}
func (runtime *ServiceRuntime) StopLAN(ctx context.Context, sha string) error {
	if !fullID(sha) {
		return errors.New("exact retained LAN lease digest required")
	}
	_, e := runtime.request(ctx, runtimeCommand{kind: "lan-stop", lanLeaseSHA: sha})
	return e
}

// These internal typed adapters are supplied by the permanent native owner,
// never JSON/renderer claims. Production helper registration remains gated.
type LANWorker interface {
	Identity() lanfirewall.CallerIdentity
	WaitHello(context.Context) error
	Send(context.Context, lanhelper.Frame) error
	Stop()
	Wait(context.Context) error
}
type LANAdministrativeHelper interface {
	Tombstones(context.Context) ([]lanlease.Tombstone, error)
	Stage(context.Context, lanhelper.StageRequest) (lanhelper.Prepared, error)
	Commit(context.Context, lanhelper.Prepared, lanfirewall.CallerIdentity, lanlease.Observation) (lanlease.RuleAcknowledgement, error)
	ReadRule(context.Context, lanhelper.Prepared) (lanlease.RuleAcknowledgement, error)
	Retire(context.Context, lanhelper.Prepared, lanlease.RuleAcknowledgement) (lanlease.RemovalAcknowledgement, error)
	// Close positively joins all existing administrative work and releases its
	// native owners. Failure retains them; runtime shutdown must retain its ledger.
	Close(context.Context) error
}
type LANAdapters struct {
	Installation        string // Separate native owner namespace32, never global UUID36.
	WorkerSHA256        string // Fixed, independently attested dedicated worker asset; never request JSON.
	Manager             func(context.Context) (lanfirewall.CallerIdentity, error)
	ValidatePublication func(context.Context, lanhelper.StageRequest) (time.Time, error)
	Observe             func(context.Context, lanhelper.Prepared, lanfirewall.CallerIdentity) (lanlease.Observation, error)
	Helper              LANAdministrativeHelper
	Launch              func(context.Context, lanhelper.Prepared, func(context.Context, lanhelper.Stream) (net.Conn, error)) (LANWorker, error)
	LaunchDatagram      func(context.Context, lanhelper.Prepared, func(context.Context, lanhelper.Stream) (landatagram.Connection, error)) (LANWorker, error)
}
type LANStatus struct {
	LeaseSHA         string       `json:"leaseSha256"`
	Phase            string       `json:"phase"`
	Error            string       `json:"error,omitempty"`
	OperationID      string       `json:"operationId"`
	ReviewID         string       `json:"reviewId"`
	ReviewSHA256     string       `json:"reviewSha256,omitempty"`
	RuntimeInstance  string       `json:"runtimeInstance,omitempty"`
	Selection        LANSelection `json:"selection"`
	Sequence         uint64       `json:"sequence"`
	UpdatedAt        string       `json:"updatedAt"`
	Ingress          string       `json:"ingress"`
	Firewall         string       `json:"firewall"`
	CanCancel        bool         `json:"canCancel"`
	CanInspect       bool         `json:"canInspect"`
	CanReviewRemoval bool         `json:"canReviewRemoval"`
	CanSettleRemoval bool         `json:"canSettleRemoval"`
	CanReviewCleanup bool         `json:"canReviewCleanup"`
	ReasonCode       string       `json:"reasonCode,omitempty"`
}
type lanSession struct {
	mu                                             sync.Mutex
	prepared                                       lanhelper.Prepared
	lease                                          *lanlease.Lease
	worker                                         LANWorker
	handle                                         NamespaceHandle
	borrowed, ruleOwned, unknownCommit, childBound bool
	sequence                                       uint64
	closeOnce                                      sync.Once
	resourceOnce                                   sync.Once
	closed                                         chan struct{}
	result                                         error
	setupDone                                      chan struct{}
	setupCancel                                    context.CancelFunc
	observedAt                                     time.Time
	changedAt                                      time.Time
	refreshActive                                  bool
	failure                                        string
	ingressClosed                                  bool
	retireActive, retirementDispatched             bool
}
type lanController struct {
	mu           sync.Mutex
	installation string
	shared       *sharedNamespaces
	adapters     *LANAdapters
	owner        *lanlease.Owner
	sessions     map[string]*lanSession
	rules        map[string]*lanSession // Dormant protected receipts; no active namespace/child ownership.
	closing      bool
	once         sync.Once
	done         chan struct{}
	result       error
	ctx          context.Context
	cancel       context.CancelFunc
	setup        sync.WaitGroup
	setupActive  bool
	refreshMu    sync.Mutex
	observations sync.WaitGroup
	onChange     func()
}

func newLANController(installation string, shared *sharedNamespaces, adapters *LANAdapters) *lanController {
	if adapters != nil {
		copied := *adapters
		if copied.Manager == nil {
			copied.Manager = nativeLANManager
		}
		if copied.Launch == nil {
			copied.Launch = nativeLANLaunch
		}
		if copied.LaunchDatagram == nil {
			copied.LaunchDatagram = nativeLANLaunchDatagram
		}
		adapters = &copied
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &lanController{installation: installation, shared: shared, adapters: adapters, sessions: make(map[string]*lanSession), rules: make(map[string]*lanSession), done: make(chan struct{}), ctx: ctx, cancel: cancel}
}
func (c *lanController) start(ctx context.Context, stage lanhelper.StageRequest) (LANStatus, error) {
	return c.startWithProgress(ctx, stage, nil)
}

// One setup worker may wait on attended consent; c.mu never covers native or
// helper calls. Existing committed leases keep their independent refresh path.
// A setup-done barrier contains a late created child before teardown can settle.
func (c *lanController) startWithProgress(ctx context.Context, stage lanhelper.StageRequest, progress func(string)) (LANStatus, error) {
	return c.startObserved(ctx, stage, progress, nil)
}
func (c *lanController) startObserved(ctx context.Context, stage lanhelper.StageRequest, progress func(string), observedStatus func(*lanSession)) (LANStatus, error) {
	c.mu.Lock()
	c.compactClosedLocked()
	a := c.adapters
	if !validLANID(c.installation) || stage.Base.Installation != c.installation {
		c.mu.Unlock()
		return LANStatus{}, errors.New("LAN stage differs from the separately bound native owner namespace")
	}
	if c.closing || a == nil || a.Manager == nil || a.ValidatePublication == nil || a.Helper == nil || a.Launch == nil {
		c.mu.Unlock()
		return LANStatus{}, ErrLANAdapterUnavailable
	}
	if len(c.sessions)+len(c.rules) >= 16 || c.setupActive {
		c.mu.Unlock()
		return LANStatus{}, errors.New("LAN lease retention budget reached; settle existing leases before new activation")
	}
	if len(c.rules) >= lanlease.MaxRetainedLeaseIDs {
		c.mu.Unlock()
		return LANStatus{}, errors.New("retained protected LAN rule history exhausted")
	}
	c.setupActive = true
	c.setup.Add(1)
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.setupActive = false; c.mu.Unlock(); c.setup.Done() }()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	if ctx.Err() != nil {
		return LANStatus{}, ctx.Err()
	}
	report := func(phase string) {
		if progress != nil {
			progress(phase)
		}
	}
	manager, e := a.Manager(ctx)
	if e != nil {
		return LANStatus{}, e
	}
	if e = lanhelper.ValidateStage(stage, manager, time.Now()); e != nil {
		return LANStatus{}, e
	}
	if _, e = a.ValidatePublication(ctx, stage); e != nil {
		return LANStatus{}, e
	} // BEFORE helper/staging/launch.
	c.mu.Lock()
	needsOwner := c.owner == nil
	c.mu.Unlock()
	if needsOwner {
		owner := lanlease.NewOwner(c.installation, manager)
		tombs, e := a.Helper.Tombstones(ctx)
		if e != nil {
			return LANStatus{}, e
		}
		if len(tombs) > lanlease.MaxRetainedLeaseIDs {
			return LANStatus{}, errors.New("protected lease tombstone budget exceeded")
		}
		for _, tomb := range tombs {
			if e = owner.RestoreTombstone(tomb); e != nil {
				return LANStatus{}, e
			}
		}
		c.mu.Lock()
		if c.closing {
			c.mu.Unlock()
			return LANStatus{}, errors.New("LAN generation is closing")
		}
		c.owner = owner
		c.mu.Unlock()
	}
	report("awaiting-admin")
	prepared, e := a.Helper.Stage(ctx, stage)
	if e != nil {
		return LANStatus{}, e
	}
	prepared.Spec.Request.Scope.ClientNetworks = append([]string(nil), prepared.Spec.Request.Scope.ClientNetworks...)
	if prepared.Sequence != 2 || prepared.Spec.Request.Digest() != stage.Base.Digest() || prepared.Spec.Publication != stage.Publication {
		return LANStatus{}, errors.New("protected helper stage differs from reviewed request")
	}
	c.mu.Lock()
	if c.closing || ctx.Err() != nil {
		c.mu.Unlock()
		return LANStatus{}, errors.New("LAN setup cancelled before worker admission")
	}
	lease, e := c.owner.Prepare(prepared.Spec, time.Now())
	if e != nil {
		c.mu.Unlock()
		return LANStatus{}, e
	}
	sha := lease.Snapshot(time.Now()).LeaseSHA
	if sha != prepared.LeaseSHA {
		c.mu.Unlock()
		return LANStatus{}, errors.New("protected staged lease digest differs")
	}
	s := &lanSession{prepared: prepared, lease: lease, closed: make(chan struct{}), setupDone: make(chan struct{}), setupCancel: cancel, changedAt: time.Now()}
	c.sessions[sha] = s
	c.mu.Unlock()
	defer close(s.setupDone)
	if observedStatus != nil {
		observedStatus(s)
	}
	report("starting")
	status := func(e error) (LANStatus, error) {
		if e != nil {
			c.closeSession(s)
		}
		return c.sessionStatus(s), e
	}
	handle, e := c.shared.borrowLAN(ctx, sha, stage.Publication.ContainerID, stage.Publication.StartedAt)
	if e != nil {
		return status(e)
	}
	s.mu.Lock()
	s.handle, s.borrowed = handle, true
	s.mu.Unlock()
	if e = c.shared.renewOwner(ctx, sha, s.handle); e != nil {
		return status(e)
	}
	route := ServiceRoute{ContainerID: stage.Publication.ContainerID, StartedAt: stage.Publication.StartedAt, HelperID: s.handle.ID, Port: stage.Publication.ContainerPort, NamespaceLeaseSHA: prepared.LeaseSHA}
	var worker LANWorker
	if prepared.Spec.Request.Scope.Protocol == "udp" {
		worker, e = a.LaunchDatagram(ctx, prepared, func(ctx context.Context, stream lanhelper.Stream) (landatagram.Connection, error) {
			return c.shared.dialDatagramOwner(ctx, sha, route)
		})
	} else {
		worker, e = a.Launch(ctx, prepared, func(ctx context.Context, stream lanhelper.Stream) (net.Conn, error) {
			// The ordinary worker cannot choose an engine, helper, container or port.
			return c.shared.dialOwner(ctx, sha, route)
		})
	}
	s.mu.Lock()
	s.worker = worker
	s.mu.Unlock()
	if e != nil {
		return status(e)
	}
	if s.worker == nil {
		return status(errors.New("ordinary worker adapter returned no owned child"))
	}
	if e = s.worker.WaitHello(ctx); e != nil {
		return status(e)
	}
	observed, e := c.observe(ctx, s)
	if e != nil {
		return status(e)
	}
	s.mu.Lock()
	e = lease.BindChild(1, s.worker.Identity(), observed, time.Now())
	if e == nil {
		s.childBound = true
	}
	s.mu.Unlock()
	if e != nil {
		return status(e)
	}
	// A transport loss after Commit is never retried/adopted. Mark uncertainty
	// BEFORE dispatch; only a positive flushed typed acknowledgement clears it.
	s.mu.Lock()
	s.unknownCommit = true
	s.mu.Unlock()
	report("commit-dispatched")
	var rule lanlease.RuleAcknowledgement
	// Native cleanup metadata needs the actual full helper ID created/held by
	// this namespace owner. It is never a renderer selection or name lookup.
	if native, ok := a.Helper.(interface {
		CommitNamespace(context.Context, lanhelper.Prepared, lanfirewall.CallerIdentity, lanlease.Observation, string) (lanlease.RuleAcknowledgement, error)
	}); ok {
		rule, e = native.CommitNamespace(ctx, prepared, s.worker.Identity(), observed, s.handle.ID)
	} else {
		rule, e = a.Helper.Commit(ctx, prepared, s.worker.Identity(), observed)
	}
	if e != nil {
		return status(e)
	}
	s.mu.Lock()
	if lease.Snapshot(time.Now()).Phase == "closing" {
		e = lease.AcknowledgeRuleWhileClosing(lease.Snapshot(time.Now()).Revision, rule)
		if e == nil {
			s.ruleOwned, s.unknownCommit = true, false
			e = errors.New("LAN setup closed after positive native completion")
		}
	} else {
		e = lease.AcknowledgeRule(2, rule, observed, time.Now())
		if e == nil {
			s.ruleOwned, s.unknownCommit = true, false
		}
		// A stale completion may still retain positively acknowledged cleanup
		// ownership while the pure policy correctly keeps ingress closed.
		if e != nil && lease.Snapshot(time.Now()).Phase == "closing" && lease.AcknowledgeRuleWhileClosing(lease.Snapshot(time.Now()).Revision, rule) == nil {
			s.ruleOwned, s.unknownCommit = true, false
		}
	}
	s.mu.Unlock()
	if e != nil {
		return status(e)
	}
	report("rule-created")
	if rule.ReadAt.Before(observed.ReadAt) {
		observed.ReadAt = rule.ReadAt
	}
	s.mu.Lock()
	e = lease.ReadyForIngress(lease.Snapshot(time.Now()).Revision, observed, time.Now())
	s.mu.Unlock()
	if e != nil {
		return status(e)
	}
	grant := lanhelper.Frame{Version: 2, Kind: "worker-grant", Sequence: 1, LeaseSHA: sha, Grant: &lanhelper.Grant{Observation: observed, Rule: rule}}
	if e = s.worker.Send(ctx, grant); e != nil {
		return status(e)
	}
	s.mu.Lock()
	s.sequence = 1
	s.observedAt = observed.ReadAt
	s.mu.Unlock()
	return c.sessionStatus(s), nil
}
func (c *lanController) sessionStatus(s *lanSession) LANStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.lease.Snapshot(time.Now())
	status := LANStatus{LeaseSHA: state.LeaseSHA, Phase: state.Phase, Ingress: "closed", Firewall: "not-dispatched", Sequence: s.sequence, UpdatedAt: s.observedAt.UTC().Format(time.RFC3339Nano)}
	if state.Phase != "ready" && !s.changedAt.IsZero() {
		status.UpdatedAt = s.changedAt.UTC().Format(time.RFC3339Nano)
	}
	if s.ruleOwned {
		status.Firewall = "created"
		status.CanReviewRemoval = true
	}
	if s.unknownCommit {
		status.Firewall = "create-unknown"
		status.CanInspect = true
	}
	if s.retirementDispatched && !state.RuleRetired {
		status.Firewall = "remove-unknown"
		status.CanInspect = true
		status.CanReviewRemoval = false
		status.CanSettleRemoval = s.ingressClosed && !s.borrowed && !s.retireActive
	}
	if state.RuleRetired {
		status.Firewall = "removed"
		status.CanReviewRemoval = false
	}
	if state.Phase == "ready" && s.observedAt.IsZero() {
		status.Phase = "starting"
	}
	if state.Phase == "ready" && !state.IngressEligible {
		status.Phase = "observation-expired"
		status.Ingress = "unknown"
	}
	if state.IngressEligible && !s.observedAt.IsZero() {
		status.Ingress = "ready"
	} else if state.Phase == "closing" && !s.ingressClosed && s.worker != nil {
		status.Ingress = "unknown"
	}
	select {
	case <-s.closed:
		status.Ingress = "closed"
		if state.RuleRetired {
			status.Phase = "removed"
		} else if s.ruleOwned || s.unknownCommit {
			status.Phase = "cleanup-pending"
		} else {
			status.Phase = "closed"
		}
	default:
	}
	if s.result != nil {
		status.Error = "LAN cleanup remains unconfirmed; retained exact ownership"
		status.ReasonCode = "namespace-retained"
		status.CanInspect = true
	}
	if s.failure != "" {
		status.Error = s.failure
		status.ReasonCode = "observation-changed"
	}
	status.CanCancel = state.Phase == "ready" || state.Phase == "prepared" || state.Phase == "child-bound" || state.Phase == "rule-acknowledged"
	return status
}

type lanRemovalSettlement interface {
	SettleRemoval(context.Context, lanhelper.Prepared) (lanlease.RemovalAcknowledgement, error)
}

// This is an explicitly consumed NEW review/operation. It never re-enters
// Retire/BeginRetirement: only the absence-only native settlement capability
// can seal the existing unknown deletion, while ingress remains closed.
func (c *lanController) settleRemoval(ctx context.Context, sha string, progress func(string)) error {
	s := c.findSession(sha)
	if s == nil {
		return errors.New("exact retained removal unavailable")
	}
	select {
	case <-s.closed:
	case <-ctx.Done():
		return ctx.Err()
	}
	settlement, ok := c.adapters.Helper.(lanRemovalSettlement)
	if !ok {
		return errors.New("absence-only removal settlement capability unavailable")
	}
	s.mu.Lock()
	if !s.ruleOwned || s.unknownCommit || !s.retirementDispatched || s.retireActive || s.borrowed || !s.ingressClosed || s.lease.Snapshot(time.Now()).RuleRetired {
		s.mu.Unlock()
		return errors.New("positive closed resources and pending exact removal required")
	}
	s.retireActive = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.retireActive = false; s.mu.Unlock() }()
	if progress != nil {
		progress("settle-dispatched")
	}
	ack, err := settlement.SettleRemoval(ctx, s.prepared)
	if err != nil {
		return err
	}
	s.mu.Lock()
	err = s.lease.AcknowledgeRemoved(s.lease.Snapshot(time.Now()).Revision, ack)
	if err == nil {
		s.changedAt = time.Now()
	}
	s.mu.Unlock()
	if err == nil {
		c.mu.Lock()
		delete(c.rules, sha)
		delete(c.sessions, sha)
		c.mu.Unlock()
	}
	return err
}
func (c *lanController) statuses() []LANStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.compactClosedLocked()
	out := make([]LANStatus, 0, len(c.sessions))
	for _, s := range c.sessions {
		out = append(out, c.sessionStatus(s))
	}
	for _, s := range c.rules {
		if len(out) >= 64 {
			break
		}
		out = append(out, c.sessionStatus(s))
	}
	return out
}
func (c *lanController) refresh(ctx context.Context) error {
	if !c.refreshMu.TryLock() {
		return errors.New("LAN refresh is already active")
	}
	defer c.refreshMu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	c.mu.Lock()
	if c.closing {
		c.mu.Unlock()
		return nil
	}
	sessions := make([]*lanSession, 0, len(c.sessions))
	for _, s := range c.sessions {
		sessions = append(sessions, s)
	}
	c.mu.Unlock()
	var errs []error
	for _, s := range sessions {
		if e := c.refreshOne(ctx, s); e != nil {
			errs = append(errs, e)
		}
	}
	c.changed()
	return errors.Join(errs...)
}

// The actor only kicks bounded per-resource observations. One stuck Windows
// provider/read child cannot block HTTP renewal or other LAN observations.
func (c *lanController) kickRefresh() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return
	}
	for _, s := range c.sessions {
		s.mu.Lock()
		eligible := !s.refreshActive && !s.observedAt.IsZero() && s.lease.Snapshot(time.Now()).Phase == "ready"
		if eligible {
			s.refreshActive = true
		}
		s.mu.Unlock()
		if !eligible {
			continue
		}
		c.observations.Add(1)
		go func(s *lanSession) {
			defer c.observations.Done()
			ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
			defer cancel()
			err := c.refreshOne(ctx, s)
			s.mu.Lock()
			s.refreshActive = false
			s.mu.Unlock()
			if !errors.Is(err, lanhelper.ErrObservationBusy) {
				c.changed()
			}
		}(s)
	}
}
func (c *lanController) changed() {
	if c.onChange != nil {
		c.onChange()
	}
}
func (c *lanController) refreshOne(ctx context.Context, s *lanSession) error {
	s.mu.Lock()
	snapshot := s.lease.Snapshot(time.Now())
	starting := s.observedAt.IsZero()
	s.mu.Unlock()
	if snapshot.Phase != "ready" || starting {
		return nil
	}
	current, e := c.adapters.Helper.ReadRule(ctx, s.prepared)
	if errors.Is(e, lanhelper.ErrObservationBusy) {
		return e
	}
	var observed lanlease.Observation
	if e == nil {
		observed, e = c.observe(ctx, s)
	}
	if current.ReadAt.Before(observed.ReadAt) {
		observed.ReadAt = current.ReadAt
	}
	if e == nil {
		s.mu.Lock()
		e = s.lease.ObserveRule(snapshot.Revision, current, observed, time.Now())
		s.mu.Unlock()
	}
	if e == nil {
		s.mu.Lock()
		if s.sequence >= lanfirewall.MaxSequence {
			e = errors.New("LAN worker sequence exhausted")
		} else {
			s.sequence++
		}
		sequence := s.sequence
		s.mu.Unlock()
		if e == nil {
			e = s.worker.Send(ctx, lanhelper.Frame{Version: 2, Kind: "refresh", Sequence: sequence, LeaseSHA: snapshot.LeaseSHA, Observation: &observed})
		}
	}
	if e == nil {
		e = c.shared.renewOwner(ctx, s.prepared.LeaseSHA, s.handle)
	}
	if e != nil {
		s.mu.Lock()
		s.failure = "LAN observation failed or changed; ingress is closing"
		s.mu.Unlock()
		c.closeSession(s)
	} else {
		s.mu.Lock()
		s.observedAt = observed.ReadAt
		s.mu.Unlock()
	}
	return e
}

func (c *lanController) observe(ctx context.Context, s *lanSession) (lanlease.Observation, error) {
	sourceRead, e := c.adapters.ValidatePublication(ctx, lanhelper.StageRequest{Base: s.prepared.Spec.Request, Publication: s.prepared.Spec.Publication})
	if e != nil {
		return lanlease.Observation{}, e
	}
	if c.adapters.Observe != nil {
		o, e := c.adapters.Observe(ctx, s.prepared, s.worker.Identity())
		if sourceRead.Before(o.ReadAt) {
			o.ReadAt = sourceRead
		}
		return o, e
	}
	observer, ok := s.worker.(interface {
		Observation(context.Context) (lanlease.Observation, error)
	})
	if !ok {
		return lanlease.Observation{}, ErrLANAdapterUnavailable
	}
	observed, e := observer.Observation(ctx)
	if e != nil {
		return lanlease.Observation{}, e
	}
	backend, ok := c.shared.base.(*OwnedServiceBackend)
	if !ok || backend.closing.Load() {
		return lanlease.Observation{}, ErrLANAdapterUnavailable
	}
	ledger := backend.namespaces.ledger
	ledger.mu.Lock()
	owned := ledger.owned()
	ledger.mu.Unlock()
	if !owned {
		return lanlease.Observation{}, errors.New("authoritative local-services ledger ownership lost")
	}
	observed.OwnerLeaseHeld = true
	if sourceRead.Before(observed.ReadAt) {
		observed.ReadAt = sourceRead
	}
	return observed, nil
}
func (c *lanController) closeSession(s *lanSession) {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		_ = s.lease.BeginClose(s.lease.Snapshot(time.Now()).Revision)
		s.changedAt = time.Now()
		if s.setupCancel != nil {
			s.setupCancel()
		}
		initialWorker := s.worker
		s.mu.Unlock()
		if initialWorker != nil {
			initialWorker.Stop()
		}
		c.changed()
		go func() {
			if s.setupDone != nil {
				<-s.setupDone
			}
			// Setup may return a retained native child after cancellation. Only
			// this SAME close worker owns its Stop/Wait and exact cleanup barrier.
			if s.worker != nil && initialWorker == nil {
				s.worker.Stop()
			}
			// The existing exact child owns its native exit/join operation. Wait does
			// not create/retry any process/provider action and can remain pending.
			fail := func(e error) { s.mu.Lock(); s.result = e; s.mu.Unlock() }
			if s.worker != nil {
				if e := s.worker.Wait(context.Background()); e != nil {
					s.mu.Lock()
					s.result = e
					s.mu.Unlock()
					return
				}
			}
			if s.worker != nil && s.childBound {
				ack := lanlease.CloseAcknowledgement{LeaseSHA: s.prepared.LeaseSHA, Worker: s.worker.Identity(), IngressClosed: true, FlowsJoined: true, ProcessExited: true, NoUnknownChild: true}
				s.mu.Lock()
				e := s.lease.AcknowledgeClosed(s.lease.Snapshot(time.Now()).Revision, ack)
				s.mu.Unlock()
				if e != nil {
					fail(e)
					return
				}
			}
			s.mu.Lock()
			s.ingressClosed = true
			s.changedAt = time.Now()
			s.mu.Unlock()
			if s.borrowed {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				e := c.shared.release(ctx, s.prepared.LeaseSHA, s.handle)
				cancel()
				if e != nil {
					fail(e)
					return
				}
				s.mu.Lock()
				s.borrowed = false
				s.mu.Unlock()
			}
			s.resourceOnce.Do(func() { close(s.closed) })
			// Protected rule/intent ownership persists in the administrative store.
			// A dormant unique program path cannot authorize a new listener. No
			// automatic failure/drift/shutdown path is allowed to trigger UAC.
			c.mu.Lock()
			c.compactClosedLocked()
			c.mu.Unlock()
			c.changed()
		}()
	})
}

func (c *lanController) compactClosedLocked() {
	for sha, s := range c.sessions {
		select {
		case <-s.closed:
			s.mu.Lock()
			retained := (s.ruleOwned || s.unknownCommit) && !s.lease.Snapshot(time.Now()).RuleRetired
			s.mu.Unlock()
			if retained {
				c.rules[sha] = s
			}
			delete(c.sessions, sha)
		default:
		}
	}
}

// retire is called ONLY after the ordinary owner consumes an explicit removal
// review. Automatic teardown stops ingress/releases references, retaining the
// protected dormant rule for this separate attended operation.
func (c *lanController) retire(ctx context.Context, sha string, progress func(string)) error {
	c.mu.Lock()
	s := c.sessions[sha]
	if s == nil {
		s = c.rules[sha]
	}
	c.mu.Unlock()
	if s == nil {
		return errors.New("exact retained LAN rule is unavailable")
	}
	c.closeSession(s)
	select {
	case <-s.closed:
	case <-ctx.Done():
		return ctx.Err()
	}
	s.mu.Lock()
	if !s.ruleOwned || s.unknownCommit || s.retireActive || s.retirementDispatched || s.lease.Snapshot(time.Now()).RuleRetired {
		s.mu.Unlock()
		return errors.New("LAN rule retirement is unowned, active or uncertain")
	}
	s.retireActive = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.retireActive = false; s.mu.Unlock() }()
	current, e := c.adapters.Helper.ReadRule(ctx, s.prepared)
	if e != nil {
		return e
	}
	s.mu.Lock()
	e = s.lease.BeginRetirement(s.lease.Snapshot(time.Now()).Revision, current, time.Now())
	if e == nil {
		s.retirementDispatched = true
		s.changedAt = time.Now()
	}
	s.mu.Unlock()
	if e != nil {
		return e
	}
	if progress != nil {
		progress("retire-dispatched")
	}
	removed, e := c.adapters.Helper.Retire(ctx, s.prepared, current)
	if e != nil {
		return e
	} // Unknown Delete is never automatically dispatched again.
	s.mu.Lock()
	e = s.lease.AcknowledgeRemoved(s.lease.Snapshot(time.Now()).Revision, removed)
	if e == nil {
		s.changedAt = time.Now()
	}
	s.mu.Unlock()
	if e == nil {
		c.mu.Lock()
		delete(c.rules, sha)
		delete(c.sessions, sha)
		c.mu.Unlock()
	}
	return e
}
func (c *lanController) stop(ctx context.Context, sha string) error {
	c.mu.Lock()
	s := c.sessions[sha]
	if s == nil {
		s = c.rules[sha]
	}
	if s == nil {
		c.mu.Unlock()
		return errors.New("LAN lease is not owned by this generation")
	}
	c.closeSession(s)
	c.mu.Unlock()
	select {
	case <-s.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *lanController) Close(ctx context.Context) error {
	c.once.Do(func() {
		c.cancel()
		c.mu.Lock()
		c.closing = true
		sessions := make([]*lanSession, 0, len(c.sessions))
		for _, s := range c.sessions {
			sessions = append(sessions, s)
			c.closeSession(s)
		}
		c.mu.Unlock()
		go func() {
			c.setup.Wait()
			c.observations.Wait()
			for _, s := range sessions {
				<-s.closed
			}
			close(c.done)
		}()
	})
	return c.Wait(ctx)
}
func (c *lanController) Wait(ctx context.Context) error {
	select {
	case <-c.done:
		return c.result
	default:
	}
	select {
	case <-c.done:
		return c.result
	case <-ctx.Done():
		return ctx.Err()
	}
}

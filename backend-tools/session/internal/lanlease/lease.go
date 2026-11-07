// Package lanlease is a side-effect-free containment transition policy for the
// existing ServiceRuntime actor. It authenticates no OS fact, starts no worker,
// owns no socket/store and authorizes no native operation by itself. All process,
// protected-record and provider facts must come from separately qualified native
// adapters. This package deliberately has no JSON/wire or localnet dependency.
package lanlease

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"orbwin.local/session/internal/lanfirewall"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Publication struct {
	OwnerInstance                  string
	Daemon, ContainerID, StartedAt string
	Generation                     uint64
	HostPort, ContainerPort        uint16
	Protocol                       string
}
type Spec struct {
	Request             lanfirewall.Request // Frozen v1 review remains unchanged; NOT a v2 wire.
	Publication         Publication
	LeaseID, WorkerRoot string
	Worker              lanfirewall.WorkerBinding
}
type Observation struct {
	ReadAt                                                                                                        time.Time
	Manager, Worker                                                                                               lanfirewall.CallerIdentity
	WorkerFile                                                                                                    lanfirewall.WorkerBinding
	Publication                                                                                                   Publication
	Scope                                                                                                         lanfirewall.Scope
	ManagerPipePID, WorkerPipePID                                                                                 uint32
	OwnerLeaseHeld, ManagerAlive, WorkerAlive, WorkerFileHeld, ProtectedPath, PublicationActive, InterfacePresent bool
}

// These are copied native facts, not caller proof or parsed receipt authority.
type RuleAcknowledgement struct {
	ReadAt                                                                     time.Time
	LeaseSHA, ReviewedScopeSHA, NativeKeysSHA, TypedSnapshotSHA                string
	Sequence                                                                   uint64
	Outcome                                                                    string // Only "created" is a positive acknowledged owner.
	TerminalSuccess, OperationClosed, ProtectedReceipt, AcknowledgementFlushed bool
	RuleProperties, FilterFamilies, ReverseIdentities                          int
}
type CloseAcknowledgement struct {
	LeaseSHA                                                  string
	Worker                                                    lanfirewall.CallerIdentity
	IngressClosed, FlowsJoined, ProcessExited, NoUnknownChild bool
}
type RemovalAcknowledgement struct {
	LeaseSHA, NativeKeysSHA                                                                    string
	Sequence                                                                                   uint64
	TerminalSuccess, OperationClosed, ObservedAbsent, ProtectedReceipt, AcknowledgementFlushed bool
}
type Tombstone struct{ LeaseID, WorkerPath, Transaction, Nonce string }
type Snapshot struct {
	Phase, LeaseSHA                         string
	Revision                                uint64
	IngressEligible, Contained, RuleRetired bool
}

// Owner is only the existing actor's pure in-memory bookkeeping. It must be
// driven serially by that actor; it creates no second orchestrator or goroutine.
// Restored tombstones must come from the protected native store before Prepare.
const MaxRetainedLeaseIDs = 4096

type Owner struct {
	installation                 string
	manager                      lanfirewall.CallerIdentity
	usedIDs, usedPaths           map[string]string
	usedTransactions, usedNonces map[string]string
}
type Lease struct {
	owner      *Owner
	spec       Spec
	sha, phase string
	revision   uint64
	child      lanfirewall.CallerIdentity
	rule       RuleAcknowledgement
	lastRead   time.Time
}

func NewOwner(installation string, manager lanfirewall.CallerIdentity) *Owner {
	return &Owner{installation: installation, manager: manager, usedIDs: map[string]string{}, usedPaths: map[string]string{}, usedTransactions: map[string]string{}, usedNonces: map[string]string{}}
}
func exactHex(s string, n int) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == n && hex.EncodeToString(b) == s
}
func positiveDecimal(s string) bool {
	n, e := strconv.ParseUint(s, 10, 64)
	return e == nil && n > 0 && strconv.FormatUint(n, 10) == s
}
func validPublication(p Publication, scope lanfirewall.Scope) bool {
	started, e := time.Parse(time.RFC3339Nano, p.StartedAt)
	return exactHex(p.OwnerInstance, 16) && p.Daemon != "" && len(p.Daemon) <= 128 && utf8.ValidString(p.Daemon) && !strings.ContainsFunc(p.Daemon, unicode.IsControl) && exactHex(p.ContainerID, 32) && e == nil && !started.IsZero() && p.Generation > 0 && p.HostPort == scope.LocalPort && p.ContainerPort > 0 && p.Protocol == scope.Protocol
}
func workerPath(root, installation, lease string) (string, error) {
	if len(root) < 3 || len(root) > 512 || root[0] < 'A' || root[0] > 'Z' || root[1:3] != `:\` || !strings.HasSuffix(root, `\OrbWin\LAN`) || strings.ContainsAny(root, `/%*?[]`+"`\x00") || !utf8.ValidString(root) || strings.ContainsFunc(root, unicode.IsControl) {
		return "", errors.New("fixed native local LAN worker root required")
	}
	for _, part := range strings.Split(root[3:], `\`) {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, " .") != part || strings.ContainsAny(part, `:<>"|`) {
			return "", errors.New("worker root contains unsafe path component")
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return "", errors.New("worker root contains reserved device component")
		}
	}
	if !exactHex(installation, 16) || !exactHex(lease, 16) {
		return "", errors.New("fresh canonical installation/lease identity required")
	}
	return root + `\` + installation + `\` + lease + `\orbwin-session.exe`, nil
}
func validWorker(w lanfirewall.WorkerBinding, scope lanfirewall.Scope) bool {
	return exactHex(w.FileID, 16) && positiveDecimal(w.VolumeSerial) && w.SHA256 == scope.WorkerSHA256 && w.InterfaceAlias != "" && len(w.InterfaceAlias) <= 1024 && utf8.ValidString(w.InterfaceAlias) && !strings.ContainsFunc(w.InterfaceAlias, unicode.IsControl) && !strings.ContainsAny(w.InterfaceAlias, "*?[]`")
}
func (o *Owner) RestoreTombstone(t Tombstone) error {
	if o == nil || !exactHex(o.installation, 16) || !exactHex(t.LeaseID, 16) || !exactHex(t.Transaction, 16) || !exactHex(t.Nonce, 32) || len(t.WorkerPath) > 1024 {
		return errors.New("tombstone identity invalid")
	}
	suffix := `\` + o.installation + `\` + t.LeaseID + `\orbwin-session.exe`
	if !strings.HasSuffix(t.WorkerPath, suffix) {
		return errors.New("tombstone path identity differs")
	}
	root := strings.TrimSuffix(t.WorkerPath, suffix)
	want, e := workerPath(root, o.installation, t.LeaseID)
	if e != nil || want != t.WorkerPath {
		return errors.New("tombstone root invalid")
	}
	key := strings.ToLower(t.WorkerPath)
	if previous := o.usedIDs[t.LeaseID]; previous != "" {
		if previous == key && o.usedTransactions[t.Transaction] == t.LeaseID && o.usedNonces[t.Nonce] == t.LeaseID {
			return nil
		}
		return errors.New("retained lease identity cannot be rebound to another root/path")
	}
	if o.usedPaths[key] != "" || o.usedTransactions[t.Transaction] != "" || o.usedNonces[t.Nonce] != "" || len(o.usedIDs) >= MaxRetainedLeaseIDs {
		return errors.New("retained lease index conflict/full; preserve existing tombstones")
	}
	o.usedIDs[t.LeaseID] = key
	o.usedPaths[key] = t.LeaseID
	o.usedTransactions[t.Transaction] = t.LeaseID
	o.usedNonces[t.Nonce] = t.LeaseID
	return nil
}
func (o *Owner) Prepare(s Spec, now time.Time) (*Lease, error) {
	if o == nil || s.Request.Installation != o.installation || s.Request.Operation != "create" || s.Request.ExpectedSequence != 0 {
		return nil, errors.New("fresh lease must belong to the authoritative manager installation")
	}
	if e := lanfirewall.GuardCaller(s.Request, o.manager, now); e != nil {
		return nil, e
	}
	want, e := workerPath(s.WorkerRoot, o.installation, s.LeaseID)
	if e != nil || s.Worker.Path != want || !validWorker(s.Worker, s.Request.Scope) || !validPublication(s.Publication, s.Request.Scope) {
		return nil, errors.New("lease requires exact fresh worker path/file/publication scope")
	}
	if o.usedIDs[s.LeaseID] != "" || o.usedPaths[strings.ToLower(s.Worker.Path)] != "" {
		return nil, errors.New("retained lease/path cannot be reused or reauthorized")
	}
	if len(o.usedIDs) >= MaxRetainedLeaseIDs {
		return nil, errors.New("retained lease index full; no evidence eviction or nonce reuse")
	}
	if o.usedTransactions[s.Request.Transaction] != "" || o.usedNonces[s.Request.Nonce] != "" {
		return nil, errors.New("single-use transaction/nonce cannot stage another lease")
	}
	s.Request.Scope.ClientNetworks = append([]string(nil), s.Request.Scope.ClientNetworks...)
	encoded, e := json.Marshal(s)
	if e != nil || len(encoded) > 32<<10 {
		return nil, errors.New("lease description exceeds bounded review")
	}
	sum := sha256.Sum256(append([]byte("orbwin-lan-worker-lease-policy-v1\n"), encoded...))
	// Reserve permanently, including failed setup. Native integration must flush
	// this reservation/tombstone before staging a file or spawning a process.
	o.usedIDs[s.LeaseID] = strings.ToLower(s.Worker.Path)
	o.usedPaths[strings.ToLower(s.Worker.Path)] = s.LeaseID
	o.usedTransactions[s.Request.Transaction] = s.LeaseID
	o.usedNonces[s.Request.Nonce] = s.LeaseID
	return &Lease{owner: o, spec: s, sha: hex.EncodeToString(sum[:]), phase: "prepared", revision: 1}, nil
}
func freshAt(read, now time.Time) bool {
	return !read.IsZero() && !now.Before(read) && now.Sub(read) <= 5*time.Second
}
func (l *Lease) Snapshot(now time.Time) Snapshot {
	if l == nil {
		return Snapshot{}
	}
	return Snapshot{l.phase, l.sha, l.revision, l.phase == "ready" && freshAt(l.lastRead, now), l.phase == "closed" || l.phase == "retire-pending" || l.phase == "retired", l.phase == "retired"}
}
func (l *Lease) Tombstone() Tombstone {
	if l == nil {
		return Tombstone{}
	}
	return Tombstone{l.spec.LeaseID, l.spec.Worker.Path, l.spec.Request.Transaction, l.spec.Request.Nonce}
}
func (l *Lease) check(revision uint64, phase string) error {
	if l == nil || l.owner == nil || l.revision != revision || l.phase != phase {
		return errors.New("lease revision/transition differs from authoritative actor")
	}
	return nil
}
func (l *Lease) fresh(observed Observation, now time.Time, child lanfirewall.CallerIdentity) error {
	if !freshAt(observed.ReadAt, now) || !observed.OwnerLeaseHeld || !observed.ManagerAlive || !observed.WorkerAlive || !observed.WorkerFileHeld || !observed.ProtectedPath || !observed.PublicationActive || !observed.InterfacePresent {
		return errors.New("lease observation stale, unknown, unowned or unavailable")
	}
	if observed.Manager != l.owner.manager || observed.Worker != child || observed.ManagerPipePID != l.owner.manager.PID || observed.WorkerPipePID != child.PID || observed.WorkerFile != l.spec.Worker || observed.Publication != l.spec.Publication || observed.Scope.Validate() != nil || observed.Scope.Digest() != l.spec.Request.Scope.Digest() {
		return errors.New("native manager/child/file/publication/interface scope changed")
	}
	return nil
}
func (l *Lease) BindChild(revision uint64, spawned lanfirewall.CallerIdentity, observed Observation, now time.Time) error {
	if e := l.check(revision, "prepared"); e != nil {
		return e
	}
	parentTime, _ := strconv.ParseUint(l.owner.manager.CreationTime, 10, 64)
	childTime, _ := strconv.ParseUint(spawned.CreationTime, 10, 64)
	if spawned.PID == 0 || spawned.PID == l.owner.manager.PID || spawned.UserSID != l.owner.manager.UserSID || spawned.ImageSHA256 != l.spec.Worker.SHA256 || !positiveDecimal(spawned.CreationTime) || childTime < parentTime {
		return errors.New("spawn result must be an exact ordinary-user child incarnation")
	}
	l.child = spawned
	if e := l.spec.Request.Validate(now); e != nil {
		l.phase = "closing"
		l.revision++
		return e
	}
	if e := l.fresh(observed, now, spawned); e != nil {
		l.phase = "closing"
		l.revision++
		return e
	}
	l.phase = "child-bound"
	l.revision++
	return nil
}
func (l *Lease) validRule(a RuleAcknowledgement) bool {
	return !a.ReadAt.IsZero() && a.LeaseSHA == l.sha && a.ReviewedScopeSHA == l.spec.Request.Scope.Digest() && exactHex(a.NativeKeysSHA, 32) && exactHex(a.TypedSnapshotSHA, 32) && a.Sequence > 0 && a.Sequence <= lanfirewall.MaxSequence-2 && a.Outcome == "created" && a.TerminalSuccess && a.OperationClosed && a.ProtectedReceipt && a.AcknowledgementFlushed && a.RuleProperties == 39 && a.FilterFamilies == 7 && a.ReverseIdentities == 7
}

// CheckRetainedRule is read-only inspection of the exact acknowledged rule. It
// does not renew combined source/interface/worker evidence or permit ingress.
func (l *Lease) CheckRetainedRule(current RuleAcknowledgement, now time.Time) error {
	if l == nil || !l.validRule(l.rule) || !freshAt(current.ReadAt, now) {
		return errors.New("fresh exact acknowledged native rule required")
	}
	retained := l.rule
	retained.ReadAt = current.ReadAt
	if current != retained {
		return errors.New("retained native rule identity/properties changed")
	}
	return nil
}
func (l *Lease) AcknowledgeRule(revision uint64, a RuleAcknowledgement, observed Observation, now time.Time) error {
	if e := l.check(revision, "child-bound"); e != nil {
		return e
	}
	if !l.validRule(a) {
		l.phase = "blocked"
		l.revision++
		return errors.New("duplicate/unknown/unqualified native creation cannot grant ingress ownership")
	}
	l.rule = a
	if !freshAt(a.ReadAt, now) {
		l.phase = "closing"
		l.revision++
		return errors.New("native rule readback stale/future; retain ownership with ingress closed")
	}
	if e := l.spec.Request.Validate(now); e != nil {
		l.phase = "closing"
		l.revision++
		return e
	}
	if e := l.fresh(observed, now, l.child); e != nil {
		l.phase = "closing"
		l.revision++
		return e
	}
	l.phase = "rule-acknowledged"
	l.revision++
	return nil
}

// ReadyForIngress is a pure transition; no native bind permission is produced.
// The real adapter must independently enforce these facts and close on failure.
func (l *Lease) ReadyForIngress(revision uint64, observed Observation, now time.Time) error {
	if e := l.check(revision, "rule-acknowledged"); e != nil {
		return e
	}
	if !freshAt(l.rule.ReadAt, now) {
		l.phase = "closing"
		l.revision++
		return errors.New("native rule readback expired before ingress")
	}
	if e := l.spec.Request.Validate(now); e != nil {
		l.phase = "closing"
		l.revision++
		return e
	}
	if e := l.fresh(observed, now, l.child); e != nil {
		l.phase = "closing"
		l.revision++
		return e
	}
	l.lastRead = observed.ReadAt
	l.phase = "ready"
	l.revision++
	return nil
}
func (l *Lease) Observe(revision uint64, observed Observation, now time.Time) error {
	if e := l.check(revision, "ready"); e != nil {
		return e
	}
	if e := l.fresh(observed, now, l.child); e != nil {
		l.phase = "closing"
		l.revision++
		return e
	}
	l.lastRead = observed.ReadAt
	return nil
}

// ObserveRule requires fresh read-only state of the SAME acknowledged native
// rule. A changed/unknown rule closes eligibility; it never adopts new keys.
func (l *Lease) ObserveRule(revision uint64, current RuleAcknowledgement, observed Observation, now time.Time) error {
	if e := l.check(revision, "ready"); e != nil {
		return e
	}
	retained := l.rule
	retained.ReadAt = current.ReadAt
	if current != retained || !l.validRule(current) || !freshAt(current.ReadAt, now) {
		l.phase = "closing"
		l.revision++
		return errors.New("acknowledged native rule changed or could not be freshly observed")
	}
	if current.ReadAt.Before(observed.ReadAt) {
		observed.ReadAt = current.ReadAt
	}
	if e := l.Observe(revision, observed, now); e != nil {
		return e
	}
	l.rule = current
	return nil
}

// A late positive provider completion after cancellation grants cleanup
// ownership ONLY. The child/flows still require positive join before retirement.
// No freshness or source condition here can reactivate an already closing lease.
func (l *Lease) AcknowledgeRuleWhileClosing(revision uint64, current RuleAcknowledgement) error {
	if l == nil || l.revision != revision || (l.phase != "closing" && l.phase != "closed") {
		return errors.New("cleanup-only acknowledgement requires retained closing or closed child ownership")
	}
	if l.child.PID == 0 || !l.validRule(current) {
		return errors.New("late completion does not establish exact acknowledged ownership")
	}
	if l.validRule(l.rule) {
		retained := l.rule
		retained.ReadAt = current.ReadAt
		if retained != current {
			return errors.New("late completion changed retained native ownership")
		}
	}
	l.rule = current
	l.revision++
	return nil
}
func (l *Lease) AllowsPeer(peer netip.Addr, now time.Time) bool {
	if l == nil || l.phase != "ready" || !freshAt(l.lastRead, now) || !peer.IsValid() || !peer.Is4() {
		return false
	}
	for _, raw := range l.spec.Request.Scope.ClientNetworks {
		prefix, e := netip.ParsePrefix(raw)
		if e == nil && prefix.Contains(peer) {
			return true
		}
	}
	return false
}
func (l *Lease) BeginClose(revision uint64) error {
	if l == nil || l.revision != revision || l.phase == "closed" || l.phase == "retire-pending" || l.phase == "retired" {
		return errors.New("lease close revision/state differs")
	}
	l.phase = "closing"
	l.revision++
	return nil
}
func (l *Lease) AcknowledgeClosed(revision uint64, a CloseAcknowledgement) error {
	if e := l.check(revision, "closing"); e != nil {
		return e
	}
	if a.LeaseSHA != l.sha || a.Worker != l.child || !a.IngressClosed || !a.FlowsJoined || !a.ProcessExited || !a.NoUnknownChild {
		return errors.New("close request/timeout is not positive socket/join/process closure")
	}
	l.phase = "closed"
	l.revision++
	return nil
}
func (l *Lease) BeginRetirement(revision uint64, current RuleAcknowledgement, now time.Time) error {
	if e := l.check(revision, "closed"); e != nil {
		return e
	}
	retained := l.rule
	retained.ReadAt = current.ReadAt
	if !l.validRule(l.rule) || !freshAt(current.ReadAt, now) || current != retained {
		return errors.New("no exact acknowledged rule/state ownership; retain unknown or altered rule")
	}
	l.phase = "retire-pending"
	l.revision++
	return nil
}
func (l *Lease) AcknowledgeRemoved(revision uint64, a RemovalAcknowledgement) error {
	if e := l.check(revision, "retire-pending"); e != nil {
		return e
	}
	if a.LeaseSHA != l.sha || a.NativeKeysSHA != l.rule.NativeKeysSHA || a.Sequence != l.rule.Sequence+2 || !a.TerminalSuccess || !a.OperationClosed || !a.ObservedAbsent || !a.ProtectedReceipt || !a.AcknowledgementFlushed {
		return errors.New("rule retirement unknown/unacknowledged; retain pending receipt and tombstone")
	}
	l.phase = "retired"
	l.revision++
	return nil
}

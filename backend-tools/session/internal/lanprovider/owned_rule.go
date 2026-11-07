package lanprovider

// This adapter is intentionally unregistered. MI exclusive Create, fixed ALLOW
// enum semantics and native containment still require separate qualification.
// The authenticated elevated helper owns consent, worker/child identity and the
// protected journal; copied receipts or these Go structs do not authenticate it.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
)

var ErrOwnedRuleUnknown = errors.New("owned LAN rule outcome unknown; protected intent retained")

const OwnedRuleEventBudget = 256 << 10

func ownedElevationAllowed(value, size uint32, threadAbsent bool) bool {
	return value == 1 && size == 4 && threadAbsent
}

// NativeJournal must synchronously append AND flush to its independently
// authenticated protected native store. A nil error is the durability boundary.
// Implementations must not publish ProtectedBytes to UI, logs or ordinary IPC.
type NativeJournal interface{ FlushOwnedRuleEvent(OwnedRuleEvent) error }

type ownedDefinition struct {
	keys    observedRuleKeys
	rule    typedInstance
	filters [7]typedInstance
}
type ownedCompletion struct {
	dispatched, terminal, closed, returnedFullKeys bool
	status                                         uint32
	keys                                           observedRuleKeys
}
type ownedPolicy struct {
	spec     lanfirewall.WMIDisabledCreate
	receipt  lanfirewall.Receipt
	leaseSHA string
}
type ownedBackend interface {
	Create(context.Context, ownedPolicy, func() error) (ownedCompletion, error)
	Read(context.Context, observedRuleKeys) (ownedDefinition, error)
	Delete(context.Context, observedRuleKeys, func() error) error
	Absent(context.Context, string) (bool, error)
}

// OwnedRule is a live, non-serializable capability. No name/key/JSON constructor
// or adoption fallback exists. Failed/unknown removal permanently fences it.
type OwnedRule struct {
	state *ownedRuleState
}

func (OwnedRule) String() string   { return "OwnedRule(private native capability)" }
func (OwnedRule) GoString() string { return "OwnedRule(private native capability)" }

// Copies of the exported value share the same serial authority and unknown
// latch. Copying a value must never clone mutation permission or its mutex.
type ownedRuleState struct {
	mu               sync.Mutex
	policy           ownedPolicy
	baseline         ownedDefinition
	backend          ownedBackend
	ack              lanlease.RuleAcknowledgement
	pending, retired bool
	sequence         uint64
}

// OwnedRuleEvent is opaque. Only ProtectedBytes deliberately reveals the full
// typed native baseline to the protected journal. It grants no restore authority.
// Native NULL/empty array state is significant and can encode JSON null here.
// A future authenticated recovery parser must preserve those exact flags/nulls;
// the ordinary LAN IPC validator that forbids null is not this storage format.
type OwnedRuleEvent struct {
	phase      string
	sequence   uint64
	policy     ownedPolicy
	definition ownedDefinition
	at         time.Time
}

func (OwnedRuleEvent) String() string   { return "OwnedRuleEvent(protected native values omitted)" }
func (OwnedRuleEvent) GoString() string { return "OwnedRuleEvent(protected native values omitted)" }

func (e OwnedRuleEvent) Kind() string     { return e.phase }
func (e OwnedRuleEvent) LeaseSHA() string { return e.policy.leaseSHA }

type ownedWireValue struct {
	Kind, Flags, Scalar uint32
	Text                string
	Shorts              []uint16
	Texts               []string
	Timestamp           bool
	Parts               [7]uint32
	UTC                 int32
}
type ownedWireInstance struct {
	Class  string
	Values map[string]ownedWireValue
}
type ownedWireDefinition struct {
	Keys    [5]string
	Rule    ownedWireInstance
	Filters [7]ownedWireInstance
}

func ownedInstanceWire(t typedInstance) ownedWireInstance {
	out := ownedWireInstance{Class: t.class, Values: make(map[string]ownedWireValue, len(t.values))}
	for name, v := range t.values {
		out.Values[name] = ownedWireValue{v.kind, v.flags, v.scalar, v.text, v.shorts, v.texts, v.date.timestamp, v.date.parts, v.date.utc}
	}
	return out
}
func ownedKeysWire(k observedRuleKeys) [5]string {
	return [5]string{k.instanceID, k.creationClassName, k.policyRuleName, k.systemCreationClassName, k.systemName}
}
func ownedDefinitionWire(d ownedDefinition) ownedWireDefinition {
	var filters [7]ownedWireInstance
	for i := range filters {
		filters[i] = ownedInstanceWire(d.filters[i])
	}
	return ownedWireDefinition{ownedKeysWire(d.keys), ownedInstanceWire(d.rule), filters}
}
func ownedDigest(v any) string {
	data, _ := json.Marshal(v)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (e OwnedRuleEvent) ProtectedBytes() ([]byte, error) {
	if e.phase != "create-intent" && e.phase != "create-returned" && e.phase != "created" && e.phase != "remove-intent" && e.phase != "removed" {
		return nil, errors.New("invalid protected LAN event")
	}
	if e.sequence == 0 || e.at.IsZero() || e.policy.leaseSHA == "" {
		return nil, errors.New("incomplete protected LAN event")
	}
	data, err := json.Marshal(struct {
		Version  int
		Phase    string
		Sequence uint64
		At       time.Time
		LeaseSHA string
		Receipt  lanfirewall.Receipt
		Native   any
	}{1, e.phase, e.sequence, e.at, e.policy.leaseSHA, e.policy.receipt, ownedDefinitionWire(e.definition)})
	if err != nil || len(data) > OwnedRuleEventBudget {
		return nil, errors.New("protected LAN event exceeds bounded encoding")
	}
	return data, nil
}
func flushOwned(j NativeJournal, e OwnedRuleEvent) error {
	if j == nil {
		return errors.New("protected native LAN journal required")
	}
	if _, err := e.ProtectedBytes(); err != nil {
		return err
	}
	if err := j.FlushOwnedRuleEvent(e); err != nil {
		return errors.New("protected LAN event flush failed")
	}
	return nil
}
func ownedEvent(p ownedPolicy, d ownedDefinition, phase string, seq uint64) OwnedRuleEvent {
	return OwnedRuleEvent{phase, seq, p, d, time.Now().UTC()}
}
func completeOwnedKeys(k observedRuleKeys) bool {
	for _, s := range ownedKeysWire(k) {
		if s == "" || len(s) > 1024 || strings.ContainsRune(s, 0) {
			return false
		}
	}
	return true
}
func prepareOwnedPolicy(receipt lanfirewall.Receipt, binding lanfirewall.InterfaceBinding, leaseSHA string) (ownedPolicy, error) {
	raw, err := hex.DecodeString(leaseSHA)
	if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != leaseSHA || leaseSHA == strings.Repeat("0", 64) {
		return ownedPolicy{}, errors.New("canonical lease digest required")
	}
	spec, err := lanfirewall.PrepareWMIDisabledCreate(receipt, binding, time.Now().UTC())
	if err != nil {
		return ownedPolicy{}, err
	}
	if receipt.Sequence > lanfirewall.MaxSequence-4 {
		return ownedPolicy{}, errors.New("LAN protected event sequence lacks create headroom")
	}
	// Reuse validated fixed scope, never WBEM_CREATE_ONLY flags: this is MI.
	spec.Flags = 0
	spec.Properties.Enabled = 1
	receipt.Request.Scope.ClientNetworks = append([]string(nil), receipt.Request.Scope.ClientNetworks...)
	return ownedPolicy{spec, receipt, leaseSHA}, nil
}
func newOwnedAck(p ownedPolicy, d ownedDefinition, seq uint64) lanlease.RuleAcknowledgement {
	return lanlease.RuleAcknowledgement{ReadAt: time.Now().UTC(), LeaseSHA: p.leaseSHA, ReviewedScopeSHA: p.receipt.ScopeSHA256, NativeKeysSHA: ownedDigest(ownedKeysWire(d.keys)), TypedSnapshotSHA: ownedDigest(ownedDefinitionWire(d)), Sequence: seq, Outcome: "created", TerminalSuccess: true, OperationClosed: true, ProtectedReceipt: true, AcknowledgementFlushed: true, RuleProperties: 39, FilterFamilies: 7, ReverseIdentities: 7}
}

// CreateOwnedRule is for the authenticated one-shot native helper only. The
// helper must enforce external process containment: MI's synchronous calls and
// cancellation/cleanup cannot themselves promise a wall-clock retirement bound.
func CreateOwnedRule(ctx context.Context, receipt lanfirewall.Receipt, binding lanfirewall.InterfaceBinding, leaseSHA string, j NativeJournal) (*OwnedRule, error) {
	if ctx == nil {
		return nil, errors.New("bounded LAN operation context required")
	}
	if j == nil {
		return nil, errors.New("protected native LAN journal required")
	}
	p, err := prepareOwnedPolicy(receipt, binding, leaseSHA)
	if err != nil {
		return nil, err
	}
	backend, err := newOwnedNativeBackend(ctx)
	if err != nil {
		return nil, err
	}
	return createOwned(ctx, p, j, backend)
}
func createOwned(ctx context.Context, p ownedPolicy, j NativeJournal, b ownedBackend) (*OwnedRule, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	seq := p.receipt.Sequence
	flushed := false
	c, err := b.Create(ctx, p, func() error {
		if flushed {
			return errors.New("LAN create intent callback reused")
		}
		e := flushOwned(j, ownedEvent(p, ownedDefinition{}, "create-intent", seq))
		flushed = e == nil
		return e
	})
	if err != nil || !flushed || !c.dispatched || !c.terminal || !c.closed || !c.returnedFullKeys || c.status != 0 || !completeOwnedKeys(c.keys) || c.keys.instanceID != p.receipt.RuleName {
		if c.dispatched {
			return nil, ErrOwnedRuleUnknown
		}
		return nil, errors.Join(err, errors.New("LAN create not positively acknowledged"))
	}
	seq++
	// Preserve positively returned exact keys before subsequent native reads.
	// This is a recovery candidate only, never an adoption/ownership grant.
	if err = flushOwned(j, ownedEvent(p, ownedDefinition{keys: c.keys}, "create-returned", seq)); err != nil {
		return nil, ErrOwnedRuleUnknown
	}
	readAt := time.Now().UTC()
	d, err := b.Read(ctx, c.keys)
	if err != nil || validateOwnedDefinition(d, p) != nil || d.keys != c.keys {
		return nil, ErrOwnedRuleUnknown
	}
	seq++
	if err = flushOwned(j, ownedEvent(p, d, "created", seq)); err != nil {
		return nil, ErrOwnedRuleUnknown
	}
	ack := newOwnedAck(p, d, seq)
	ack.ReadAt = readAt
	return &OwnedRule{state: &ownedRuleState{policy: p, baseline: d, backend: b, ack: ack, sequence: seq}}, nil
}
func (owner *OwnedRule) Acknowledgement() (lanlease.RuleAcknowledgement, error) {
	if owner == nil || owner.state == nil {
		return lanlease.RuleAcknowledgement{}, errors.New("live LAN owner required")
	}
	o := owner.state
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.pending || o.retired || o.backend == nil {
		return lanlease.RuleAcknowledgement{}, ErrOwnedRuleUnknown
	}
	return o.ack, nil
}

// Refresh never mutates/adopts native state. A failed read gives no fresh ACK.
func RefreshOwnedRule(ctx context.Context, owner *OwnedRule, j NativeJournal) (lanlease.RuleAcknowledgement, error) {
	if ctx == nil {
		return lanlease.RuleAcknowledgement{}, errors.New("bounded LAN operation context required")
	}
	if owner == nil || owner.state == nil {
		return lanlease.RuleAcknowledgement{}, errors.New("live LAN owner required")
	}
	o := owner.state
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.pending || o.retired || o.backend == nil {
		return lanlease.RuleAcknowledgement{}, ErrOwnedRuleUnknown
	}
	readAt := time.Now().UTC()
	d, err := o.backend.Read(ctx, o.baseline.keys)
	if err != nil || !reflect.DeepEqual(d, o.baseline) {
		return lanlease.RuleAcknowledgement{}, errors.New("LAN typed state unavailable or changed")
	}
	// Observation does not mint/revise ownership or consume mutation sequence.
	// lanlease compares every retained ownership field except this timestamp.
	o.ack.ReadAt = readAt
	return o.ack, nil
}
func RetireOwnedRule(ctx context.Context, owner *OwnedRule, closure lanlease.CloseAcknowledgement, j NativeJournal) (lanlease.RemovalAcknowledgement, error) {
	var empty lanlease.RemovalAcknowledgement
	if ctx == nil {
		return empty, errors.New("bounded LAN operation context required")
	}
	if owner == nil || owner.state == nil {
		return empty, errors.New("live LAN owner required")
	}
	o := owner.state
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.pending || o.retired || o.backend == nil {
		return empty, ErrOwnedRuleUnknown
	}
	if o.sequence > lanfirewall.MaxSequence-2 {
		return empty, errors.New("LAN journal sequence exhausted")
	}
	if closure.LeaseSHA != o.policy.leaseSHA || !closure.IngressClosed || !closure.FlowsJoined || !closure.ProcessExited || !closure.NoUnknownChild || closure.Worker.PID == 0 || closure.Worker.CreationTime == "" || closure.Worker.ImageSHA256 != o.policy.receipt.Worker.SHA256 || closure.Worker.UserSID != o.policy.receipt.Request.UserSID {
		return empty, errors.New("positive exact-lease child and ingress closure required")
	}
	d, err := o.backend.Read(ctx, o.baseline.keys)
	if err != nil || !reflect.DeepEqual(d, o.baseline) {
		return empty, errors.New("LAN removal refuses changed or unavailable typed baseline")
	}
	seq := o.sequence + 1
	err = o.backend.Delete(ctx, o.baseline.keys, func() error {
		if o.pending {
			return ErrOwnedRuleUnknown
		}
		if e := flushOwned(j, ownedEvent(o.policy, d, "remove-intent", seq)); e != nil {
			return e
		}
		o.pending = true
		o.sequence = seq
		return nil
	})
	if err != nil {
		if o.pending {
			return empty, ErrOwnedRuleUnknown
		}
		return empty, err
	}
	if !o.pending {
		return empty, ErrOwnedRuleUnknown
	}
	absent, err := o.backend.Absent(ctx, o.policy.receipt.RuleName)
	if err != nil || !absent {
		return empty, ErrOwnedRuleUnknown
	}
	seq++
	if err = flushOwned(j, ownedEvent(o.policy, d, "removed", seq)); err != nil {
		return empty, ErrOwnedRuleUnknown
	}
	o.retired = true
	o.sequence = seq
	return lanlease.RemovalAcknowledgement{LeaseSHA: o.policy.leaseSHA, NativeKeysSHA: o.ack.NativeKeysSHA, Sequence: seq, TerminalSuccess: true, OperationClosed: true, ObservedAbsent: true, ProtectedReceipt: true, AcknowledgementFlushed: true}, nil
}

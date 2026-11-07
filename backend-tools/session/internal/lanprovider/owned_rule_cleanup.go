package lanprovider

import (
	"context"
	"errors"
	"orbwin.local/session/internal/lanlease"
	"reflect"
	"sync"
	"time"
)

var ErrCleanupPending = errors.New("protected LAN cleanup remains pending; no mutation replay or ingress authority")

// CleanupAcknowledgement is observation/correlation only. Its distinct type and
// outcome cannot satisfy lanlease's created-rule grant or make a worker Ready.
type CleanupAcknowledgement struct {
	ReadAt                                                               time.Time
	Outcome, LeaseSHA, ReviewedScopeSHA, NativeKeysSHA, TypedSnapshotSHA string
	Sequence                                                             uint64
	ChildExited, NamespaceAbsent                                         bool
}

// CleanupObservation is immutable correlation/closure evidence only. Original
// caller, publication and owner mapping remain unchanged across SDK restarts.
// JSON is not a constructor for any native capability or ingress permission.
type CleanupObservation struct {
	RuntimeInstance, ReviewID, ReviewSHA256, OperationID, LeaseSHA string
	Spec                                                           lanlease.Spec
	Phase                                                          string
	ReadAt                                                         time.Time
	ChildExited, NamespaceAbsent, RuleAbsent                       bool
	NativeSequence                                                 uint64
	NativeKeysSHA, TypedSnapshotSHA                                string
}

func (CleanupObservation) String() string   { return "CleanupObservation(protected correlation)" }
func (CleanupObservation) GoString() string { return "CleanupObservation(protected correlation)" }
func cleanupObservation(r ownedProtectedObservation, absent bool, at time.Time) CleanupObservation {
	spec := r.metadata.Prepared.Spec
	spec.Request.Scope.ClientNetworks = append([]string(nil), spec.Request.Scope.ClientNetworks...)
	return CleanupObservation{RuntimeInstance: r.metadata.RuntimeInstance, ReviewID: r.metadata.ReviewID, ReviewSHA256: r.metadata.ReviewSHA256, OperationID: r.metadata.OperationID, LeaseSHA: r.policy.leaseSHA, Spec: spec, Phase: r.journalPhase, ReadAt: at, ChildExited: true, NamespaceAbsent: true, RuleAbsent: absent, NativeSequence: r.journalSequence, NativeKeysSHA: ownedDigest(ownedKeysWire(r.definition.keys)), TypedSnapshotSHA: ownedDigest(ownedDefinitionWire(r.definition))}
}

type cleanupSource interface {
	Record(context.Context) (ownedProtectedObservation, error)
	Closure(context.Context, ownedProtectedMetadata) error
	Read(context.Context, observedRuleKeys) (ownedDefinition, error)
	Absent(context.Context, string) (bool, error)
	Delete(context.Context, observedRuleKeys, func() error) error
	Append(context.Context, OwnedRuleEvent) error
	Close() error
}

// CleanupRule has only deletion/absence-settlement capability. It cannot be
// converted into OwnedRule or a ready/created acknowledgement; copies share one
// admission latch. Native protected-file/peer constructors alone mint it.
type CleanupRule struct{ state *cleanupState }
type cleanupState struct {
	mu                       sync.Mutex
	source                   cleanupSource
	original                 ownedProtectedObservation
	pending, closed, removed bool
	closeErr                 error
	readAt                   time.Time
	ruleAbsent               bool
}

func (CleanupRule) String() string   { return "CleanupRule(protected cleanup-only capability)" }
func (CleanupRule) GoString() string { return "CleanupRule(protected cleanup-only capability)" }
func RestoreCleanupRule(ctx context.Context, installation, leaseSHA string, sdkPipe uintptr) (*CleanupRule, error) {
	if ctx == nil || ctx.Err() != nil || !ownedHex(installation, 16) || !ownedHex(leaseSHA, 32) || sdkPipe == 0 {
		return nil, ErrCleanupPending
	}
	source, err := openCleanupSource(ctx, installation, leaseSHA, sdkPipe)
	if err != nil {
		if source != nil {
			return &CleanupRule{state: &cleanupState{source: source, pending: true, closed: true, closeErr: err}}, err
		}
		return nil, err
	}
	cap, err := restoreCleanup(ctx, source)
	if err != nil {
		if source.Close() != nil {
			return &CleanupRule{state: &cleanupState{source: source, pending: true, closed: true, closeErr: ErrCleanupPending}}, ErrCleanupPending
		}
		return nil, err
	}
	return cap, nil
}
func restoreCleanup(ctx context.Context, source cleanupSource) (*CleanupRule, error) {
	if ctx == nil || source == nil {
		return nil, ErrCleanupPending
	}
	record, err := source.Record(ctx)
	if err != nil || source.Closure(ctx, record.metadata) != nil {
		return nil, ErrCleanupPending
	}
	absent, err := source.Absent(ctx, record.policy.receipt.RuleName)
	if err != nil {
		return nil, ErrCleanupPending
	}
	if !absent {
		if record.journalPhase != "created" {
			return nil, ErrCleanupPending
		}
		d, err := source.Read(ctx, record.definition.keys)
		if err != nil || !reflect.DeepEqual(d, record.definition) {
			return nil, ErrCleanupPending
		}
	}
	if ctx.Err() != nil {
		return nil, ErrCleanupPending
	}
	if source.Closure(ctx, record.metadata) != nil {
		return nil, ErrCleanupPending
	}
	return &CleanupRule{&cleanupState{source: source, original: record, readAt: time.Now().UTC(), ruleAbsent: absent}}, nil
}

// OriginalMapping binds a fresh removal/settlement review to the original
// protected record. It conveys no permission to reuse the prior owner's SDK.
func (c *CleanupRule) OriginalMapping() (CleanupObservation, error) {
	if c == nil || c.state == nil {
		return CleanupObservation{}, ErrCleanupPending
	}
	s := c.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.pending || s.removed {
		return CleanupObservation{}, ErrCleanupPending
	}
	return cleanupObservation(s.original, s.ruleAbsent, s.readAt), nil
}
func (c *CleanupRule) ReviewScope() (lanlease.Spec, error) {
	if c == nil || c.state == nil {
		return lanlease.Spec{}, ErrCleanupPending
	}
	s := c.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return lanlease.Spec{}, ErrCleanupPending
	}
	spec := s.original.metadata.Prepared.Spec
	spec.Request.Scope.ClientNetworks = append([]string(nil), spec.Request.Scope.ClientNetworks...)
	return spec, nil
}
func (c *CleanupRule) Acknowledgement() (CleanupAcknowledgement, error) {
	if c == nil || c.state == nil {
		return CleanupAcknowledgement{}, ErrCleanupPending
	}
	s := c.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.pending {
		return CleanupAcknowledgement{}, ErrCleanupPending
	}
	p := s.original.policy
	d := s.original.definition
	return CleanupAcknowledgement{ReadAt: s.readAt, Outcome: "cleanup-only", LeaseSHA: p.leaseSHA, ReviewedScopeSHA: p.receipt.ScopeSHA256, NativeKeysSHA: ownedDigest(ownedKeysWire(d.keys)), TypedSnapshotSHA: ownedDigest(ownedDefinitionWire(d)), Sequence: s.original.sequence, ChildExited: true, NamespaceAbsent: true}, nil
}
func (c *CleanupRule) Close() error {
	if c == nil || c.state == nil {
		return nil
	}
	s := c.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.closeErr = s.source.Close()
	}
	return s.closeErr
}
func sameCleanupRecord(a, b ownedProtectedObservation) bool {
	return reflect.DeepEqual(a.policy, b.policy) && reflect.DeepEqual(a.definition, b.definition) && reflect.DeepEqual(a.metadata, b.metadata)
}
func RetireCleanupRule(ctx context.Context, c *CleanupRule) (lanlease.RemovalAcknowledgement, error) {
	return retireCleanup(ctx, c, false)
}

// SettleCleanupRule is a separate attended recovery action. It can only seal
// prior remove-intent after fresh absence, and never calls native Delete.
func SettleCleanupRule(ctx context.Context, c *CleanupRule) (lanlease.RemovalAcknowledgement, error) {
	return retireCleanup(ctx, c, true)
}
func retireCleanup(ctx context.Context, c *CleanupRule, settleOnly bool) (lanlease.RemovalAcknowledgement, error) {
	var zero lanlease.RemovalAcknowledgement
	if ctx == nil || ctx.Err() != nil || c == nil || c.state == nil {
		return zero, ErrCleanupPending
	}
	s := c.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.pending || s.removed {
		return zero, ErrCleanupPending
	}
	record, err := s.source.Record(ctx)
	if err != nil || !sameCleanupRecord(record, s.original) || s.source.Closure(ctx, record.metadata) != nil {
		return zero, ErrCleanupPending
	}
	if settleOnly {
		if record.journalPhase != "remove-intent" && record.journalPhase != "removed" {
			return zero, ErrCleanupPending
		}
	} else if record.journalPhase != "created" {
		return zero, ErrCleanupPending
	}
	absent, err := s.source.Absent(ctx, record.policy.receipt.RuleName)
	if err != nil {
		return zero, ErrCleanupPending
	}
	if !absent && record.journalPhase != "created" {
		return zero, ErrCleanupPending
	} // No replay of admitted/unknown Delete.
	appendIntent := func() error {
		if s.pending || s.source.Closure(ctx, record.metadata) != nil {
			return ErrCleanupPending
		}
		now, err := s.source.Record(ctx)
		if err != nil || !sameCleanupRecord(now, record) || now.journalPhase != "created" {
			return ErrCleanupPending
		}
		if !absent {
			current, err := s.source.Read(ctx, record.definition.keys)
			if err != nil || !reflect.DeepEqual(current, record.definition) {
				return ErrCleanupPending
			}
		}
		if err = s.source.Append(ctx, ownedEvent(record.policy, record.definition, "remove-intent", 4)); err != nil {
			s.pending = true
			return ErrCleanupPending
		}
		s.pending = true
		return nil
	}
	if record.journalPhase == "created" {
		if absent {
			if appendIntent() != nil {
				return zero, ErrCleanupPending
			}
		} else {
			d, err := s.source.Read(ctx, record.definition.keys)
			if err != nil || !reflect.DeepEqual(d, record.definition) {
				return zero, ErrCleanupPending
			}
			if err = s.source.Delete(ctx, record.definition.keys, appendIntent); err != nil {
				return zero, ErrCleanupPending
			}
			if !s.pending {
				return zero, ErrCleanupPending
			}
		}
	} else {
		s.pending = true
	}
	if s.source.Closure(ctx, record.metadata) != nil {
		return zero, ErrCleanupPending
	}
	absent, err = s.source.Absent(ctx, record.policy.receipt.RuleName)
	if err != nil || !absent {
		return zero, ErrCleanupPending
	}
	if record.journalPhase != "removed" {
		if s.source.Append(ctx, ownedEvent(record.policy, record.definition, "removed", 5)) != nil {
			return zero, ErrCleanupPending
		}
	}
	s.removed = true
	return lanlease.RemovalAcknowledgement{LeaseSHA: record.policy.leaseSHA, NativeKeysSHA: ownedDigest(ownedKeysWire(record.definition.keys)), Sequence: 5, TerminalSuccess: true, OperationClosed: true, ObservedAbsent: true, ProtectedReceipt: true, AcknowledgementFlushed: true}, nil
}

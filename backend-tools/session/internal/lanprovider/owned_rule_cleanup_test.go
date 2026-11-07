package lanprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
)

type cleanupResponseFixture struct {
	io.Reader
	closes int
}

func (b *cleanupResponseFixture) Close() error { b.closes++; return nil }
func TestOwnedCleanupDaemonResponseBoundIncludesUnreadTail(t *testing.T) {
	for _, size := range []int{12, cleanupDaemonResponseBudget, cleanupDaemonResponseBudget + 1} {
		var failed atomic.Bool
		b := &cleanupResponseFixture{Reader: bytes.NewReader(bytes.Repeat([]byte{'x'}, size))}
		r := &http.Response{Body: b, ContentLength: -1}
		cleanupResponseBound(&failed)(r)
		var first [1]byte
		if _, e := r.Body.Read(first[:]); e != nil {
			t.Fatal(e)
		}
		e := r.Body.Close()
		oversize := size > cleanupDaemonResponseBudget
		if failed.Load() != oversize || (e != nil) != oversize || b.closes != 1 {
			t.Fatal("response tail budget", size, e, failed.Load(), b.closes)
		}
	}
	var failed atomic.Bool
	b := &cleanupResponseFixture{Reader: bytes.NewReader(nil)}
	r := &http.Response{Body: b, ContentLength: cleanupDaemonResponseBudget + 1}
	cleanupResponseBound(&failed)(r)
	if !failed.Load() {
		t.Fatal("declared oversize ignored")
	}
	_ = r.Body.Close()
	if b.closes != 1 {
		t.Fatal("body not closed")
	}
}

func TestOwnedCleanupOriginalMappingPreservesActorAndOpaqueAuthority(t *testing.T) {
	f := cleanupFixture(t)
	c, e := restoreCleanup(context.Background(), f)
	if e != nil {
		t.Fatal(e)
	}
	obs, e := c.OriginalMapping()
	if e != nil || obs.RuntimeInstance != f.record.metadata.RuntimeInstance || obs.ReviewID != f.record.metadata.ReviewID || obs.OperationID != f.record.metadata.OperationID || obs.LeaseSHA != f.record.policy.leaseSHA || obs.Spec.Request.CallerPID != f.record.metadata.Prepared.Spec.Request.CallerPID || obs.Phase != "created" || obs.NativeSequence != 3 || !obs.ChildExited || !obs.NamespaceAbsent || obs.RuleAbsent {
		t.Fatal("original native mapping changed", e)
	}
	obs.Spec.Request.Scope.ClientNetworks[0] = "new"
	again, _ := c.OriginalMapping()
	if reflect.DeepEqual(obs.Spec, again.Spec) {
		t.Fatal("original mapping aliased")
	}
	if fmt.Sprintf("%#v", again) != "CleanupObservation(protected correlation)" {
		t.Fatal("native correlation logged")
	}
	data, e := json.Marshal(again)
	if e != nil {
		t.Fatal(e)
	}
	var decoded CleanupRule
	if e = json.Unmarshal(data, &decoded); e != nil {
		t.Fatal(e)
	}
	if _, e = RetireCleanupRule(context.Background(), &decoded); e == nil {
		t.Fatal("observation JSON constructed mutation capability")
	}
	if _, e = SettleCleanupRule(context.Background(), &decoded); e == nil {
		t.Fatal("observation JSON constructed settlement capability")
	}
	if c.Close() != nil {
		t.Fatal("close")
	}
	if _, e = c.OriginalMapping(); e == nil {
		t.Fatal("closed cap mapping admitted")
	}
}

type cleanupFake struct {
	record                                     ownedProtectedObservation
	absent                                     bool
	closureErr, deleteErr, appendErr, closeErr error
	changed                                    bool
	deletes, closes                            int
	phases                                     []string
}

func (f *cleanupFake) Record(context.Context) (ownedProtectedObservation, error) {
	return f.record, nil
}
func (f *cleanupFake) Closure(context.Context, ownedProtectedMetadata) error { return f.closureErr }
func (f *cleanupFake) Read(context.Context, observedRuleKeys) (ownedDefinition, error) {
	if f.changed {
		return ownedDefinition{}, ErrCleanupPending
	}
	return f.record.definition, nil
}
func (f *cleanupFake) Absent(context.Context, string) (bool, error) { return f.absent, nil }
func (f *cleanupFake) Delete(_ context.Context, _ observedRuleKeys, ready func() error) error {
	if err := ready(); err != nil {
		return err
	}
	f.deletes++
	if len(f.phases) != 1 || f.phases[0] != "remove-intent" {
		panic("Delete without protected intent")
	}
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.absent = true
	return nil
}
func (f *cleanupFake) Append(_ context.Context, e OwnedRuleEvent) error {
	f.phases = append(f.phases, e.Kind())
	if f.appendErr != nil {
		return f.appendErr
	}
	f.record.journalPhase = e.Kind()
	f.record.journalSequence = e.sequence
	return nil
}
func (f *cleanupFake) Close() error { f.closes++; return f.closeErr }
func cleanupFixture(t *testing.T) *cleanupFake {
	t.Helper()
	m, e, p, d := ownedProtectedFixture(t)
	r, err := parseOwnedProtectedMode(m, e, p.receipt.Request.Installation, p.leaseSHA, p.receipt.Request.UserSID, d, true)
	if err != nil {
		t.Fatal(err)
	}
	return &cleanupFake{record: r}
}
func TestOwnedCleanupOnlyAndExplicitDelete(t *testing.T) {
	f := cleanupFixture(t)
	c, e := restoreCleanup(context.Background(), f)
	if e != nil {
		t.Fatal(e)
	}
	if f.deletes != 0 || len(f.phases) != 0 {
		t.Fatal("restore mutated")
	}
	a, e := c.Acknowledgement()
	if e != nil || a.Outcome != "cleanup-only" {
		t.Fatal(a, e)
	}
	spec, e := c.ReviewScope()
	if e != nil {
		t.Fatal(e)
	}
	spec.Request.Scope.ClientNetworks[0] = "changed"
	again, _ := c.ReviewScope()
	if reflect.DeepEqual(spec, again) {
		t.Fatal("scope was aliased")
	}
	ack, e := RetireCleanupRule(context.Background(), c)
	if e != nil || !ack.ObservedAbsent || ack.Sequence != 5 || f.deletes != 1 || !reflect.DeepEqual(f.phases, []string{"remove-intent", "removed"}) {
		t.Fatal(ack, e, f.phases)
	}
	if _, e = RetireCleanupRule(context.Background(), c); e == nil || f.deletes != 1 {
		t.Fatal("retirement replay")
	}
}
func TestOwnedCleanupUnknownDeleteNoReplay(t *testing.T) {
	f := cleanupFixture(t)
	f.deleteErr = errors.New("unknown")
	c, e := restoreCleanup(context.Background(), f)
	if e != nil {
		t.Fatal(e)
	}
	copy := *c
	if _, e = RetireCleanupRule(context.Background(), c); e == nil {
		t.Fatal("unknown accepted")
	}
	if _, e = RetireCleanupRule(context.Background(), &copy); e == nil || f.deletes != 1 {
		t.Fatal("unknown replay")
	}
	if _, e = c.Acknowledgement(); e == nil {
		t.Fatal("unknown ACK")
	}
}
func TestOwnedCleanupClosureAndChangedBaselineRefuse(t *testing.T) {
	for _, kind := range []string{"closure", "baseline"} {
		t.Run(kind, func(t *testing.T) {
			f := cleanupFixture(t)
			if kind == "closure" {
				f.closureErr = ErrCleanupPending
			} else {
				f.changed = true
			}
			if _, e := restoreCleanup(context.Background(), f); e == nil || f.deletes != 0 {
				t.Fatal("unsafe restoration")
			}
		})
	}
}
func TestOwnedCleanupPendingAndSettledNeverReplay(t *testing.T) {
	for _, phase := range []string{"remove-intent", "removed"} {
		for _, absent := range []bool{false, true} {
			t.Run(phase+map[bool]string{true: "-absent", false: "-present"}[absent], func(t *testing.T) {
				f := cleanupFixture(t)
				f.record.journalPhase = phase
				f.record.journalSequence = 4
				if phase == "removed" {
					f.record.journalSequence = 5
				}
				f.absent = absent
				c, e := restoreCleanup(context.Background(), f)
				if !absent {
					if e == nil {
						t.Fatal("pending present accepted")
					}
					return
				}
				if e != nil {
					t.Fatal(e)
				}
				a, e := SettleCleanupRule(context.Background(), c)
				if e != nil || !a.ObservedAbsent || f.deletes != 0 {
					t.Fatal(a, e)
				}
				want := 0
				if phase == "remove-intent" {
					want = 1
				}
				if len(f.phases) != want {
					t.Fatal(f.phases)
				}
			})
		}
	}
}
func TestOwnedCleanupIntentFailureFencesCopies(t *testing.T) {
	f := cleanupFixture(t)
	c, e := restoreCleanup(context.Background(), f)
	if e != nil {
		t.Fatal(e)
	}
	f.appendErr = ErrCleanupPending
	if _, e = RetireCleanupRule(context.Background(), c); e == nil || f.deletes != 0 {
		t.Fatal("failed intent dispatched")
	}
	if _, e = RetireCleanupRule(context.Background(), c); e == nil || len(f.phases) != 1 {
		t.Fatal("intent retried")
	}
	f.closeErr = ErrCleanupPending
	if c.Close() == nil || c.Close() == nil || f.closes != 1 {
		t.Fatal("close unknown lost")
	}
}

func TestOwnedCleanupSettleCannotDeleteOrAdmitCreated(t *testing.T) {
	f := cleanupFixture(t)
	c, e := restoreCleanup(context.Background(), f)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = SettleCleanupRule(context.Background(), c); e == nil || f.deletes != 0 || len(f.phases) != 0 {
		t.Fatal("settle admitted created ownership")
	}
	f = cleanupFixture(t)
	f.record.journalPhase = "remove-intent"
	f.record.journalSequence = 4
	f.absent = true
	c, e = restoreCleanup(context.Background(), f)
	if e != nil {
		t.Fatal(e)
	}
	f.absent = false
	if _, e = SettleCleanupRule(context.Background(), c); e == nil || f.deletes != 0 || len(f.phases) != 0 {
		t.Fatal("settle dispatched or accepted present rule")
	}
	f.absent = true
	if _, e = RetireCleanupRule(context.Background(), c); e == nil || f.deletes != 0 || len(f.phases) != 0 {
		t.Fatal("initial retire accepted recovery phase")
	}
	a, e := SettleCleanupRule(context.Background(), c)
	if e != nil || !a.ObservedAbsent || f.deletes != 0 || !reflect.DeepEqual(f.phases, []string{"removed"}) {
		t.Fatal(a, e, f.phases)
	}
}

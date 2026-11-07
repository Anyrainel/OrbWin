package localnet

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
	"orbwin.local/session/internal/lanprovider"
)

type priorCleanupFake struct {
	*lanActorHelper
	original                       lanhelper.ProtectedReviewObservation
	proof                          lanprovider.CleanupObservation
	current                        lanfirewall.CallerIdentity
	persisted                      *LANProtectedReview
	readErr, cleanupErr            error
	cleanups                       int
	purpose                        string
	persistEntered, persistRelease chan struct{}
}

func (f *priorCleanupFake) RecoverReview(_ context.Context, id, sha string) (lanhelper.ProtectedReviewObservation, error) {
	if id == f.original.Record.ReviewID && sha == f.original.Record.ReviewSHA256 {
		return f.original, nil
	}
	if f.persisted != nil && id == f.persisted.ReviewID && sha == f.persisted.ReviewSHA256 {
		return lanhelper.ProtectedReviewObservation{Record: *f.persisted}, nil
	}
	return lanhelper.ProtectedReviewObservation{}, errors.New("missing protected exact review")
}
func (f *priorCleanupFake) ReadCleanup(context.Context, string) (lanprovider.CleanupObservation, error) {
	p := f.proof
	p.ReadAt = time.Now()
	p.Spec.Request.Scope.ClientNetworks = append([]string(nil), p.Spec.Request.Scope.ClientNetworks...)
	return p, f.readErr
}
func (f *priorCleanupFake) PersistReview(ctx context.Context, record LANProtectedReview) error {
	if err := record.Validate(f.current, time.Now()); err != nil {
		return err
	}
	if f.persistEntered != nil {
		close(f.persistEntered)
		select {
		case <-f.persistRelease:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	copy := record
	f.persisted = &copy
	return nil
}

func TestLANPriorCleanupReconcileKeepsCurrentAttendedOperationAuthoritative(t *testing.T) {
	r, h, ctx := priorCleanupFixture(t)
	recoverPriorCleanup(t, r, h, ctx)
	response := make(chan runtimeReply, 1)
	r.apply(runtimeCommand{kind: "lan-cleanup-review", lanReviewID: h.original.Record.ReviewID, lanReviewSHA: h.original.Record.ReviewSHA256, ctx: ctx, reply: response})
	drainLANWork(t, r)
	review := (<-response).lanReview
	h.persistEntered, h.persistRelease = make(chan struct{}), make(chan struct{})
	accepted := applyLANFixtureCommand(t, r, runtimeCommand{kind: "lan-cleanup", revision: 1, lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256})
	if accepted.err != nil {
		t.Fatal(accepted.err)
	}
	select {
	case <-h.persistEntered:
	case <-time.After(time.Second):
		t.Fatal("native consent did not start")
	}
	reply := r.apply(runtimeCommand{kind: "lan-reconcile-review", lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256, ctx: ctx, reply: make(chan runtimeReply, 1)})
	if reply.deferred || reply.lanReconciliation == nil || reply.lanReconciliation.Outcome != "operation" || reply.lanReconciliation.OperationID != accepted.lan.OperationID {
		t.Fatal("historical read replaced current attended owner")
	}
	operation := r.lanOperations[accepted.lan.OperationID]
	if operation.settled || operation.status.Phase != "awaiting-admin" || !operation.status.CanCancel {
		t.Fatal("current consent state overwritten by historical observation")
	}
	close(h.persistRelease)
	drainLANWork(t, r)
}
func (f *priorCleanupFake) CleanupPrior(_ context.Context, p lanhelper.Prepared, purpose string) (lanlease.RemovalAcknowledgement, error) {
	f.cleanups++
	f.purpose = purpose
	if !reflect.DeepEqual(p.Spec, f.proof.Spec) || p.LeaseSHA != f.proof.LeaseSHA {
		return lanlease.RemovalAcknowledgement{}, errors.New("original protected lease changed")
	}
	if f.cleanupErr != nil {
		return lanlease.RemovalAcknowledgement{}, f.cleanupErr
	}
	f.proof.Phase, f.proof.NativeSequence, f.proof.RuleAbsent = "removed", 5, true
	return lanlease.RemovalAcknowledgement{LeaseSHA: p.LeaseSHA, NativeKeysSHA: f.proof.NativeKeysSHA, Sequence: 5, TerminalSuccess: true, OperationClosed: true, ObservedAbsent: true, ProtectedReceipt: true, AcknowledgementFlushed: true}, nil
}

func priorCleanupFixture(t *testing.T) (*ServiceRuntime, *priorCleanupFake, context.Context) {
	t.Helper()
	f := makeLANActor(t)
	r := LANProtectedReview{RuntimeInstance: strings.Repeat("1", 32), ReviewID: strings.Repeat("2", 32), ReviewSHA256: f.stage.Base.ReviewSHA256, OperationID: strings.Repeat("3", 32), Transaction: f.stage.Base.Transaction, Purpose: "activate", Stage: f.stage}
	prepared := f.helper.prepared
	h := &priorCleanupFake{lanActorHelper: f.helper, original: lanhelper.ProtectedReviewObservation{Record: r, Prepared: &prepared}, proof: lanprovider.CleanupObservation{RuntimeInstance: r.RuntimeInstance, ReviewID: r.ReviewID, ReviewSHA256: r.ReviewSHA256, OperationID: r.OperationID, LeaseSHA: prepared.LeaseSHA, Spec: prepared.Spec, Phase: "created", ReadAt: time.Now(), ChildExited: true, NamespaceAbsent: true, NativeSequence: 3, NativeKeysSHA: f.helper.rule.NativeKeysSHA, TypedSnapshotSHA: f.helper.rule.TypedSnapshotSHA}}
	h.current = f.observation.Manager
	h.current.PID += 100
	h.current.CreationTime = "900"
	h.current.ImageSHA256 = strings.Repeat("8", 64)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	owner := &ServiceRuntime{ctx: ctx, opts: RuntimeOptions{LAN: &LANAdapters{Helper: h}}, lanReviews: newLANReviewOwner(), lanOperations: map[string]*lanOperation{}, lanEvents: make(chan lanOperationEvent, 16), lanJournal: newMemoryLANJournal(), settings: SettingsSnapshot{Revision: 1}, state: RuntimeSnapshot{Instance: strings.Repeat("4", 32)}}
	return owner, h, ctx
}
func drainLANWork(t *testing.T, r *ServiceRuntime) {
	t.Helper()
	for {
		r.lanWork.Wait()
		for len(r.lanEvents) > 0 {
			r.applyLANEvent(<-r.lanEvents)
		}
		if !r.lanJournalActive {
			return
		}
	}
}

func recoverPriorCleanup(t *testing.T, r *ServiceRuntime, h *priorCleanupFake, ctx context.Context) LANReconciliation {
	t.Helper()
	reply := make(chan runtimeReply, 1)
	accepted := r.apply(runtimeCommand{kind: "lan-reconcile-review", lanReviewID: h.original.Record.ReviewID, lanReviewSHA: h.original.Record.ReviewSHA256, ctx: ctx, reply: reply})
	if !accepted.deferred {
		t.Fatal("protected read did not leave actor")
	}
	drainLANWork(t, r)
	response := <-reply
	if response.err != nil || response.lanReconciliation == nil {
		t.Fatal("protected recovery failed", response.err)
	}
	return *response.lanReconciliation
}

func TestLANPriorOwnerCleanupPreservesOldStageAndUsesFreshCurrentPeerReview(t *testing.T) {
	for _, phase := range []string{"created", "remove-intent"} {
		t.Run(phase, func(t *testing.T) {
			r, h, ctx := priorCleanupFixture(t)
			h.proof.Phase = phase
			if phase == "remove-intent" {
				h.proof.NativeSequence, h.proof.RuleAbsent = 4, true
			}
			if got := recoverPriorCleanup(t, r, h, ctx); got.Outcome != "operation" {
				t.Fatal(got)
			}
			rows := r.lanOperationStatuses()
			if len(rows) != 1 || !rows[0].CanReviewCleanup || rows[0].Ingress != "closed" || rows[0].CanReviewRemoval || rows[0].CanSettleRemoval || rows[0].Phase == "ready" {
				t.Fatal("closure proof adopted live authority", rows)
			}
			if rows[0].ReviewSHA256 != h.original.Record.ReviewSHA256 || rows[0].RuntimeInstance != h.original.Record.RuntimeInstance {
				t.Fatal("snapshot lost protected original review digest/runtime")
			}
			response := make(chan runtimeReply, 1)
			accepted := r.apply(runtimeCommand{kind: "lan-cleanup-review", lanReviewID: h.original.Record.ReviewID, lanReviewSHA: h.original.Record.ReviewSHA256, ctx: ctx, reply: response})
			if !accepted.deferred {
				t.Fatal(accepted.err)
			}
			drainLANWork(t, r)
			reviewReply := <-response
			if reviewReply.err != nil {
				t.Fatal(reviewReply.err)
			}
			review := reviewReply.lanReview
			purpose := "cleanup-remove"
			if phase == "remove-intent" {
				purpose = "cleanup-settle"
			}
			if review.Purpose != purpose {
				t.Fatal("wrong prior cleanup capability", review.Purpose)
			}
			if bad := r.apply(runtimeCommand{kind: "lan-remove", revision: 1, lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256}); bad.err == nil {
				t.Fatal("generic remove accepted prior cleanup review")
			}
			accepted = applyLANFixtureCommand(t, r, runtimeCommand{kind: "lan-cleanup", revision: 1, lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256})
			if accepted.err != nil {
				t.Fatal(accepted.err)
			}
			drainLANWork(t, r)
			if h.persisted == nil || h.persisted.Cleanup == nil || h.persisted.RuntimeInstance != r.state.Instance || h.persisted.ReviewID == h.original.Record.ReviewID || h.persisted.OperationID == h.original.Record.OperationID || h.persisted.Transaction == h.original.Record.Transaction || !reflect.DeepEqual(h.persisted.Stage, h.original.Record.Stage) || h.persisted.Stage.Base.CallerPID == h.current.PID {
				t.Fatal("fresh cleanup rewrote or reused old native owner")
			}
			if h.cleanups != 1 || h.purpose != purpose || h.stages.Load() != 0 || h.commits.Load() != 0 || h.retires.Load() != 0 {
				t.Fatal("prior cleanup staged/created/replayed ordinary retirement")
			}
			got := recoverPriorCleanup(t, r, h, ctx)
			if got.Outcome != "retired" || got.RuntimeInstance != h.original.Record.RuntimeInstance || got.OperationID != h.original.Record.OperationID || got.LeaseSHA256 != h.proof.LeaseSHA || got.NativeSequence != 5 || got.ObservedAt == "" {
				t.Fatal("positive original retirement proof lost", got)
			}
			if replay := r.apply(runtimeCommand{kind: "lan-cleanup", revision: 1, lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256}); replay.err == nil || h.cleanups != 1 {
				t.Fatal("cleanup review replayed")
			}
		})
	}
}

func TestLANCrossOwnerRetirementRequiresFullOriginalNativeProof(t *testing.T) {
	for _, mutate := range []func(*priorCleanupFake){func(h *priorCleanupFake) { h.proof.RuleAbsent = false }, func(h *priorCleanupFake) { h.proof.ChildExited = false }, func(h *priorCleanupFake) { h.proof.NamespaceAbsent = false }, func(h *priorCleanupFake) { h.proof.NativeSequence = 4 }, func(h *priorCleanupFake) { h.proof.RuntimeInstance = strings.Repeat("5", 32) }, func(h *priorCleanupFake) { h.original.Record.RuntimeInstance = "" }, func(h *priorCleanupFake) { h.proof.ReviewSHA256 = strings.Repeat("6", 64) }, func(h *priorCleanupFake) { h.proof.OperationID = strings.Repeat("7", 32) }, func(h *priorCleanupFake) { h.readErr = errors.New("protected native read failed") }} {
		r, h, ctx := priorCleanupFixture(t)
		h.proof.Phase, h.proof.NativeSequence, h.proof.RuleAbsent = "removed", 5, true
		mutate(h)
		got := recoverPriorCleanup(t, r, h, ctx)
		if got.Outcome == "retired" {
			t.Fatal("missing/foreign/unknown proof cleared original owner", got)
		}
		for _, row := range r.lanOperationStatuses() {
			if row.CanReviewCleanup || row.Phase == "ready" || row.Phase == "removed" {
				t.Fatal("invalid proof granted cleanup authority", row)
			}
		}
	}
}

func TestLANCrossOwnerRetirementReturnsRequestedProtectedRemovalMappingWithoutSubstitution(t *testing.T) {
	r, h, ctx := priorCleanupFixture(t)
	metadata := h.proof
	// The pending original request is an earlier SDK's removal operation. Its
	// immutable protected mapping differs from the activation metadata mapping.
	h.original.Record.Purpose = "remove"
	h.original.Record.RuntimeInstance = strings.Repeat("6", 32)
	h.original.Record.ReviewID = strings.Repeat("7", 32)
	h.original.Record.ReviewSHA256 = strings.Repeat("8", 64)
	h.original.Record.OperationID = strings.Repeat("9", 32)
	h.original.Record.Stage.Base.ReviewSHA256 = h.original.Record.ReviewSHA256
	h.original.Record.LeaseSHA256 = metadata.LeaseSHA
	h.original.Prepared = nil
	h.proof.Phase, h.proof.NativeSequence, h.proof.RuleAbsent = "removed", 5, true
	got := recoverPriorCleanup(t, r, h, ctx)
	if got.Outcome != "retired" || got.RuntimeInstance != h.original.Record.RuntimeInstance || got.OperationID != h.original.Record.OperationID || got.ReviewID != h.original.Record.ReviewID || got.ReviewSHA256 != h.original.Record.ReviewSHA256 || got.OperationID == metadata.OperationID || got.RuntimeInstance == metadata.RuntimeInstance {
		t.Fatal("activation metadata substituted for requested protected pending removal", got)
	}
	// A current fresh cleanup record also reconciles using its own durable
	// mapping while retaining the rule's original activation mapping separately.
	r, h, ctx = priorCleanupFixture(t)
	now := time.Now()
	original := h.original.Record
	h.original.Record.Purpose = "cleanup-settle"
	h.original.Record.LeaseSHA256 = h.proof.LeaseSHA
	h.original.Record.RuntimeInstance = strings.Repeat("6", 32)
	h.original.Record.ReviewID = strings.Repeat("7", 32)
	h.original.Record.ReviewSHA256 = strings.Repeat("8", 64)
	h.original.Record.OperationID = strings.Repeat("9", 32)
	h.original.Record.Transaction = strings.Repeat("0", 32)
	h.original.Record.Cleanup = &lanhelper.CleanupReview{RuntimeInstance: original.RuntimeInstance, ReviewID: original.ReviewID, ReviewSHA256: original.ReviewSHA256, OperationID: original.OperationID, IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(3 * time.Minute).Format(time.RFC3339Nano)}
	h.original.Prepared = nil
	h.proof.Phase, h.proof.NativeSequence, h.proof.RuleAbsent = "removed", 5, true
	got = recoverPriorCleanup(t, r, h, ctx)
	if got.Outcome != "retired" || got.OperationID != h.original.Record.OperationID || got.RuntimeInstance != h.original.Record.RuntimeInstance {
		t.Fatal("fresh cleanup's durable operation mapping lost", got)
	}
}

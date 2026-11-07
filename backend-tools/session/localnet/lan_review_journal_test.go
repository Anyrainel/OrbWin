package localnet

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func durableReviewFixture(t *testing.T) (ledgerLANReviewJournal, *stateOriginFake, storedLANReview, context.Context) {
	t.Helper()
	f := makeLANActor(t)
	ledger, state := stateOriginFixture(t)
	global := "00000000-0000-4000-8000-000000000001"
	binding, err := deriveLANProductBinding(ledger.daemon, global, f.stage.Base.UserSID)
	if err != nil {
		t.Fatal(err)
	}
	stage := f.stage
	stage.Base.Installation = binding.Namespace
	owner := newLANReviewOwner()
	review, err := owner.issue(fixtureLANSelection(f), 1, stage, "activate", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	stored, err := snapshotLANReview(owner.records[review.ReviewID], global, ledger.daemon, strings.Repeat("1", 32))
	if err != nil || stored.validate(global, ledger.daemon) != nil {
		t.Fatal("native-scoped issuance fixture invalid", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	return ledgerLANReviewJournal{ledger: ledger, installation: global}, state, stored, ctx
}

func TestLANDurableIssuedReviewCanOnlyBePositivelyRevokedFromExactNativeReceipt(t *testing.T) {
	journal, state, issued, ctx := durableReviewFixture(t)
	unknown, err := journal.Reconcile(ctx, issued.Review.ReviewID, issued.Review.ReviewSHA256, strings.Repeat("2", 32))
	if err != nil || unknown.Outcome != "unknown" {
		t.Fatal("absence revoked an undispatched review", unknown, err)
	}
	if err = journal.Issue(ctx, issued); err != nil {
		t.Fatal(err)
	}
	if _, err = journal.Reconcile(ctx, issued.Review.ReviewID, issued.Review.ReviewSHA256, issued.RuntimeInstance); err == nil {
		t.Fatal("current actor issuance was cross-owner revoked")
	}
	got, err := journal.Reconcile(ctx, issued.Review.ReviewID, issued.Review.ReviewSHA256, strings.Repeat("2", 32))
	if err != nil || got.Outcome != "review-revoked" || got.RuntimeInstance != issued.RuntimeInstance || got.ManagedSequence != 2 || got.OperationID != "" || got.LeaseSHA256 != "" || got.ObservedAt == "" {
		t.Fatal("positive native issuance revocation lost", got, err)
	}
	var record storedLANReview
	if json.Unmarshal(state.records["lan-review-"+issued.Review.ReviewID+".json"], &record) != nil || record.State != "revoked" || record.Sequence != 2 {
		t.Fatal("revocation returned before durable record")
	}
	if journal.Consume(ctx, issued, strings.Repeat("3", 32)) == nil {
		t.Fatal("revoked issuance consumed")
	}
}
func TestLANDurableConsumedReviewNeverRevokesOrReplaysWithoutNativeCompletion(t *testing.T) {
	journal, _, issued, ctx := durableReviewFixture(t)
	if journal.Issue(ctx, issued) != nil {
		t.Fatal("issue")
	}
	op := strings.Repeat("3", 32)
	if err := journal.Consume(ctx, issued, op); err != nil {
		t.Fatal(err)
	}
	got, err := journal.Reconcile(ctx, issued.Review.ReviewID, issued.Review.ReviewSHA256, strings.Repeat("2", 32))
	if err != nil || got.Outcome != "operation" || got.OperationID != op || got.ManagedSequence != 0 {
		t.Fatal("consumed record falsely revoked", got, err)
	}
	if journal.Consume(ctx, issued, op) == nil {
		t.Fatal("consume replayed")
	}
}
func TestLANDurableReviewForgedPartialForeignAndUnknownOriginRemainUnknown(t *testing.T) {
	for _, kind := range []string{"partial", "digest", "owner", "runtime", "namespace", "read-origin", "replace-unknown", "close-unknown"} {
		t.Run(kind, func(t *testing.T) {
			journal, state, issued, ctx := durableReviewFixture(t)
			if journal.Issue(ctx, issued) != nil {
				t.Fatal("issue")
			}
			name := "lan-review-" + issued.Review.ReviewID + ".json"
			switch kind {
			case "partial":
				state.records[name] = state.records[name][:20]
			case "digest":
				var r storedLANReview
				_ = json.Unmarshal(state.records[name], &r)
				r.Stage.Base.Scope.LocalPort++
				state.records[name], _ = json.Marshal(r)
			case "owner":
				var r storedLANReview
				_ = json.Unmarshal(state.records[name], &r)
				r.Installation = strings.Repeat("f", 32)
				state.records[name], _ = json.Marshal(r)
			case "runtime":
				var r storedLANReview
				_ = json.Unmarshal(state.records[name], &r)
				r.RuntimeInstance = ""
				state.records[name], _ = json.Marshal(r)
			case "namespace":
				var r storedLANReview
				_ = json.Unmarshal(state.records[name], &r)
				r.Stage.Base.Installation = strings.Repeat("e", 32)
				state.records[name], _ = json.Marshal(r)
			case "read-origin":
				state.readErr = errors.New("native receipt provenance unavailable")
			case "replace-unknown":
				state.writeErr = errors.New("revocation admission unknown")
			case "close-unknown":
				state.finishErr = errors.New("native close unconfirmed")
			}
			got, err := journal.Reconcile(ctx, issued.Review.ReviewID, issued.Review.ReviewSHA256, strings.Repeat("2", 32))
			if err == nil || got.Outcome != "unknown" && kind != "close-unknown" {
				t.Fatal("unproven receipt revoked review", got, err)
			}
		})
	}
}

type memoryLANJournal struct {
	mu                                                         sync.Mutex
	records                                                    map[string]storedLANReview
	issueEntered, issueRelease, consumeEntered, consumeRelease chan struct{}
	issueErr, consumeErr                                       error
	issued, consumed                                           int
}

func newMemoryLANJournal() *memoryLANJournal {
	return &memoryLANJournal{records: map[string]storedLANReview{}}
}
func (j *memoryLANJournal) Issue(ctx context.Context, r storedLANReview) error {
	if j.issueEntered != nil {
		close(j.issueEntered)
		select {
		case <-j.issueRelease:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.issueErr != nil {
		return j.issueErr
	}
	if _, ok := j.records[r.Review.ReviewID]; ok {
		return errors.New("exists")
	}
	j.records[r.Review.ReviewID] = r
	j.issued++
	return nil
}
func (j *memoryLANJournal) Consume(ctx context.Context, r storedLANReview, op string) error {
	if j.consumeEntered != nil {
		close(j.consumeEntered)
		select {
		case <-j.consumeRelease:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.consumeErr != nil {
		return j.consumeErr
	}
	current, ok := j.records[r.Review.ReviewID]
	if !ok || current.State != "issued" || current.Review.ReviewSHA256 != r.Review.ReviewSHA256 {
		return errors.New("not issued")
	}
	current.State, current.OperationID, current.Sequence = "consumed", op, 2
	j.records[r.Review.ReviewID] = current
	j.consumed++
	return nil
}
func (j *memoryLANJournal) Reconcile(_ context.Context, id, sha, current string) (LANReconciliation, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := LANReconciliation{ReviewID: id, ReviewSHA256: sha, Outcome: "unknown"}
	record, ok := j.records[id]
	if !ok || record.Review.ReviewSHA256 != sha {
		return out, nil
	}
	if record.State == "consumed" {
		out.Outcome, out.OperationID = "operation", record.OperationID
		return out, nil
	}
	if record.RuntimeInstance == current {
		return out, nil
	}
	record.State, record.Sequence = "revoked", 2
	j.records[id] = record
	out.Outcome, out.RuntimeInstance, out.ManagedSequence, out.ObservedAt = "review-revoked", record.RuntimeInstance, 2, time.Now().UTC().Format(time.RFC3339Nano)
	return out, nil
}

func seedLANReceiptFixture(t *testing.T, r *ServiceRuntime, id string) {
	t.Helper()
	journal, ok := r.lanJournal.(*memoryLANJournal)
	if !ok {
		journal = newMemoryLANJournal()
		r.lanJournal = journal
	}
	record := r.lanReviews.records[id]
	snapshot, err := snapshotLANReview(record, r.opts.Installation, record.stage.Publication.Daemon, r.state.Instance)
	if err != nil {
		t.Fatal(err)
	}
	journal.records[id] = snapshot
	record.receipt = &snapshot
	record.durable = true
}
func applyLANFixtureCommand(t *testing.T, r *ServiceRuntime, command runtimeCommand) runtimeReply {
	t.Helper()
	if command.ctx == nil {
		command.ctx = r.ctx
	}
	command.reply = make(chan runtimeReply, 1)
	reply := r.apply(command)
	if !reply.deferred {
		return reply
	}
	deadline := time.After(time.Second)
	for {
		select {
		case event := <-r.lanEvents:
			r.applyLANEvent(event)
		case reply := <-command.reply:
			return reply
		case <-deadline:
			t.Fatal("fixture admission did not complete")
			return runtimeReply{}
		}
	}
}

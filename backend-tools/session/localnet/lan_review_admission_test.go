package localnet

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func admissionActorFixture(t *testing.T) (*ServiceRuntime, *lanActorFixture, *pendingReviewStore, *memoryLANJournal, context.CancelFunc) {
	t.Helper()
	f := makeLANActor(t)
	store := &pendingReviewStore{lanActorHelper: f.helper, entered: make(chan LANProtectedReview, 1)}
	f.controller.adapters.Helper = store
	ctx, cancel := context.WithCancel(context.Background())
	journal := newMemoryLANJournal()
	r := &ServiceRuntime{ctx: ctx, opts: RuntimeOptions{LAN: f.controller.adapters}, generation: &serviceGeneration{lan: f.controller}, lanReviews: newLANReviewOwner(), lanOperations: map[string]*lanOperation{}, lanEvents: make(chan lanOperationEvent, 32), lanJournal: journal, settings: SettingsSnapshot{Revision: 1}, state: RuntimeSnapshot{Instance: strings.Repeat("1", 32)}}
	t.Cleanup(func() {
		cancel()
		r.lanWork.Wait()
		select {
		case <-f.worker.joined:
		default:
			close(f.worker.joined)
		}
		_ = f.controller.Close(context.Background())
	})
	return r, f, store, journal, cancel
}
func ownerReviewCommand(f *lanActorFixture) runtimeCommand {
	return runtimeCommand{kind: "lan-owner-review", revision: 1, lanReviewInput: &lanReviewRecord{review: LANReview{Selection: fixtureLANSelection(f)}, stage: f.stage}}
}

func TestLANReviewResponseWaitsForDurableIssuanceAndLateReplyCanBeInvalidated(t *testing.T) {
	r, f, store, journal, _ := admissionActorFixture(t)
	journal.issueEntered, journal.issueRelease = make(chan struct{}), make(chan struct{})
	command := ownerReviewCommand(f)
	command.ctx = r.ctx
	command.reply = make(chan runtimeReply, 1)
	if reply := r.apply(command); !reply.deferred {
		t.Fatal("native persistence ran synchronously", reply.err)
	}
	select {
	case <-journal.issueEntered:
	case <-time.After(time.Second):
		t.Fatal("issuance did not start")
	}
	select {
	case <-command.reply:
		t.Fatal("review published before native issuance")
	default:
	}
	var record *lanReviewRecord
	for _, candidate := range r.lanReviews.records {
		record = candidate
	}
	reply := r.apply(runtimeCommand{kind: "lan-reconcile-review", lanReviewID: record.review.ReviewID, lanReviewSHA: record.review.ReviewSHA256})
	if reply.lanReconciliation == nil || reply.lanReconciliation.Outcome != "revoked" {
		t.Fatal("actor unavailable during native issue")
	}
	close(journal.issueRelease)
	r.lanWork.Wait()
	r.applyLANEvent(<-r.lanEvents)
	if got := <-command.reply; got.err == nil || got.lanReview != nil {
		t.Fatal("late revoked issuance published")
	}
	if f.helper.stages.Load() != 0 || len(store.entered) != 0 {
		t.Fatal("issuance dispatched helper effects")
	}
}

func TestLANConsumeWaitsForDurableAdmissionBeforeAdminStopOrWorkerEffects(t *testing.T) {
	r, f, store, journal, cancel := admissionActorFixture(t)
	issued := applyLANFixtureCommand(t, r, ownerReviewCommand(f))
	if issued.err != nil || issued.lanReview == nil {
		t.Fatal(issued.err)
	}
	review := issued.lanReview
	journal.consumeEntered, journal.consumeRelease = make(chan struct{}), make(chan struct{})
	command := runtimeCommand{kind: "lan-activate", revision: 1, lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256, ctx: r.ctx, reply: make(chan runtimeReply, 1)}
	if reply := r.apply(command); !reply.deferred {
		t.Fatal("consume did not reserve async admission", reply.err)
	}
	select {
	case <-journal.consumeEntered:
	case <-time.After(time.Second):
		t.Fatal("consume did not start")
	}
	if len(store.entered) != 0 || f.base.acquires.Load() != 0 || f.worker.stops.Load() != 0 {
		t.Fatal("effects preceded durable consumed record")
	}
	unknown := r.apply(runtimeCommand{kind: "lan-reconcile-review", lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256, ctx: r.ctx, reply: make(chan runtimeReply, 1)})
	if unknown.lanReconciliation == nil || unknown.lanReconciliation.Outcome != "unknown" || unknown.lanReconciliation.OperationID != "" || len(r.lanOperationStatuses()) != 0 {
		t.Fatal("unconfirmed operation identity escaped pending admission")
	}
	close(journal.consumeRelease)
	deadline := time.After(time.Second)
	var accepted runtimeReply
	for accepted.lan.OperationID == "" {
		select {
		case event := <-r.lanEvents:
			r.applyLANEvent(event)
		case accepted = <-command.reply:
			if accepted.err != nil {
				t.Fatal(accepted.err)
			}
		case <-deadline:
			t.Fatal("durable consume did not continue")
		}
	}
	select {
	case mapping := <-store.entered:
		journal.mu.Lock()
		record := journal.records[review.ReviewID]
		journal.mu.Unlock()
		if record.State != "consumed" || record.OperationID != mapping.OperationID || mapping.OperationID != accepted.lan.OperationID {
			t.Fatal("helper preceded exact consumed receipt")
		}
	case <-time.After(time.Second):
		t.Fatal("admitted helper did not start")
	}
	cancel()
	r.lanWork.Wait()
}

func TestLANUnknownConsumePersistenceNeverDispatchesOrLeaksOperationIdentity(t *testing.T) {
	r, f, store, journal, _ := admissionActorFixture(t)
	issued := applyLANFixtureCommand(t, r, ownerReviewCommand(f))
	review := issued.lanReview
	journal.consumeErr = errors.New("private consume completion unknown")
	reply := applyLANFixtureCommand(t, r, runtimeCommand{kind: "lan-activate", revision: 1, lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256})
	if reply.err == nil || reply.lan.OperationID != "" || len(store.entered) != 0 || f.base.acquires.Load() != 0 {
		t.Fatal("unknown admission dispatched or leaked operation")
	}
	if got := r.lanReviews.reconcile(review.ReviewID, review.ReviewSHA256); got.Outcome != "unknown" || got.OperationID != "" {
		t.Fatal("failed durable admission exposed provisional opID")
	}
}

func TestLANSDKLossAroundConsumedBoundaryNeverInfersRevocationFromMissingHelperHistory(t *testing.T) {
	r, f, store, journal, cancel := admissionActorFixture(t)
	issued := applyLANFixtureCommand(t, r, ownerReviewCommand(f))
	review := issued.lanReview
	journal.consumeEntered, journal.consumeRelease = make(chan struct{}), make(chan struct{})
	command := runtimeCommand{kind: "lan-activate", revision: 1, lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256, ctx: r.ctx, reply: make(chan runtimeReply, 1)}
	if !r.apply(command).deferred {
		t.Fatal("admission unavailable")
	}
	<-journal.consumeEntered
	close(journal.consumeRelease)
	r.lanWork.Wait()
	// Model SDK death after the private consumed record but before the actor
	// dispatch event. No process/native execution occurs in this test.
	journal.mu.Lock()
	consumed := journal.records[review.ReviewID]
	journal.mu.Unlock()
	cancel()
	if consumed.State != "consumed" || consumed.OperationID == "" || len(store.entered) != 0 || f.helper.stages.Load() != 0 {
		t.Fatal("crash boundary was not before helper history")
	}
	ctx, done := context.WithCancel(context.Background())
	defer done()
	next := &ServiceRuntime{ctx: ctx, lanJournal: journal, lanReviews: newLANReviewOwner(), lanOperations: map[string]*lanOperation{}, lanEvents: make(chan lanOperationEvent, 8), state: RuntimeSnapshot{Instance: strings.Repeat("2", 32)}}
	reply := make(chan runtimeReply, 1)
	accepted := next.apply(runtimeCommand{kind: "lan-reconcile-review", lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256, ctx: ctx, reply: reply})
	if !accepted.deferred {
		t.Fatal("recovery did not read durable owner")
	}
	next.lanWork.Wait()
	next.applyLANEvent(<-next.lanEvents)
	got := (<-reply).lanReconciliation
	if got == nil || got.Outcome != "operation" || got.OperationID != consumed.OperationID || got.ManagedSequence != 0 || len(next.lanOperations) != 0 {
		t.Fatal("consumed admission was revoked/adopted without native completion", got)
	}
}

func TestLANIssuedReviewLossIsPositivelyRevokedByNewExclusiveOwner(t *testing.T) {
	r, f, _, journal, cancel := admissionActorFixture(t)
	issued := applyLANFixtureCommand(t, r, ownerReviewCommand(f))
	review := issued.lanReview
	cancel()
	ctx, done := context.WithCancel(context.Background())
	defer done()
	next := &ServiceRuntime{ctx: ctx, lanJournal: journal, lanReviews: newLANReviewOwner(), lanOperations: map[string]*lanOperation{}, lanEvents: make(chan lanOperationEvent, 8), state: RuntimeSnapshot{Instance: strings.Repeat("2", 32)}}
	reply := make(chan runtimeReply, 1)
	if !next.apply(runtimeCommand{kind: "lan-reconcile-review", lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256, ctx: ctx, reply: reply}).deferred {
		t.Fatal("durable revocation did not start")
	}
	next.lanWork.Wait()
	next.applyLANEvent(<-next.lanEvents)
	got := (<-reply).lanReconciliation
	if got == nil || got.Outcome != "review-revoked" || got.RuntimeInstance != r.state.Instance || got.ManagedSequence != 2 || got.OperationID != "" || got.LeaseSHA256 != "" {
		t.Fatal("exact issued revocation proof lost", got)
	}
}

func TestLANConsumedGenerationCancellationRetainsAndJoinsLateOwners(t *testing.T) {
	r, f, store, _, _ := admissionActorFixture(t)
	issued := applyLANFixtureCommand(t, r, ownerReviewCommand(f))
	if issued.err != nil {
		t.Fatal(issued.err)
	}
	r.generation = nil
	entered, release := make(chan struct{}), make(chan struct{})
	lan := &delayedGenerationOwner{joined: make(chan struct{})}
	host := &delayedGenerationOwner{joined: make(chan struct{})}
	backend := &delayedGenerationOwner{joined: make(chan struct{})}
	r.opts.lanGenerationBuilder = func(context.Context, RuntimeOptions, string, chan lanOperationEvent) (*serviceGeneration, error) {
		close(entered)
		<-release // Model a resource constructor completing after cancellation.
		return &serviceGeneration{shutdown: newGenerationShutdown(host, lan, backend)}, nil
	}
	command := runtimeCommand{kind: "lan-activate", revision: 1, lanReviewID: issued.lanReview.ReviewID, lanReviewSHA: issued.lanReview.ReviewSHA256, ctx: r.ctx, reply: make(chan runtimeReply, 1)}
	if !r.apply(command).deferred {
		t.Fatal("consume did not defer")
	}
	select {
	case event := <-r.lanEvents:
		r.applyLANEvent(event)
	case <-time.After(time.Second):
		t.Fatal("consume did not finish")
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("builder did not start")
	}
	record := r.lanReviews.records[issued.lanReview.ReviewID]
	if got := r.apply(runtimeCommand{kind: "lan-cancel", lanOperationID: record.operationID}); got.err != nil {
		t.Fatal(got.err)
	}
	close(release)
	select {
	case event := <-r.lanEvents:
		r.applyLANEvent(event)
	case <-time.After(time.Second):
		t.Fatal("late generation missing")
	}
	if got := <-command.reply; got.err == nil {
		t.Fatal("cancelled late generation was adopted")
	}
	joined := make(chan struct{})
	go func() { r.lanWork.Wait(); close(joined) }()
	select {
	case <-joined:
		t.Fatal("late generation released before LAN positive join")
	case <-time.After(20 * time.Millisecond):
	}
	close(lan.joined)
	select {
	case <-joined:
		t.Fatal("late generation released before host/backend joins")
	case <-time.After(20 * time.Millisecond):
	}
	close(host.joined)
	close(backend.joined)
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("late generation did not join")
	}
	if r.generation != nil || len(store.entered) != 0 || f.helper.stages.Load() != 0 || host.closes.Load() != 1 || lan.closes.Load() != 1 {
		t.Fatal("cancelled generation leaked or dispatched")
	}
}

func TestLANBufferedGenerationEventDoesNotReleaseUnreceivedOwnerOnSDKShutdown(t *testing.T) {
	r, f, _, _, cancel := admissionActorFixture(t)
	issued := applyLANFixtureCommand(t, r, ownerReviewCommand(f))
	r.generation = nil
	lan := &delayedGenerationOwner{joined: make(chan struct{})}
	host := &delayedGenerationOwner{joined: make(chan struct{})}
	backend := &delayedGenerationOwner{joined: make(chan struct{})}
	r.opts.lanGenerationBuilder = func(context.Context, RuntimeOptions, string, chan lanOperationEvent) (*serviceGeneration, error) {
		return &serviceGeneration{shutdown: newGenerationShutdown(host, lan, backend)}, nil
	}
	command := runtimeCommand{kind: "lan-activate", revision: 1, lanReviewID: issued.lanReview.ReviewID, lanReviewSHA: issued.lanReview.ReviewSHA256, ctx: r.ctx, reply: make(chan runtimeReply, 1)}
	if !r.apply(command).deferred {
		t.Fatal("consume did not defer")
	}
	select {
	case event := <-r.lanEvents:
		r.applyLANEvent(event)
	case <-time.After(time.Second):
		t.Fatal("consume missing")
	}
	// Receive the buffered generation without admitting it, modeling an actor
	// that exits before executing this continuation.
	select {
	case <-r.lanEvents:
	case <-time.After(time.Second):
		t.Fatal("generation missing")
	}
	cancel()
	joined := make(chan struct{})
	go func() { r.lanWork.Wait(); close(joined) }()
	select {
	case <-joined:
		t.Fatal("unreceived generation owner escaped shutdown join")
	case <-time.After(20 * time.Millisecond):
	}
	close(lan.joined)
	close(host.joined)
	close(backend.joined)
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("unreceived generation failed to join")
	}
	if r.generation != nil || lan.closes.Load() != 1 || host.closes.Load() != 1 {
		t.Fatal("late generation adopted or shutdown repeated")
	}
}

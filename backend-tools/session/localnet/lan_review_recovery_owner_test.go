package localnet

import (
	"context"
	"orbwin.local/session/internal/lanhelper"
	"strings"
	"testing"
	"time"
)

type protectedReviewReaderFake struct {
	*lanActorHelper
	entered, release chan struct{}
	observation      lanhelper.ProtectedReviewObservation
}

func (f *protectedReviewReaderFake) RecoverReview(context.Context, string, string) (lanhelper.ProtectedReviewObservation, error) {
	close(f.entered)
	<-f.release
	return f.observation, nil
}
func TestLANPriorOwnerRecoveryIsBoundedAndCreatesOnlyDenialCleanupState(t *testing.T) {
	f := makeLANActor(t)
	review, sha, operation := strings.Repeat("2", 32), f.stage.Base.ReviewSHA256, strings.Repeat("3", 32)
	record := lanhelper.OwnerReview{RuntimeInstance: strings.Repeat("4", 32), ReviewID: review, ReviewSHA256: sha, OperationID: operation, Transaction: f.stage.Base.Transaction, Purpose: "activate", Stage: f.stage}
	reader := &protectedReviewReaderFake{lanActorHelper: f.helper, entered: make(chan struct{}), release: make(chan struct{}), observation: lanhelper.ProtectedReviewObservation{Record: record, Prepared: &f.helper.prepared}}
	adapters := *f.controller.adapters
	adapters.Helper = reader
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner := &ServiceRuntime{ctx: ctx, opts: RuntimeOptions{LAN: &adapters}, lanReviews: newLANReviewOwner(), lanOperations: map[string]*lanOperation{}, lanEvents: make(chan lanOperationEvent, 16)}
	response := make(chan runtimeReply, 1)
	command := runtimeCommand{kind: "lan-reconcile-review", lanReviewID: review, lanReviewSHA: sha, ctx: ctx, reply: response}
	if reply := owner.apply(command); !reply.deferred {
		t.Fatal("native protected read blocked actor or did not defer")
	}
	<-reader.entered
	if reply := owner.apply(command); reply.deferred || reply.lanReconciliation.Outcome != "unknown" {
		t.Fatal("recovery observer queued or manufactured absence")
	}
	current, err := owner.lanReviews.issue(fixtureLANSelection(f), 1, f.stage, "activate", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if reply := owner.apply(runtimeCommand{kind: "lan-reconcile-review", lanReviewID: current.ReviewID, lanReviewSHA: current.ReviewSHA256}); reply.lanReconciliation.Outcome != "revoked" {
		t.Fatal("stalled historical read blocked current revocation")
	}
	close(reader.release)
	select {
	case event := <-owner.lanEvents:
		owner.applyLANEvent(event)
	case <-time.After(time.Second):
		t.Fatal("protected observation not delivered")
	}
	result := <-response
	if result.lanReconciliation.Outcome != "operation" || result.lanReconciliation.OperationID != operation {
		t.Fatal("positive protected correlation lost")
	}
	rows := owner.lanOperationStatuses()
	if len(rows) != 1 || rows[0].Phase != "cleanup-pending" || rows[0].Ingress != "unknown" || rows[0].CanCancel || rows[0].CanReviewRemoval || !rows[0].CanInspect {
		t.Fatal("historical mapping restored live authority", rows)
	}
	if _, err := owner.lanReviews.consume(review, sha, "activate", 0, time.Now()); err == nil {
		t.Fatal("historical protected mapping became replayable")
	}
	if reader.stages.Load() != 0 || reader.commits.Load() != 0 || reader.retires.Load() != 0 {
		t.Fatal("recovery dispatched native mutation")
	}
	owner.lanWork.Wait()
}

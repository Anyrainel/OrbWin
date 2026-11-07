package localnet

import (
	"context"
	"strings"
	"testing"
	"time"
)

func fixtureLANSelection(f *lanActorFixture) LANSelection {
	scope, p := f.stage.Base.Scope, f.stage.Publication
	return LANSelection{ContainerID: p.ContainerID, StartedAt: p.StartedAt, HostPort: p.HostPort, ContainerPort: p.ContainerPort, Protocol: p.Protocol, InterfaceGUID: scope.InterfaceGUID, InterfaceLUID: scope.InterfaceLUID, LocalAddress: scope.LocalAddress, Profile: scope.Profile, ClientNetworks: append([]string(nil), scope.ClientNetworks...)}
}
func TestLANReviewReconciliationFencesLateActivationAndNeverInfersAbsence(t *testing.T) {
	f := makeLANActor(t)
	owner := newLANReviewOwner()
	now := time.Now()
	review, err := owner.issue(fixtureLANSelection(f), 1, f.stage, "activate", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if owner.reconcile(review.ReviewID, strings.Repeat("0", 64)).Outcome != "unknown" {
		t.Fatal("wrong digest revealed record")
	}
	result := owner.reconcile(review.ReviewID, review.ReviewSHA256)
	if result.Outcome != "revoked" || result.OperationID != "" {
		t.Fatal(result)
	}
	if _, err = owner.consume(review.ReviewID, review.ReviewSHA256, "activate", 1, now); err == nil {
		t.Fatal("late activation bypassed atomic revocation")
	}
	if newLANReviewOwner().reconcile(review.ReviewID, review.ReviewSHA256).Outcome != "unknown" {
		t.Fatal("prior runtime absence claimed revocation")
	}
}
func TestLANReviewConsumeReservesExactBindingBeforeDispatchAndRetainsMapping(t *testing.T) {
	f := makeLANActor(t)
	owner := newLANReviewOwner()
	now := time.Now()
	first, _ := owner.issue(fixtureLANSelection(f), 1, f.stage, "activate", "", now)
	second, _ := owner.issue(fixtureLANSelection(f), 1, f.stage, "activate", "", now)
	operation, err := owner.consume(first.ReviewID, first.ReviewSHA256, "activate", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.consume(second.ReviewID, second.ReviewSHA256, "activate", 1, now); err == nil {
		t.Fatal("duplicate binding dispatched")
	}
	result := owner.reconcile(first.ReviewID, first.ReviewSHA256)
	if result.Outcome != "operation" || result.OperationID != operation.operationID {
		t.Fatal(result)
	}
	owner.releaseBinding(operation)
	if _, err = owner.consume(second.ReviewID, second.ReviewSHA256, "activate", 1, now); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.consume(first.ReviewID, first.ReviewSHA256, "activate", 1, now); err == nil {
		t.Fatal("completed binding release made original review replayable")
	}
}

type pendingReviewStore struct {
	*lanActorHelper
	entered chan LANProtectedReview
}

func (store *pendingReviewStore) PersistReview(ctx context.Context, record LANProtectedReview) error {
	store.entered <- record
	<-ctx.Done()
	return ctx.Err()
}
func TestLANAsyncActorRemainsAvailableDuringProtectedReviewConsentAndLostReply(t *testing.T) {
	f := makeLANActor(t)
	store := &pendingReviewStore{lanActorHelper: f.helper, entered: make(chan LANProtectedReview, 1)}
	f.controller.adapters.Helper = store
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &ServiceRuntime{ctx: ctx, opts: RuntimeOptions{LAN: f.controller.adapters}, generation: &serviceGeneration{lan: f.controller}, lanReviews: newLANReviewOwner(), lanOperations: map[string]*lanOperation{}, lanEvents: make(chan lanOperationEvent, 16), settings: SettingsSnapshot{Revision: 1}, state: RuntimeSnapshot{Instance: strings.Repeat("1", 32)}}
	review, err := runtime.lanReviews.issue(fixtureLANSelection(f), 1, f.stage, "activate", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	seedLANReceiptFixture(t, runtime, review.ReviewID)
	reply := applyLANFixtureCommand(t, runtime, runtimeCommand{kind: "lan-activate", revision: 1, lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256})
	if reply.err != nil || reply.lan.Phase != "awaiting-admin" || reply.lan.OperationID == "" {
		t.Fatal(reply.err, reply.lan)
	}
	mapping := <-store.entered
	if mapping.ReviewID != review.ReviewID || mapping.ReviewSHA256 != review.ReviewSHA256 || mapping.OperationID != reply.lan.OperationID || mapping.Transaction != f.stage.Base.Transaction {
		t.Fatal("protected mapping differs", mapping)
	}
	settled := runtime.apply(runtimeCommand{kind: "lan-reconcile-review", lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256})
	if settled.err != nil || settled.lanReconciliation.Outcome != "operation" || settled.lanReconciliation.OperationID != reply.lan.OperationID {
		t.Fatal("lost response did not map while consent stalled")
	}
	if f.helper.stages.Load() != 0 || f.helper.commits.Load() != 0 {
		t.Fatal("Stage dispatched before protected mapping completed")
	}
	if stopped := runtime.apply(runtimeCommand{kind: "lan-cancel", lanOperationID: reply.lan.OperationID}); stopped.err != nil {
		t.Fatal(stopped.err)
	}
	select {
	case event := <-runtime.lanEvents:
		runtime.applyLANEvent(event)
	case <-time.After(time.Second):
		t.Fatal("owned cancellation did not drain")
	}
	if f.helper.stages.Load() != 0 || runtime.lanOperations[reply.lan.OperationID].status.Ingress != "closed" {
		t.Fatal("cancelled persistence dispatched setup")
	}
}

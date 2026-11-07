package localnet

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLANRemovalJoinsChildAndDedicatedHelperBeforeAdministrativeReview(t *testing.T) {
	f := makeLANActor(t)
	http, err := f.controller.shared.Acquire(context.Background(), f.stage.Publication.ContainerID, f.stage.Publication.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	status, err := f.controller.start(context.Background(), f.stage)
	if err != nil {
		t.Fatal(err)
	}
	store := &pendingReviewStore{lanActorHelper: f.helper, entered: make(chan LANProtectedReview, 1)}
	f.controller.adapters.Helper = store
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &ServiceRuntime{ctx: ctx, opts: RuntimeOptions{LAN: f.controller.adapters}, generation: &serviceGeneration{lan: f.controller}, lanReviews: newLANReviewOwner(), lanOperations: map[string]*lanOperation{}, lanEvents: make(chan lanOperationEvent, 16), settings: SettingsSnapshot{Revision: 1}, state: RuntimeSnapshot{Instance: strings.Repeat("1", 32)}}
	now := time.Now()
	activation, _ := runtime.lanReviews.issue(fixtureLANSelection(f), 1, f.stage, "activate", "", now)
	original, err := runtime.lanReviews.consume(activation.ReviewID, activation.ReviewSHA256, "activate", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	runtime.lanOperations[original.operationID] = &lanOperation{record: original, controller: f.controller, status: status, settled: true}
	removal, err := runtime.lanReviews.issue(fixtureLANSelection(f), 1, f.stage, "remove", status.LeaseSHA, now)
	if err != nil {
		t.Fatal(err)
	}
	seedLANReceiptFixture(t, runtime, removal.ReviewID)
	journal := runtime.lanJournal.(*memoryLANJournal)
	journal.consumeEntered, journal.consumeRelease = make(chan struct{}), make(chan struct{})
	command := runtimeCommand{kind: "lan-remove", revision: 1, lanReviewID: removal.ReviewID, lanReviewSHA: removal.ReviewSHA256, ctx: ctx, reply: make(chan runtimeReply, 1)}
	if !runtime.apply(command).deferred {
		t.Fatal("removal admission did not defer")
	}
	select {
	case <-journal.consumeEntered:
	case <-time.After(time.Second):
		t.Fatal("removal admission missing")
	}
	if f.worker.stops.Load() != 0 || f.base.releases.Load() != 0 || len(store.entered) != 0 {
		t.Fatal("removal stopped ingress before durable admission")
	}
	close(journal.consumeRelease)
	select {
	case event := <-runtime.lanEvents:
		runtime.applyLANEvent(event)
	case <-time.After(time.Second):
		t.Fatal("removal admission completion missing")
	}
	reply := <-command.reply
	if reply.err != nil {
		t.Fatal(reply.err)
	}
	select {
	case <-store.entered:
		t.Fatal("administrative review preceded actual child join")
	case <-time.After(10 * time.Millisecond):
	}
	if f.base.releases.Load() != 0 {
		t.Fatal("namespace released while worker remained live")
	}
	close(f.worker.joined)
	select {
	case mapping := <-store.entered:
		if mapping.Purpose != "remove" || mapping.LeaseSHA256 != status.LeaseSHA || f.base.releases.Load() != 1 {
			t.Fatal("administrative review preceded dedicated namespace release")
		}
	case <-time.After(time.Second):
		t.Fatal("positive closure did not reach review")
	}
	if err := f.controller.shared.Renew(context.Background(), http); err != nil {
		t.Fatal("LAN removal stopped unrelated HTTP helper", err)
	}
	cancel()
	runtime.lanWork.Wait()
	if err := f.controller.shared.Release(context.Background(), http); err != nil {
		t.Fatal(err)
	}
	if err := f.controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

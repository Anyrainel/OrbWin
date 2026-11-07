package localnet

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
)

type removalSettlementFake struct {
	*lanActorHelper
	settlements int
	bad         bool
}

func (f *removalSettlementFake) SettleRemoval(context.Context, lanhelper.Prepared) (lanlease.RemovalAcknowledgement, error) {
	f.settlements++
	ack := lanlease.RemovalAcknowledgement{LeaseSHA: f.prepared.LeaseSHA, NativeKeysSHA: f.rule.NativeKeysSHA, Sequence: f.rule.Sequence + 2, TerminalSuccess: true, OperationClosed: true, ObservedAbsent: true, ProtectedReceipt: true, AcknowledgementFlushed: true}
	if f.bad {
		ack.AcknowledgementFlushed = false
	}
	return ack, nil
}
func (f *removalSettlementFake) PersistReview(context.Context, LANProtectedReview) error { return nil }

func TestLANSettlementIPCContainsOnlyExactReviewOrLeaseIdentity(t *testing.T) {
	valid := map[string]string{
		"lan-settle-remove-review": `{"version":1,"operation":"lan-settle-remove-review","lanLeaseSha256":"` + strings.Repeat("a", 64) + `"}`,
		"lan-settle-remove":        `{"version":1,"operation":"lan-settle-remove","revision":1,"lanReviewId":"` + strings.Repeat("b", 32) + `","lanReviewSha256":"` + strings.Repeat("c", 64) + `"}`,
	}
	for operation, data := range valid {
		if err := validateLANIPC([]byte(data), operation); err != nil {
			t.Fatal("closed settlement shape refused", err)
		}
		for _, field := range []string{`,"namespaceId":"other"`, `,"delete":true`, `,"absent":true`, `,"command":"remove"`} {
			if err := validateLANIPC([]byte(strings.TrimSuffix(data, "}")+field+"}"), operation); err == nil {
				t.Fatal("settlement accepted caller-supplied authority")
			}
		}
	}
}

func TestLANRemovalSettlementUsesDistinctCapabilityAndNeverRetriesRetire(t *testing.T) {
	f := makeLANActor(t)
	helper := &removalSettlementFake{lanActorHelper: f.helper}
	f.controller.adapters.Helper = helper
	status, err := f.controller.start(context.Background(), f.stage)
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	if f.controller.settleRemoval(short, status.LeaseSHA, nil) == nil || helper.settlements != 0 {
		t.Fatal("settlement admitted before closure/initial removal")
	}
	cancel()
	close(f.worker.joined)
	f.helper.retireErr = errors.New("remove outcome unknown")
	if f.controller.retire(context.Background(), status.LeaseSHA, nil) == nil {
		t.Fatal("lost removal reply appeared successful")
	}
	s := f.controller.sessionStatus(f.controller.findSession(status.LeaseSHA))
	if !s.CanSettleRemoval || s.CanReviewRemoval || s.Ingress != "closed" {
		t.Fatal("unknown removal did not expose distinct attended settlement", s)
	}
	if f.controller.retire(context.Background(), status.LeaseSHA, nil) == nil || f.helper.retires.Load() != 1 {
		t.Fatal("initial retirement replayed")
	}
	helper.bad = true
	if f.controller.settleRemoval(context.Background(), status.LeaseSHA, nil) == nil || helper.settlements != 1 {
		t.Fatal("unflushed settlement accepted")
	}
	helper.bad = false
	if err = f.controller.settleRemoval(context.Background(), status.LeaseSHA, nil); err != nil || helper.settlements != 2 || f.helper.retires.Load() != 1 {
		t.Fatal("absence-only settlement replayed Delete or failed", err)
	}
	if err = f.controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLANSettlementReviewConsumesFreshIdentityAndCannotReplayOriginalRemoval(t *testing.T) {
	f := makeLANActor(t)
	helper := &removalSettlementFake{lanActorHelper: f.helper}
	f.controller.adapters.Helper = helper
	status, err := f.controller.start(context.Background(), f.stage)
	if err != nil {
		t.Fatal(err)
	}
	close(f.worker.joined)
	f.helper.retireErr = errors.New("lost remove result")
	if f.controller.retire(context.Background(), status.LeaseSHA, nil) == nil {
		t.Fatal("expected unknown removal")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := &ServiceRuntime{ctx: ctx, opts: RuntimeOptions{LAN: f.controller.adapters}, generation: &serviceGeneration{lan: f.controller}, lanReviews: newLANReviewOwner(), lanOperations: map[string]*lanOperation{}, lanEvents: make(chan lanOperationEvent, 16), lanJournal: newMemoryLANJournal(), settings: SettingsSnapshot{Revision: 1}, state: RuntimeSnapshot{Instance: strings.Repeat("1", 32)}}
	now := time.Now()
	old, _ := runtime.lanReviews.issue(fixtureLANSelection(f), 1, f.stage, "remove", status.LeaseSHA, now)
	original, err := runtime.lanReviews.consume(old.ReviewID, old.ReviewSHA256, "remove", 1, now)
	if err != nil {
		t.Fatal(err)
	}
	runtime.lanOperations[original.operationID] = &lanOperation{record: original, controller: f.controller, status: f.controller.sessionStatus(f.controller.findSession(status.LeaseSHA)), settled: true}
	reviewReply := applyLANFixtureCommand(t, runtime, runtimeCommand{kind: "lan-settle-remove-review", lanLeaseSHA: status.LeaseSHA})
	if reviewReply.err != nil || reviewReply.lanReview.Purpose != "settle-remove" || reviewReply.lanReview.ReviewID == old.ReviewID {
		t.Fatal("settlement did not issue a distinct review", reviewReply.err)
	}
	review := reviewReply.lanReview
	if rejected := runtime.apply(runtimeCommand{kind: "lan-remove", revision: 1, lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256}); rejected.err == nil {
		t.Fatal("settlement review authorized initial removal")
	}
	reply := applyLANFixtureCommand(t, runtime, runtimeCommand{kind: "lan-settle-remove", revision: 1, lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256})
	if reply.err != nil || reply.lan.OperationID == original.operationID {
		t.Fatal("settlement reused original operation", reply.err)
	}
	runtime.lanWork.Wait()
	for len(runtime.lanEvents) > 0 {
		runtime.applyLANEvent(<-runtime.lanEvents)
	}
	if helper.settlements != 1 || f.helper.retires.Load() != 1 || runtime.lanOperations[reply.lan.OperationID].status.Firewall != "removed" {
		t.Fatal("settlement failed or initial removal replayed")
	}
	if replay := runtime.apply(runtimeCommand{kind: "lan-settle-remove", revision: 1, lanReviewID: review.ReviewID, lanReviewSHA: review.ReviewSHA256}); replay.err == nil {
		t.Fatal("settlement review replayed")
	}
	if replay := runtime.apply(runtimeCommand{kind: "lan-remove", revision: 1, lanReviewID: old.ReviewID, lanReviewSHA: old.ReviewSHA256}); replay.err == nil {
		t.Fatal("original removal review revived")
	}
	if err = f.controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

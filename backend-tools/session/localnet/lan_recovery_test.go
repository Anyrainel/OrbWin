package localnet

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
)

type acknowledgedRecoveryHelper struct {
	*lanActorHelper
	bad bool
}

func (h *acknowledgedRecoveryHelper) RecoverCreate(context.Context, lanhelper.Prepared) (lanlease.RuleAcknowledgement, error) {
	ack := h.rule
	if h.bad {
		ack.AcknowledgementFlushed = false
	}
	return ack, nil
}
func (h *acknowledgedRecoveryHelper) RecoverRemoval(context.Context, lanhelper.Prepared) (lanlease.RemovalAcknowledgement, error) {
	return lanlease.RemovalAcknowledgement{}, errors.New("no protected positive removal")
}
func TestLANRecoveryNeedsPositiveProtectedAcknowledgementAndCannotReopenIngress(t *testing.T) {
	f := makeLANActor(t)
	f.helper.commitErr = errors.New("lost protected acknowledgement")
	status, err := f.controller.start(context.Background(), f.stage)
	if err == nil {
		t.Fatal("unknown create appeared successful")
	}
	close(f.worker.joined)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = f.controller.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.controller.inspect(ctx, status.LeaseSHA); err == nil || f.helper.reads.Load() != 0 {
		t.Fatal("ordinary rule lookup adopted an unknown create")
	}
	recovery := &acknowledgedRecoveryHelper{lanActorHelper: f.helper, bad: true}
	f.controller.adapters.Helper = recovery
	if got, e := f.controller.inspect(ctx, status.LeaseSHA); e == nil || got.Firewall != "create-unknown" {
		t.Fatal("unflushed acknowledgement adopted", got, e)
	}
	recovery.bad = false
	got, err := f.controller.inspect(ctx, status.LeaseSHA)
	if err != nil || got.Firewall != "created" || got.Ingress != "closed" || got.Phase != "cleanup-pending" || !got.CanReviewRemoval {
		t.Fatal("positive recovery was not cleanup-only", got, err)
	}
	if err = f.controller.retire(ctx, status.LeaseSHA, nil); err != nil || f.helper.retires.Load() != 1 {
		t.Fatal("fresh explicit retirement after positive recovery failed", err)
	}
}
func TestLANHelperSettlementUsesExactAbsenceWithoutRetryingRelease(t *testing.T) {
	f := makeLANActor(t)
	f.base.releaseErr = errors.New("lost exact delete completion")
	var observations atomic.Int32
	f.controller.shared.settle = func(_ context.Context, h NamespaceHandle) error {
		observations.Add(1)
		if h.ID == "" {
			t.Fatal("missing exact helper identity")
		}
		return nil
	}
	status, err := f.controller.start(context.Background(), f.stage)
	if err != nil {
		t.Fatal(err)
	}
	close(f.worker.joined)
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if f.controller.Close(short) == nil {
		t.Fatal("uncertain reference released owner")
	}
	cancel()
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	got, err := f.controller.settleHelper(ctx, status.LeaseSHA)
	if err != nil || got.Ingress != "closed" || f.base.releases.Load() != 1 || observations.Load() != 1 || f.helper.retires.Load() != 0 {
		t.Fatal("settlement replayed release/admin or failed", got, err)
	}
	if err = f.controller.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.controller.settleHelper(ctx, status.LeaseSHA); err == nil || observations.Load() != 1 {
		t.Fatal("settled namespace dispatched again")
	}
}

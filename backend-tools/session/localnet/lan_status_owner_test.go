package localnet

import (
	"context"
	"errors"
	"net"
	"orbwin.local/session/internal/lanhelper"
	"strings"
	"testing"
	"time"
)

func runtimeForLANResource(f *lanActorFixture, resource *lanSession, status LANStatus) *ServiceRuntime {
	op, review := strings.Repeat("1", 32), strings.Repeat("2", 32)
	status.OperationID = op
	status.ReviewID = review
	status.Selection = fixtureLANSelection(f)
	status.Sequence = 10
	ctx, cancel := context.WithCancel(context.Background())
	record := &lanReviewRecord{review: LANReview{ReviewID: review, Selection: status.Selection}, operationID: op}
	return &ServiceRuntime{ctx: ctx, lanReviews: newLANReviewOwner(), lanOperations: map[string]*lanOperation{op: {record: record, controller: f.controller, resource: resource, status: status, cancel: cancel, settled: true}}}
}
func TestLANPublicStatusRetainsPositiveClosureAfterResourceCompaction(t *testing.T) {
	f := makeLANActor(t)
	f.controller.adapters.Launch = func(context.Context, lanhelper.Prepared, func(context.Context, lanhelper.Stream) (net.Conn, error)) (LANWorker, error) {
		return nil, errors.New("failed before any native child")
	}
	var resource *lanSession
	status, err := f.controller.startObserved(context.Background(), f.stage, nil, func(session *lanSession) { resource = session })
	if err == nil || resource == nil {
		t.Fatal("missing retained exact setup resource")
	}
	select {
	case <-resource.closed:
	case <-time.After(time.Second):
		t.Fatal("known empty child did not join")
	}
	if rows := f.controller.statuses(); len(rows) != 0 {
		t.Fatal("fully closed no-rule resource not compacted")
	}
	runtime := runtimeForLANResource(f, resource, status)
	rows := runtime.lanOperationStatuses()
	if len(rows) != 1 || rows[0].Phase != "closed" || rows[0].Ingress != "closed" || rows[0].Sequence <= 10 {
		t.Fatal("positive resource closure became snapshot absence", rows)
	}
	first := rows[0]
	second := runtime.lanOperationStatuses()[0]
	if first.Sequence != second.Sequence || first.UpdatedAt != second.UpdatedAt {
		t.Fatal("snapshot invented resource transition/freshness")
	}
}
func TestLANPublicReadyTimeAndSequenceAdvanceOnlyOnActualCompletedObservation(t *testing.T) {
	f := makeLANActor(t)
	var resource *lanSession
	status, err := f.controller.startObserved(context.Background(), f.stage, nil, func(session *lanSession) { resource = session })
	if err != nil {
		t.Fatal(err)
	}
	resource.mu.Lock()
	resource.observedAt = time.Now().Add(-2 * time.Second)
	resource.mu.Unlock()
	runtime := runtimeForLANResource(f, resource, status)
	first := runtime.lanOperationStatuses()[0]
	same := runtime.lanOperationStatuses()[0]
	if first.Phase != "ready" || first.UpdatedAt != same.UpdatedAt || first.Sequence != same.Sequence {
		t.Fatal("snapshot refreshed evidence", first, same)
	}
	if err = f.controller.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	updated := runtime.lanOperationStatuses()[0]
	if updated.Phase != "ready" || updated.Sequence <= same.Sequence || updated.UpdatedAt == same.UpdatedAt {
		t.Fatal("completed source/rule/worker observation not projected", same, updated)
	}
	close(f.worker.joined)
	if err = f.controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

package localnet

import (
	"context"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanprovider"
	"time"
)

type lanReviewRecovery interface {
	RecoverReview(context.Context, string, string) (lanhelper.ProtectedReviewObservation, error)
}
type lanReviewRecoveryEvent struct {
	requested LANReconciliation
	observed  lanhelper.ProtectedReviewObservation
	err       error
	reply     chan runtimeReply
	cleanup   *lanprovider.CleanupObservation
	durable   *LANReconciliation
}

// Actor-only admission; native IO runs independently and returns to this same
// actor. At most one recovery read is pending, with no queued native observers.
func (runtime *ServiceRuntime) recoverLANReview(command runtimeCommand, result LANReconciliation) runtimeReply {
	unavailable := runtimeReply{lanReconciliation: &result}
	if runtime.lanRecoveryActive || runtime.lanJournalActive || command.ctx == nil || command.reply == nil {
		return unavailable
	}
	var reader lanReviewRecovery
	if runtime.opts.LAN != nil {
		reader, _ = runtime.opts.LAN.Helper.(lanReviewRecovery)
	}
	journal, journalErr := runtime.reviewJournal()
	if reader == nil && journalErr != nil {
		return unavailable
	}
	runtime.lanRecoveryActive = true
	runtime.lanJournalActive = true
	currentRuntime := runtime.state.Instance
	runtime.lanWork.Add(1)
	go func() {
		defer runtime.lanWork.Done()
		ctx, cancel := context.WithTimeout(command.ctx, 5*time.Second)
		defer cancel()
		stop := context.AfterFunc(runtime.ctx, cancel)
		defer stop()
		var durable *LANReconciliation
		if journalErr == nil {
			if receipt, e := journal.Reconcile(ctx, result.ReviewID, result.ReviewSHA256, currentRuntime); e == nil && (receipt.Outcome == "review-revoked" || receipt.Outcome == "operation") {
				durable = &receipt
			}
		}
		if durable != nil && durable.Outcome == "review-revoked" {
			event := lanOperationEvent{reconciliation: &lanReviewRecoveryEvent{requested: result, reply: command.reply, durable: durable}}
			select {
			case runtime.lanEvents <- event:
			case <-runtime.ctx.Done():
			}
			return
		}
		var observed lanhelper.ProtectedReviewObservation
		var err error
		if reader != nil {
			observed, err = reader.RecoverReview(ctx, result.ReviewID, result.ReviewSHA256)
		} else {
			err = context.Canceled
		}
		var cleanup *lanprovider.CleanupObservation
		if err == nil {
			leaseSHA := observed.Record.LeaseSHA256
			if observed.Prepared != nil {
				leaseSHA = observed.Prepared.LeaseSHA
			}
			if native, ok := runtime.opts.LAN.Helper.(lanPriorCleanup); ok && fullID(leaseSHA) {
				if proof, e := native.ReadCleanup(ctx, leaseSHA); e == nil && matchesProtectedCleanup(observed, proof) {
					cleanup = &proof
				}
			}
		}
		event := lanOperationEvent{reconciliation: &lanReviewRecoveryEvent{requested: result, observed: observed, err: err, reply: command.reply, cleanup: cleanup, durable: durable}}
		select {
		case runtime.lanEvents <- event:
		case <-runtime.ctx.Done():
		}
	}()
	return runtimeReply{deferred: true}
}
func (runtime *ServiceRuntime) applyLANReviewRecovery(event lanReviewRecoveryEvent) {
	runtime.lanRecoveryActive = false
	runtime.lanJournalActive = false
	result := event.requested
	if proof := event.durable; proof != nil && proof.ReviewID == result.ReviewID && proof.ReviewSHA256 == result.ReviewSHA256 {
		if proof.Outcome == "review-revoked" && validLANID(proof.RuntimeInstance) && proof.RuntimeInstance != runtime.state.Instance && proof.ManagedSequence == 2 && proof.OperationID == "" && proof.LeaseSHA256 == "" {
			read, err := time.Parse(time.RFC3339Nano, proof.ObservedAt)
			if err == nil && !time.Now().Before(read) && time.Since(read) <= 5*time.Second {
				event.reply <- runtimeReply{lanReconciliation: proof}
				return
			}
		}
		if proof.Outcome == "operation" && validLANID(proof.OperationID) {
			result.Outcome, result.OperationID = "operation", proof.OperationID
		}
	}
	record := event.observed.Record
	if existing := runtime.lanOperations[record.OperationID]; existing != nil && existing.status.ReviewID != record.ReviewID {
		event.err = context.Canceled
	}
	if existing := runtime.lanReviews.records[record.ReviewID]; existing != nil && (existing.review.ReviewSHA256 != record.ReviewSHA256 || existing.operationID != record.OperationID) {
		event.err = context.Canceled
	}
	// A native read may add denial/correlation information only. Never replace
	// a current owner decision, restart ingress, or infer closure from absence.
	if event.err == nil && record.ReviewID == result.ReviewID && record.ReviewSHA256 == result.ReviewSHA256 && validLANID(record.OperationID) && validLANID(record.RuntimeInstance) && len(runtime.lanReviews.records) < maxLANReviewHistory && runtime.lanReviews.records[record.ReviewID] == nil {
		scope, pub := record.Stage.Base.Scope, record.Stage.Publication
		selection := LANSelection{ContainerID: pub.ContainerID, StartedAt: pub.StartedAt, HostPort: pub.HostPort, ContainerPort: pub.ContainerPort, Protocol: pub.Protocol, InterfaceGUID: scope.InterfaceGUID, InterfaceLUID: scope.InterfaceLUID, LocalAddress: scope.LocalAddress, Profile: scope.Profile, ClientNetworks: append([]string(nil), scope.ClientNetworks...)}
		retained := &lanReviewRecord{review: LANReview{ReviewID: record.ReviewID, ReviewSHA256: record.ReviewSHA256, ExpiresAt: record.Stage.Base.ExpiresAt, Selection: selection, PublicationGeneration: pub.Generation, Purpose: record.Purpose, Available: false, ReasonCode: "protected-recovery"}, stage: record.Stage, operationID: record.OperationID, revoked: true, leaseSHA: record.LeaseSHA256}
		runtime.lanReviews.records[record.ReviewID] = retained
		if _, exists := runtime.lanOperations[record.OperationID]; !exists && len(runtime.lanOperations) < 16 {
			status := LANStatus{OperationID: record.OperationID, ReviewID: record.ReviewID, ReviewSHA256: record.ReviewSHA256, RuntimeInstance: record.RuntimeInstance, Selection: selection, Phase: "cleanup-pending", Ingress: "unknown", Firewall: "create-unknown", ReasonCode: "protected-recovery", Sequence: 1, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
			if event.observed.Prepared != nil {
				status.LeaseSHA = event.observed.Prepared.LeaseSHA
				retained.leaseSHA = status.LeaseSHA
				status.CanInspect = true
			}
			if status.LeaseSHA == "" && fullID(record.LeaseSHA256) {
				status.LeaseSHA = record.LeaseSHA256
				status.CanInspect = true
			}
			observed := event.observed
			runtime.lanOperations[record.OperationID] = &lanOperation{record: retained, status: status, recovered: &observed, settled: true, cancel: func() {}}
		}
		runtime.publishLANOperations()
	}
	if event.err == nil && record.ReviewID == result.ReviewID && record.ReviewSHA256 == result.ReviewSHA256 && event.cleanup != nil && matchesProtectedCleanup(event.observed, *event.cleanup) {
		if operation := runtime.lanOperations[record.OperationID]; operation != nil && operation.record.review.ReviewID == record.ReviewID && operation.record.review.ReviewSHA256 == record.ReviewSHA256 && (operation.recovered != nil || operation.record.cleanup != nil) {
			original := event.observed
			operation.recovered = &original
			proof := *event.cleanup
			operation.cleanup = &proof
			operation.status = priorCleanupStatus(operation.status, event.observed, proof)
			runtime.publishLANOperations()
		}
		if retired, ok := retiredReconciliation(event.observed, *event.cleanup); ok {
			event.reply <- runtimeReply{lanReconciliation: &retired}
			return
		}
	}
	// Re-check actor state: another command may have changed a current review
	// while the observation ran. Never revoke a previously consumed review.
	if current := runtime.lanReviews.records[result.ReviewID]; current != nil && current.review.ReviewSHA256 == result.ReviewSHA256 && current.operationID != "" {
		result.Outcome = "operation"
		result.OperationID = current.operationID
	}
	event.reply <- runtimeReply{lanReconciliation: &result}
}

package localnet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
	"orbwin.local/session/internal/lanprovider"
)

type lanPriorCleanup interface {
	ReadCleanup(context.Context, string) (lanprovider.CleanupObservation, error)
	CleanupPrior(context.Context, lanhelper.Prepared, string) (lanlease.RemovalAcknowledgement, error)
}

// Correlation is checked against the separately held protected ORIGINAL review,
// never runtime IDs from a renderer or user-home recovery file. ReadCleanup is
// native observation only: no Ready, Stage or child adoption is possible here.
func matchesProtectedCleanup(original lanhelper.ProtectedReviewObservation, proof lanprovider.CleanupObservation) bool {
	r := original.Record
	issuedAt := r.Stage.Base.IssuedAt
	if r.Cleanup != nil {
		issuedAt = r.Cleanup.IssuedAt
	}
	issued, err := time.Parse(time.RFC3339Nano, issuedAt)
	identity := lanfirewall.CallerIdentity{PID: r.Stage.Base.CallerPID, CreationTime: r.Stage.Base.CallerCreationTime, UserSID: r.Stage.Base.UserSID, ImageSHA256: r.Stage.Base.CallerImageSHA256}
	if err != nil || r.Validate(identity, issued) != nil {
		return false
	}
	if !validLANID(r.RuntimeInstance) || !validLANID(r.ReviewID) || !fullID(r.ReviewSHA256) || !validLANID(r.OperationID) || !validLANID(proof.RuntimeInstance) || !validLANID(proof.ReviewID) || !fullID(proof.ReviewSHA256) || !validLANID(proof.OperationID) || !fullID(proof.LeaseSHA) || !fullID(proof.NativeKeysSHA) || !fullID(proof.TypedSnapshotSHA) {
		return false
	}
	if !proof.ChildExited || !proof.NamespaceAbsent || proof.ReadAt.IsZero() || time.Now().Before(proof.ReadAt) || time.Since(proof.ReadAt) > 5*time.Second {
		return false
	}
	if r.Stage.Base.Installation != proof.Spec.Request.Installation || r.Stage.Base.UserSID != proof.Spec.Request.UserSID || !reflect.DeepEqual(r.Stage.Base.Scope, proof.Spec.Request.Scope) || r.Stage.Publication != proof.Spec.Publication {
		return false
	}
	if r.Cleanup != nil {
		c := r.Cleanup
		if c.RuntimeInstance != proof.RuntimeInstance || c.ReviewID != proof.ReviewID || c.ReviewSHA256 != proof.ReviewSHA256 || c.OperationID != proof.OperationID || r.LeaseSHA256 != proof.LeaseSHA || !reflect.DeepEqual(r.Stage.Base, proof.Spec.Request) {
			return false
		}
	} else if r.Purpose == "activate" {
		if r.RuntimeInstance != proof.RuntimeInstance || r.ReviewID != proof.ReviewID || r.ReviewSHA256 != proof.ReviewSHA256 || r.OperationID != proof.OperationID || original.Prepared == nil || original.Prepared.LeaseSHA != proof.LeaseSHA || !reflect.DeepEqual(original.Prepared.Spec, proof.Spec) {
			return false
		}
	} else if (r.Purpose != "remove" && r.Purpose != "settle-remove") || r.LeaseSHA256 != proof.LeaseSHA {
		return false
	}
	return proof.Phase == "created" && proof.NativeSequence == 3 || proof.RuleAbsent && (proof.Phase == "remove-intent" && proof.NativeSequence == 4 || proof.Phase == "removed" && proof.NativeSequence == 5)
}

func retiredReconciliation(original lanhelper.ProtectedReviewObservation, proof lanprovider.CleanupObservation) (LANReconciliation, bool) {
	if !matchesProtectedCleanup(original, proof) || proof.Phase != "removed" || proof.NativeSequence != 5 || !proof.RuleAbsent {
		return LANReconciliation{}, false
	}
	r := original.Record
	return LANReconciliation{ReviewID: r.ReviewID, ReviewSHA256: r.ReviewSHA256, Outcome: "retired", OperationID: r.OperationID, RuntimeInstance: r.RuntimeInstance, LeaseSHA256: proof.LeaseSHA, NativeSequence: 5, ObservedAt: proof.ReadAt.UTC().Format(time.RFC3339Nano)}, true
}

func priorCleanupStatus(status LANStatus, original lanhelper.ProtectedReviewObservation, proof lanprovider.CleanupObservation) LANStatus {
	if !matchesProtectedCleanup(original, proof) {
		return status
	}
	status.LeaseSHA, status.Ingress = proof.LeaseSHA, "closed"
	status.ReviewID, status.ReviewSHA256, status.RuntimeInstance = original.Record.ReviewID, original.Record.ReviewSHA256, original.Record.RuntimeInstance
	status.CanCancel, status.CanReviewRemoval, status.CanSettleRemoval = false, false, false
	status.CanReviewCleanup, status.CanInspect = proof.Phase != "removed", proof.Phase != "removed"
	status.Phase, status.ReasonCode = "cleanup-pending", "protected-cleanup-ready"
	status.UpdatedAt = proof.ReadAt.UTC().Format(time.RFC3339Nano)
	switch proof.Phase {
	case "created":
		status.Firewall = "created"
	case "remove-intent":
		status.Firewall = "remove-unknown"
	case "removed":
		status.Firewall, status.Phase = "removed", "removed"
	}
	return status
}

func (owner *lanReviewOwner) issuePriorCleanup(proof lanprovider.CleanupObservation, revision uint64, now time.Time) (LANReview, error) {
	if len(owner.records) >= maxLANReviewHistory {
		return LANReview{}, errors.New("LAN denial history budget exhausted")
	}
	active := 0
	for _, record := range owner.records {
		expires, _ := time.Parse(time.RFC3339Nano, record.review.ExpiresAt)
		if record.operationID == "" && !record.revoked && now.Before(expires) {
			active++
		}
	}
	if active >= maxLANReviews {
		return LANReview{}, errors.New("LAN current review budget exhausted")
	}
	purpose := "cleanup-remove"
	if proof.Phase == "remove-intent" {
		purpose = "cleanup-settle"
	} else if proof.Phase != "created" {
		return LANReview{}, errors.New("protected removal is already complete or unconfirmed")
	}
	id, err := lanRandomID()
	if err != nil {
		return LANReview{}, err
	}
	transaction, err := lanRandomID()
	if err != nil {
		return LANReview{}, err
	}
	scope, pub := proof.Spec.Request.Scope, proof.Spec.Publication
	selection := LANSelection{ContainerID: pub.ContainerID, StartedAt: pub.StartedAt, HostPort: pub.HostPort, ContainerPort: pub.ContainerPort, Protocol: pub.Protocol, InterfaceGUID: scope.InterfaceGUID, InterfaceLUID: scope.InterfaceLUID, LocalAddress: scope.LocalAddress, Profile: scope.Profile, ClientNetworks: append([]string(nil), scope.ClientNetworks...)}
	review := LANReview{ReviewID: id, ExpiresAt: now.Add(time.Minute).UTC().Format(time.RFC3339Nano), Selection: selection, PublicationGeneration: pub.Generation, Purpose: purpose, Available: true}
	stage := lanhelper.StageRequest{Base: proof.Spec.Request, Publication: pub}
	stage.Base.Scope.ClientNetworks = append([]string(nil), scope.ClientNetworks...)
	cleanup := &lanhelper.CleanupReview{RuntimeInstance: proof.RuntimeInstance, ReviewID: proof.ReviewID, ReviewSHA256: proof.ReviewSHA256, OperationID: proof.OperationID, IssuedAt: now.UTC().Format(time.RFC3339Nano), ExpiresAt: now.Add(3 * time.Minute).UTC().Format(time.RFC3339Nano)}
	data, _ := json.Marshal(struct {
		Review                LANReview
		Revision              uint64
		LeaseSHA, Transaction string
		Stage                 lanhelper.StageRequest
		Cleanup               *lanhelper.CleanupReview
	}{review, revision, proof.LeaseSHA, transaction, stage, cleanup})
	digest := sha256.Sum256(append([]byte("orbwin-lan-prior-cleanup-review-v1\n"), data...))
	review.ReviewSHA256 = hex.EncodeToString(digest[:])
	spec := proof.Spec
	spec.Request.Scope.ClientNetworks = append([]string(nil), scope.ClientNetworks...)
	owner.records[id] = &lanReviewRecord{review: review, revision: revision, stage: stage, leaseSHA: proof.LeaseSHA, cleanup: cleanup, transaction: transaction, cleanupKeysSHA: proof.NativeKeysSHA, cleanupSpec: &spec}
	return review, nil
}

type lanCleanupReviewEvent struct {
	operationID string
	original    lanhelper.ProtectedReviewObservation
	proof       lanprovider.CleanupObservation
	err         error
	reply       chan runtimeReply
	revision    uint64
	ctx         context.Context
}

func (runtime *ServiceRuntime) reviewPriorCleanup(command runtimeCommand) runtimeReply {
	if runtime.settingsUncertain || runtime.rotationPending || command.ctx == nil || command.reply == nil || runtime.opts.LAN == nil {
		return runtimeReply{err: errors.New("current protected cleanup owner unavailable")}
	}
	var operation *lanOperation
	for _, candidate := range runtime.lanOperations {
		if candidate.recovered != nil && candidate.record.review.ReviewID == command.lanReviewID && candidate.record.review.ReviewSHA256 == command.lanReviewSHA {
			operation = candidate
			break
		}
	}
	reader, ok := runtime.opts.LAN.Helper.(lanPriorCleanup)
	if !ok || operation == nil || operation.reading || !operation.settled || operation.status.LeaseSHA == "" {
		return runtimeReply{err: errors.New("exact protected prior-owner cleanup unavailable")}
	}
	operation.reading = true
	original := *operation.recovered
	leaseSHA, operationID, revision := operation.status.LeaseSHA, operation.record.operationID, runtime.settings.Revision
	runtime.lanWork.Add(1)
	go func() {
		defer runtime.lanWork.Done()
		ctx, cancel := context.WithTimeout(command.ctx, 5*time.Second)
		defer cancel()
		stop := context.AfterFunc(runtime.ctx, cancel)
		defer stop()
		proof, err := reader.ReadCleanup(ctx, leaseSHA)
		event := lanOperationEvent{cleanupReview: &lanCleanupReviewEvent{operationID: operationID, original: original, proof: proof, err: err, reply: command.reply, revision: revision, ctx: command.ctx}}
		select {
		case runtime.lanEvents <- event:
		case <-runtime.ctx.Done():
		}
	}()
	return runtimeReply{deferred: true}
}

func (runtime *ServiceRuntime) applyPriorCleanupReview(event lanCleanupReviewEvent) {
	operation := runtime.lanOperations[event.operationID]
	if operation != nil {
		operation.reading = false
	}
	if event.err != nil || operation == nil || operation.recovered == nil || runtime.settingsUncertain || runtime.rotationPending || event.revision != runtime.settings.Revision || !matchesProtectedCleanup(event.original, event.proof) {
		event.reply <- runtimeReply{err: errors.New("protected original cleanup closure remains unconfirmed")}
		return
	}
	proof := event.proof
	operation.cleanup = &proof
	operation.status = priorCleanupStatus(operation.status, event.original, proof)
	runtime.publishLANOperations()
	review, err := runtime.lanReviews.issuePriorCleanup(proof, event.revision, time.Now())
	if err != nil {
		event.reply <- runtimeReply{err: err}
		return
	}
	reply := runtime.persistLANIssued(runtimeCommand{ctx: event.ctx, reply: event.reply}, review)
	if !reply.deferred {
		event.reply <- reply
	}
}

func (runtime *ServiceRuntime) consumePriorCleanup(command runtimeCommand) runtimeReply {
	if command.lanAdmitted == nil {
		return runtime.admitLANConsume(command, true)
	}
	if runtime.settingsUncertain || runtime.rotationPending || command.revision != runtime.settings.Revision || runtime.opts.LAN == nil {
		return runtimeReply{err: errors.New("current native cleanup owner unavailable")}
	}
	helper, ok := runtime.opts.LAN.Helper.(lanPriorCleanup)
	store, persistent := runtime.opts.LAN.Helper.(lanReviewPersistence)
	if !ok || !persistent {
		return runtimeReply{err: ErrLANAdapterUnavailable}
	}
	for _, operation := range runtime.lanOperations {
		if !operation.settled && operation.record.operationID != command.lanAdmitted.operationID {
			return runtimeReply{err: errors.New("settle existing administrative operation before cleanup")}
		}
	}
	pending := runtime.lanReviews.records[command.lanReviewID]
	if pending == nil || pending.cleanup == nil || pending.cleanupSpec == nil || (pending.review.Purpose != "cleanup-remove" && pending.review.Purpose != "cleanup-settle") {
		return runtimeReply{err: errors.New("distinct fresh prior-owner cleanup review required")}
	}
	record := command.lanAdmitted
	if record != pending || record.operationID == "" || record.admissionPending {
		return runtimeReply{err: errors.New("positive durable cleanup admission required")}
	}
	ctx, cancel := context.WithTimeout(runtime.ctx, 120*time.Second)
	operation := &lanOperation{record: record, cancel: cancel, status: LANStatus{OperationID: record.operationID, ReviewID: record.review.ReviewID, Selection: record.review.Selection, LeaseSHA: record.leaseSHA, Phase: "awaiting-admin", Ingress: "closed", Firewall: "remove-unknown", Sequence: 1, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano), CanCancel: true}}
	runtime.lanOperations[record.operationID] = operation
	runtime.publishLANOperations()
	cleanup := *record.cleanup
	mapping := LANProtectedReview{RuntimeInstance: runtime.state.Instance, ReviewID: record.review.ReviewID, ReviewSHA256: record.review.ReviewSHA256, OperationID: record.operationID, Transaction: record.transaction, Purpose: record.review.Purpose, LeaseSHA256: record.leaseSHA, Stage: record.stage, Cleanup: &cleanup}
	initialStatus := operation.status
	runtime.lanWork.Add(1)
	go func() {
		defer runtime.lanWork.Done()
		defer cancel()
		status := initialStatus
		status.CanCancel = false
		err := store.PersistReview(ctx, mapping)
		if err == nil {
			var ack lanlease.RemovalAcknowledgement
			ack, err = helper.CleanupPrior(ctx, lanhelper.Prepared{Spec: *record.cleanupSpec, LeaseSHA: record.leaseSHA, Sequence: 2}, record.review.Purpose)
			if err == nil && (ack.LeaseSHA != record.leaseSHA || ack.NativeKeysSHA != record.cleanupKeysSHA || ack.Sequence != 5 || !ack.TerminalSuccess || !ack.OperationClosed || !ack.ObservedAbsent || !ack.ProtectedReceipt || !ack.AcknowledgementFlushed) {
				err = errors.New("positive protected native cleanup acknowledgement required")
			}
		}
		status.Phase, status.Firewall = "cleanup-pending", "remove-unknown"
		status.CanInspect = true
		if err == nil {
			status.Phase, status.Firewall = "removed", "removed"
			status.CanInspect = false
		}
		event := lanOperationEvent{operationID: record.operationID, status: status, err: err, complete: true}
		select {
		case runtime.lanEvents <- event:
		case <-runtime.ctx.Done():
		}
	}()
	return runtimeReply{lan: operation.status}
}

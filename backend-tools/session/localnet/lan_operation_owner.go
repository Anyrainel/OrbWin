package localnet

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanprovider"
)

// PersistReview must durably bind these exact owner-generated identities in
// the authenticated protected store BEFORE Stage or Commit can be dispatched.
// A JSON acknowledgement or user-home receipt cannot implement this contract.
type LANProtectedReview = lanhelper.OwnerReview
type lanReviewPersistence interface {
	PersistReview(context.Context, LANProtectedReview) error
}
type lanOperation struct {
	recovered        *lanhelper.ProtectedReviewObservation
	cleanup          *lanprovider.CleanupObservation
	record           *lanReviewRecord
	controller       *lanController
	status           LANStatus
	cancel           context.CancelFunc
	settled          bool
	reading          bool
	cancelRequested  bool
	resource         *lanSession
	resourceSequence uint64
}
type lanOperationEvent struct {
	journal             *lanJournalEvent
	reconciliation      *lanReviewRecoveryEvent
	cleanupReview       *lanCleanupReviewEvent
	cleanupProof        *lanprovider.CleanupObservation
	operationID         string
	phase               string
	status              LANStatus
	err                 error
	complete            bool
	observation         bool
	resource            *lanSession
	resourceObservation bool
}

// Called only by the ServiceRuntime actor. A caller deadline after consume
// cannot undo or replay a dispatch; reconcile returns this same operation.
func (runtime *ServiceRuntime) activateLAN(command runtimeCommand) runtimeReply {
	if command.lanAdmitted == nil {
		return runtime.admitLANConsume(command, false)
	}
	if runtime.settingsUncertain || runtime.rotationPending || command.revision != runtime.settings.Revision {
		return runtimeReply{err: errors.New("current LAN resource owner is unavailable")}
	}
	if runtime.opts.LAN == nil || runtime.opts.LAN.Helper == nil {
		return runtimeReply{err: ErrLANAdapterUnavailable}
	}
	if _, ok := runtime.opts.LAN.Helper.(lanReviewPersistence); !ok {
		return runtimeReply{err: ErrLANAdapterUnavailable}
	}
	if command.kind == "lan-activate" {
		owners := map[string]bool{}
		for _, status := range runtime.lanOperationStatuses() {
			if status.OperationID == command.lanAdmitted.operationID {
				continue
			}
			if status.Phase != "closed" && status.Phase != "removed" {
				key := status.LeaseSHA
				if key == "" {
					key = status.OperationID
				}
				owners[key] = true
			}
		}
		if len(owners) >= 16 {
			return runtimeReply{err: errors.New("retained active or uncertain LAN owner budget reached")}
		}
	}
	var controller *lanController
	if command.kind == "lan-remove" || command.kind == "lan-settle-remove" {
		if review := runtime.lanReviews.records[command.lanReviewID]; review != nil && review.review.ReviewSHA256 == command.lanReviewSHA {
			for _, existing := range runtime.lanOperations {
				if existing.status.LeaseSHA == review.leaseSHA && existing.controller != nil {
					controller = existing.controller
					break
				}
			}
		}
		if controller == nil {
			return runtimeReply{err: errors.New("exact retained LAN removal owner unavailable")}
		}
	} else if runtime.generation == nil {
		if runtime.settings.Settings.Enabled {
			return runtimeReply{err: errors.New("enabled local service generation is unavailable")}
		}
		if err := runtime.startLANOnlyGeneration(command.ctx); err != nil {
			return runtimeReply{err: err}
		}
	}
	if controller == nil {
		controller = runtime.generation.lan
	}
	if controller.adapters == nil || controller.adapters.Helper == nil {
		return runtimeReply{err: ErrLANAdapterUnavailable}
	}
	store, ok := controller.adapters.Helper.(lanReviewPersistence)
	if !ok {
		return runtimeReply{err: errors.New("protected LAN review persistence is unregistered")}
	}
	for _, operation := range runtime.lanOperations {
		if !operation.settled && operation.record.operationID != command.lanAdmitted.operationID {
			return runtimeReply{err: errors.New("settle the existing LAN administrative operation before another dispatch")}
		}
	}
	purpose := "activate"
	if command.kind == "lan-remove" {
		purpose = "remove"
	}
	if command.kind == "lan-settle-remove" {
		purpose = "settle-remove"
	}
	record := command.lanAdmitted
	if record.review.Purpose != purpose || record.operationID == "" || record.admissionPending {
		return runtimeReply{err: errors.New("positive durable LAN consume admission required")}
	}
	ctx, cancel := context.WithTimeout(runtime.ctx, 120*time.Second)
	operation := &lanOperation{record: record, controller: controller, cancel: cancel, status: LANStatus{OperationID: record.operationID, ReviewID: record.review.ReviewID, Selection: record.review.Selection, Sequence: 1, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano), Phase: "awaiting-admin", Ingress: "closed", Firewall: "not-dispatched", CanCancel: true}}
	if purpose != "activate" {
		operation.status.LeaseSHA = record.leaseSHA
		operation.status.Phase = "closing"
		operation.status.Ingress = "unknown"
		operation.status.Firewall = "created"
	}
	runtime.lanOperations[record.operationID] = operation
	runtime.publishLANOperations()
	mapping := LANProtectedReview{RuntimeInstance: runtime.state.Instance, ReviewID: record.review.ReviewID, ReviewSHA256: record.review.ReviewSHA256, OperationID: record.operationID, Transaction: record.stage.Base.Transaction, Stage: record.stage, Purpose: purpose, LeaseSHA256: record.leaseSHA}
	runtime.lanWork.Add(1)
	go func() {
		defer runtime.lanWork.Done()
		defer cancel()
		send := func(event lanOperationEvent) {
			event.operationID = record.operationID
			select {
			case runtime.lanEvents <- event:
			case <-runtime.ctx.Done():
			}
		}
		retainedRemovalStatus := func() LANStatus {
			if purpose != "activate" {
				if resource := controller.findSession(record.leaseSHA); resource != nil {
					return controller.sessionStatus(resource)
				}
			}
			return LANStatus{}
		}
		if purpose != "activate" {
			// Explicitly consuming this removal authorizes exact ingress closure.
			// Native cleanup recovery must observe the old child exited before
			// asking for administrative consent. Shared HTTP refs stay intact.
			if err := controller.stop(ctx, record.leaseSHA); err != nil {
				send(lanOperationEvent{status: retainedRemovalStatus(), err: err, complete: true})
				return
			}
			send(lanOperationEvent{phase: "awaiting-admin", status: retainedRemovalStatus()})
		}
		if err := store.PersistReview(ctx, mapping); err != nil {
			send(lanOperationEvent{status: retainedRemovalStatus(), err: err, complete: true})
			return
		}
		if purpose != "activate" {
			progress := func(phase string) { send(lanOperationEvent{phase: phase}) }
			var err error
			if purpose == "settle-remove" {
				err = controller.settleRemoval(ctx, record.leaseSHA, progress)
			} else {
				err = controller.retire(ctx, record.leaseSHA, progress)
			}
			status := LANStatus{LeaseSHA: record.leaseSHA, Ingress: "closed", Firewall: "removed", Phase: "removed", UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
			if err != nil {
				status.Firewall = "remove-unknown"
				status.Phase = "cleanup-pending"
				status.Ingress = "unknown"
				for _, current := range controller.statuses() {
					if current.LeaseSHA == record.leaseSHA {
						status = current
					}
				}
			}
			send(lanOperationEvent{status: status, err: err, complete: true})
			return
		}
		status, err := controller.startObserved(ctx, record.stage, func(phase string) { send(lanOperationEvent{phase: phase}) }, func(resource *lanSession) {
			send(lanOperationEvent{status: controller.sessionStatus(resource), resource: resource})
		})
		send(lanOperationEvent{status: status, err: err, complete: true})
	}()
	return runtimeReply{lan: operation.status}
}
func (runtime *ServiceRuntime) applyLANEvent(event lanOperationEvent) {
	if event.journal != nil {
		runtime.applyLANJournalEvent(*event.journal)
		return
	}
	if event.cleanupReview != nil {
		runtime.applyPriorCleanupReview(*event.cleanupReview)
		return
	}
	if event.reconciliation != nil {
		runtime.applyLANReviewRecovery(*event.reconciliation)
		return
	}
	operation := runtime.lanOperations[event.operationID]
	if operation == nil || (operation.settled && !event.observation) {
		return
	}
	if event.observation {
		operation.reading = false
	}
	if event.cleanupProof != nil && operation.recovered != nil && matchesProtectedCleanup(*operation.recovered, *event.cleanupProof) {
		proof := *event.cleanupProof
		operation.cleanup = &proof
	}
	if event.resource != nil {
		operation.resource = event.resource
	}
	operation.status.Sequence++
	if event.status.LeaseSHA != "" && !event.complete {
		operation.status.LeaseSHA = event.status.LeaseSHA
		operation.status.Ingress = event.status.Ingress
		operation.status.Firewall = event.status.Firewall
	}
	if event.phase != "" {
		operation.status.Phase = event.phase
	}
	if event.phase == "commit-dispatched" {
		operation.status.Firewall = "create-unknown"
	}
	if event.phase == "rule-created" {
		operation.status.Firewall = "created"
	}
	if event.phase == "retire-dispatched" {
		operation.status.Firewall = "remove-unknown"
	}
	if event.complete {
		if !event.observation {
			operation.settled = true
		}
		if event.status.LeaseSHA != "" {
			sequence := operation.status.Sequence
			operation.status = event.status
			operation.status.Sequence = sequence
			operation.status.OperationID = event.operationID
			operation.status.ReviewID = operation.record.review.ReviewID
			operation.status.Selection = operation.record.review.Selection
		} else {
			operation.status.Phase = "closed"
			operation.status.CanCancel = false
			runtime.lanReviews.releaseBinding(operation.record)
		}
		if event.err != nil {
			operation.status.Error = "LAN setup did not complete; inspect retained exact ownership"
		}
		if event.status.Firewall == "removed" && event.err == nil {
			for _, other := range runtime.lanOperations {
				if other.status.LeaseSHA == event.status.LeaseSHA {
					other.status.Phase = "removed"
					other.status.Firewall = "removed"
					other.status.Ingress = "closed"
					other.status.CanCancel = false
					other.status.CanInspect = false
					other.status.CanReviewRemoval = false
					other.status.CanSettleRemoval = false
					other.status.CanReviewCleanup = false
					other.status.Sequence++
					runtime.lanReviews.releaseBinding(other.record)
				}
			}
		}
	}
	runtime.publishLANOperations()
}
func (runtime *ServiceRuntime) lanOperationStatuses() []LANStatus {
	out := make([]LANStatus, 0, len(runtime.lanOperations))
	for _, operation := range runtime.lanOperations {
		previous := publicLANStatus(operation.status)
		if operation.cancelRequested {
			previous.CanCancel = false
		}
		status := previous
		completedObservation := false
		if status.LeaseSHA != "" && operation.controller != nil {
			resources := []LANStatus{}
			if operation.resource != nil {
				resources = append(resources, operation.controller.sessionStatus(operation.resource))
			} else {
				resources = operation.controller.statuses()
			}
			for _, current := range resources {
				if current.LeaseSHA == status.LeaseSHA {
					completedObservation = current.Sequence != operation.resourceSequence
					operation.resourceSequence = current.Sequence
					current.OperationID = status.OperationID
					current.ReviewID = status.ReviewID
					current.Selection = status.Selection
					current.Sequence = previous.Sequence
					if !operation.settled {
						current.CanCancel = previous.CanCancel
					}
					if operation.record.review.Purpose != "activate" && !operation.settled {
						current.Phase = previous.Phase
						current.CanReviewRemoval = false
						current.CanSettleRemoval = false
					}
					status = current
					if current.Ingress == "closed" && (current.Phase == "closed" || current.Phase == "removed" || current.Phase == "cleanup-pending") {
						runtime.lanReviews.releaseBinding(operation.record)
					}
					break
				}
			}
		}
		if operation.cancelRequested {
			status.CanCancel = false
		}
		status = publicLANStatus(status)
		if completedObservation || !reflect.DeepEqual(previous, status) {
			if previous.Sequence < lanfirewall.MaxSequence {
				status.Sequence = previous.Sequence + 1
			} else {
				status.Sequence = lanfirewall.MaxSequence
				status.Phase = "closing"
				status.Ingress = "unknown"
				status.ReasonCode = "sequence-exhausted"
				if !operation.cancelRequested {
					operation.cancelRequested = true
					operation.cancel()
					if operation.controller != nil {
						go operation.controller.stop(runtime.ctx, status.LeaseSHA)
					}
				}
			}
		}
		operation.status = status
		out = append(out, status)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OperationID < out[j].OperationID })
	// Public summaries roll only after positive closure. Active and uncertain
	// ownership is always retained; the separate denial registry never rolls.
	if len(out) > 64 {
		retained := make([]LANStatus, 0, 64)
		closed := make([]LANStatus, 0, len(out))
		for _, status := range out {
			if status.Phase == "closed" || status.Phase == "removed" {
				closed = append(closed, status)
			} else {
				retained = append(retained, status)
			}
		}
		remaining := 64 - len(retained)
		if remaining > 0 {
			if len(closed) > remaining {
				closed = closed[len(closed)-remaining:]
			}
			retained = append(retained, closed...)
		}
		out = retained
	}
	return out
}
func publicLANStatus(status LANStatus) LANStatus {
	switch status.Phase {
	case "awaiting-admin", "awaiting-confirmation", "staging", "starting", "ready", "closing", "closed", "cancelled", "cleanup-pending", "removed", "failed":
	case "commit-dispatched", "rule-created", "prepared", "child-bound", "rule-acknowledged":
		status.Phase = "starting"
	case "observation-expired":
		status.Phase = "closing"
		status.ReasonCode = "observation-expired"
	case "retire-dispatched", "retire-pending", "settle-dispatched":
		status.Phase = "cleanup-pending"
	case "retired":
		status.Phase = "removed"
	default:
		status.Phase = "failed"
		status.Ingress = "unknown"
	}
	return status
}
func (runtime *ServiceRuntime) publishLANOperations() {
	statuses := runtime.lanOperationStatuses()
	runtime.mu.Lock()
	runtime.state.LAN = statuses
	runtime.touchStateLocked()
	runtime.mu.Unlock()
}

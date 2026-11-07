package localnet

import (
	"context"
	"errors"
	"time"
)

type lanJournalEvent struct {
	kind           string
	command        runtimeCommand
	snapshot       storedLANReview
	err            error
	reconciliation LANReconciliation
	complete       func(error)
}

func (runtime *ServiceRuntime) reviewJournal() (lanReviewJournal, error) {
	if runtime.lanJournal != nil {
		return runtime.lanJournal, nil
	}
	if runtime.opts.Ledger == nil || runtime.opts.Ledger.stateStore == nil {
		return nil, errors.New("native private review journal unavailable")
	}
	return ledgerLANReviewJournal{ledger: runtime.opts.Ledger, installation: runtime.opts.Installation}, nil
}

// Actor admission is bounded to one native receipt operation. IO runs off the
// actor; no receipt persistence, caller timeout or reply loss dispatches effects.
func (runtime *ServiceRuntime) persistLANIssued(command runtimeCommand, review LANReview) runtimeReply {
	record := runtime.lanReviews.records[review.ReviewID]
	if record == nil {
		return runtimeReply{err: errors.New("exact issued review unavailable")}
	}
	journal, err := runtime.reviewJournal()
	if err != nil || runtime.lanJournalActive || command.ctx == nil || command.reply == nil {
		record.revoked = true
		return runtimeReply{err: errors.New("bounded private review persistence unavailable")}
	}
	daemon := record.stage.Publication.Daemon
	if runtime.opts.Ledger != nil {
		daemon = runtime.opts.Ledger.daemon
	}
	snapshot, err := snapshotLANReview(record, runtime.opts.Installation, daemon, runtime.state.Instance)
	if err != nil {
		record.revoked = true
		return runtimeReply{err: err}
	}
	runtime.lanJournalActive = true
	runtime.lanWork.Add(1)
	go func() {
		defer runtime.lanWork.Done()
		ctx, cancel := context.WithTimeout(runtime.ctx, 5*time.Second)
		defer cancel()
		stop := context.AfterFunc(command.ctx, cancel)
		defer stop()
		err := journal.Issue(ctx, snapshot)
		event := lanOperationEvent{journal: &lanJournalEvent{kind: "issued", command: command, snapshot: snapshot, err: err}}
		select {
		case runtime.lanEvents <- event:
		case <-runtime.ctx.Done():
		}
	}()
	return runtimeReply{deferred: true}
}

func (runtime *ServiceRuntime) applyLANJournalEvent(event lanJournalEvent) {
	runtime.lanJournalActive = false
	if event.kind == "issued" {
		record := runtime.lanReviews.records[event.snapshot.Review.ReviewID]
		if record == nil || record.review.ReviewSHA256 != event.snapshot.Review.ReviewSHA256 {
			event.command.reply <- runtimeReply{err: errors.New("issued review owner changed")}
			return
		}
		if event.err == nil {
			copy := event.snapshot
			record.receipt = &copy
			record.durable = true
		}
		if event.err != nil || event.command.ctx.Err() != nil || record.revoked || runtime.settingsUncertain || runtime.rotationPending || record.revision != runtime.settings.Revision {
			record.revoked = true
			event.command.reply <- runtimeReply{err: errors.New("review was not published; private issuance remains nondispatched")}
			return
		}
		review := record.review
		event.command.reply <- runtimeReply{lanReview: &review}
		return
	}
	if event.complete != nil {
		event.complete(event.err)
	}
}

func (runtime *ServiceRuntime) admitLANConsume(command runtimeCommand, prior bool) runtimeReply {
	if runtime.lanJournalActive || runtime.settingsUncertain || runtime.rotationPending || command.revision != runtime.settings.Revision || command.ctx == nil || command.reply == nil {
		return runtimeReply{err: errors.New("bounded current LAN consume admission unavailable")}
	}
	journal, err := runtime.reviewJournal()
	if err != nil {
		return runtimeReply{err: err}
	}
	record := runtime.lanReviews.records[command.lanReviewID]
	if record == nil || !record.durable || record.receipt == nil || record.review.ReviewSHA256 != command.lanReviewSHA {
		return runtimeReply{err: errors.New("positively persisted exact issuance required")}
	}
	purpose := "activate"
	if command.kind == "lan-remove" {
		purpose = "remove"
	}
	if command.kind == "lan-settle-remove" {
		purpose = "settle-remove"
	}
	if prior {
		purpose = record.review.Purpose
		if record.cleanup == nil || (purpose != "cleanup-remove" && purpose != "cleanup-settle") {
			return runtimeReply{err: errors.New("exact prior cleanup review required")}
		}
	} else if record.review.Purpose != purpose {
		return runtimeReply{err: errors.New("LAN consume purpose differs")}
	}
	if runtime.opts.LAN == nil || runtime.opts.LAN.Helper == nil {
		return runtimeReply{err: ErrLANAdapterUnavailable}
	}
	if _, ok := runtime.opts.LAN.Helper.(lanReviewPersistence); !ok {
		return runtimeReply{err: ErrLANAdapterUnavailable}
	}
	for _, operation := range runtime.lanOperations {
		if !operation.settled {
			return runtimeReply{err: errors.New("settle existing administrative operation before another admission")}
		}
	}
	if !prior && purpose != "activate" {
		found := false
		for _, operation := range runtime.lanOperations {
			if operation.status.LeaseSHA == record.leaseSHA && operation.controller != nil {
				found = true
				break
			}
		}
		if !found {
			return runtimeReply{err: errors.New("exact retained LAN removal owner unavailable")}
		}
	}
	if !prior && purpose == "activate" && runtime.generation == nil && runtime.settings.Settings.Enabled {
		return runtimeReply{err: errors.New("enabled service generation unavailable")}
	}
	if !prior && purpose == "activate" {
		owners := map[string]bool{}
		for _, status := range runtime.lanOperationStatuses() {
			if status.Phase != "closed" && status.Phase != "removed" {
				key := status.LeaseSHA
				if key == "" {
					key = status.OperationID
				}
				owners[key] = true
			}
		}
		if len(owners) >= 16 {
			return runtimeReply{err: errors.New("retained LAN owner budget reached before consume admission")}
		}
	}
	record, err = runtime.lanReviews.consume(command.lanReviewID, command.lanReviewSHA, purpose, command.revision, time.Now())
	if err != nil {
		return runtimeReply{err: err}
	}
	record.admissionPending = true
	runtime.lanJournalActive = true
	snapshot := *record.receipt
	operationID := record.operationID
	runtime.lanWork.Add(1)
	go func() {
		defer runtime.lanWork.Done()
		ctx, cancel := context.WithTimeout(runtime.ctx, 5*time.Second)
		defer cancel()
		err := journal.Consume(ctx, snapshot, operationID)
		event := lanOperationEvent{journal: &lanJournalEvent{kind: "consumed", err: err, complete: func(err error) {
			record.admissionPending = false
			if err != nil {
				record.admissionFailed = true
				command.reply <- runtimeReply{err: errors.New("private consume outcome unknown; no effects were dispatched")}
				return
			}
			runtime.continueLANConsumed(command, record, prior)
		}}}
		select {
		case runtime.lanEvents <- event:
		case <-runtime.ctx.Done():
		}
	}()
	return runtimeReply{deferred: true}
}

func (runtime *ServiceRuntime) continueLANConsumed(command runtimeCommand, record *lanReviewRecord, prior bool) {
	command.lanAdmitted = record
	// Caller loss cannot revoke a durably consumed admission. This SDK owns the
	// bounded continuation and every late resource until its actual join.
	ctx, cancel := context.WithTimeout(runtime.ctx, 120*time.Second)
	command.ctx = ctx
	pending := &lanOperation{record: record, cancel: cancel, status: LANStatus{OperationID: record.operationID, ReviewID: record.review.ReviewID, Selection: record.review.Selection, Phase: "starting", Ingress: "closed", Firewall: "not-dispatched", CanCancel: true, Sequence: 1, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
	if record.review.Purpose != "activate" {
		pending.status.LeaseSHA = record.leaseSHA
		pending.status.Ingress = "unknown"
		pending.status.Firewall = "created"
	}
	runtime.lanOperations[record.operationID] = pending
	runtime.publishLANOperations()
	finish := func() {
		defer cancel()
		var reply runtimeReply
		if ctx.Err() != nil {
			runtime.failLANAdmitted(record, ctx.Err())
			command.reply <- runtimeReply{err: ctx.Err()}
			return
		}
		if prior {
			reply = runtime.consumePriorCleanup(command)
		} else {
			reply = runtime.activateLAN(command)
		}
		if reply.err != nil && runtime.lanOperations[record.operationID] == pending {
			runtime.failLANAdmitted(record, reply.err)
			reply.lan = runtime.lanOperations[record.operationID].status
		}
		command.reply <- reply
	}
	if prior || runtime.generation != nil {
		finish()
		return
	}
	runtime.lanJournalActive = true
	opts, installation, events := runtime.opts, runtime.lanInstallation, runtime.lanEvents
	runtime.lanWork.Add(1)
	go func() {
		defer runtime.lanWork.Done()
		builder := opts.lanGenerationBuilder
		if builder == nil {
			builder = buildLANOnlyGeneration
		}
		generation, err := builder(ctx, opts, installation, events)
		accepted := make(chan struct{})
		event := lanOperationEvent{journal: &lanJournalEvent{kind: "generation", err: err, complete: func(err error) {
			close(accepted) // Transfer ownership only when this actor receives it.
			defer cancel()
			if err != nil {
				if generation != nil {
					runtime.lanWork.Add(1)
					go func() { defer runtime.lanWork.Done(); _ = generation.shutdown.Close(context.Background()) }()
				}
				runtime.failLANAdmitted(record, err)
				command.reply <- runtimeReply{err: err}
				return
			}
			if runtime.ctx.Err() != nil || ctx.Err() != nil || runtime.generation != nil {
				runtime.lanWork.Add(1)
				go func() { defer runtime.lanWork.Done(); _ = generation.shutdown.Close(context.Background()) }()
				command.reply <- runtimeReply{err: errors.New("late LAN generation was not adopted")}
				runtime.failLANAdmitted(record, errors.New("cancelled generation was not adopted"))
				return
			}
			runtime.generation = generation
			finish()
		}}}
		select {
		case runtime.lanEvents <- event:
			select {
			case <-accepted:
			case <-runtime.ctx.Done():
				// A buffered event is not an ownership transfer: the actor may
				// have exited without reading it. Keep this worker joined until
				// every late attachment has positively closed.
				if generation != nil {
					_ = generation.shutdown.Close(context.Background())
				}
			}
		case <-runtime.ctx.Done():
			if generation != nil {
				_ = generation.shutdown.Close(context.Background())
			}
			cancel()
		}
	}()
}

func (runtime *ServiceRuntime) failLANAdmitted(record *lanReviewRecord, cause error) {
	runtime.lanOperations[record.operationID] = &lanOperation{record: record, cancel: func() {}, settled: true, status: LANStatus{OperationID: record.operationID, ReviewID: record.review.ReviewID, Selection: record.review.Selection, Phase: "failed", Ingress: "closed", Firewall: "not-dispatched", Error: "LAN preparation failed before helper dispatch", ReasonCode: "admission-preparation-failed", Sequence: 1, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
	if record.review.Purpose != "activate" {
		status := &runtime.lanOperations[record.operationID].status
		status.LeaseSHA = record.leaseSHA
		status.Phase, status.Ingress, status.Firewall = "cleanup-pending", "unknown", "created"
		status.CanInspect = true
		if record.review.Purpose == "settle-remove" || record.review.Purpose == "cleanup-settle" {
			status.Firewall = "remove-unknown"
		}
	}
	runtime.lanReviews.releaseBinding(record)
	runtime.publishLANOperations()
}

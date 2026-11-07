//go:build windows && amd64

package lanhelper

import (
	"context"
	"errors"
	"sort"
	"sync/atomic"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
	"orbwin.local/session/internal/lanprovider"
)

// RunNativeHelper is ONLY the dedicated one-shot executable. It owns no engine,
// workload, global firewall defaults, service registration or arbitrary command.
// Product registration remains unavailable until source/recovery/native gates.
func RunNativeHelper(parent context.Context, args []string) error {
	if parent == nil || len(args) != 2 || !recordID(args[0]) || !recordID(args[1]) {
		return errors.New("fixed helper identities required")
	}
	processDeadline := time.Now().Add(150 * time.Second)
	stopCap := startNativeProcessCap(processDeadline)
	defer stopCap()
	ctx, cancel := context.WithTimeout(parent, 120*time.Second)
	defer cancel()
	name, _ := AdminPipeName(args[0], args[1])
	connect, stop := context.WithTimeout(ctx, 5*time.Second)
	conn, err := winio.DialPipeAccessImpLevel(connect, name, windows.GENERIC_READ|windows.GENERIC_WRITE, winio.PipeImpLevelIdentification)
	stop()
	if err != nil {
		return errors.New("fixed ordinary owner channel unavailable")
	}
	defer conn.Close()
	fd, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("native helper owner channel unavailable")
	}
	manager, err := AuthenticateManagerPipe(windows.Handle(fd.Fd()))
	var authority *AuthorizedInstallation
	defer func() { finishFixedRoleOwners(authority, manager) }()
	if err != nil {
		return err
	}
	authority, err = manager.AuthorizeInstallation(args[0])
	if err != nil {
		return err
	}
	// Identification-only impersonation on the server requires a message from
	// this client first. This fixed greeting carries no review or authority.
	if conn.SetWriteDeadline(time.Now().Add(5*time.Second)) != nil || writeLaunch(conn, launchFrame{Version: 1, Kind: "helper-ready"}) != nil {
		return errors.New("native helper identification greeting unavailable")
	}
	var pulses atomic.Pointer[time.Time]
	initial := time.Now()
	pulses.Store(&initial)
	stopWatch := startNativeWatchdog(parent, manager, &pulses, processDeadline)
	defer stopWatch()
	conversation := &adminConversation{}
	defer conversation.close()
	commands := make(chan AdminFrame, 1)
	readerDone := make(chan struct{})
	stopIO := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopIO()
	go func() {
		defer close(readerDone)
		defer cancel()
		for {
			if conn.SetReadDeadline(time.Now().Add(5*time.Second)) != nil {
				return
			}
			frame, e := ReadAdmin(conn)
			if e != nil || conversation.accept(frame) != nil {
				return
			}
			now := time.Now()
			pulses.Store(&now)
			if frame.Kind == "pulse" {
				continue
			}
			if frame.Kind == "close" {
				return
			}
			select {
			case commands <- frame:
			default:
				return
			}
		}
	}()
	defer func() { cancel(); conn.Close(); <-readerDone }()
	var sequence uint64
	reply := func(request AdminFrame, response AdminFrame) error {
		sequence++
		response.Version = 2
		response.Sequence = sequence
		response.RequestSequence = request.Sequence
		if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}
		return WriteAdmin(conn, response)
	}
	fail := func(request AdminFrame, code string) error {
		_ = reply(request, AdminFrame{Kind: "error", ErrorCode: code})
		return errors.New("native helper operation did not complete; retained protected state")
	}
	var record OwnerReview
	var store *NativeStore
	var journal *ProtectedJournal
	var prepared Prepared
	var cleanup *lanprovider.CleanupRule
	var cleanupScope *lanlease.Spec
	var tombstones []lanlease.Tombstone
	nextPage := uint32(0)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case frame := <-commands:
			switch frame.Kind {
			case "review":
				record = *frame.Review
				if record.Stage.Base.Installation != args[0] || record.Transaction != args[1] || record.Validate(manager.identity, time.Now()) != nil {
					return fail(frame, "refused")
				}
				if record.Purpose != "activate" {
					// Restore only cleanup authority, after positive native child/namespace
					// closure. The retained protected scope must match what consent shows.
					cleanup, err = lanprovider.RestoreCleanupRule(ctx, args[0], record.LeaseSHA256, fd.Fd())
					if cleanup != nil {
						defer cleanup.Close()
					}
					if err != nil {
						return fail(frame, "refused")
					}
					spec, e := cleanup.ReviewScope()
					if e != nil || validateCleanupReview(record, spec) != nil {
						return fail(frame, "refused")
					}
					cleanupScope = &spec
					if record.Cleanup != nil {
						mapping, e := cleanup.OriginalMapping()
						if e != nil || validatePriorCleanupMapping(record, mapping) != nil {
							return fail(frame, "refused")
						}
					}
				}
				consent, e := confirmNativeScope(ctx, authority, record, cleanupScope)
				if e != nil {
					return fail(frame, "cancelled")
				}
				store, e = OpenNativeStore(authority, consent)
				if e != nil {
					return fail(frame, "refused")
				}
				defer store.Close()
				journal, e = store.PersistReview(record, time.Now())
				if e != nil {
					return fail(frame, "unknown")
				}
				if conversation.reviewed() != nil {
					return fail(frame, "refused")
				}
				if reply(frame, AdminFrame{Kind: "review-accepted"}) != nil {
					return errors.New("protected review acknowledgement transport unknown")
				}
			case "read-tombstones":
				if store == nil || frame.Page == nil || *frame.Page != nextPage {
					return fail(frame, "refused")
				}
				if tombstones == nil {
					tombstones, err = store.Tombstones()
					if err != nil {
						return fail(frame, "refused")
					}
					sort.Slice(tombstones, func(i, j int) bool { return tombstones[i].LeaseID < tombstones[j].LeaseID })
				}
				start := int(*frame.Page) * TombstonePageSize
				if start > len(tombstones) {
					return fail(frame, "refused")
				}
				end := start + TombstonePageSize
				if end > len(tombstones) {
					end = len(tombstones)
				}
				page := append([]lanlease.Tombstone{}, tombstones[start:end]...)
				more := end < len(tombstones)
				nextPage++
				if reply(frame, AdminFrame{Kind: "tombstones", Page: frame.Page, Tombstones: &page, More: &more}) != nil {
					return errors.New("protected tombstone read transport unknown")
				}
			case "stage":
				if journal == nil {
					return fail(frame, "refused")
				}
				prepared, err = journal.Stage(ctx)
				if err != nil {
					return fail(frame, "refused")
				}
				if conversation.prepared() != nil {
					return fail(frame, "refused")
				}
				if reply(frame, AdminFrame{Kind: "prepared", Prepared: &prepared}) != nil {
					return errors.New("protected stage acknowledgement transport unknown")
				}
			case "commit":
				if store == nil || prepared.LeaseSHA == "" || frame.Child == nil || frame.Observation == nil {
					return fail(frame, "refused")
				}
				child, e := holdPreparedChild(prepared, *frame.Child, manager)
				defer finishFixedRoleOwners(child, nil)
				if e != nil {
					return fail(frame, "refused")
				}
				if validateNativeCommit(ctx, prepared, manager, child, *frame.Observation) != nil {
					return fail(frame, "refused")
				}
				ruleJournal, e := store.newRuleJournal(record, prepared, child, frame.NamespaceID)
				if e != nil {
					return fail(frame, "refused")
				}
				defer ruleJournal.Close()
				now := time.Now().UTC().Format(time.RFC3339Nano)
				request := prepared.Spec.Request
				receipt := lanfirewall.Receipt{Version: 1, Request: request, Sequence: 1, State: "create-intent", CreatedAt: now, UpdatedAt: now, RuleName: request.RuleName(), ScopeSHA256: request.Scope.Digest(), Worker: prepared.Spec.Worker}
				binding, e := observeConsentInterface(ctx, request.Scope)
				if e != nil {
					return fail(frame, "refused")
				}
				operation, stop := context.WithTimeout(ctx, 5*time.Second)
				owned, e := lanprovider.CreateOwnedRule(operation, receipt, binding, prepared.LeaseSHA, ruleJournal)
				stop()
				if e != nil {
					return fail(frame, "unknown")
				}
				ack, e := owned.Acknowledgement()
				if e != nil || validateNativeCommit(ctx, prepared, manager, child, *frame.Observation) != nil {
					return fail(frame, "unknown")
				}
				if reply(frame, AdminFrame{Kind: "committed", Rule: &ack}) != nil {
					return errors.New("native rule acknowledgement transport unknown")
				}
				return nil
			case "remove", "settle-remove", "cleanup-remove", "cleanup-settle":
				if cleanup == nil || journal == nil || record.Purpose != frame.Kind {
					return fail(frame, "refused")
				}
				operation, stop := context.WithTimeout(ctx, 5*time.Second)
				var removed lanlease.RemovalAcknowledgement
				var e error
				if frame.Kind == "settle-remove" || frame.Kind == "cleanup-settle" {
					removed, e = lanprovider.SettleCleanupRule(operation, cleanup)
				} else {
					removed, e = lanprovider.RetireCleanupRule(operation, cleanup)
				}
				stop()
				if e != nil {
					return fail(frame, "unknown")
				}
				if reply(frame, AdminFrame{Kind: "removed", Removal: &removed}) != nil {
					return errors.New("native removal acknowledgement transport unknown")
				}
				return nil
			default:
				return fail(frame, "refused")
			}
		}
	}
}

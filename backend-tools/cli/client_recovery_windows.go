//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"time"

	"golang.org/x/sys/windows"
)

func rootBirth(handle windows.Handle) (writerBirthRecord, error) {
	var v writerBirthRecord
	pid, e := windows.GetProcessId(handle)
	if e != nil || pid == 0 {
		return v, errClientConfigRecovery
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(handle, &created, &exited, &kernel, &user) != nil {
		return v, errClientConfigRecovery
	}
	var session uint32
	if windows.ProcessIdToSessionId(pid, &session) != nil {
		return v, errClientConfigRecovery
	}
	v.RootPID, v.RootCreated, v.RootSession = pid, uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), session
	return v, nil
}
func (n *nativeClientConfig) recordWriterBirth(h uintptr, binding [32]byte) error {
	if n.check() != nil || n.intent == nil || n.birth != nil {
		return errClientConfigRecovery
	}
	body, e := n.intent.read(4096)
	intent, e2 := parseWriterIntent(body)
	if e != nil || e2 != nil || intent.Binding != hex.EncodeToString(binding[:]) {
		return errClientConfigRecovery
	}
	if _, e = n.readOriginal(); e != nil {
		return errClientConfigRecovery
	}
	b, e := rootBirth(windows.Handle(h))
	if e != nil {
		return e
	}
	b.Schema, b.Kind, b.Binding, b.IntentSHA, b.IntentBody = 3, "docker-writer-birth", intent.Binding, digestBytes(body), string(body)
	encoded, e := json.Marshal(b)
	if e != nil || len(encoded) > 8192 {
		return errClientConfigRecovery
	}
	n.birth, e = n.createMutationRecord(n.parent(), "writer.birth", encoded)
	return e
}

// A query handle is held through the check. An absent PID or a different actual
// creation generation is obsolete; access denial and query failure are unknown.
// Review never kills a PID and never restarts an engine or WSL distribution.
func (n *nativeClientConfig) stoppedWriterRoot(b writerBirthRecord) error {
	if n.unknownQuery != 0 {
		return errClientConfigRecovery
	}
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, b.RootPID)
	if errors.Is(e, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if e != nil {
		return errClientConfigRecovery
	}
	n.unknownQuery = h
	actual := writerBirthRecord{RootPID: b.RootPID}
	var created, exited, kernel, user windows.Filetime
	e = windows.GetProcessTimes(h, &created, &exited, &kernel, &user)
	actual.RootCreated = uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	if e == nil && actual.RootCreated == b.RootCreated {
		e = windows.ProcessIdToSessionId(b.RootPID, &actual.RootSession)
	}
	signaled := false
	if e == nil && actual.RootCreated == b.RootCreated {
		var status uint32
		status, e = windows.WaitForSingleObject(h, 0)
		signaled = e == nil && status == windows.WAIT_OBJECT_0
	}
	closeErr := windows.CloseHandle(h)
	if closeErr == nil {
		n.unknownQuery = 0
	} else {
		n.poison = true
	}
	if !writerGenerationStopped(b, actual, signaled, e) || closeErr != nil {
		return errClientConfigRecovery
	}
	return nil
}
func configMatchesMetadata(data *configData, volume, high, low uint32, sd, sha string) error {
	if data == nil || data.check() != nil {
		return errClientConfigRecovery
	}
	var id windows.ByHandleFileInformation
	h := windows.Handle(data.file.Fd())
	if windows.GetFileInformationByHandle(h, &id) != nil || id.VolumeSerialNumber != volume || id.FileIndexHigh != high || id.FileIndexLow != low {
		return errClientConfigRecovery
	}
	descriptor, e := configDescriptorSHA(h)
	if e != nil || descriptor != sd {
		return errClientConfigRecovery
	}
	body, e := data.read(clientConfigBudget)
	if e != nil || digestBytes(body) != sha {
		return errClientConfigRecovery
	}
	return nil
}
func (n *nativeClientConfig) recoveryRecord(name string, budget int64) (*configData, []byte, error) {
	h, e := configOpen(n.parent(), name, windows.FILE_OPEN, false, false, false, nil)
	if errors.Is(e, windows.STATUS_OBJECT_NAME_NOT_FOUND) {
		return nil, nil, nil
	}
	if e != nil {
		return nil, nil, errClientConfigRecovery
	}
	record, e := n.hold(h, false)
	if e != nil {
		return nil, nil, e
	}
	body, e := record.read(budget)
	return record, body, e
}
func (n *nativeClientConfig) releaseRecoveryOwner() error {
	// Closing review releases native ownership, preserving every pending byte.
	// Unknown closes keep the owner retained. Controller is always closed last.
	if n.successorUnknown || n.unknownQuery != 0 || n.checkOrigin() != nil {
		return errClientConfigRecovery
	}
	for _, d := range n.data {
		if d == n.owner || d.file == nil {
			continue
		}
		// Release does not accept these bytes as authority. A failed owned
		// write may intentionally differ from its initial size/time snapshot;
		// positively closing the actual retained capability preserves its data.
		if d.file.Close() != nil {
			return n.successorFence()
		}
		d.file = nil
	}
	for i := len(n.ancestors) - 1; i >= 0; i-- {
		o := &n.ancestors[i]
		if o.recheck() != nil {
			return errClientConfigRecovery
		}
		if o.file.Close() != nil {
			return n.successorFence()
		}
		o.file = nil
	}
	if n.owner.check() != nil {
		return errClientConfigRecovery
	}
	if n.owner.file.Close() != nil {
		return n.successorFence()
	}
	n.owner.file = nil
	retainedConfigGuards.Lock()
	defer retainedConfigGuards.Unlock()
	for i, held := range retainedConfigGuards.roots {
		if held == n {
			retainedConfigGuards.roots = append(retainedConfigGuards.roots[:i], retainedConfigGuards.roots[i+1:]...)
			break
		}
	}
	return nil
}

func preparePrivateWriterRecovery(ctx context.Context) (review *clientWriterRecovery, resultErr error) {
	if fresh, handled, e := preparePrivateFreshRecovery(ctx); handled {
		return fresh, e
	}
	g, e := acquirePrivateClientConfigMode(ctx, true)
	if e != nil {
		return nil, e
	}
	return prepareHeldWriterRecovery(ctx, g)
}

// Reentry is only from the explicit EOF-prepare review below, which has already
// persisted a full native source-bound prepare while retaining this same owner.
func prepareHeldWriterRecovery(ctx context.Context, g *clientConfigGuard) (review *clientWriterRecovery, resultErr error) {
	n := g.native.(*nativeClientConfig)
	defer func() {
		if resultErr != nil {
			if closeErr := n.releaseRecoveryOwner(); closeErr != nil {
				resultErr = errors.Join(errClientConfigRecovery, closeErr)
			}
		}
	}()
	// Recovery is narrowly for this writer protocol; other pending workflows
	// remain owned by their distinct provider and cannot be swept here.
	if snapshotAbsent(n.parent(), "snapshot.intent", false) != nil {
		return nil, errClientConfigRecovery
	}
	intentFile, intentBytes, e := n.recoveryRecord("writer.intent", 4096)
	if e != nil {
		return nil, e
	}
	birthFile, birthBytes, e := n.recoveryRecord("writer.birth", 8192)
	if e != nil {
		return nil, e
	}
	originalFile, originalBytes, e := n.recoveryRecord("writer.original", clientConfigBudget)
	if e != nil {
		return nil, e
	}
	prepareFile, prepareBytes, e := n.recoveryRecord("writer.prepare", 4096)
	if e != nil {
		return nil, e
	}
	var prepare writerPrepareRecord
	recoveryFile, recoveryBytes, e := n.recoveryRecord("recovery.intent", recoveryV2Budget)
	if e != nil {
		return nil, e
	}
	nextFile, nextBytes, e := n.recoveryRecord("recovery.next", recoveryV2Budget)
	if e != nil {
		return nil, errClientConfigRecovery
	}
	dispatch, dispatchErr := classifyRecoveryDispatch(intentFile != nil, intentBytes, recoverySiblingPresence{prepare: prepareFile != nil, original: originalFile != nil, birth: birthFile != nil, recovery: recoveryFile != nil, next: nextFile != nil}, digestBytes(g.body))
	if dispatchErr != nil {
		return nil, dispatchErr
	}
	switch dispatch.route {
	case localEOFRoute:
		return prepareLocalEOFReview(ctx, g, intentFile, intentBytes)
	case localCompleteRoute:
		return prepareLocalReplaceReview(ctx, g, intentFile, intentBytes, dispatch.local, dispatch.nextSHA)
	}
	if prepareFile != nil && incompleteWriterBirth(prepareBytes) {
		if intentFile != nil || originalFile != nil || birthFile != nil || recoveryFile != nil || nextFile != nil || snapshotAbsent(n.directory(), "config.stage", false) != nil {
			return nil, errClientConfigRecovery
		}
		return preparePartialWriterPrepareReview(ctx, g, n, prepareFile, prepareBytes)
	}
	if prepareFile != nil {
		prepare, e = parseWriterPrepare(prepareBytes)
		if e != nil {
			return nil, e
		}
	}
	var recovery writerRecoveryRecord
	var recoveryV2 *writerRecoveryV2
	incompleteRecovery := recoveryFile != nil && incompleteRecoveryRecord(recoveryBytes)
	recoveryAuthority := recoveryFile != nil && !incompleteRecovery
	if recoveryAuthority {
		recovery, recoveryV2, e = parseRecoveryAny(recoveryBytes)
		if e != nil {
			return nil, errClientConfigRecovery
		}
	}
	if intentFile == nil && originalFile == nil && prepareFile == nil && birthFile == nil && recoveryFile == nil {
		if snapshotAbsent(n.directory(), "config.stage", false) != nil {
			return nil, errClientConfigRecovery
		}
		return nil, errClientConfigNoRecovery
	}
	var intent writerIntentRecord
	var birth writerBirthRecord
	incompleteBirth := birthFile != nil && incompleteWriterBirth(birthBytes)
	completeBirth := birthFile != nil && !incompleteBirth
	incompleteIntent := intentFile != nil && len(intentBytes) <= 4096 && incompleteWriterBirth(intentBytes)
	if incompleteIntent && (completeBirth || prepareFile == nil) {
		return nil, errClientConfigRecovery
	}
	preIntent := !completeBirth && (intentFile == nil || incompleteIntent) && prepareFile != nil
	if completeBirth {
		var embedded writerIntentRecord
		birth, embedded, e = parseWriterBirth(birthBytes)
		if e != nil || n.stoppedWriterRoot(birth) != nil {
			return nil, errClientConfigRecovery
		}
		intent = embedded
		if intentFile != nil && !bytes.Equal(intentBytes, []byte(birth.IntentBody)) {
			return nil, errClientConfigRecovery
		}
	} else {
		metadata := intentBytes
		if (intentFile == nil || incompleteIntent) && recoveryAuthority {
			metadata = []byte(recovery.IntentBody)
		}
		if (intentFile == nil || incompleteIntent) && !recoveryAuthority && prepareFile != nil {
			preIntent = true
			intent = prepare.intent()
			if originalFile != nil {
				var id windows.ByHandleFileInformation
				if windows.GetFileInformationByHandle(windows.Handle(originalFile.file.Fd()), &id) != nil {
					return nil, errClientConfigRecovery
				}
				intent.OriginalVolume, intent.OriginalHigh, intent.OriginalLow = id.VolumeSerialNumber, id.FileIndexHigh, id.FileIndexLow
				intent.OriginalSD, e = configDescriptorSHA(windows.Handle(originalFile.file.Fd()))
				if e != nil {
					return nil, e
				}
			}
			metadata, _ = json.Marshal(intent)
		}
		intent, e = parseWriterIntent(metadata)
		if e != nil || (intentFile == nil && !recoveryAuthority && prepareFile == nil) {
			return nil, errClientConfigRecovery
		}
		// No birth is no-resume evidence only under schema 3, and the source
		// must retain its exact original native identity, SD, digest and bytes.
		if !recoveryAuthority && configMatchesMetadata(n.config, intent.BaseVolume, intent.BaseHigh, intent.BaseLow, intent.BaseSD, intent.BaseSHA) != nil {
			return nil, errClientConfigRecovery
		}
	}
	if prepareFile != nil && !prepare.matches(intent) {
		return nil, errClientConfigRecovery
	}
	if originalFile == nil && ((intentFile != nil && !incompleteIntent) || (!completeBirth && !recoveryAuthority && prepareFile == nil)) {
		return nil, errClientConfigRecovery
	}
	if completeBirth && intentFile == nil && originalFile == nil && recoveryFile == nil && nextFile == nil {
		return prepareBirthCleanupRefresh(ctx, g, n, birthFile, birthBytes, birth, prepareFile, prepareBytes)
	}
	profile, _, e := nativeLocations()
	if e != nil {
		return nil, e
	}
	baseline := originalBytes
	if preIntent {
		baseline = g.body
	}
	fromOriginal := originalFile != nil || preIntent
	strictCurrent := objectDataStrict(n.config) == nil
	prior := false
	if recoveryAuthority {
		prior = configMatchesMetadata(n.config, recovery.CurrentVolume, recovery.CurrentHigh, recovery.CurrentLow, recovery.CurrentSD, recovery.CurrentSHA) == nil
	}
	// Native authority is checked before any current-selector rewrite. The pure
	// data coordinator below includes the formerly failing missing-original
	// precheck, so owner-published B is not required to already route to C.
	checkAuthority := func() error {
		if n.checkOrigin() != nil || n.config.check() != nil {
			return errClientConfigRecovery
		}
		if fromOriginal && !preIntent && originalFile != nil && configMatchesMetadata(originalFile, intent.OriginalVolume, intent.OriginalHigh, intent.OriginalLow, intent.OriginalSD, intent.OriginalSHA) != nil {
			return errClientConfigRecovery
		}
		if completeBirth && n.stoppedWriterRoot(birth) != nil {
			return errClientConfigRecovery
		}
		if recoveryAuthority {
			if recovery.Binding != intent.Binding || (completeBirth && recovery.IntentBody != birth.IntentBody) || (intentFile != nil && !incompleteIntent && recovery.IntentBody != string(intentBytes)) {
				return errClientConfigRecovery
			}
			ownerTarget := strictCurrent && digestBytes(g.body) == recovery.RestoreSHA
			if !prior && !ownerTarget {
				return errClientConfigRecovery
			}
		}
		return nil
	}
	var existingRecovery *writerRecoveryRecord
	if recoveryAuthority {
		existingRecovery = &recovery
	}
	plan, e := prepareRecoveryDataPlan(baseline, g.body, profile, g.plugins, fromOriginal, existingRecovery, recoveryV2, prior, strictCurrent, checkAuthority)
	if e != nil {
		return nil, e
	}
	baseline, target, needsPublish := plan.baseline, plan.target, plan.needsPublish

	// A stage is never enumerated/swept. Under the validated writer journal and
	// stopped/no-resume root, admit precisely this fixed private object. It may
	// precede recovery.intent after interrupted natural settlement. Restoration
	// persists its recovery intent before disposing this captured partial stage.
	var stage *configData
	var stageBytes []byte
	h, stageErr := configOpen(n.directory(), "config.stage", windows.FILE_OPEN, false, true, false, nil)
	if stageErr == nil {
		stage, e = n.hold(h, false)
		if e != nil {
			return nil, e
		}
		stageBytes, e = stage.read(clientConfigBudget)
		if e != nil {
			return nil, e
		}
	} else if !errors.Is(stageErr, windows.STATUS_OBJECT_NAME_NOT_FOUND) {
		return nil, errClientConfigRecovery
	}
	current := n.config
	currentBytes := append([]byte(nil), g.body...)
	var successor writerRecoveryV2
	if recoveryAuthority {
		successor, e = n.successorWitness(recovery, recoveryFile, recoveryBytes, current, currentBytes, stage, stageBytes, g.plugins, target)
		if e != nil {
			return nil, e
		}
		if originalFile == nil && !preIntent {
			successor.PublishedBaselineSHA = digestBytes(currentBytes)
		}
		if nextFile != nil {
			if _, e = validateNextBody(nextBytes, successor, baseline, profile); e != nil {
				return nil, e
			}
		}
	} else if nextFile != nil {
		return nil, errClientConfigRecovery
	}
	n.intent, n.original, n.birth, n.prepare = intentFile, originalFile, birthFile, prepareFile
	if originalFile != nil {
		n.originalID, n.originalSD, n.originalSHA = "recovery-bound", originalFile.sd, sha256.Sum256(originalBytes)
	}
	review = &clientWriterRecovery{deadline: time.Now().Add(5 * time.Minute), finishOnly: !needsPublish, preparingOnly: preIntent, closeNative: n.releaseRecoveryOwner}
	recheckSources := func() error {
		if ctx.Err() != nil || n.checkOrigin() != nil || current.check() != nil {
			return errClientConfigRecovery
		}
		actual, err := current.read(clientConfigBudget)
		if err != nil || !bytes.Equal(actual, currentBytes) {
			return errClientConfigRecovery
		}
		for _, r := range []struct {
			file *configData
			body []byte
		}{{intentFile, intentBytes}, {birthFile, birthBytes}, {originalFile, originalBytes}, {prepareFile, prepareBytes}, {recoveryFile, recoveryBytes}, {stage, stageBytes}} {
			if r.file != nil {
				body, err := r.file.read(clientConfigBudget)
				if err != nil || !bytes.Equal(body, r.body) {
					return errClientConfigRecovery
				}
			}
		}
		if completeBirth && n.stoppedWriterRoot(birth) != nil {
			return errClientConfigRecovery
		}
		return nil
	}
	review.restoreNative = func() (*clientConfigGuard, error) {
		var err error
		if recheckSources() != nil {
			return nil, errClientConfigRecovery
		}
		// EOF-only partial recovery metadata never supplies authority. The
		// independently validated source records remain held while explicit
		// restore removes this exact captured object and creates a fresh full
		// durable recovery intent before touching any stage or current config.
		if incompleteRecovery {
			if n.disposeExactRecord("recovery.intent", recoveryFile) != nil {
				return nil, errClientConfigRecovery
			}
			recoveryFile = nil
		}
		if recoveryFile == nil && (needsPublish || stage != nil || incompleteRecovery) {
			metadata := intentBytes
			if intentFile == nil {
				metadata = []byte(birth.IntentBody)
			}
			if preIntent {
				metadata, _ = json.Marshal(intent)
			}
			recovery = writerRecoveryRecord{Schema: 1, Kind: "restore-original", Binding: intent.Binding, CurrentSHA: digestBytes(currentBytes), RestoreSHA: digestBytes(target), IntentBody: string(metadata)}
			var id windows.ByHandleFileInformation
			if windows.GetFileInformationByHandle(windows.Handle(current.file.Fd()), &id) != nil {
				return nil, errClientConfigRecovery
			}
			recovery.CurrentVolume, recovery.CurrentHigh, recovery.CurrentLow = id.VolumeSerialNumber, id.FileIndexHigh, id.FileIndexLow
			recovery.CurrentSD, err = configDescriptorSHA(windows.Handle(current.file.Fd()))
			if err != nil {
				return nil, err
			}
			v2 := recoveryV2From(recovery, g.plugins)
			body, err := recoveryV2Bytes(v2)
			if err != nil {
				return nil, err
			}
			body, err = recoveryQualificationBytes(n, recovery, body)
			if err != nil {
				return nil, err
			}
			recoveryFile, err = n.createSnapshotRecord(n.parent(), "recovery.intent", body)
			if err != nil {
				return nil, err
			}
			recoveryQualificationCheckpoint(n, recoveryHistoricalJournal)
		}
		// A complete successor is durable before OLD or any config/stage
		// effect. Existing same-target schema1 remains readable and immutable.
		if recoveryAuthority && (recovery.RestoreSHA != digestBytes(target) || nextFile != nil) {
			recoveryFile, recoveryBytes, err = n.advanceRecovery(recoveryFile, recoveryBytes, nextFile, nextBytes, successor, baseline, profile, recheckSources)
			if err != nil {
				return nil, err
			}
			recovery, recoveryV2, err = parseRecoveryAny(recoveryBytes)
			if err != nil {
				return nil, err
			}
			nextFile = nil
		}
		n.poison = true
		if stage != nil {
			if stage.check() != nil {
				return nil, errClientConfigRecovery
			}
			if e := n.disposeOwnedFile(n.directory(), "config.stage", stage); e != nil {
				return nil, e
			}
			recoveryQualificationCheckpoint(n, recoveryStageRetired)
		}
		if needsPublish && !(bytes.Equal(currentBytes, target) && objectDataStrict(current) == nil) {
			if current.file.Close() != nil {
				return nil, n.successorFence()
			}
			current.file = nil
			if n.publishMergedConfig(target) != nil {
				return nil, n.successorFence()
			}
		}
		if n.config == nil || objectDataStrict(n.config) != nil {
			return nil, errClientConfigRecovery
		}
		body, err := n.config.read(clientConfigBudget)
		if err != nil || !bytes.Equal(body, target) || privateClientRouting(body, g.plugins) != nil {
			return nil, errClientConfigRecovery
		}
		// Complete birth is last. Without one, the recovery intent is last;
		// an EOF-truncated birth is removed while prepare/recovery still bind
		// the source. Every cut retains a complete source metadata record.
		cleanup := []struct {
			name string
			file *configData
		}{{"writer.intent", intentFile}, {"writer.original", originalFile}}
		if incompleteBirth {
			cleanup = append(cleanup, struct {
				name string
				file *configData
			}{"writer.birth", birthFile})
		}
		cleanup = append(cleanup, struct {
			name string
			file *configData
		}{"writer.prepare", prepareFile})
		if completeBirth {
			cleanup = append(cleanup, struct {
				name string
				file *configData
			}{"recovery.intent", recoveryFile}, struct {
				name string
				file *configData
			}{"writer.birth", birthFile})
		} else {
			cleanup = append(cleanup, struct {
				name string
				file *configData
			}{"recovery.intent", recoveryFile})
		}
		for _, r := range cleanup {
			if r.file != nil && n.disposeExactRecord(r.name, r.file) != nil {
				return nil, n.successorFence()
			}
			if r.name == "writer.prepare" {
				recoveryQualificationCheckpoint(n, recoveryPublishedOriginalRemoved)
			}
		}
		n.intent, n.original, n.birth, n.prepare = nil, nil, nil, nil
		n.poison = false
		g.body = append([]byte(nil), target...)
		return g, nil
	}
	return review, nil
}

// writer.prepare is a schema3-only fixed namespace. Its own producer cannot
// create another record or loan the source until this file is complete, flushed,
// read back, closed and independently admitted. Cleanup unlinks; it never
// truncates. Thus an EOF-only prepare with every other record absent is a
// pre-admission process/filesystem crash cut. This does not claim that arbitrary
// disk corruption or power-loss durability can be diagnosed from a prefix.
func preparePartialWriterPrepareReview(ctx context.Context, g *clientConfigGuard, n *nativeClientConfig, record *configData, partial []byte) (*clientWriterRecovery, error) {
	if len(partial) > 4096 || objectDataStrict(n.config) != nil {
		return nil, errClientConfigRecovery
	}
	profile, _, e := nativeLocations()
	if e != nil {
		return nil, e
	}
	_, changed, e := refreshPrivateManagedField(g.body, g.plugins, profile)
	if e != nil {
		return nil, e
	}
	current := n.config
	baseline := append([]byte(nil), g.body...)
	review := &clientWriterRecovery{deadline: time.Now().Add(5 * time.Minute), finishOnly: !changed, preparingOnly: true, closeNative: n.releaseRecoveryOwner}
	review.restoreNative = func() (*clientConfigGuard, error) {
		if ctx.Err() != nil || n.checkOrigin() != nil || objectDataStrict(current) != nil {
			return nil, errClientConfigRecovery
		}
		actual, err := current.read(clientConfigBudget)
		if err != nil || !bytes.Equal(actual, baseline) {
			return nil, errClientConfigRecovery
		}
		actual, err = record.read(4096)
		if err != nil || !bytes.Equal(actual, partial) || !incompleteWriterBirth(actual) {
			return nil, errClientConfigRecovery
		}
		for _, name := range []string{"writer.intent", "writer.original", "writer.birth", "recovery.intent", "recovery.next", "snapshot.intent"} {
			if snapshotAbsent(n.parent(), name, false) != nil {
				return nil, errClientConfigRecovery
			}
		}
		if snapshotAbsent(n.directory(), "config.stage", false) != nil {
			return nil, errClientConfigRecovery
		}
		if n.disposeExactRecord("writer.prepare", record) != nil {
			return nil, errClientConfigRecovery
		}
		n.prepare = nil
		if !changed {
			return g, nil
		}
		// The unchanged strict native source is authoritative at this cut.
		// A crash after unlink is healthy; an interrupted replacement prepare
		// is the same EOF-only state. No settings change precedes its durability.
		var binding [32]byte
		if _, err = rand.Read(binding[:]); err != nil || binding == [32]byte{} {
			return nil, errClientConfigRecovery
		}
		var id windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(windows.Handle(current.file.Fd()), &id) != nil {
			return nil, errClientConfigRecovery
		}
		sd, err := configDescriptorSHA(windows.Handle(current.file.Fd()))
		if err != nil {
			return nil, err
		}
		prepare := writerPrepareRecord{Schema: 3, Kind: "docker-writer-prepare", Mutation: string(writerRootUnchanged), Binding: hex.EncodeToString(binding[:]), KeysSHA: digestBytes([]byte("[]")), BaseSHA: digestBytes(baseline), BaseVolume: id.VolumeSerialNumber, BaseHigh: id.FileIndexHigh, BaseLow: id.FileIndexLow, BaseSD: sd}
		body, err := json.Marshal(prepare)
		if err != nil {
			return nil, err
		}
		n.prepare, err = n.createSnapshotRecord(n.parent(), "writer.prepare", body)
		if err != nil {
			return nil, err
		}
		if n.prepare.check() != nil || n.prepare.file.Close() != nil {
			return nil, errClientConfigRecovery
		}
		n.prepare.file = nil
		n.prepare = nil
		// Transfer the one native owner to the normal review loader. That
		// loader owns checked release on failure; this consumed review must not
		// close the same owner a second time.
		review.closeNative = nil
		bound, err := prepareHeldWriterRecovery(ctx, g)
		if err != nil {
			return nil, err
		}
		guard, err := bound.restore(ctx)
		if err != nil {
			return nil, errors.Join(err, bound.close())
		}
		return guard, nil
	}
	return review, nil
}
func objectDataStrict(data *configData) error {
	if data == nil || data.check() != nil {
		return errClientConfigRecovery
	}
	_, e := dataPolicy(windows.Handle(data.file.Fd()), data.sid, false)
	return e
}

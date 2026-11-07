//go:build windows

package sdkstate

import (
	"context"
	"golang.org/x/sys/windows"
)

type nativeColdLeafRetention struct {
	master                                   *nativeColdLANMaster
	leaf                                     *LANObservationLeaf
	expected                                 transitionProcess
	process, stoppedReturn                   windows.Handle // SDK roundtrips from actual Rdd
	unknown, stopped, closeAttempted, closed bool
	closeErr                                 error
	unknownCloses                            []windows.Handle
}

func (m *nativeColdLANMaster) leafOwner(ctx context.Context, leaf *LANObservationLeaf) (*coldLANPhysicalAdapter, error) {
	if m == nil || leaf == nil || m.factory == nil || leaf.factory != m.factory || m.factory.identity != m.factory || m.pump == nil || m.pump.factory != m.factory || m.checkLANLeafAdoption(ctx, m.self) != nil {
		return nil, ErrColdLeaf
	}
	n, ok := leaf.nativeOwner().(*coldLANPhysicalAdapter)
	if !ok || n == nil || n.leaf != leaf || n.physical == nil {
		return nil, ErrColdLeaf
	}
	return n, nil
}

func (m *nativeColdLANMaster) validateLeafOperation(ctx context.Context, leaf *LANObservationLeaf, kind string) error {
	n, err := m.leafOwner(ctx, leaf)
	if err != nil {
		return err
	}
	m.leafMu.Lock()
	defer m.leafMu.Unlock()
	switch kind {
	case "birth":
		if m.retained != nil || !n.physical.NeverResumed() {
			return ErrColdLeaf
		}
	case "stopped":
		if m.retained == nil || m.retained.leaf != leaf || m.retained.unknown || !n.physical.KnownJoined() {
			return ErrColdLeaf
		}
	case "closed":
		if m.retained == nil || m.retained.leaf != leaf || m.retained.unknown || !m.retained.closed || !n.physical.KnownClosed() {
			return ErrColdLeaf
		}
	default:
		return ErrColdLeaf
	}
	return nil
}

func (m *nativeColdLANMaster) describeLeafOperation(ctx context.Context, op *coldLANLeafOperation) (ColdLeafOfferRecord, error) {
	var record ColdLeafOfferRecord
	if op == nil || m.validateLeafOperation(ctx, op.leaf, op.kind) != nil {
		return record, ErrColdLeaf
	}
	if op.kind == "closed" {
		m.leafMu.Lock()
		defer m.leafMu.Unlock()
		if m.retained == nil || m.retained.leaf != op.leaf {
			return record, ErrColdLeaf
		}
		record.PID, record.Creation = m.retained.expected.PID, m.retained.expected.Creation
		return record, nil
	}
	query, pid, creation, err := op.leaf.NativeCohortProcess()
	if err != nil || query == 0 || pid == 0 || creation == 0 {
		return record, ErrColdLeaf
	}
	record.Process, record.PID, record.Creation = uint64(query), pid, creation
	return record, nil
}

func (m *nativeColdLANMaster) acknowledgeLeafOperation(ctx context.Context, op *coldLANLeafOperation, ack ColdLeafAckRecord) error {
	if op == nil || m.validateLeafOperation(ctx, op.leaf, op.kind) != nil {
		return ErrColdLeaf
	}
	m.leafMu.Lock()
	defer m.leafMu.Unlock()
	if op.kind == "closed" {
		if ack.Process != 0 || m.retained == nil || m.retained.leaf != op.leaf || !m.retained.closed {
			return ErrColdLeaf
		}
		m.retained = nil // all SDK copies already checked closed; no descriptor retry
		return nil
	}
	if ack.Process == 0 || ack.Process == uint64(^uintptr(0)) || uint64(uintptr(ack.Process)) != ack.Process {
		return ErrColdLeaf
	}
	query, pid, creation, err := op.leaf.NativeCohortProcess()
	if err != nil {
		return ErrColdLeaf
	}
	rights := uint32(windows.PROCESS_QUERY_INFORMATION | windows.SYNCHRONIZE)
	if op.kind == "birth" {
		if m.retained != nil {
			return ErrColdLeaf
		}
		r := &nativeColdLeafRetention{master: m, leaf: op.leaf, expected: transitionProcess{pid, creation, m.control.authority.session, m.control.authority.grant.Scope.SDKImage}, unknown: true}
		m.retained = r // own partial output BEFORE duplicate or following errors
		if windows.DuplicateHandle(m.supervisor.process, windows.Handle(ack.Process), windows.CurrentProcess(), &r.process, rights, false, 0) != nil || r.process == 0 || sameTransitionNativeObject(query, r.process) != nil || transitionLeafProcessRights(r.process) != nil || transitionProcessMatch(r.process, r.expected, false) != nil || ctx.Err() != nil {
			return ErrColdLeaf
		}
		r.unknown = false
		return nil
	}
	r := m.retained
	if op.kind != "stopped" || r == nil || r.master != m || r.leaf != op.leaf || r.unknown || r.stopped || r.expected.PID != pid || r.expected.Creation != creation {
		return ErrColdLeaf
	}
	r.unknown = true // failed stopped roundtrip cannot be retried
	if windows.DuplicateHandle(m.supervisor.process, windows.Handle(ack.Process), windows.CurrentProcess(), &r.stoppedReturn, rights, false, 0) != nil || r.stoppedReturn == 0 || sameTransitionNativeObject(r.process, r.stoppedReturn) != nil || sameTransitionNativeObject(query, r.stoppedReturn) != nil || transitionLeafProcessRights(r.stoppedReturn) != nil || transitionProcessMatch(r.process, r.expected, true) != nil || ctx.Err() != nil {
		return ErrColdLeaf
	}
	var exit uint32
	if windows.GetExitCodeProcess(r.process, &exit) != nil {
		return ErrColdLeaf
	}
	r.stopped, r.unknown = true, false
	return nil
}

func (m *nativeColdLANMaster) verifyRetained(leaf *LANObservationLeaf, ctx context.Context) error {
	if _, err := m.leafOwner(ctx, leaf); err != nil {
		return err
	}
	m.leafMu.Lock()
	defer m.leafMu.Unlock()
	r := m.retained
	if r == nil || r.master != m || r.leaf != leaf || r.unknown || r.stopped || r.closeAttempted || r.process == 0 {
		return ErrColdLeaf
	}
	query, pid, creation, err := leaf.NativeCohortProcess()
	if err != nil || pid != r.expected.PID || creation != r.expected.Creation || sameTransitionNativeObject(query, r.process) != nil || transitionLeafProcessRights(r.process) != nil || transitionProcessMatch(r.process, r.expected, false) != nil {
		return ErrColdLeaf
	}
	return nil
}

func (m *nativeColdLANMaster) RecordClosed(ctx context.Context, leaf *LANObservationLeaf) error {
	n, err := m.leafOwner(ctx, leaf)
	if err != nil || !n.physical.KnownClosed() {
		return ErrColdLeaf
	}
	m.leafMu.Lock()
	r := m.retained
	if r == nil || r.master != m || r.leaf != leaf || r.unknown || !r.stopped || r.closeAttempted || r.process == 0 || r.stoppedReturn == 0 {
		m.leafMu.Unlock()
		return ErrColdLeaf
	}
	r.closeAttempted, r.closeErr, r.unknown = true, ErrColdLeaf, true
	for _, slot := range []*windows.Handle{&r.stoppedReturn, &r.process} {
		h := *slot
		*slot = 0
		if windows.CloseHandle(h) != nil {
			r.unknownCloses = append(r.unknownCloses, h)
			m.leafMu.Unlock()
			return ErrColdLeaf
		}
	}
	r.closed, r.unknown, r.closeErr = true, false, nil
	m.leafMu.Unlock()
	return m.pump.request(ctx, leaf, "closed")
}

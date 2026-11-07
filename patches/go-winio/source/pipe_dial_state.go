package winio

import (
	"errors"
	"syscall"
)

var errOriginalDialUnknown = errors.New("original pipe birth or IOCP completion unknown")
var errOriginalDialOwner = errors.New("original pipe operation identity or lifecycle refused")

type originalDialPhase struct {
	entered, returned, unknown, handedOff, closed bool
	closeAttempted                                bool
	closeErr                                      error
}

func (s *originalDialPhase) enter() bool {
	if s.unknown || s.handedOff || s.closed || s.entered && !s.returned {
		s.unknown = true
		return false
	}
	s.entered = true
	s.returned = false
	return true
}
func (s *originalDialPhase) complete() {
	if !s.entered || s.returned {
		s.unknown = true
	}
	s.returned = true
}
func (s *originalDialPhase) rejectable() bool {
	return !s.unknown && !s.handedOff && (!s.entered || s.returned)
}
func (s *originalDialPhase) beginClose() bool {
	if s.closeAttempted || !s.rejectable() {
		return false
	}
	s.closeAttempted = true
	s.closeErr = errOriginalDialUnknown
	return true
}
func (s *originalDialPhase) closedReply(err error) {
	if !s.closeAttempted || s.closed || s.unknown {
		s.unknown = true
		return
	}
	if err != nil {
		s.unknown = true
		s.closeErr = err
		return
	}
	s.closed = true
	s.closeErr = nil
}

// Only this documented completed no-output result permits another CreateFile.
func originalDialBusy(raw, invalid uintptr, last error) bool {
	e, ok := last.(syscall.Errno)
	return raw == invalid && ok && e == 231
}
func originalDialHandle(raw, invalid uintptr) bool {
	return raw != 0 && raw != invalid && raw != invalid-1 && raw != invalid-2
}
func originalDialBOOL(raw uintptr) bool                  { return uint32(raw) != 0 }
func originalDialAssociation(raw, expected uintptr) bool { return expected != 0 && raw == expected }

// One original singleton frame is prearmed before its existing sync.Once call.
// The state cannot certify a consumed Once with no positive native result.
type originalIOInitState struct {
	attempted, entered, returned, ready, unknown, processor bool
	err                                                     error
}

func (s *originalIOInitState) begin() bool {
	if s.attempted {
		return false
	}
	s.attempted = true
	s.err = errOriginalDialUnknown
	s.entered = true
	return true
}
func (s *originalIOInitState) publish(raw, invalid uintptr) bool {
	s.returned = true
	if !s.attempted || s.unknown || !originalDialHandle(raw, invalid) {
		s.unknown = true
		return false
	}
	return true
}
func (s *originalIOInitState) result() error {
	if s.unknown || !s.ready || !s.processor {
		if s.err != nil {
			return s.err
		}
		return errOriginalDialUnknown
	}
	return nil
}

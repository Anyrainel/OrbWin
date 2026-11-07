//go:build windows

package winio

import (
	"errors"
	"sync"
	"syscall"
)

var errOriginalCloseUnknown = errors.New("winio: original handle close completion unknown")

// This private ledger is DATA, not handle ownership. The real win32File holds
// its mutex through its one concrete native close and result publication.
type originalFileCloseState struct {
	mu        sync.Mutex
	attempted bool
	refused   bool
	closed    bool
	err       error
}

// Caller holds mu. Prearm failure before any cancellation, join or native call.
func (s *originalFileCloseState) begin() bool {
	if s.attempted {
		return false
	}
	s.attempted = true
	s.err = errOriginalCloseUnknown
	return true
}

func (s *originalFileCloseState) refuse(err error) {
	if !s.attempted || s.closed || s.refused {
		return
	}
	s.refused = true
	if err == nil {
		err = errOriginalCloseUnknown
	}
	s.err = err
}

// Only the actual owner calls this AFTER its raw positive reply and clearing
// its owned handle. A failed/panicked attempt cannot later publish success.
func (s *originalFileCloseState) publishClosed() bool {
	if !s.attempted || s.refused || s.closed {
		return false
	}
	s.closed = true
	s.err = nil
	return true
}

func (s *originalFileCloseState) result() error {
	if !s.attempted || !s.closed && s.err == nil {
		return errOriginalCloseUnknown
	}
	return s.err
}

// A Win32 BOOL is low32. LastError is irrelevant on success; preserve the
// actual error on failure, including pending, without treating it as release.
func originalFileCloseReply(raw uintptr, lastError error) error {
	if uint32(raw) != 0 {
		return nil
	}
	if code, ok := lastError.(syscall.Errno); ok && code != 0 {
		return code
	}
	return errOriginalCloseUnknown
}

//go:build windows

package sdkself

import (
	"golang.org/x/sys/windows"
	"os"
)

// Actual checked extra-self-lease closure, never Root/cohort retirement. Only
// CloseForJoinedSource constructs it from a live native self lease before Close.
type JoinedLeaseClose struct {
	lease          *Lease
	state          *leaseState
	identity       Identity
	unknownFiles   []joinedFileCloseWitness
	unknownHandles []windows.Handle
	attempted      bool
}

// Diagnostic only: Go's FD.destroy overwrites Sysfd even if CloseHandle fails.
// Never close, query, duplicate, or use this captured number as authority.
type joinedFileCloseWitness struct {
	file   *os.File
	native windows.Handle
}

// Private native-effect adapter edge. Public receipt construction still needs
// a genuine native self lease and positive completion of the entire close.
func (r *JoinedLeaseClose) closeFile(w joinedFileCloseWitness, perform func() error) error {
	completed := false
	defer func() {
		if !completed {
			r.unknownFiles = append(r.unknownFiles, w)
		}
	}()
	if w.file == nil || perform == nil {
		return errLease
	}
	if perform() != nil {
		return errLease
	}
	completed = true
	return nil
}

func (l *Lease) CloseForJoinedSource() (*JoinedLeaseClose, error) {
	if l == nil || l.leaseState == nil {
		return nil, errLease
	}
	l.mu.Lock()
	previous := l.joinedClose
	l.mu.Unlock()
	if previous != nil {
		_, err := previous.Backing(l)
		return previous, err
	}
	identity, err := l.Identity()
	if err != nil {
		return nil, errLease
	}
	l.mu.Lock()
	if l.closed || l.caller != nil || l.joinedClose != nil || !SameIdentity(identity, l.identity) {
		l.mu.Unlock()
		return nil, errLease
	}
	r := &JoinedLeaseClose{lease: l, state: l.leaseState, identity: identity}
	l.joinedClose = r // actual owner stored BEFORE one native close; no retry
	defer l.mu.Unlock()
	l.closed, l.closeErr = true, errLease // BEFORE native effects; panic never nil
	if r.closeExtraLocked() != nil {
		return r, errLease
	}
	l.closeComplete, l.closeErr = true, nil
	return r, nil
}

// Caller holds the actual lease mutex and owns the entry's extra self lease.
// This distinct close path stops at first uncertainty; ordinary Close is not
// changed. No caller callbacks/flags mint its positive completion.
func (r *JoinedLeaseClose) closeExtraLocked() error {
	if r == nil || r.lease == nil || r.state == nil || r.lease.leaseState != r.state || r.state.joinedClose != r || !r.state.closed || r.state.closeComplete || r.state.caller != nil || r.attempted {
		return errLease
	}
	r.attempted = true
	l := r.lease
	for i := len(l.held) - 1; i >= 0; i-- {
		file := l.held[i].file
		if file == nil {
			l.held[i] = object{}
			return errLease
		}
		// All extra-lease file I/O uses this lease mutex; actual runtime/leaf
		// owners are already joined. No borrower can close or change this FD.
		w := joinedFileCloseWitness{file: file, native: windows.Handle(file.Fd())}
		l.held[i] = object{} // consume AFTER raw capture, BEFORE native Close
		if w.native == 0 || w.native == windows.InvalidHandle {
			r.unknownFiles = append(r.unknownFiles, w)
			return errLease
		}
		if r.closeFile(w, file.Close) != nil {
			return errLease
		}
	}
	if l.process == 0 {
		return errLease
	}
	h := l.process
	l.process = 0
	completed := false
	defer func() {
		if !completed {
			r.unknownHandles = append(r.unknownHandles, h)
		}
	}()
	if windows.CloseHandle(h) != nil {
		return errLease
	}
	completed = true
	return nil
}

func (r *JoinedLeaseClose) Backing(l *Lease) (Identity, error) {
	if r == nil || l == nil || l.leaseState == nil || r.lease != l || r.state != l.leaseState {
		return Identity{}, errLease
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.joinedClose != r || !r.attempted || !l.closed || !l.closeComplete || l.closeErr != nil || l.caller != nil || !SameIdentity(r.identity, l.identity) {
		return Identity{}, errLease
	}
	return r.identity, nil
}

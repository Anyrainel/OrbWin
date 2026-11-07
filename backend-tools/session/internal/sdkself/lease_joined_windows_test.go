//go:build windows

package sdkself

import (
	"golang.org/x/sys/windows"
	"os"
	"testing"
)

func TestColdJoinedFileUnknownPreservesPreCloseRawTupleOnErrorAndPanic(t *testing.T) {
	for _, mode := range []string{"error", "panic", "success"} {
		t.Run(mode, func(t *testing.T) {
			r := &JoinedLeaseClose{}
			file := &os.File{} // inert wrapper: native method is never called
			raw := windows.Handle(711)
			calls := 0
			func() {
				defer func() {
					if p := recover(); p != nil && mode != "panic" {
						t.Fatal(p)
					}
				}()
				_ = r.closeFile(joinedFileCloseWitness{file: file, native: raw}, func() error {
					calls++
					raw = windows.InvalidHandle // model Go destroy's post-close invalidation
					if mode == "panic" {
						panic("native close unknown")
					}
					if mode == "error" {
						return errLease
					}
					return nil
				})
			}()
			if calls != 1 {
				t.Fatal("native edge not one attempt")
			}
			if mode == "success" {
				if len(r.unknownFiles) != 0 {
					t.Fatal("known close retained false unknown")
				}
				return
			}
			if len(r.unknownFiles) != 1 || r.unknownFiles[0].file != file || r.unknownFiles[0].native != windows.Handle(711) {
				t.Fatal("actual pre-close wrapper/raw tuple lost")
			}
		})
	}
}

func TestColdJoinedLeaseRequiresNormalCheckedCloseNotClosedFlag(t *testing.T) {
	l := &Lease{leaseState: &leaseState{held: []object{{}}}}
	// A nil os.File returns ErrInvalid without a native call. Actual failed
	// Close latches a failure before completion; it cannot mint a closed receipt.
	if l.Close() == nil || !l.closed || l.closeComplete || l.closeErr == nil {
		t.Fatal("failed close cached completion")
	}
	r := &JoinedLeaseClose{lease: l, state: l.leaseState}
	l.joinedClose = r
	if _, err := r.Backing(l); err == nil {
		t.Fatal("closed flag became checked-close authority")
	}
	if _, err := l.CloseForJoinedSource(); err == nil || l.closeComplete {
		t.Fatal("unknown close replayed into success")
	}
	if _, err := r.Backing(&Lease{leaseState: l.leaseState}); err == nil {
		t.Fatal("copied lease wrapper borrowed receipt")
	}
}

func TestColdJoinedExtraCloseStopsAndNeverReplaysRemainingExclusions(t *testing.T) {
	l := &Lease{leaseState: &leaseState{closed: true, closeErr: errLease, held: []object{{id: "later"}, {id: "first"}}}}
	r := &JoinedLeaseClose{lease: l, state: l.leaseState}
	l.joinedClose = r
	// Missing file is an inert edge failure BEFORE any native call. The actual
	// private close coordinator consumes that slot and must preserve the next.
	if r.closeExtraLocked() == nil || !r.attempted || l.held[1].id != "" || l.held[0].id != "later" || l.closeComplete {
		t.Fatal("close did not stop at first uncertainty")
	}
	if r.closeExtraLocked() == nil || l.held[0].id != "later" {
		t.Fatal("native close attempt replayed")
	}
	if _, err := l.CloseForJoinedSource(); err == nil || l.held[0].id != "later" {
		t.Fatal("public retry consumed remaining exclusion")
	}
}

package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type inertWriterAdmission [32]byte

func (a inertWriterAdmission) binding() [32]byte { return [32]byte(a) }

type inertClientTree struct {
	trace                                                                           []string
	waits                                                                           int
	firstWaitErr, errorWait, errorEmpty, errorIO, errorClose, copyErr, terminateErr error
	code                                                                            uint32
}

func (t *inertClientTree) waitRoot(context.Context) (uint32, error) {
	t.trace = append(t.trace, "root")
	t.waits++
	if t.waits == 1 && t.firstWaitErr != nil {
		return 0, t.firstWaitErr
	}
	return t.code, t.errorWait
}
func (t *inertClientTree) terminate() error {
	t.trace = append(t.trace, "terminate")
	return t.terminateErr
}
func (t *inertClientTree) empty() (bool, error) {
	t.trace = append(t.trace, "empty")
	return true, t.errorEmpty
}
func (t *inertClientTree) finishIO(context.Context) (error, error) {
	t.trace = append(t.trace, "io-join")
	return t.copyErr, t.errorIO
}
func (t *inertClientTree) close() error { t.trace = append(t.trace, "close"); return t.errorClose }
func TestClientRetirementTokenRequiresRootTreeIOAndPositiveClose(t *testing.T) {
	admission := inertWriterAdmission{1}
	for _, fault := range []string{"root", "tree", "io", "close", "none", "exit-failed", "copy-failed", "cancelled", "terminate-failed"} {
		t.Run(fault, func(t *testing.T) {
			tree := &inertClientTree{}
			bad := errors.New("inert uncertainty")
			unknown := false
			switch fault {
			case "root":
				tree.errorWait = bad
			case "tree":
				tree.errorEmpty = bad
			case "io":
				tree.errorIO = bad
			case "close":
				tree.errorClose = bad
			case "exit-failed":
				tree.code = 17
			case "copy-failed":
				tree.copyErr = bad
			case "cancelled":
				tree.firstWaitErr = context.Canceled
			case "terminate-failed":
				tree.terminateErr = bad
			}
			token, err := retireClientTree(context.Background(), tree, admission, func() { unknown = true })
			positive := fault == "none" || fault == "exit-failed"
			if positive {
				if token == nil || unknown {
					t.Fatal("positive retirement lost", err)
				}
				if token.consume([32]byte{2}) || !token.consume(admission.binding()) || token.consume(admission.binding()) {
					t.Fatal("token binding/replay violated")
				}
			} else if fault == "cancelled" || fault == "copy-failed" || fault == "terminate-failed" {
				if token != nil || unknown || err == nil {
					t.Fatal("known cleanup granted interrupted writer settlement", token, err)
				}
			} else if token != nil || !unknown || err == nil {
				t.Fatal("unknown minted retirement", token, err)
			}
			if fault == "none" && !reflect.DeepEqual(tree.trace, []string{"root", "terminate", "empty", "io-join", "close"}) {
				t.Fatal(tree.trace)
			}
			if fault == "cancelled" && !reflect.DeepEqual(tree.trace, []string{"root", "terminate", "root", "empty", "io-join", "close"}) {
				t.Fatal("cancellation did not rewait actual root", tree.trace)
			}
			if fault != "none" && err == nil {
				t.Fatal("semantic failure hidden")
			}
		})
	}
}
func TestClientWriterAdmissionExclusiveAndUnknownCannotRetry(t *testing.T) {
	var owner clientOwnership
	if !owner.begin() || owner.begin() {
		t.Fatal("concurrent writer admission")
	}
	owner.end()
	if !owner.begin() {
		t.Fatal("positive empty failed admission did not release")
	}
	owner.retain(3)
	owner.end()
	if owner.begin() {
		t.Fatal("live resources released admission")
	}
	owner.markUnknown()
	if owner.begin() {
		t.Fatal("unknown replayed admission")
	}
	var zero retiredClientTree
	if zero.consume([32]byte{}) {
		t.Fatal("zero token minted authority")
	}
}

func TestClientNeverStartedGrantIsDistinctBoundAndOneUse(t *testing.T) {
	admission := inertWriterAdmission{9}
	cause := errors.New("inert precreation failure")
	var start *writerStartError
	if errors.As(clientNeverStartedError(cause, admission, false), &start) {
		t.Fatal("unknown cleanup minted unstarted grant")
	}
	wrapped := clientNeverStartedError(cause, admission, true)
	if !errors.Is(wrapped, cause) || !errors.As(wrapped, &start) {
		t.Fatal("known producer lost typed grant")
	}
	token := start.takeNeverStarted()
	if token == nil || start.takeNeverStarted() != nil || token.consume([32]byte{3}) || !token.consume(admission.binding()) || token.consume(admission.binding()) {
		t.Fatal("unstarted binding/replay boundary violated")
	}
}

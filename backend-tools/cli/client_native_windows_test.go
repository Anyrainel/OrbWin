//go:build windows

package main

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"os/exec"
	"reflect"
	"sync/atomic"
	"syscall"
	"testing"
	"unsafe"
)

func TestClientAtomicStartupWindowsABIAndBoundedEnvironment(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("qualified ABI is Windows amd64")
	}
	if unsafe.Sizeof(windows.StartupInfoEx{}) != 112 || unsafe.Offsetof(windows.StartupInfoEx{}.ProcThreadAttributeList) != 104 || unsafe.Sizeof(windows.ProcessInformation{}) != 24 || unsafe.Sizeof(windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}) != 144 || clientAttributeJobList != 0x2000d || clientAttributeHandleList != 0x20002 {
		t.Fatal("Windows atomic startup ABI differs")
	}
	block, err := clientEnvironmentBlock([]string{"A=one", "B=two"})
	if err != nil {
		t.Fatal(err)
	}
	want := []uint16{'A', '=', 'o', 'n', 'e', 0, 'B', '=', 't', 'w', 'o', 0, 0}
	if !reflect.DeepEqual(block, want) {
		t.Fatal("environment block framing", block)
	}
	if block, err = clientEnvironmentBlock(nil); err != nil || !reflect.DeepEqual(block, []uint16{0, 0}) {
		t.Fatal("empty environment framing")
	}
	if _, err = clientEnvironmentBlock([]string{"A=bad\x00tail"}); err == nil {
		t.Fatal("environment NUL accepted")
	}
}

type inertClaimAdmission struct {
	value   [32]byte
	claimed *atomic.Bool
}

func (a inertClaimAdmission) binding() [32]byte { return a.value }
func (a inertClaimAdmission) claim() ([32]byte, bool) {
	return a.value, a.claimed.CompareAndSwap(false, true)
}
func (a inertClaimAdmission) validateInvocation(*exec.Cmd) error {
	return errors.New("inert invocation deliberately refused")
}
func TestClientInvocationRefusalReturnsBoundNoStartBeforeNativeAdmission(t *testing.T) {
	admission := inertClaimAdmission{value: [32]byte{8}, claimed: &atomic.Bool{}}
	token, err := runClientWriter(context.Background(), nil, admission)
	var start *writerStartError
	if token != nil || !errors.As(err, &start) {
		t.Fatal("invocation refusal lost prebirth distinction", err)
	}
	proof := start.takeNeverStarted()
	if proof == nil || !proof.consume(admission.binding()) || proof.consume(admission.binding()) {
		t.Fatal("invalid invocation grant was unbound or reusable")
	}
	if retainedJobs.active || len(retainedJobs.resources) != 0 || retainedJobs.isUnknown() {
		t.Fatal("invalid invocation reached native resource admission")
	}
}
func TestClientCancelledProducerClaimsAdmissionOnceAndReturnsOnlyNoChildGrant(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	admission := inertClaimAdmission{value: [32]byte{6}, claimed: &atomic.Bool{}}
	duplicate := admission
	retired, err := runClientWriter(ctx, nil, admission)
	var start *writerStartError
	if retired != nil || !errors.Is(err, context.Canceled) || !errors.As(err, &start) {
		t.Fatal("known precreation cancellation lost grant", err)
	}
	if token := start.takeNeverStarted(); token == nil || !token.consume(admission.binding()) {
		t.Fatal("cancelled producer wrong binding")
	}
	_, err = runClientWriter(ctx, nil, duplicate)
	var replay *writerStartError
	if err == nil || errors.As(err, &replay) {
		t.Fatal("copied/reused admission minted second unstarted grant")
	}
}

func TestClientIOJoinDistinguishesCopyFailureFromUnjoinedWriter(t *testing.T) {
	copyFailure := errors.New("inert destination write failure")
	done := make(chan error, 1)
	done <- copyFailure
	tree := &nativeClientTree{ios: []*nativeClientIO{{done: done, started: true}}}
	copyErr, joinErr := tree.finishIO(context.Background())
	if !errors.Is(copyErr, copyFailure) || joinErr != nil {
		t.Fatal("joined output failure became tree uncertainty", copyErr, joinErr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tree = &nativeClientTree{ios: []*nativeClientIO{{done: make(chan error), started: true}}}
	if _, err := tree.finishIO(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("unjoined arbitrary writer reported positive", err)
	}
}

func TestClientNativePreparationRejectsUnsupportedBeforeAnyNativeAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var owner clientOwnership
	if tree, err := createNativeClient(ctx, &exec.Cmd{Path: `C:\inert.exe`, Args: []string{"inert"}}, &owner); tree != nil || err != context.Canceled {
		t.Fatal("cancelled preparation admitted")
	}
	for _, cmd := range []*exec.Cmd{nil, {Path: "relative.exe", Args: []string{"inert"}}, {Path: `C:\inert.exe`, Args: []string{"inert"}, SysProcAttr: &syscall.SysProcAttr{CreationFlags: windows.CREATE_BREAKAWAY_FROM_JOB}}} {
		if tree, err := createNativeClient(context.Background(), cmd, &owner); tree != nil || err == nil {
			t.Fatal("unsupported command admitted")
		}
	}
	if len(owner.resources) != 0 || owner.unknown {
		t.Fatal("pre-native validation touched resources")
	}
}

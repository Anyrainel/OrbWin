//go:build windows

package winio

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

// Compile-only during native pause. These actual guards must refuse before
// resolving a DLL, calling native code, or closing any invented test handle.
func TestOriginalPipeDialRejectsReboundSlotBeforeClose(t *testing.T) {
	var slot PipeDialOperation
	o := &pipeDialOperation{slot: &slot, file: &win32File{handle: windows.Handle(7)}}
	o.self = o
	o.file.dial = o
	slot = o
	slot = nil
	if o.Close() == nil || !o.phase.unknown || o.file.closeEntered || o.file.handle != 7 {
		t.Fatal("rebound slot admitted")
	}
}
func TestOriginalPipeDialUnknownAndHandoffNeverClose(t *testing.T) {
	for _, handed := range []bool{false, true} {
		var slot PipeDialOperation
		o := &pipeDialOperation{slot: &slot, file: &win32File{handle: windows.Handle(7)}}
		o.self = o
		o.file.dial = o
		slot = o
		o.phase.unknown = !handed
		o.phase.handedOff = handed
		if o.Close() == nil || o.file.closeEntered || o.file.handle != 7 {
			t.Fatal("unknown/handed-off close")
		}
	}
}

func TestOriginalPipeDialCachedCloseResultSurvivesUnknown(t *testing.T) {
	failure := errors.New("original checked close failure")
	var slot PipeDialOperation
	o := &pipeDialOperation{slot: &slot, file: &win32File{handle: windows.Handle(7)}}
	o.self = o
	o.file.dial = o
	slot = o
	o.phase.closeAttempted = true
	o.phase.unknown = true
	o.phase.closeErr = failure
	if o.Close() != failure || o.Close() != failure || o.file.closeEntered || o.file.handle != 7 {
		t.Fatal("original error replay lost")
	}
}

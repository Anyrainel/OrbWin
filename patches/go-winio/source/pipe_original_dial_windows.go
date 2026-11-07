//go:build windows

package winio

import (
	"context"
	"net"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PipeDialOperation retains an original pipe birth, including nil-Conn failure.
// Its actual private issuer is checked by identity. It grants no product permission.
// The original slot passed to Into must remain immutable for its whole lifetime.
type PipeDialOperation interface {
	Close() error
	originalPipeDial() *pipeDialOperation
}

type pipeDialOperation struct {
	mu        sync.Mutex
	self      *pipeDialOperation
	slot      *PipeDialOperation
	phase     originalDialPhase
	file      *win32File // sole original handle cell, allocated before any native effect
	path      string
	name      []uint16
	access    uint32
	flags     uint32
	openRaw   uintptr
	openError error
	infoFlags uint32
	infoRaw   uintptr
	infoError error
	conn      net.Conn // same original file; successful facade is sole closer
}

func (o *pipeDialOperation) originalPipeDial() *pipeDialOperation { return o }
func (o *pipeDialOperation) original() bool {
	return o != nil && o.self == o && o.slot != nil && *o.slot == o && o.file != nil && o.file.dial == o
}
func (o *pipeDialOperation) guard() bool {
	if !o.original() || o.phase.unknown || originalIOCP.failed.Load() {
		o.phase.unknown = true
		return false
	}
	return true
}
func (o *pipeDialOperation) Close() error {
	if o == nil {
		return errOriginalDialOwner
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.original() {
		o.phase.unknown = true
		return errOriginalDialOwner
	}
	// A replay observes the original result, never current ambient state. It
	// performs no new effect and cannot reset failed/unknown completion.
	if o.phase.closeAttempted {
		return o.phase.closeErr
	}
	if !o.guard() || o.phase.handedOff {
		return errOriginalDialUnknown
	}
	if !o.phase.beginClose() {
		return errOriginalDialUnknown
	}
	defer func() {
		if p := recover(); p != nil {
			o.phase.unknown = true
			runtime.KeepAlive(o)
			panic(p)
		}
	}()
	var err error
	if o.file.handle != 0 {
		err = o.file.Close()
	}
	o.phase.closedReply(err)
	runtime.KeepAlive(o)
	return o.phase.closeErr
}

var originalPipeCreate = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreateFileW")
var originalPipeInfo = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetNamedPipeInfo")

// DialPipeAccessImpLevelInto publishes the actual operation/file in original
// before all native work. A failed operation must be retained and settled even
// when no Conn is returned. A successful Conn is the exclusive native closer.
// This bounded variant requires a deadline; old Dial APIs remain unchanged.
func DialPipeAccessImpLevelInto(ctx context.Context, path string, access uint32, impLevel PipeImpLevel, original *PipeDialOperation) (conn net.Conn, err error) {
	if original == nil || *original != nil || ctx == nil {
		return nil, errOriginalDialOwner
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, errOriginalDialOwner
	}
	switch impLevel {
	case PipeImpLevelAnonymous, PipeImpLevelIdentification, PipeImpLevelImpersonation, PipeImpLevelDelegation:
	default:
		return nil, errOriginalDialOwner
	}
	name, e := windows.UTF16FromString(path)
	if e != nil || len(name) < 2 {
		return nil, errOriginalDialOwner
	}
	o := &pipeDialOperation{slot: original, file: &win32File{}, path: path, name: name, access: access, flags: windows.FILE_FLAG_OVERLAPPED | windows.SECURITY_SQOS_PRESENT | uint32(impLevel)}
	o.self = o
	o.file.dial = o
	*original = o
	o.mu.Lock()
	defer o.mu.Unlock()
	defer func() {
		if p := recover(); p != nil {
			o.phase.unknown = true
			runtime.KeepAlive(o)
			panic(p)
		}
		if !o.guard() {
			conn = nil
			err = errOriginalDialUnknown
		}
		runtime.KeepAlive(o)
	}()
	for {
		if !o.guard() {
			return nil, errOriginalDialUnknown
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if !o.phase.enter() {
			return nil, errOriginalDialUnknown
		}
		o.openRaw, _, o.openError = originalPipeCreate.Call(uintptr(unsafe.Pointer(&o.name[0])), uintptr(o.access), 0, 0, uintptr(windows.OPEN_EXISTING), uintptr(o.flags), 0)
		o.phase.complete()
		if !o.guard() {
			return nil, errOriginalDialUnknown
		}
		if originalDialBusy(o.openRaw, uintptr(windows.InvalidHandle), o.openError) {
			// Completed no-output only. One fixed frame, no unbounded retry history.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Millisecond):
				continue
			}
		}
		if !originalDialHandle(o.openRaw, uintptr(windows.InvalidHandle)) {
			o.phase.unknown = true
			return nil, errOriginalDialUnknown
		}
		o.file.handle = windows.Handle(o.openRaw)
		break
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if !o.guard() || !o.phase.enter() {
		return nil, errOriginalDialUnknown
	}
	o.infoRaw, _, o.infoError = originalPipeInfo.Call(uintptr(o.file.handle), uintptr(unsafe.Pointer(&o.infoFlags)), 0, 0, 0)
	o.phase.complete()
	if !o.guard() || !originalDialBOOL(o.infoRaw) {
		o.phase.unknown = true
		return nil, errOriginalDialUnknown
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if !o.guard() || !o.phase.enter() {
		return nil, errOriginalDialUnknown
	}
	err = registerOriginalFile(o.file)
	o.phase.complete()
	if err != nil || !o.guard() {
		o.phase.unknown = true
		return nil, errOriginalDialUnknown
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if o.infoFlags&windows.PIPE_TYPE_MESSAGE != 0 {
		o.conn = &win32MessageBytePipe{win32Pipe: win32Pipe{win32File: o.file, path: o.path}}
	} else {
		o.conn = &win32Pipe{win32File: o.file, path: o.path}
	}
	if !o.guard() {
		return nil, errOriginalDialUnknown
	}
	o.phase.handedOff = true
	return o.conn, nil
}

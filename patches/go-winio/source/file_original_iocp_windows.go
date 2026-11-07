//go:build windows

package winio

import (
	"runtime"
	"sync/atomic"

	"golang.org/x/sys/windows"
)

var originalIOCPCreate = windows.NewLazySystemDLL("kernel32.dll").NewProc("CreateIoCompletionPort")
var originalIOCPNotify = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetFileCompletionNotificationModes")

// This is the SAME existing singleton and processor. No other port is created
// after failure. Its original native reply remains rooted for process lifetime.
type originalIOCPFrame struct {
	state     originalIOInitState
	raw       uintptr
	lastError error
	port      windows.Handle
	failed    atomic.Bool
}

var originalIOCP originalIOCPFrame

type originalFileRegistration struct {
	singleton                               *originalIOCPFrame // same process-lifetime borrower published before init
	port                                    windows.Handle     // borrower, never a closer
	associationEntered, associationReturned bool
	associationRaw                          uintptr
	associationError                        error
	notifyEntered, notifyReturned           bool
	notifyRaw                               uintptr
	notifyError                             error
	unknown                                 bool
}

func ensureOriginalIO() error {
	ioInitOnce.Do(initIO)
	if originalIOCP.failed.Load() {
		return errOriginalDialUnknown
	}
	if originalIOCP.state.result() != nil || originalIOCP.port == 0 || ioCompletionPort != originalIOCP.port {
		return errOriginalDialUnknown
	}
	return nil
}

func initOriginalIO() {
	if !originalIOCP.state.begin() {
		originalIOCP.failed.Store(true)
		return
	}
	defer func() {
		if p := recover(); p != nil {
			originalIOCP.state.unknown = true
			originalIOCP.failed.Store(true)
			panic(p)
		}
	}()
	originalIOCP.raw, _, originalIOCP.lastError = originalIOCPCreate.Call(uintptr(windows.InvalidHandle), 0, 0, 0xffffffff)
	if !originalIOCP.state.publish(originalIOCP.raw, uintptr(windows.InvalidHandle)) {
		originalIOCP.failed.Store(true)
		return
	}
	originalIOCP.port = windows.Handle(originalIOCP.raw)
	ioCompletionPort = originalIOCP.port
	// Publication and launch cannot panic into a later default-success Once.
	originalIOCP.state.processor = true
	go ioCompletionProcessor(originalIOCP.port)
	originalIOCP.state.ready = true
	originalIOCP.state.err = nil
}

// f and its original output cells must already have a real caller lifetime root.
// Legacy makeWin32File callers retain their historical caller-custody gaps.
func originalRegistrationOwner(f *win32File) bool {
	return f != nil && (f.dial == nil || f.dial.file == f && f.dial.guard())
}
func registerOriginalFile(f *win32File) (err error) {
	r := &f.registration
	defer func() {
		if p := recover(); p != nil {
			r.unknown = true
			originalIOCP.failed.Store(true)
			runtime.KeepAlive(f)
			panic(p)
		}
		runtime.KeepAlive(f)
	}()
	if !originalRegistrationOwner(f) || f.handle == 0 || f.handle == windows.InvalidHandle || r.associationEntered || r.notifyEntered {
		return errOriginalDialOwner
	}
	r.singleton = &originalIOCP
	if err = ensureOriginalIO(); err != nil {
		return err
	}
	if !originalRegistrationOwner(f) || r.singleton != &originalIOCP {
		return errOriginalDialUnknown
	}
	r.port = originalIOCP.port
	r.associationEntered = true
	r.associationRaw, _, r.associationError = originalIOCPCreate.Call(uintptr(f.handle), uintptr(r.port), 0, 0xffffffff)
	r.associationReturned = true
	if !originalRegistrationOwner(f) || !originalDialAssociation(r.associationRaw, uintptr(r.port)) {
		r.unknown = true
		originalIOCP.failed.Store(true)
		return errOriginalDialUnknown
	}
	r.notifyEntered = true
	r.notifyRaw, _, r.notifyError = originalIOCPNotify.Call(uintptr(f.handle), uintptr(windows.FILE_SKIP_COMPLETION_PORT_ON_SUCCESS|windows.FILE_SKIP_SET_EVENT_ON_HANDLE))
	r.notifyReturned = true
	if !originalRegistrationOwner(f) || !originalDialBOOL(r.notifyRaw) {
		r.unknown = true
		originalIOCP.failed.Store(true)
		return errOriginalDialUnknown
	}
	if originalIOCP.failed.Load() {
		r.unknown = true
		return errOriginalDialUnknown
	}
	f.readDeadline.channel = make(timeoutChan)
	f.writeDeadline.channel = make(timeoutChan)
	return nil
}

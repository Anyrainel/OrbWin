//go:build windows && amd64

package lanhelper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SDK22621 ShellExecuteExW layout/type assertions are compiled without linking
// in artifacts/qualification/p2-lan-helper-channel-source-f0cbb71a.
type shellExecuteInfo struct {
	size, mask                        uint32
	parent                            uintptr
	verb, file, parameters, directory *uint16
	show                              int32
	instance, idList, class, key      uintptr
	hotKey                            uint32
	icon                              uintptr
	process                           windows.Handle
}

var executeElevated = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// No Terminate right is assumed over another administrator's helper. Only the
// helper's own watchdog terminates it; this owner retains the original process
// handle until actual positive exit. Caller timeout/cancel never closes done.
type elevatedProcess struct {
	process  windows.Handle
	pid      uint32
	creation string
	image    *os.File
	done     chan struct{}
	mu       sync.Mutex
	result   error
	waitOnce sync.Once
	waiting  bool
}

func retainElevatedProcess(process windows.Handle) *elevatedProcess {
	child := &elevatedProcess{process: process, done: make(chan struct{})}
	return child
}
func (child *elevatedProcess) beginWait() {
	child.waitOnce.Do(func() {
		child.mu.Lock()
		child.waiting = true
		child.mu.Unlock()
		go func() {
			state, err := windows.WaitForSingleObject(child.process, windows.INFINITE)
			if err != nil || state != uint32(windows.WAIT_OBJECT_0) {
				child.mu.Lock()
				child.result = errors.New("exact elevated process exit remains unknown")
				child.mu.Unlock()
				return
			} // Handle/lifetime deliberately retained.
			close(child.done)
		}()
	})
}
func (child *elevatedProcess) Wait(ctx context.Context) error {
	if child == nil || ctx == nil {
		return errors.New("retained elevated process and wait context required")
	}
	child.beginWait()
	select {
	case <-child.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Only the ordinary launcher's original, non-waited handle is closed here.
// The SDK must duplicate/retain its own handle BEFORE sending any review. If
// handoff is lost, the helper receives no authority and self-watchdogs. Closing
// this source is never a claim that the elevated process exited.
func (child *elevatedProcess) closeLauncherSource() error {
	child.mu.Lock()
	defer child.mu.Unlock()
	if child.waiting {
		return errors.New("cannot transfer a handle with a native wait in progress")
	}
	var err error
	if child.image != nil {
		err = child.image.Close()
		child.image = nil
	}
	if child.process != 0 {
		err = errors.Join(err, windows.CloseHandle(child.process))
		child.process = 0
	}
	return err
}
func (child *elevatedProcess) release() error {
	select {
	case <-child.done:
	default:
		return errors.New("cannot release an unjoined elevated helper")
	}
	var err error
	if child.image != nil {
		err = child.image.Close()
		child.image = nil
	}
	if child.process != 0 {
		err = errors.Join(err, windows.CloseHandle(child.process))
		child.process = 0
	}
	return err
}

// Called ONLY after the actor consumes explicit activation/removal review.
// Construction/discovery, refresh, drift and shutdown never call this function.
// The UAC broker's synchronous wait is not context-cancellable; a late result
// remains owned, while the cancelled SDK closes its fixed channel before any
// late helper can obtain authority. No broad argument/command/path API exists.
func launchElevatedHelper(ctx context.Context, authority *AuthorizedInstallation, transaction string) (child *elevatedProcess, err error) {
	if ctx == nil || authority == nil || authority.worker == nil || !recordID(transaction) {
		return nil, errors.New("signed held native helper and transaction required")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	err = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE)
	if err != nil && err != syscall.Errno(1) {
		return nil, errors.New("native elevation apartment unavailable")
	}
	defer windows.CoUninitialize()
	if err = executeElevated.Find(); err != nil {
		return nil, errors.New("native explicit elevation unavailable")
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	helperPath := filepath.Join(filepath.Dir(authority.manager.path), "lan", "orbwin-lan-helper.exe")
	file, _ := windows.UTF16PtrFromString(helperPath)
	parameters, _ := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{authority.installation, transaction}))
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	directory, _ := windows.UTF16PtrFromString(system)
	info := shellExecuteInfo{size: uint32(unsafe.Sizeof(shellExecuteInfo{})), mask: 0x40 | 0x100, verb: verb, file: file, parameters: parameters, directory: directory, show: 0}
	var pinned runtime.Pinner
	for _, pointer := range []any{&info, verb, file, parameters, directory} {
		pinned.Pin(pointer)
	}
	defer pinned.Unpin()
	result, _, _ := executeElevated.Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(info)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(file)
	runtime.KeepAlive(parameters)
	runtime.KeepAlive(directory)
	if info.process != 0 {
		child = retainElevatedProcess(info.process)
	}
	if result == 0 || child == nil {
		return child, errors.New("explicit elevation was declined, failed or returned no owned process")
	}
	pid, err := windows.GetProcessId(info.process)
	if err != nil || pid == 0 {
		return child, errors.New("elevated process identity unavailable")
	}
	child.pid = pid
	// Exact process-handle incarnation and image are checked again against the
	// actual pipe endpoint before any OwnerReview is sent.
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(info.process, &created, &exited, &kernel, &user); err != nil {
		return child, errors.New("elevated process incarnation unavailable")
	}
	child.creation = strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(info.process, 0, &buffer[0], &size); err != nil || size == 0 || size >= uint32(len(buffer)) || windows.UTF16ToString(buffer[:size]) != helperPath {
		return child, errors.New("elevated process is not the held signed helper path")
	}
	child.image, err = heldFile(helperPath, false, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
	if err != nil {
		return child, err
	}
	sha, err := imageDigest(child.image)
	if err != nil || sha != authority.assets.Assets[1].SHA256 {
		return child, errors.New("elevated process image differs from signed helper asset")
	}
	if err = ctx.Err(); err != nil {
		return child, err
	}
	return child, nil
}

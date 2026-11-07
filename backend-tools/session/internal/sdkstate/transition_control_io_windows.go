//go:build windows

package sdkstate

import (
	"context"
	"golang.org/x/sys/windows"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

// The SAME control FILE_OBJECT crosses the serving-owner handoff. It must never
// be wrapped in os.File/WinIO or associated with an IOCP in either process.
// Borrowed pipe ownership remains with the enclosing native launch owner. One
// bounded event-based operation is admitted at a time; unknown completion pins
// its buffer/OVERLAPPED/event in this owner forever rather than freeing memory
// while the kernel may still use it. No process termination is performed here.
type transitionControlIO struct {
	mu      sync.Mutex
	pipe    windows.Handle
	op      *transitionControlOperation
	unknown bool
	closing bool
}
type transitionControlOperation struct {
	ov     windows.Overlapped
	buffer []byte
	pinned runtime.Pinner
}

// Process-lifetime memory quarantine, NEVER an authority source. A caller may
// drop its returned owner after an error; the kernel's pending buffer pointers
// must still outlive GC/Pinner finalizers. Unknown entries have no removal API.
var transitionControlQuarantine = struct {
	sync.Mutex
	owners map[*transitionControlIO]struct{}
}{owners: map[*transitionControlIO]struct{}{}}

func (p *transitionControlIO) perform(ctx context.Context, data []byte, write bool) (int, error) {
	if p == nil || ctx == nil || ctx.Err() != nil || len(data) == 0 || len(data) > 64<<10 {
		return 0, ErrRecoveryRequired
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return 0, ErrRecoveryRequired
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	// The closed request/reply pump has one native I/O owner. Refuse accidental
	// concurrent callers immediately, without waiting past their cancellation.
	if !p.mu.TryLock() {
		return 0, ErrRecoveryRequired
	}
	defer func() {
		if p.unknown || p.op != nil {
			p.unknown = true
			transitionControlQuarantine.Lock()
			transitionControlQuarantine.owners[p] = struct{}{}
			transitionControlQuarantine.Unlock()
		}
		p.mu.Unlock()
	}()
	if p.pipe == 0 || p.pipe == windows.InvalidHandle || p.unknown || p.closing || p.op != nil {
		return 0, ErrRecoveryRequired
	}
	o := &transitionControlOperation{buffer: make([]byte, len(data))}
	p.op = o // Retain every partial operation before admission.
	var err error
	o.ov.HEvent, err = windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		p.unknown = true
		return 0, ErrRecoveryRequired
	}
	if write {
		copy(o.buffer, data)
	}
	o.pinned.Pin(&o.ov)
	o.pinned.Pin(&o.buffer[0])
	// Use the native overlapped API with NULL byte-count as documented. The
	// pinned x/sys convenience wrappers dereference that pointer under -race;
	// their synchronous byte-count instrumentation is not this async lifetime.
	name := "ReadFile"
	if write {
		name = "WriteFile"
	}
	accepted, _, nativeErr := windows.NewLazySystemDLL("kernel32.dll").NewProc(name).Call(uintptr(p.pipe), uintptr(unsafe.Pointer(&o.buffer[0])), uintptr(len(o.buffer)), 0, uintptr(unsafe.Pointer(&o.ov)))
	err = nil
	if accepted == 0 {
		err = nativeErr
	}
	if err != nil && err != windows.ERROR_IO_PENDING {
		// Native refusal before asynchronous admission has no outstanding I/O.
		o.pinned.Unpin()
		if windows.CloseHandle(o.ov.HEvent) != nil {
			p.unknown = true
			return 0, ErrRecoveryRequired
		}
		p.op, p.unknown = nil, true
		return 0, ErrRecoveryRequired
	}
	canceled := false
	var cleanupDeadline time.Time
	for {
		if ctx.Err() != nil && !canceled {
			canceled = true
			cleanupDeadline = time.Now().Add(2 * time.Second)
			if cancelErr := windows.CancelIoEx(p.pipe, &o.ov); cancelErr != nil && cancelErr != windows.ERROR_NOT_FOUND {
				p.unknown = true
				return 0, ErrRecoveryRequired // Original pinned operation remains reachable.
			}
		}
		state, waitErr := windows.WaitForSingleObject(o.ov.HEvent, 25)
		if waitErr != nil || state != uint32(windows.WAIT_OBJECT_0) && state != uint32(windows.WAIT_TIMEOUT) {
			p.unknown = true
			return 0, ErrRecoveryRequired
		}
		if state == uint32(windows.WAIT_OBJECT_0) {
			var count uint32
			resultErr := windows.GetOverlappedResult(p.pipe, &o.ov, &count, false)
			if resultErr == windows.ERROR_IO_INCOMPLETE {
				p.unknown = true
				return 0, ErrRecoveryRequired
			}
			// The private unsignaled event is kernel-completed. Only now may the
			// pinned operation memory be released, including canceled operations.
			o.pinned.Unpin()
			if windows.CloseHandle(o.ov.HEvent) != nil {
				p.unknown = true
				return 0, ErrRecoveryRequired
			}
			p.op = nil
			if count > uint32(len(data)) || resultErr != nil || canceled || count == 0 {
				p.unknown = true
				return 0, ErrRecoveryRequired
			}
			if !write {
				copy(data, o.buffer[:count])
			}
			clear(o.buffer)
			return int(count), nil
		}
		if canceled && time.Now().After(cleanupDeadline) {
			p.unknown = true
			return 0, ErrRecoveryRequired
		}
	}
}

func (p *transitionControlIO) prepareClose() error {
	if p == nil || !p.mu.TryLock() {
		return ErrRecoveryRequired
	}
	defer p.mu.Unlock()
	if p.pipe == 0 || p.unknown || p.op != nil || p.closing {
		return ErrRecoveryRequired
	}
	p.closing = true
	return nil
}

func (p *transitionControlIO) closeFinal() error {
	if p == nil || !p.mu.TryLock() {
		return ErrRecoveryRequired
	}
	defer p.mu.Unlock()
	if !p.closing || p.unknown || p.op != nil || p.pipe == 0 {
		return ErrRecoveryRequired
	}
	if windows.CloseHandle(p.pipe) != nil {
		p.unknown = true
		transitionControlQuarantine.Lock()
		transitionControlQuarantine.owners[p] = struct{}{}
		transitionControlQuarantine.Unlock()
		return ErrRecoveryRequired
	}
	p.pipe = 0
	return nil
}

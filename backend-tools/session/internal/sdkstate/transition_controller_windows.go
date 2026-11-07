//go:build windows

package sdkstate

import (
	"context"
	"golang.org/x/sys/windows"
	"runtime"
	"sync"
	"unsafe"
)

// ColdSupervisorOffer is created only from an actual committed transition and
// its retained native candidate. These are duplicates of the SAME file objects,
// never reopened paths. Both SDK and candidate must keep their copies until
// actual root exit, descendant-job empty and pending-I/O join permit retirement.
// No destructor/Close drops a partial remote duplicate or its return proof.
type ColdSupervisorOffer struct {
	mu                         sync.Mutex
	backend                    *nativeColdBackend
	transition                 *ColdTransition
	target                     *transitionPendingSupervisor
	original, remote, returned [4]windows.Handle
	unknown, verified          bool
	supervisorAccepted         bool
}

// The controller roundtrip is not a final release grant. This remains closed
// until the actual candidate acknowledges all held exclusions and the SDK
// positively switches its logical supervisor; the candidate must then retain
// them through SDK root exit, job empty and I/O join. No caller flag can open it.
func (o *ColdSupervisorOffer) prepareOwnerRelease() error {
	if o == nil {
		return ErrRecoveryRequired
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.unknown || !o.verified || !o.supervisorAccepted || o.target == nil {
		return ErrRecoveryRequired
	}
	return nil
}

// AcceptSupervisor follows the candidate's positive native retention operation
// on the same authenticated control pipe. VerifyRetained has retained actual
// return objects before this one-way switch; no caller Boolean selects an owner.
func (o *ColdSupervisorOffer) AcceptSupervisor(ctx context.Context) error {
	if o == nil || o.VerifyRetained(ctx) != nil {
		return ErrRecoveryRequired
	}
	t := o.transition
	t.mu.Lock()
	defer t.mu.Unlock()
	t.root.s.mu.Lock()
	defer t.root.s.mu.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()
	if t.closed || t.unknown || t.root.s.closed || t.root.s.poison || o.unknown || !o.verified || o.supervisorAccepted || o.backend.check(ctx) != nil {
		return ErrRecoveryRequired
	}
	a := o.backend.authority
	a.supervisorMu.Lock()
	defer a.supervisorMu.Unlock()
	if a.logicalSupervisor != nil || a.pendingSupervisor != o.target || a.recheckPendingSupervisorLocked() != nil || ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	a.logicalSupervisor = o.target
	o.supervisorAccepted = true
	return nil
}

// NativeHandles describes only this actual one-use remote duplication. The
// four fixed roles are new writable controller, old read-only controller, old
// legacy root, and sealed denial. These numbers are not an admission API.
func (o *ColdSupervisorOffer) NativeHandles() ([4]uint64, error) {
	var result [4]uint64
	if o == nil {
		return result, ErrRecoveryRequired
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.unknown || o.backend == nil || o.target == nil {
		return result, ErrRecoveryRequired
	}
	for n, h := range o.remote {
		if h == 0 || h == windows.InvalidHandle {
			return [4]uint64{}, ErrRecoveryRequired
		}
		result[n] = uint64(h)
	}
	return result, nil
}

func (t *ColdTransition) OfferNativeSupervisor(ctx context.Context) (*ColdSupervisorOffer, error) {
	if t == nil || !bounded(ctx) {
		return nil, ErrState
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	b, ok := t.native.(*nativeColdBackend)
	if !ok || !t.committed || t.closed || t.unknown || t.root == nil || t.root.s == nil {
		return nil, ErrRecoveryRequired
	}
	t.root.s.mu.Lock()
	defer t.root.s.mu.Unlock()
	if t.root.s.active != nil {
		return nil, ErrBusy
	}
	if t.root.s.closed || t.root.s.poison || b.supervisorOffer != nil || b.commitRecord == nil || b.commitRecord.check() != nil || b.check(ctx) != nil || b.r.owner == nil {
		return nil, ErrRecoveryRequired
	}
	a := b.authority
	a.supervisorMu.RLock()
	defer a.supervisorMu.RUnlock()
	if a.pendingSupervisor == nil || a.recheckPendingSupervisorLocked() != nil {
		return nil, ErrRecoveryRequired
	}
	o := &ColdSupervisorOffer{backend: b, transition: t, target: a.pendingSupervisor, unknown: true}
	b.supervisorOffer = o // retain the owner BEFORE the first native duplicate
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// root guard excludes record transactions/Close. owner.mu also excludes a
	// native record close while obtaining this exact original controller handle.
	b.r.owner.mu.Lock()
	defer b.r.owner.mu.Unlock()
	if b.r.owner.closed || b.r.owner.o.file == nil || b.r.owner.o.recheck() != nil {
		return o, ErrRecoveryRequired
	}
	o.original = [4]windows.Handle{windows.Handle(b.r.owner.o.file.Fd()), a.controller, a.legacy, windows.Handle(a.grant.Denial)}
	for n, h := range o.original {
		if h == 0 || h == windows.InvalidHandle || windows.DuplicateHandle(windows.CurrentProcess(), h, o.target.process, &o.remote[n], 0, false, windows.DUPLICATE_SAME_ACCESS) != nil || o.remote[n] == 0 || o.remote[n] == windows.InvalidHandle {
			return o, ErrRecoveryRequired
		}
	}
	if a.recheckPendingSupervisorLocked() != nil || ctx.Err() != nil {
		return o, ErrRecoveryRequired
	}
	o.unknown = false
	return o, nil
}

// VerifyRetained is only called after a positive operation on the original
// authenticated control pipe by the candidate's one protocol owner. It proves
// retained remote kernel objects, not logical supervisor acceptance or retirement.
// Each remote return handle is retained BEFORE an error can escape; no retry can
// replace an unknown roundtrip with another native object.
func (o *ColdSupervisorOffer) VerifyRetained(ctx context.Context) error {
	if o == nil || o.transition == nil || !bounded(ctx) {
		return ErrState
	}
	t := o.transition
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.root == nil || t.root.s == nil || t.closed || t.unknown {
		return ErrRecoveryRequired
	}
	t.root.s.mu.Lock()
	defer t.root.s.mu.Unlock()
	if t.root.s.closed || t.root.s.poison {
		return ErrRecoveryRequired
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.unknown || o.verified || o.backend == nil || o.target == nil {
		return ErrRecoveryRequired
	}
	b, a := o.backend, o.backend.authority
	if b.check(ctx) != nil {
		return ErrRecoveryRequired
	}
	a.supervisorMu.RLock()
	defer a.supervisorMu.RUnlock()
	if a.pendingSupervisor != o.target || a.recheckPendingSupervisorLocked() != nil {
		return ErrRecoveryRequired
	}
	o.unknown = true
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	compare := windows.NewLazySystemDLL("kernelbase.dll").NewProc("CompareObjectHandles")
	if compare.Find() != nil {
		return ErrRecoveryRequired
	}
	for n, remote := range o.remote {
		if windows.DuplicateHandle(o.target.process, remote, windows.CurrentProcess(), &o.returned[n], 0, false, windows.DUPLICATE_SAME_ACCESS) != nil || o.returned[n] == 0 || o.returned[n] == windows.InvalidHandle {
			return ErrRecoveryRequired
		}
		same, _, _ := compare.Call(uintptr(o.original[n]), uintptr(o.returned[n]))
		// Same FILE_OBJECT preserves its original sharing reservation. Granted
		// access is per handle, so compare it separately along with file mode.
		if same == 0 || sameTransitionFileHandleCaps(o.original[n], o.returned[n]) != nil {
			return ErrRecoveryRequired
		}
	}
	if a.recheckPendingSupervisorLocked() != nil || ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	o.verified, o.unknown = true, false
	return nil
}

func sameTransitionFileHandleCaps(original, returned windows.Handle) error {
	// FileAccessInformation belongs to the FILE_OBJECT, so it cannot prove a
	// duplicate handle's granted access. Query the actual handle table grants.
	objectQuery := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryObject")
	var firstObject, secondObject [14]uint32 // PUBLIC_OBJECT_BASIC_INFORMATION
	var length uint32
	code, _, _ := objectQuery.Call(uintptr(original), 0, uintptr(unsafe.Pointer(&firstObject)), unsafe.Sizeof(firstObject), uintptr(unsafe.Pointer(&length)))
	if code != 0 {
		return ErrRecoveryRequired
	}
	code, _, _ = objectQuery.Call(uintptr(returned), 0, uintptr(unsafe.Pointer(&secondObject)), unsafe.Sizeof(secondObject), uintptr(unsafe.Pointer(&length)))
	if code != 0 || firstObject[1] != secondObject[1] {
		return ErrRecoveryRequired
	}
	query := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationFile")
	for _, class := range []uint32{16} { // FileModeInformation
		var first, second uint32
		var status windows.IO_STATUS_BLOCK
		code, _, _ := query.Call(uintptr(original), uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&first)), 4, uintptr(class))
		if code != 0 {
			return ErrRecoveryRequired
		}
		code, _, _ = query.Call(uintptr(returned), uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&second)), 4, uintptr(class))
		if code != 0 || first != second {
			return ErrRecoveryRequired
		}
	}
	return nil
}

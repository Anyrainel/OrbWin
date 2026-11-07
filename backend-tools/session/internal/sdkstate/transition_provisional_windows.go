//go:build windows

package sdkstate

import (
	"context"
	"golang.org/x/sys/windows"
	"unsafe"
)

// ProvisionNativeSupervisor is reachable only with the actual cold transition
// and its one original authenticated control. A record by itself cannot mint
// a process/Root/supervisor capability. TEMP duplicates its actual suspended
// candidate PI handle into this SDK and must receive native retention ACK before
// Resume; final-slot matching and logical switch are separate later operations.
func (t *ColdTransition) ProvisionNativeSupervisor(ctx context.Context, control *ColdControl, record ColdSupervisorRecord) error {
	if t == nil || !bounded(ctx) || control == nil || !record.Valid() {
		return ErrState
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	b, ok := t.native.(*nativeColdBackend)
	if !ok || b == nil || t.committed || t.closed || t.unknown || t.root == nil || t.root.s == nil {
		return ErrRecoveryRequired
	}
	t.root.s.mu.Lock()
	defer t.root.s.mu.Unlock()
	a := b.authority
	if a == nil || control.authority != a || control.io == nil || control.io != a.controlIO || !a.controlTaken || b.r == nil || t.root.s.closed || t.root.s.poison || t.root.s.active != nil || b.check(ctx) != nil {
		return ErrRecoveryRequired
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.check(ctx) != nil {
		return ErrRecoveryRequired
	}
	a.supervisorMu.Lock()
	defer a.supervisorMu.Unlock()
	if a.pendingSupervisor != nil || a.logicalSupervisor != nil {
		return ErrRecoveryRequired
	}
	p := transitionProcess{record.PID, record.Creation, record.Session, record.Image}
	a.unknown = true // only explicit fully verified completion clears this admission
	s := &transitionPendingSupervisor{process: windows.Handle(record.Process), expected: p, provisional: control, transition: t}
	a.pendingSupervisor = s // retain actual remote output before first inspection
	var made, ended, kernel, user windows.Filetime
	if windows.GetProcessTimes(windows.CurrentProcess(), &made, &ended, &kernel, &user) != nil || p.PID == windows.GetCurrentProcessId() || p.PID == a.grant.Parent.PID || p.Creation <= uint64(made.HighDateTime)<<32|uint64(made.LowDateTime) || p.Session != a.session || p.Image != a.grant.Parent.Image {
		return ErrRecoveryRequired
	}
	var flags uint32
	got, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation").Call(uintptr(s.process), uintptr(unsafe.Pointer(&flags)))
	var access [14]uint32
	var size uint32
	status, _, _ := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryObject").Call(uintptr(s.process), 0, uintptr(unsafe.Pointer(&access)), unsafe.Sizeof(access), uintptr(unsafe.Pointer(&size)))
	required := uint32(windows.PROCESS_QUERY_INFORMATION | windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.PROCESS_DUP_HANDLE | windows.SYNCHRONIZE)
	if got == 0 || flags&windows.HANDLE_FLAG_INHERIT != 0 || status != 0 || access[1] != required || transitionProcessMatch(s.process, p, false) != nil {
		return ErrRecoveryRequired
	}
	var basic windows.PROCESS_BASIC_INFORMATION
	if windows.NtQueryInformationProcess(s.process, 0, unsafe.Pointer(&basic), uint32(unsafe.Sizeof(basic)), &size) != nil || size != uint32(unsafe.Sizeof(basic)) || uint32(basic.UniqueProcessId) != p.PID || uint32(basic.InheritedFromUniqueProcessId) != a.grant.Parent.PID {
		return ErrRecoveryRequired
	}
	if transitionSupervisorOutsideJobs(s.process) != nil {
		return ErrRecoveryRequired
	}
	var err error
	s.token, err = a.holdProcessToken(s.process, 0)
	if err != nil || a.recheckPendingSupervisorLocked() != nil || ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	a.unknown = false
	return nil
}

func transitionSupervisorOutsideJobs(process windows.Handle) error {
	var member uint32
	status, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(process), 0, uintptr(unsafe.Pointer(&member)))
	if status == 0 || member != 0 {
		return ErrRecoveryRequired
	}
	return nil
}

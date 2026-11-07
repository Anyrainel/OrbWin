//go:build windows

package sdkstate

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

func transitionStableID(h windows.Handle) (string, error) {
	var info windows.ByHandleFileInformation
	if _, err := snapshot(h, false); err != nil {
		return "", err
	}
	if windows.GetFileInformationByHandle(h, &info) != nil {
		return "", ErrState
	}
	return fmt.Sprintf("%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}
func (a *nativeColdAuthority) holdFinalSlot(ctx context.Context) error {
	final := a.grant.Final
	h, event := windows.Handle(final.File), windows.Handle(final.Event)
	if final.Nonce == [32]byte{} || final.Object == "" || h == event || transitionInherited(h) != nil || transitionInherited(event) != nil {
		return ErrState
	}
	a.finalFile = os.NewFile(uintptr(h), "inherited SDK final commit slot")
	a.files = append(a.files, a.finalFile)
	a.finalEvent = event
	// This handle has only read access to a NEW FILE_OBJECT. A reduced duplicate
	// of a writable object is not accepted as an immutable-finalization proof.
	var access uint32
	var status windows.IO_STATUS_BLOCK
	query := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationFile")
	code, _, _ := query.Call(uintptr(h), uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&access)), 4, 8)
	if code != 0 || access&windows.FILE_READ_DATA == 0 || access&(windows.FILE_WRITE_DATA|windows.FILE_APPEND_DATA|windows.DELETE|windows.WRITE_DAC|windows.WRITE_OWNER) != 0 {
		return ErrState
	}
	// PUBLIC_OBJECT_BASIC_INFORMATION, pinned winnt.h/ntifs.h: Attributes,
	// GrantedAccess, HandleCount, PointerCount, Reserved[10].
	var basic [14]uint32
	var length uint32
	ntQuery := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryObject")
	code, _, _ = ntQuery.Call(uintptr(event), 0, uintptr(unsafe.Pointer(&basic)), unsafe.Sizeof(basic), uintptr(unsafe.Pointer(&length)))
	if code != 0 || basic[1] != windows.SYNCHRONIZE {
		return ErrState
	}
	id, err := transitionStableID(h)
	path, pathErr := transitionPath(h)
	info, statErr := a.finalFile.Stat()
	state, waitErr := windows.WaitForSingleObject(event, 0)
	if !bounded(ctx) || err != nil || id != final.Object || pathErr != nil || !strings.EqualFold(path, filepath.Join(a.profile, ".orbwin-private", "services.final-"+a.grant.Scope.Transaction+".grant")) || objectPrivate(h, a.sid, false) != nil || statErr != nil || info.Size() != 0 || waitErr != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return ErrRecoveryRequired
	}
	return nil
}
func (a *nativeColdAuthority) consumeFinal(ctx context.Context, receipt TransitionReceipt, parent windows.Handle) (transitionFinalRecord, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var record transitionFinalRecord
	if a.finalConsumed || a.finalFile == nil || a.finalEvent == 0 || a.check(ctx) != nil {
		return record, ErrRecoveryRequired
	}
	a.finalConsumed = true
	// Only the pinned parent's close-before-signal source may release this event.
	// Event/JSON by itself never establishes writer closure or authority.
	for {
		if a.check(ctx) != nil {
			return record, ErrRecoveryRequired
		}
		state, err := windows.WaitForSingleObject(a.finalEvent, 25)
		if err != nil {
			return record, ErrRecoveryRequired
		}
		if state == uint32(windows.WAIT_OBJECT_0) {
			break
		}
		if state != uint32(windows.WAIT_TIMEOUT) {
			return record, ErrRecoveryRequired
		}
	}
	// Retain our own deny-write FILE_OBJECT as well as the original inherited
	// identity. Parent death cannot leave a writable final record behind.
	h, err := ntOpen(parent, "services.final-"+receipt.Scope.Transaction+".grant", false, windows.FILE_OPEN, nil, false, false, false)
	if err != nil {
		return record, ErrRecoveryRequired
	}
	sealed := os.NewFile(uintptr(h), "sealed SDK final commit grant")
	a.files = append(a.files, sealed)
	id, err := transitionStableID(h)
	if err != nil || id != a.grant.Final.Object || objectPrivate(h, a.sid, false) != nil {
		return record, ErrRecoveryRequired
	}
	if _, err = sealed.Seek(0, io.SeekStart); err != nil {
		return record, ErrRecoveryRequired
	}
	body, err := io.ReadAll(io.LimitReader(sealed, 4097))
	if err != nil {
		return record, ErrRecoveryRequired
	}
	digest, err := receipt.Digest()
	if err != nil || a.check(ctx) != nil {
		return transitionFinalRecord{}, ErrRecoveryRequired
	}
	record, err = parseTransitionFinal(body, receipt.Scope, digest, a.grant.Final.Nonce, a.grant.Parent)
	if err != nil || a.holdPendingSupervisor(ctx, record) != nil {
		return transitionFinalRecord{}, ErrRecoveryRequired
	}
	return record, nil
}
func (b *nativeColdBackend) commit(ctx context.Context, receipt TransitionReceipt) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if b.check(ctx) != nil || !b.finished {
		return ErrRecoveryRequired
	}
	want, err := b.receipt.Digest()
	got, other := receipt.Digest()
	if err != nil || other != nil || want != got {
		return ErrRecoveryRequired
	}
	b.unknown = true
	final, err := b.authority.consumeFinal(ctx, receipt, b.r.parent)
	if err != nil {
		return ErrRecoveryRequired
	}
	marker := transitionCommitMarker{1, receipt, final}
	data, err := json.Marshal(marker)
	if err != nil || len(data) > 2<<20 {
		return ErrRecoveryRequired
	}
	record, err := b.r.createFile(ctx, b.r.parent, "services.commit-"+receipt.Scope.Transaction, data)
	if err != nil || record == nil || record.check() != nil {
		return ErrRecoveryRequired
	}
	b.commitRecord = record
	b.unknown = false
	return b.check(ctx)
}

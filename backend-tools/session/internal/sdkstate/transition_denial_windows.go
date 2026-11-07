//go:build windows

package sdkstate

import (
	"crypto/sha256"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

func denialNativeObject(h windows.Handle) (transitionDenialObject, uint32, error) {
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil {
		return transitionDenialObject{}, 0, ErrState
	}
	return transitionDenialObject{info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow}, info.FileAttributes, nil
}
func (a *nativeColdAuthority) consumeDenial() error {
	h := windows.Handle(a.grant.Denial)
	if transitionInherited(h) != nil || objectPrivate(h, a.sid, false) != nil {
		return ErrState
	}
	f := os.NewFile(uintptr(h), "held persistent SDK denial receipt")
	a.files = append(a.files, f)
	path, err := transitionPath(h)
	if err != nil || !strings.EqualFold(path, filepath.Join(a.profile, ".orbwin-private", "services.denial-"+a.grant.Scope.Transaction+"-sealed")) {
		return ErrState
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return ErrState
	}
	body, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return ErrState
	}
	root, _, err := denialNativeObject(a.legacy)
	if err != nil {
		return ErrState
	}
	lock, attrs, err := denialNativeObject(a.controller)
	if err != nil {
		return ErrState
	}
	record, err := parseTransitionDenial(body, a.grant.Scope, root, lock, attrs)
	if err != nil {
		return err
	}
	var access uint32
	var status windows.IO_STATUS_BLOCK
	query := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationFile")
	code, _, _ := query.Call(uintptr(a.controller), uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&access)), 4, 8)
	if code != 0 || access&windows.FILE_READ_DATA == 0 || access&(windows.FILE_WRITE_DATA|windows.FILE_APPEND_DATA|windows.DELETE|windows.WRITE_DAC|windows.WRITE_OWNER) != 0 {
		return ErrState
	}
	a.denial = record
	return a.recheckDenial()
}

func (a *nativeColdAuthority) recheckDenial() error {
	if a.denial.Schema != 1 || a.denial.Transaction != a.grant.Scope.Transaction || a.denial.Stage != "readonly-owner-acquired" {
		return ErrRecoveryRequired
	}
	root, _, err := denialNativeObject(a.legacy)
	if err != nil || root != a.denial.Root {
		return ErrRecoveryRequired
	}
	lock, attrs, err := denialNativeObject(a.controller)
	if err != nil || lock != a.denial.Controller || attrs != a.denial.After {
		return ErrRecoveryRequired
	}
	for _, entry := range []struct {
		h    windows.Handle
		dir  bool
		hash [32]byte
	}{{a.legacy, true, a.denial.RootAfterACL}, {a.controller, false, a.denial.ControllerAfterACL}} {
		if objectPrivate(entry.h, a.sid, entry.dir) != nil {
			return ErrRecoveryRequired
		}
		sd, err := windows.GetSecurityInfo(entry.h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil || sd == nil || sd.Length() < 20 || sd.Length() > 16<<10 || sha256.Sum256([]byte(sd.String())) != entry.hash {
			return ErrRecoveryRequired
		}
	}
	return nil
}

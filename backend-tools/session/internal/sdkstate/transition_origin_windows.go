//go:build windows

package sdkstate

import (
	"context"
	"encoding/hex"
	"golang.org/x/sys/windows"
	"unsafe"
)

// This is an observation of a LIVE owner, never an execution lease. The file
// and process are borrowed from authority-owned handles, retained through final
// joined Close. A matching path/hash does not establish the process's image.
// ProcessImageFileMapping (44) compares the supplied FILE_OBJECT with the
// process's original image; it does not extract a section or map/execute code.
// Root's standard-user frozen v2 primitive qualified this live comparison.
func transitionMappedImage(process, image windows.Handle) error {
	if process == 0 || process == windows.InvalidHandle || image == 0 || image == windows.InvalidHandle {
		return ErrState
	}
	argument := image
	var length uint32
	status, _, _ := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationProcess").Call(uintptr(process), 44, uintptr(unsafe.Pointer(&argument)), unsafe.Sizeof(argument), uintptr(unsafe.Pointer(&length)))
	if status != 0 || argument != image {
		return ErrState
	}
	return nil
}

// The path is derived ONLY from the inherited authenticated launch tuple and
// the native Profile folder. Hold every ancestor without DELETE sharing, then
// open the fixed PE relative to that held graph. All private-role components
// keep the existing exact protected three-ACE policy. Partial admission retains
// every acquired object in a.files; unknown origin never silently releases it.
func (a *nativeColdAuthority) holdParentImage(ctx context.Context) error {
	code := &nativeRoot{sid: a.sid}
	defer func() {
		a.parentCode = code.chain
		for _, entry := range code.chain {
			a.files = append(a.files, entry.file)
		}
	}()
	if !bounded(ctx) || a.parentImage != 0 || a.parent == 0 || code.walkProfile(ctx, a.profile) != nil {
		return ErrState
	}
	for _, name := range []string{".orbwin-private", "runtime", "rdd-" + hex.EncodeToString(a.grant.Parent.Image[:])} {
		h, err := ntOpen(windows.Handle(code.chain[len(code.chain)-1].file.Fd()), name, true, windows.FILE_OPEN, nil, false, false, false)
		if err != nil || code.hold(h, true, true) != nil {
			return ErrState
		}
	}
	name, err := windows.NewNTUnicodeString("rdd.exe")
	if err != nil {
		return ErrState
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(code.chain[len(code.chain)-1].file.Fd()), ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var status windows.IO_STATUS_BLOCK
	var image windows.Handle
	err = windows.NtCreateFile(&image, windows.FILE_READ_DATA|windows.FILE_EXECUTE|windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL|windows.SYNCHRONIZE, &oa, &status, nil, 0, windows.FILE_SHARE_READ, windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil || code.hold(image, false, true) != nil {
		return ErrState
	}
	a.parentImage = image
	if transitionImageHash(ctx, image, a.grant.Parent.Image) != nil || transitionProcessMatch(a.parent, a.grant.Parent, false) != nil || transitionMappedImage(a.parent, image) != nil {
		return ErrState
	}
	return nil
}

func (a *nativeColdAuthority) recheckParentImage() error {
	if a.parent == 0 || a.parentImage == 0 || len(a.parentCode) < 5 {
		return ErrRecoveryRequired
	}
	for _, entry := range a.parentCode {
		if entry.recheck() != nil {
			return ErrRecoveryRequired
		}
	}
	// The launching owner must remain LIVE until a separate actual supervisor
	// transfer is accepted. Post-exit class 44 failure is expected; it cannot be
	// treated as an accepted handoff or as guest/resource retirement.
	if transitionProcessMatch(a.parent, a.grant.Parent, false) != nil || transitionMappedImage(a.parent, a.parentImage) != nil {
		return ErrRecoveryRequired
	}
	return nil
}

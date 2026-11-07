//go:build windows && amd64

package lanhelper

import (
	"errors"
	"path/filepath"
	"runtime"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The client explicitly uses SECURITY_IDENTIFICATION. The server may query the
// actual token SID/session/elevation without acquiring administrative execution
// authority. Revert occurs on the same locked thread before any other work.
var impersonatePipeClient = windows.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateNamedPipeClient")

func identifyPipeClient(pipe windows.Handle, wantElevated bool, session uint32) (string, error) {
	type observation struct {
		sid string
		err error
	}
	result := make(chan observation, 1)
	go func() {
		runtime.LockOSThread()
		sid, unlock, err := identifyPipeClientOnThread(pipe, wantElevated, session)
		if unlock {
			runtime.UnlockOSThread()
		}
		result <- observation{sid, err}
	}()
	got := <-result
	return got.sid, got.err
}
func identifyPipeClientOnThread(pipe windows.Handle, wantElevated bool, session uint32) (sid string, unlock bool, err error) {
	unlock = true
	var prior windows.Token
	if e := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &prior); e == nil {
		prior.Close()
		return "", false, errors.New("pre-existing SDK impersonation token refused")
	} else if !errors.Is(e, windows.ERROR_NO_TOKEN) {
		return "", false, errors.New("native SDK thread identity unavailable")
	}
	if impersonatePipeClient.Find() != nil {
		return "", true, errors.New("native pipe identification unavailable")
	}
	if ok, _, _ := impersonatePipeClient.Call(uintptr(pipe)); ok == 0 {
		return "", false, errors.New("native pipe identification unavailable")
	}
	unlock = false
	defer func() {
		if e := windows.RevertToSelf(); e != nil {
			err = errors.New("native pipe identification did not revert")
			sid = ""
		} else {
			unlock = true
		}
	}()
	var token windows.Token
	if err = windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &token); err != nil {
		return "", unlock, err
	}
	defer token.Close()
	var level, size uint32
	if err = windows.GetTokenInformation(token, windows.TokenImpersonationLevel, (*byte)(unsafe.Pointer(&level)), 4, &size); err != nil || size != 4 || level != uint32(windows.SecurityIdentification) {
		return "", unlock, errors.New("exact identification-only pipe token required")
	}
	sid, actualSession, err := nativeTokenFacts(token, wantElevated)
	if err != nil || actualSession != session {
		return "", unlock, errors.New("native pipe token session/elevation differs")
	}
	return sid, unlock, nil
}
func verifyLauncherPipe(pipe windows.Handle, launcher *ordinaryLauncher, authority *AuthorizedInstallation) error {
	var pid uint32
	if launcher == nil || windows.GetNamedPipeClientProcessId(pipe, &pid) != nil || pid != launcher.pid {
		return errors.New("launcher pipe does not belong to original created child")
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(launcher.process, &created, &exited, &kernel, &user) != nil || strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10) != launcher.creation {
		return errors.New("launcher original process incarnation differs")
	}
	sid, err := identifyPipeClient(pipe, false, authority.manager.session)
	if err != nil || sid != authority.manager.identity.UserSID {
		return errors.New("launcher actual native user differs")
	}
	return nil
}
func verifyElevatedPipe(pipe windows.Handle, child *elevatedProcess, authority *AuthorizedInstallation) error {
	var pid uint32
	if child == nil || child.process == 0 || windows.GetNamedPipeClientProcessId(pipe, &pid) != nil || pid != child.pid {
		return errors.New("helper pipe differs from retained native process")
	}
	state, err := windows.WaitForSingleObject(child.process, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return errors.New("retained native helper exited")
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(child.process, &created, &exited, &kernel, &user) != nil || strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10) != child.creation {
		return errors.New("helper retained process incarnation differs")
	}
	if _, err = identifyPipeClient(pipe, true, authority.manager.session); err != nil {
		return err
	} // Alternate administrator SID remains distinct and is permitted.
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	path := filepath.Join(filepath.Dir(authority.manager.path), "lan", "orbwin-lan-helper.exe")
	if windows.QueryFullProcessImageName(child.process, 0, &buffer[0], &size) != nil || size == 0 || size >= uint32(len(buffer)) || windows.UTF16ToString(buffer[:size]) != path {
		return errors.New("helper native image is not fixed signed sibling")
	}
	image, err := heldFile(path, false, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
	if err != nil {
		return err
	}
	sha, err := imageDigest(image)
	if err != nil || sha != authority.assets.Assets[1].SHA256 {
		image.Close()
		return errors.New("native helper image differs from signed held bundle")
	}
	child.image = image
	return nil
}

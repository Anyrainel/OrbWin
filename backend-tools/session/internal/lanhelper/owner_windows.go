//go:build windows

package lanhelper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanfirewall"
)

// NativeManager is minted only from a local pipe's actual server PID, held
// process/token/image observations. JSON identity fields cannot construct it.
// Keep it until the one-shot helper and all dispatched operations are joined.
type NativeManager struct {
	nativeOrigin    *NativeManager // Set by the actual allocation, before any partial native owner escapes.
	process         windows.Handle
	image           *os.File
	identity        lanfirewall.CallerIdentity
	session         uint32
	path            string
	pipe            windows.Handle // Borrowed from the owning one-shot session until join.
	closeOnce       sync.Once
	closeErr        error
	closeDiagnostic string
	cleanupPanic    any // Original panic from constructor cleanup; diagnostic only.
}

var errNativeOwnerCloseUnknown = errors.New("native owner close remains unconfirmed")
var errNativeOwnerCopy = errors.New("actual native owner required; copied or uninitialized owner refused")

func newNativeManager(process, pipe windows.Handle) *NativeManager {
	manager := &NativeManager{process: process, pipe: pipe}
	manager.nativeOrigin = manager
	return manager
}

func nativeToken(process windows.Handle, wantElevated bool) (string, uint32, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return "", 0, err
	}
	defer token.Close()
	return nativeTokenFacts(token, wantElevated)
}
func nativeTokenFacts(token windows.Token, wantElevated bool) (string, uint32, error) {
	var elevation, session, size uint32
	if err := windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), 4, &size); err != nil || size != 4 || (elevation != 0) != wantElevated {
		return "", 0, errors.New("native helper/manager elevation identity differs")
	}
	if err := windows.GetTokenInformation(token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4, &size); err != nil || size != 4 {
		return "", 0, errors.New("native token session unavailable")
	}
	user, err := token.GetTokenUser()
	if err != nil {
		return "", 0, err
	}
	return user.User.Sid.String(), session, nil
}
func heldFile(path string, directory bool, access, share uint32) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	h, err := windows.CreateFile(p, access, share, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &info); err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		windows.CloseHandle(h)
		return nil, errors.New("held native path kind/reparse refusal")
	}
	file := os.NewFile(uintptr(h), path)
	if file == nil {
		windows.CloseHandle(h)
		return nil, errors.New("held native file unavailable")
	}
	return file, nil
}
func imageDigest(image *os.File) (string, error) {
	stat, err := image.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > 256<<20 {
		return "", errors.New("bounded native regular executable required")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.NewSectionReader(image, 0, stat.Size()))
	if err != nil || n != stat.Size() {
		return "", errors.New("held executable digest incomplete")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func AuthenticateManagerPipe(pipe windows.Handle) (manager *NativeManager, err error) {
	return authenticateManagerPipe(pipe, true)
}
func authenticateManagerPipe(pipe windows.Handle, elevated bool) (manager *NativeManager, err error) {
	var pid uint32
	if err = windows.GetNamedPipeServerProcessId(pipe, &pid); err != nil || pid == 0 {
		return nil, errors.New("local fixed helper pipe server unavailable")
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil, err
	}
	manager = newNativeManager(process, pipe)
	owned := manager
	defer func() {
		if err != nil {
			manager, err = settleOrdinarySelfFailure(owned, err, owned.Close)
		}
	}()
	sid, session, err := nativeToken(process, false)
	if err != nil {
		return nil, err
	}
	currentSID, helperSession, err := nativeToken(windows.CurrentProcess(), elevated)
	if err != nil || helperSession != session || (!elevated && currentSID != sid) {
		return nil, errors.New("ordinary manager and elevated helper must share native session")
	}
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil {
		return nil, err
	}
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil || size == 0 || size >= uint32(len(buffer)) {
		return nil, errors.New("bounded native manager image path unavailable")
	}
	manager.path = windows.UTF16ToString(buffer[:size])
	manager.image, err = heldFile(manager.path, false, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
	if err != nil {
		return nil, err
	}
	sha, err := imageDigest(manager.image)
	if err != nil {
		return nil, err
	}
	manager.identity = lanfirewall.CallerIdentity{PID: pid, UserSID: sid, CreationTime: strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10), ImageSHA256: sha}
	manager.session = session
	if err = manager.checkPipe(pipe); err != nil {
		return nil, err
	}
	return manager, nil
}
func (manager *NativeManager) checkPipe(pipe windows.Handle) error {
	if manager == nil || manager.nativeOrigin != manager || manager.process == 0 {
		return errors.New("held manager required")
	}
	var pid uint32
	state, err := windows.WaitForSingleObject(manager.process, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) || windows.GetNamedPipeServerProcessId(pipe, &pid) != nil || pid != manager.identity.PID {
		return errors.New("held manager/pipe identity no longer matches")
	}
	return nil
}
func (manager *NativeManager) Close() error {
	if manager == nil {
		return nil
	}
	return manager.closeUsing(func() error {
		var result error
		var image uintptr
		if manager.image != nil {
			image = manager.image.Fd()
		}
		manager.closeDiagnostic = fmt.Sprintf("manager process=%#x image=%#x", manager.process, image)
		if manager.image != nil {
			result = manager.image.Close()
			manager.image = nil
		}
		if manager.process != 0 {
			result = errors.Join(result, windows.CloseHandle(manager.process))
			manager.process = 0
		}
		return result
	})
}

// The native body is fixed in Close. This small once boundary also lets pure
// fixtures prove that an uncertain close result never authorizes a retry.
func (manager *NativeManager) closeUsing(closeOwner func() error) error {
	if manager == nil || manager.nativeOrigin != manager {
		return errNativeOwnerCopy
	}
	manager.closeOnce.Do(func() {
		manager.closeErr = errNativeOwnerCloseUnknown // Cached before effects, including a body that panics.
		manager.closeErr = closeOwner()               // Only a normal return can publish success.
	})
	return manager.closeErr
}

type fixedRoleOwner interface{ Close() error }

// Only the two existing bounded one-shot roles use this fixed close ordering.
// An unknown authority cannot release its still-held native manager.
func settleFixedRoleOwners(authority, manager fixedRoleOwner) error {
	if authority != nil {
		if err := authority.Close(); err != nil {
			return err
		}
	}
	if manager != nil {
		return manager.Close()
	}
	return nil
}

func finishFixedRoleOwners(authority, manager fixedRoleOwner) {
	settled := false
	defer func() {
		if !settled {
			// Deliberately never unwind to the outer stopCap defer on unknown or
			// panic. The role's pre-existing 70/150s own-process cap stays armed.
			for {
				time.Sleep(time.Second)
				runtime.KeepAlive(authority)
				runtime.KeepAlive(manager)
			}
		}
	}()
	if settleFixedRoleOwners(authority, manager) != nil {
		return
	}
	settled = true
}

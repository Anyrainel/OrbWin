//go:build windows

package lanhelper

import (
	"errors"
	"golang.org/x/sys/windows"
	"strconv"
)

func holdOrdinarySelf() (manager *NativeManager, err error) {
	pid := windows.GetCurrentProcessId()
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil, err
	}
	manager = newNativeManager(process, 0)
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
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil {
		return nil, err
	}
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil || size == 0 || size >= uint32(len(buffer)) {
		return nil, errors.New("bounded ordinary native manager image unavailable")
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
	manager.identity.PID = pid
	manager.identity.UserSID = sid
	manager.identity.CreationTime = strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10)
	manager.identity.ImageSHA256 = sha
	manager.session = session
	return manager, nil
}

// Production binds this exact owner's Close; the method value is never stored
// or selected by IPC. Only that call's panic becomes a retained partial error.
func settleOrdinarySelfFailure(owned *NativeManager, failure error, closeOwner func() error) (partial *NativeManager, result error) {
	if owned != nil && owned.nativeOrigin != owned {
		return owned, errors.Join(failure, errNativeOwnerCopy)
	}
	partial, result = owned, failure
	closeReturned := false
	defer func() {
		if !closeReturned {
			owned.cleanupPanic = recover()
			partial, result = owned, errors.Join(failure, errNativeOwnerCloseUnknown)
		}
	}()
	closeErr := closeOwner() // Production callers bind the same actual owned.Close.
	closeReturned = true
	if closeErr != nil {
		return owned, errors.Join(failure, closeErr)
	}
	return nil, failure
}

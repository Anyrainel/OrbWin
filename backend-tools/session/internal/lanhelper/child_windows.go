//go:build windows && amd64

package lanhelper

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
)

// Child facts are re-observed from the original native handle/image, not
// accepted as authority from even the authenticated manager's JSON claims.
func holdPreparedChild(prepared Prepared, claimed lanfirewall.CallerIdentity, manager *NativeManager) (child *NativeManager, err error) {
	if claimed.PID == 0 || claimed.PID == manager.identity.PID {
		return nil, errors.New("distinct ordinary worker incarnation required")
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, claimed.PID)
	if err != nil {
		return nil, err
	}
	child = newNativeManager(process, 0)
	owned := child
	defer func() {
		if err != nil {
			child, err = settleOrdinarySelfFailure(owned, err, owned.Close)
		}
	}()
	sid, session, err := nativeToken(process, false)
	if err != nil || sid != manager.identity.UserSID || session != manager.session {
		return nil, errors.New("native worker token/session differs")
	}
	var creation, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(process, &creation, &exited, &kernel, &user); err != nil {
		return nil, err
	}
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil || size == 0 || size >= uint32(len(buffer)) {
		return nil, errors.New("bounded worker native image unavailable")
	}
	child.path = windows.UTF16ToString(buffer[:size])
	if child.path != prepared.Spec.Worker.Path {
		return nil, errors.New("worker native image is not the protected staged path")
	}
	child.image, err = heldFile(child.path, false, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
	if err != nil {
		return nil, err
	}
	descriptor, err := windows.SecurityDescriptorFromString(protectedSDDL(sid, false))
	if err != nil {
		return nil, err
	}
	if err = exactDescriptor(child.image, descriptor); err != nil {
		return nil, err
	}
	sha, err := imageDigest(child.image)
	if err != nil {
		return nil, err
	}
	child.identity = lanfirewall.CallerIdentity{PID: claimed.PID, UserSID: sid, CreationTime: strconv.FormatUint(uint64(creation.HighDateTime)<<32|uint64(creation.LowDateTime), 10), ImageSHA256: sha}
	child.session = session
	if child.identity != claimed || sha != prepared.Spec.Worker.SHA256 {
		return nil, errors.New("worker claimed/native incarnation differs")
	}
	var id stagedFileID
	if err = windows.GetFileInformationByHandleEx(windows.Handle(child.image.Fd()), windows.FileIdInfo, (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id))); err != nil {
		return nil, err
	}
	if hex.EncodeToString(id.ID[:]) != prepared.Spec.Worker.FileID || strconv.FormatUint(id.Volume, 10) != prepared.Spec.Worker.VolumeSerial {
		return nil, errors.New("worker native file identity differs")
	}
	state, err := windows.WaitForSingleObject(process, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return nil, errors.New("worker exited during native binding")
	}
	return child, nil
}
func validateNativeCommit(ctx context.Context, prepared Prepared, manager, child *NativeManager, observed lanlease.Observation) error {
	if manager.checkPipe(manager.pipe) != nil {
		return errors.New("native manager/pipe binding lost")
	}
	state, err := windows.WaitForSingleObject(child.process, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return errors.New("native worker no longer alive")
	}
	if observed.Manager != manager.identity || observed.Worker != child.identity || observed.WorkerFile != prepared.Spec.Worker || observed.ManagerPipePID != manager.identity.PID || observed.WorkerPipePID != child.identity.PID {
		return errors.New("manager observation does not match held native identities")
	}
	binding, err := observeConsentInterface(ctx, prepared.Spec.Request.Scope)
	if err != nil || binding.Alias != prepared.Spec.Worker.InterfaceAlias {
		return errors.New("native selected interface/profile changed before commit")
	}
	// The peer is the independently signed SDK image. Its source/publication and
	// exclusive ledger facts remain required; native child/interface evidence
	// above cannot turn missing manager authority into a positive observation.
	owner := lanlease.NewOwner(prepared.Spec.Request.Installation, manager.identity)
	lease, err := owner.Prepare(prepared.Spec, time.Now())
	if err != nil {
		return err
	}
	return lease.BindChild(1, child.identity, observed, time.Now())
}

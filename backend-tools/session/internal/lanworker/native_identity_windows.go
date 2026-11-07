//go:build windows

package lanworker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
)

// FILE_ID_INFO, Windows SDK 10.0.22621.0 winbase.h. No guessed function
// table or offsets: GetFileInformationByHandleEx is the x/sys typed binding.
type nativeFileID struct {
	Volume uint64
	ID     [16]byte
}

type heldNativeProcess struct {
	handle   windows.Handle
	identity lanfirewall.CallerIdentity
	image    *os.File
	path     string
	session  uint32 // Native TokenSessionId; deliberately outside frozen V1 wire.
}

func (p *heldNativeProcess) close() {
	if p.image != nil {
		p.image.Close()
		p.image = nil
	}
	if p.handle != 0 {
		windows.CloseHandle(p.handle)
		p.handle = 0
	}
}
func (p *heldNativeProcess) live() bool {
	s, e := windows.WaitForSingleObject(p.handle, 0)
	return e == nil && s == uint32(windows.WAIT_TIMEOUT)
}
func nativeOrdinaryToken(handle windows.Handle) (string, uint32, error) {
	var token windows.Token
	if e := windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token); e != nil {
		return "", 0, e
	}
	defer token.Close()
	var elevated, size uint32
	if e := windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &size); e != nil || size != 4 || elevated != 0 {
		return "", 0, errors.New("ordinary non-elevated process token required")
	}
	var session uint32
	if e := windows.GetTokenInformation(token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4, &size); e != nil || size != 4 {
		return "", 0, errors.New("native process token session unavailable")
	}
	user, e := token.GetTokenUser()
	if e != nil {
		return "", 0, e
	}
	return user.User.Sid.String(), session, nil
}
func holdNativeProcess(pid uint32) (*heldNativeProcess, error) {
	h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if e != nil {
		return nil, e
	}
	p := &heldNativeProcess{handle: h}
	ok := false
	defer func() {
		if !ok {
			p.close()
		}
	}()
	sid, session, e := nativeOrdinaryToken(h)
	if e != nil {
		return nil, e
	}
	p.session = session
	var creation, exit, kernel, user windows.Filetime
	if e = windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); e != nil {
		return nil, e
	}
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	if e = windows.QueryFullProcessImageName(h, 0, &buf[0], &size); e != nil || size == 0 || size >= uint32(len(buf)) {
		return nil, errors.New("bounded native process path unavailable")
	}
	p.path = windows.UTF16ToString(buf[:size])
	p.image, e = holdNativeFile(p.path, false)
	if e != nil {
		return nil, e
	}
	digest, e := nativeImageSHAWithLimit(p.image, 256<<20)
	if e != nil {
		return nil, e
	}
	p.identity = lanfirewall.CallerIdentity{PID: pid, UserSID: sid, CreationTime: strconv.FormatUint(uint64(creation.HighDateTime)<<32|uint64(creation.LowDateTime), 10), ImageSHA256: digest}
	if !p.live() {
		return nil, errors.New("native process exited during binding")
	}
	ok = true
	return p, nil
}
func holdNativeFile(path string, directory bool) (*os.File, error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	h, e := windows.CreateFile(p, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, flags, 0)
	if e != nil {
		return nil, e
	}
	var info windows.ByHandleFileInformation
	if e = windows.GetFileInformationByHandle(h, &info); e != nil {
		windows.CloseHandle(h)
		return nil, e
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		windows.CloseHandle(h)
		return nil, errors.New("native held path kind/reparse contract differs")
	}
	f := os.NewFile(uintptr(h), path)
	if f == nil {
		windows.CloseHandle(h)
		return nil, errors.New("native held file unavailable")
	}
	return f, nil
}
func nativeImageSHA(f *os.File) (string, error) {
	return nativeImageSHAWithLimit(f, 64<<20)
}
func nativeImageSHAWithLimit(f *os.File, limit int64) (string, error) {
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return "", errors.New("bounded regular worker image required")
	}
	h := sha256.New()
	n, e := io.Copy(h, io.NewSectionReader(f, 0, info.Size()))
	if e != nil || n != info.Size() {
		return "", errors.New("held image hash incomplete")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func nativeWorkerFile(f *os.File, path, alias string) (lanfirewall.WorkerBinding, error) {
	var id nativeFileID
	if e := windows.GetFileInformationByHandleEx(windows.Handle(f.Fd()), windows.FileIdInfo, (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id))); e != nil {
		return lanfirewall.WorkerBinding{}, e
	}
	sha, e := nativeImageSHA(f)
	if e != nil {
		return lanfirewall.WorkerBinding{}, e
	}
	return lanfirewall.WorkerBinding{Path: path, FileID: hex.EncodeToString(id.ID[:]), VolumeSerial: strconv.FormatUint(id.Volume, 10), SHA256: sha, InterfaceAlias: alias}, nil
}
func exactProtectedDescriptor(f *os.File, sid string, directory bool) error {
	inherit := ""
	if directory {
		inherit = "OICI"
	}
	want, e := windows.SecurityDescriptorFromString("O:BAD:P(A;" + inherit + ";FA;;;SY)(A;" + inherit + ";FA;;;BA)(A;" + inherit + ";FRFX;;;" + sid + ")")
	if e != nil {
		return e
	}
	got, e := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		return e
	}
	if got.String() != want.String() {
		return errors.New("protected lease descriptor differs from fixed helper contract")
	}
	return nil
}

// The helper creates and protects these paths before launch. We keep every
// descriptor open without write/delete sharing until the lease process exits.
// The program/record path is derived from its native own-image path, never argv.
type nativeBoot struct {
	prepared      lanhelper.Prepared
	self, manager *heldNativeProcess
	held          []*os.File
}

func (b *nativeBoot) close() {
	for _, f := range b.held {
		f.Close()
	}
	b.held = nil
	if b.self != nil {
		b.self.close()
	}
	if b.manager != nil {
		b.manager.close()
	}
}
func openNativeBoot() (b *nativeBoot, err error) {
	b = &nativeBoot{}
	owned := b
	defer func() {
		if err != nil {
			owned.close()
		}
	}()
	b.self, err = holdNativeProcess(windows.GetCurrentProcessId())
	if err != nil {
		return nil, err
	}
	program := b.self.path
	data, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(data, "OrbWin", "LAN")
	leaseDir := filepath.Dir(program)
	installationDir := filepath.Dir(leaseDir)
	installation, lease := filepath.Base(installationDir), filepath.Base(leaseDir)
	if !canonicalNonce(installation) || !canonicalNonce(lease) || program != filepath.Join(root, installation, lease, "orbwin-session.exe") {
		return nil, errors.New("dedicated private worker path required")
	}
	// ProgramData itself is OS-owned. Reject reparse traversal and hold it;
	// OrbWin and all descendants additionally require the fixed protected ACL.
	for _, dir := range []string{data, filepath.Join(data, "OrbWin"), root, installationDir, leaseDir} {
		f, e := holdNativeFile(dir, true)
		if e != nil {
			return nil, e
		}
		b.held = append(b.held, f)
		if dir != data {
			if e = exactProtectedDescriptor(f, b.self.identity.UserSID, true); e != nil {
				return nil, e
			}
		}
	}
	if err = exactProtectedDescriptor(b.self.image, b.self.identity.UserSID, false); err != nil {
		return nil, err
	}
	f, err := holdNativeFile(filepath.Join(leaseDir, "lease-v2.json"), false)
	if err != nil {
		return nil, err
	}
	b.held = append(b.held, f)
	if err = exactProtectedDescriptor(f, b.self.identity.UserSID, false); err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || info.Size() > lanhelper.FrameBudget+4 {
		return nil, errors.New("boot record exceeds bounded frame")
	}
	contents, err := io.ReadAll(io.NewSectionReader(f, 0, info.Size()))
	if err != nil {
		return nil, err
	}
	reader := bytes.NewReader(contents)
	frame, err := lanhelper.Read(reader)
	if err != nil || reader.Len() != 0 || frame.Kind != "prepared" || frame.Sequence != 2 {
		return nil, errors.New("protected boot record is not one prepared frame")
	}
	b.prepared = *frame.Prepared
	s := b.prepared.Spec
	if s.LeaseID != lease || s.Request.Installation != installation || s.WorkerRoot != root || s.Worker.Path != program {
		return nil, errors.New("protected boot path binding differs")
	}
	actual, err := nativeWorkerFile(b.self.image, program, s.Worker.InterfaceAlias)
	if err != nil || actual != s.Worker {
		return nil, errors.New("held worker file identity differs")
	}
	b.manager, err = holdNativeProcess(s.Request.CallerPID)
	if err != nil {
		return nil, err
	}
	if !sameNativeSession(b.manager, b.self) || b.manager.identity.UserSID != b.self.identity.UserSID || b.manager.identity.CreationTime != s.Request.CallerCreationTime || b.manager.identity.ImageSHA256 != s.Request.CallerImageSHA256 {
		return nil, errors.New("held authoritative manager differs")
	}
	return b, nil
}
func sameNativeSession(manager, child *heldNativeProcess) bool {
	return manager != nil && child != nil && manager.session == child.session
}

func ManagerIdentity() (lanfirewall.CallerIdentity, error) {
	p, e := holdNativeProcess(windows.GetCurrentProcessId())
	if e != nil {
		return lanfirewall.CallerIdentity{}, errors.New("ordinary native manager identity unavailable")
	}
	defer p.close()
	return p.identity, nil
}
func canonicalNonce(s string) bool {
	v, e := hex.DecodeString(s)
	return e == nil && len(v) == 16 && strings.ToLower(s) == s
}

func nativePipePeer(conn interface{ Fd() uintptr }, client bool, expected *heldNativeProcess) error {
	var pid uint32
	var e error
	if client {
		e = windows.GetNamedPipeClientProcessId(windows.Handle(conn.Fd()), &pid)
	} else {
		e = windows.GetNamedPipeServerProcessId(windows.Handle(conn.Fd()), &pid)
	}
	if e != nil || pid != expected.identity.PID || !expected.live() {
		return errors.New("native pipe process binding differs")
	}
	// Retained native handle prevents PID-reuse confusion. Creation/SID/path/file
	// hashes were observed from that handle; no request-supplied PID is authority.
	return nil
}

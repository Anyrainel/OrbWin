//go:build windows

package sdkself

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanhelper"
)

var errLease = errors.New("native SDK image/bundle/peer lease unavailable")

type object struct {
	file               *os.File
	id                 string
	sdSHA              string
	directory, private bool
	sid                string
}
type Lease struct{ *leaseState }
type leaseState struct {
	mu            sync.Mutex
	process       windows.Handle
	held          []object
	identity      Identity
	sid, path     string
	pipe          windows.Handle
	caller        *Lease
	closed        bool
	closeErr      error
	closeComplete bool
	joinedClose   *JoinedLeaseClose
}

func (*Lease) String() string   { return "SDKSelfLease(private native handles)" }
func (*Lease) GoString() string { return "SDKSelfLease(private native handles)" }
func processFacts(process windows.Handle) (sid string, session uint32, creation string, err error) {
	var token windows.Token
	if windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token) != nil {
		return "", 0, "", errLease
	}
	defer func() {
		if token.Close() != nil {
			err = errLease
		}
	}()
	var elevation, size uint32
	if windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), 4, &size) != nil || size != 4 || elevation != 0 {
		return "", 0, "", errLease
	}
	if windows.GetTokenInformation(token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4, &size) != nil || size != 4 || session == 0 {
		return "", 0, "", errLease
	}
	user, e := token.GetTokenUser()
	if e != nil || user == nil || user.User.Sid == nil {
		return "", 0, "", errLease
	}
	sid = user.User.Sid.String()
	if !(strings.HasPrefix(sid, "S-1-5-21-") || strings.HasPrefix(sid, "S-1-12-1-")) {
		return "", 0, "", errLease
	}
	var created, exited, kernel, used windows.Filetime
	if windows.GetProcessTimes(process, &created, &exited, &kernel, &used) != nil {
		return "", 0, "", errLease
	}
	value := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	if value == 0 {
		return "", 0, "", errLease
	}
	return sid, session, strconv.FormatUint(value, 10), nil
}
func ordinary() error {
	var token windows.Token
	if e := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &token); e == nil {
		_ = token.Close()
		return errLease
	} else if e != windows.ERROR_NO_TOKEN {
		return errLease
	}
	return nil
}
func snapshot(h windows.Handle, directory bool) (string, error) {
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
		return "", errLease
	}
	kind, e := windows.GetFileType(h)
	if e != nil || kind != windows.FILE_TYPE_DISK {
		return "", errLease
	}
	var final [1024]uint16
	n, e := windows.GetFinalPathNameByHandle(h, &final[0], uint32(len(final)), 2)
	if e != nil || n == 0 || n >= uint32(len(final)) || !strings.HasPrefix(windows.UTF16ToString(final[:n]), `\Device\HarddiskVolume`) {
		return "", errLease
	}
	var flags uint32
	if windows.GetVolumeInformationByHandle(h, nil, 0, nil, nil, &flags, nil, 0) != nil || flags&windows.FILE_PERSISTENT_ACLS == 0 {
		return "", errLease
	}
	suffix := ""
	if !directory {
		suffix = fmt.Sprintf(":%08x%08x:%08x%08x", info.FileSizeHigh, info.FileSizeLow, info.LastWriteTime.HighDateTime, info.LastWriteTime.LowDateTime)
	}
	return fmt.Sprintf("%08x:%08x%08x:%08x:%d%s", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow, info.FileAttributes, info.NumberOfLinks, suffix), nil
}
func objectPolicy(h windows.Handle, sid string, directory, private bool) (string, error) {
	sd, e := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if e != nil || sd == nil {
		return "", errLease
	}
	owner, _, e := sd.Owner()
	if e != nil || owner == nil {
		return "", errLease
	}
	acl, _, e := sd.DACL()
	if e != nil || acl == nil || acl.AceCount > 256 {
		return "", errLease
	}
	trusted := func(s string) bool {
		return s == sid || s == "S-1-5-18" || s == "S-1-5-32-544" || s == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
	}
	if !trusted(owner.String()) {
		return "", errLease
	}
	if private {
		control, _, e := sd.Control()
		if e != nil || owner.String() != sid || control&windows.SE_DACL_PROTECTED == 0 || acl.AceCount != 3 {
			return "", errLease
		}
	}
	principals := map[string]bool{}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var a *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &a) != nil || a == nil || a.Header.AceSize < 16 || a.Header.AceType > 1 || a.Header.AceFlags&^uint8(0x1f) != 0 {
			return "", errLease
		}
		principal := (*windows.SID)(unsafe.Pointer(&a.SidStart))
		if !principal.IsValid() || principal.Len() > int(a.Header.AceSize)-8 {
			return "", errLease
		}
		name := principal.String()
		if private {
			flags := uint8(0)
			if directory {
				flags = 3
			}
			if a.Header.AceType != 0 || a.Header.AceFlags != flags || a.Mask != 0x1f01ff || principals[name] || (name != sid && name != "S-1-5-18" && name != "S-1-5-32-544") {
				return "", errLease
			}
			principals[name] = true
			continue
		}
		// Unchanged managementconfig.checkReadObject directory integrity policy.
		if a.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || a.Header.AceType == 1 || trusted(name) {
			continue
		}
		bad := uint32(windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | 0x40 | windows.FILE_WRITE_DATA | windows.FILE_WRITE_ATTRIBUTES | windows.FILE_WRITE_EA | windows.GENERIC_ALL | windows.GENERIC_WRITE)
		if uint32(a.Mask)&bad != 0 {
			return "", errLease
		}
	}
	data := sd.String()
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:]), nil
}
func capture(h windows.Handle, sid string, directory, private bool) (out object, err error) {
	file := os.NewFile(uintptr(h), "retained native SDK object")
	if file == nil {
		_ = windows.CloseHandle(h)
		return out, errLease
	}
	out = object{file: file, directory: directory, private: private, sid: sid}
	id, e := snapshot(h, directory)
	if e != nil {
		return out, e
	}
	sha, e := objectPolicy(h, sid, directory, private)
	if e != nil {
		return out, e
	}
	return object{file, id, sha, directory, private, sid}, nil
}
func (l *Lease) holdObject(h windows.Handle, sid string, directory, private bool) (object, error) {
	o, err := capture(h, sid, directory, private)
	if o.file != nil {
		l.held = append(l.held, o)
	}
	return o, err
}
func (o object) recheck() error {
	h := windows.Handle(o.file.Fd())
	id, e := snapshot(h, o.directory)
	if e != nil || id != o.id {
		return errLease
	}
	sha, e := objectPolicy(h, o.sid, o.directory, o.private)
	if e != nil || sha != o.sdSHA {
		return errLease
	}
	return nil
}
func safePart(part string) bool {
	return part != "" && part != "." && part != ".." && len(part) <= 1024 && !strings.HasSuffix(part, ".") && !strings.HasSuffix(part, " ") && !strings.ContainsAny(part, ":/\\\x00<>\"|?*")
}
func relative(parent windows.Handle, part string, directory bool) (windows.Handle, error) {
	if !safePart(part) {
		return 0, errLease
	}
	text, e := windows.NewNTUnicodeString(part)
	if e != nil {
		return 0, errLease
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: text, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE)
	options := uint32(windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT)
	if directory {
		options |= windows.FILE_DIRECTORY_FILE
		// Metadata-only opens do not participate in NTFS share checks. Traverse
		// supplies the minimal execute bit so omitted SHARE_DELETE pins the inode
		// without granting directory enumeration.
		access |= windows.FILE_TRAVERSE
	} else {
		options |= windows.FILE_NON_DIRECTORY_FILE
		access |= windows.GENERIC_READ
	}
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	share := uint32(windows.FILE_SHARE_READ)
	if directory {
		share |= windows.FILE_SHARE_WRITE
	}
	if windows.NtCreateFile(&h, access, &oa, &status, nil, 0, share, windows.FILE_OPEN, options, 0, 0) != nil {
		return 0, errLease
	}
	return h, nil
}
func digestFile(ctx context.Context, file *os.File, limit uint64) (string, uint64, error) {
	stat, e := file.Stat()
	if e != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || uint64(stat.Size()) > limit {
		return "", 0, errLease
	}
	hash := sha256.New()
	reader := io.NewSectionReader(file, 0, stat.Size())
	buf := make([]byte, 65536)
	var copied uint64
	for {
		if ctx.Err() != nil {
			return "", 0, ctx.Err()
		}
		n, e := reader.Read(buf)
		if n > 0 {
			copied += uint64(n)
			_, _ = hash.Write(buf[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", 0, errLease
		}
	}
	if copied != uint64(stat.Size()) {
		return "", 0, errLease
	}
	return hex.EncodeToString(hash.Sum(nil)), copied, nil
}
func holdProcess(ctx context.Context, pid uint32) (lease *Lease, err error) {
	if ctx == nil || ctx.Err() != nil || pid == 0 {
		return nil, errLease
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if ordinary() != nil {
		return nil, errLease
	}
	currentSID, currentSession, _, e := processFacts(windows.CurrentProcess())
	if e != nil {
		return nil, e
	}
	process, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if e != nil {
		return nil, errLease
	}
	lease = &Lease{leaseState: &leaseState{process: process}}
	owned := lease
	defer func() {
		if err != nil {
			lease, err = settleFailedLease(owned, err, owned.Close)
		}
	}()
	sid, session, creation, e := processFacts(process)
	if e != nil || sid != currentSID || session != currentSession {
		return nil, errLease
	}
	lease.sid = sid
	var path [32768]uint16
	size := uint32(len(path))
	if windows.QueryFullProcessImageName(process, 0, &path[0], &size) != nil || size == 0 || size >= uint32(len(path)) {
		return nil, errLease
	}
	actual := windows.UTF16ToString(path[:size])
	lease.path = actual
	// Request only the access required by the explicit-user KnownFolder API.
	var token windows.Token
	e = windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_IMPERSONATE, &token)
	if e != nil {
		return nil, errLease
	}
	profile, e := token.KnownFolderPath(windows.FOLDERID_Profile, 0)
	closeErr := token.Close()
	if e != nil || closeErr != nil {
		return nil, errLease
	}
	if len(profile) < 4 || profile[1:3] != `:\` || strings.ContainsAny(profile[2:], ":/\x00") {
		return nil, errLease
	}
	profileParts := strings.Split(profile[3:], `\`)
	if len(profileParts) > 16 {
		return nil, errLease
	}
	for _, p := range profileParts {
		if !safePart(p) {
			return nil, errLease
		}
	}
	cacheDir := filepath.Base(filepath.Dir(actual))
	bundleSHA := strings.TrimPrefix(cacheDir, "orbwin-session-")
	if !exactSHA(bundleSHA) || cacheDir != "orbwin-session-"+bundleSHA || !strings.EqualFold(actual, filepath.Join(profile, ".orbwin-private", "runtime", cacheDir, "orbwin-session.exe")) {
		return nil, errLease
	}
	volume, e := windows.UTF16PtrFromString(profile[:3])
	if e != nil {
		return nil, errLease
	}
	h, e := windows.CreateFile(volume, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES|windows.FILE_TRAVERSE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, errLease
	}
	_, e = lease.holdObject(h, sid, true, false)
	if e != nil {
		return nil, e
	}
	for i, p := range append(append([]string{}, profileParts...), ".orbwin-private", "runtime", cacheDir) {
		h, e = relative(windows.Handle(lease.held[len(lease.held)-1].file.Fd()), p, true)
		if e != nil {
			return nil, e
		}
		_, e = lease.holdObject(h, sid, true, i >= len(profileParts))
		if e != nil {
			return nil, e
		}
	}
	managerIndex := len(lease.held)
	h, e = relative(windows.Handle(lease.held[len(lease.held)-1].file.Fd()), "orbwin-session.exe", false)
	if e != nil {
		return nil, e
	}
	image, e := lease.holdObject(h, sid, false, true)
	if e != nil {
		return nil, e
	}
	// The parent native bundle directory is retained before every fixed sibling.
	managerDir := windows.Handle(lease.held[managerIndex-1].file.Fd())
	h, e = relative(managerDir, "lan", true)
	if e != nil {
		return nil, e
	}
	lan, e := lease.holdObject(h, sid, true, true)
	if e != nil {
		return nil, e
	}
	lanDir := windows.Handle(lan.file.Fd())
	files := map[string]*os.File{}
	for _, name := range []string{"assets.json", "assets.sig", "orbwin-lan-helper.exe", "orbwin-lan-worker.exe"} {
		h, e = relative(lanDir, name, false)
		if e != nil {
			return nil, e
		}
		o, e := lease.holdObject(h, sid, false, true)
		if e != nil {
			return nil, e
		}
		files[name] = o.file
	}
	data, e := io.ReadAll(io.LimitReader(files["assets.json"], 4097))
	if e != nil || len(data) > 4096 {
		return nil, errLease
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != bundleSHA {
		return nil, errLease
	}
	signature, e := io.ReadAll(io.LimitReader(files["assets.sig"], 89))
	if e != nil || len(signature) != 88 {
		return nil, errLease
	}
	bundle, e := lanhelper.VerifyConfiguredAssetBundle(data, signature)
	if e != nil {
		return nil, errLease
	}
	imageSHA := ""
	for i, asset := range bundle.Assets {
		file := image.file
		limit := uint64(256 << 20)
		if i > 0 {
			file = files[filepath.Base(asset.Path)]
			limit = 64 << 20
		}
		if file == nil {
			return nil, errLease
		}
		sha, count, e := digestFile(ctx, file, limit)
		if e != nil || sha != asset.SHA256 || count != asset.Size {
			return nil, errLease
		}
		if i == 0 {
			imageSHA = sha
		}
	}
	lease.identity = Identity{1, pid, creation, session, imageSHA, bundleSHA, true}
	if !lease.identity.Valid() || lease.recheckLocked() != nil || ctx.Err() != nil {
		return nil, errLease
	}
	return lease, nil
}
func Hold(ctx context.Context) (*Lease, error) {
	return holdProcess(ctx, windows.GetCurrentProcessId())
}

// HoldPeer uses the ACTUAL connected named-pipe server PID before any request.
// It retains both current signed caller and same-build peer through the reply.
func HoldPeer(ctx context.Context, connection net.Conn) (peer *Lease, err error) {
	if ctx == nil || connection == nil {
		return nil, errLease
	}
	fd, ok := connection.(interface{ Fd() uintptr })
	if !ok {
		return nil, errLease
	}
	pipe := windows.Handle(fd.Fd())
	caller, e := Hold(ctx)
	if e != nil {
		return caller, e
	}
	defer func() {
		if err != nil {
			resource := peer
			if resource == nil {
				resource = caller
			} else {
				resource.caller = caller
			}
			peer, err = settleFailedLease(resource, err, resource.Close)
		}
	}()
	var pid uint32
	if windows.GetNamedPipeServerProcessId(pipe, &pid) != nil || pid == 0 {
		return nil, errLease
	}
	peer, e = holdProcess(ctx, pid)
	if e != nil {
		return peer, e
	}
	peer.pipe = pipe
	peer.caller = caller
	if peer.identity.ImageSHA256 != caller.identity.ImageSHA256 || peer.identity.BundleSHA256 != caller.identity.BundleSHA256 || peer.sid != caller.sid || peer.identity.Session != caller.identity.Session || !strings.EqualFold(peer.path, caller.path) || peer.recheckLocked() != nil {
		return peer, errLease
	}
	return peer, nil
}
func (l *Lease) recheckLocked() error {
	if l == nil || l.leaseState == nil || l.closed || !l.identity.Valid() || l.process == 0 {
		return errLease
	}
	if ordinary() != nil {
		return errLease
	}
	state, e := windows.WaitForSingleObject(l.process, 0)
	if e != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return errLease
	}
	sid, session, creation, e := processFacts(l.process)
	if e != nil || sid != l.sid || session != l.identity.Session || creation != l.identity.CreationTime {
		return errLease
	}
	for _, h := range l.held {
		if h.recheck() != nil {
			return errLease
		}
	}
	if l.pipe != 0 {
		var pid uint32
		if windows.GetNamedPipeServerProcessId(l.pipe, &pid) != nil || pid != l.identity.PID {
			return errLease
		}
	}
	return nil
}
func (l *Lease) Identity() (Identity, error) {
	if l == nil || l.leaseState == nil {
		return Identity{}, errLease
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if l.recheckLocked() != nil {
		return Identity{}, errLease
	}
	return l.identity, nil
}
func (l *Lease) Close() error {
	if l == nil || l.leaseState == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		// Construction may attach the separately held caller to a partial peer
		// after that peer's own cleanup latched unknown. Close that caller once;
		// never retry the peer's failed native handle closes.
		if l.caller != nil && l.caller.Close() != nil {
			l.closeErr = errLease
		}
		return l.closeErr
	}
	l.closed = true
	for i := len(l.held) - 1; i >= 0; i-- {
		if l.held[i].file.Close() != nil {
			l.closeErr = errLease
		}
	}
	if l.process != 0 && windows.CloseHandle(l.process) != nil {
		l.closeErr = errLease
	}
	if l.caller != nil && l.caller.Close() != nil {
		l.closeErr = errLease
	}
	return l.closeErr
}

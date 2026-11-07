//go:build windows

package main

// Native metadata/relative-open primitives copied from internal/sdkself/lease_windows.go.
// Policy is unchanged; this adapter only READS existing fixed-role cache objects.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var errLease = errors.New("private executable cache unavailable; prepare this build in OrbWin desktop")

type object struct {
	file               *os.File
	id, sdSHA          string
	directory, private bool
	sid                string
}
type cacheLease struct {
	held []object
	path string
}

var cacheLifetime struct {
	sync.Mutex
	roles  map[string]*cacheLease
	failed bool
}

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
	for _, class := range []uint32{29 /* TokenIsAppContainer (TOKEN_INFORMATION_CLASS) */} {
		var value uint32
		if windows.GetTokenInformation(token, class, (*byte)(unsafe.Pointer(&value)), 4, &size) != nil || size != 4 || value != 0 {
			return "", 0, "", errLease
		}
	}
	// TokenHasRestrictions reports any filtering, including ordinary UAC limited
	// tokens. Reject actual restricting SIDs instead, using an aligned bounded
	// TOKEN_GROUPS result. No variable SID pointer is read for rejected tokens.
	var restricted [256]uint64
	if windows.GetTokenInformation(token, windows.TokenRestrictedSids, (*byte)(unsafe.Pointer(&restricted[0])), uint32(unsafe.Sizeof(restricted)), &size) != nil || !noRestrictingSids(restricted[:], size) {
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
func (l *cacheLease) holdObject(h windows.Handle, sid string, directory, private bool) (object, error) {
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

// Metadata-only opens do not participate in NTFS sharing checks. Directory
// path retention therefore needs FILE_TRAVERSE (FILE_EXECUTE bit 0x20), without
// granting FILE_LIST_DIRECTORY. See MS-FSA 2.1.5.1.2.2; actual exclusion still
// requires native qualification, not merely an omitted SHARE_DELETE flag.
func directoryShareAccess(access uint32) uint32 { return access | windows.FILE_TRAVERSE }

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
		access = directoryShareAccess(access)
		options |= windows.FILE_DIRECTORY_FILE
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

func nativeLocations() (profile, local string, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if ordinary() != nil {
		return "", "", errLease
	}
	if _, _, _, e := processFacts(windows.CurrentProcess()); e != nil {
		return "", "", e
	}
	// Explicit-user KnownFolder lookup requires QUERY|IMPERSONATE access.
	// No impersonation is performed and no token privileges are enabled.
	var token windows.Token
	e := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_IMPERSONATE, &token)
	if e != nil {
		return "", "", errLease
	}
	defer func() {
		if token.Close() != nil {
			err = errLease
		}
	}()
	profile, e = token.KnownFolderPath(windows.FOLDERID_Profile, 0)
	if e != nil {
		return "", "", errLease
	}
	local, e = token.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if e != nil {
		return "", "", errLease
	}
	return profile, local, nil
}
func existingCache(role string) (string, error) {
	specs, e := cacheRole(role)
	if e != nil {
		return "", e
	}
	cacheLifetime.Lock()
	defer cacheLifetime.Unlock()
	if cacheLifetime.failed {
		return "", errLease
	}
	if l := cacheLifetime.roles[role]; l != nil {
		for _, o := range l.held {
			if o.recheck() != nil {
				cacheLifetime.failed = true
				return "", errLease
			}
		}
		return l.path, nil
	}
	if len(cacheLifetime.roles) >= 3 {
		return "", errLease
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	profile, _, e := nativeLocations()
	if e != nil {
		return "", e
	}
	sid, _, _, e := processFacts(windows.CurrentProcess())
	if e != nil {
		return "", e
	}
	if len(profile) < 4 || profile[1:3] != `:\` || strings.ContainsAny(profile[2:], ":/\x00") {
		return "", errLease
	}
	components := strings.Split(profile[3:], `\`)
	if len(components) > 16 {
		return "", errLease
	}
	l := &cacheLease{}
	// Even failed admission retains owned handles until CLI process exit. No retry
	// or finalizer may release unknown resources and permit another admission.
	if cacheLifetime.roles == nil {
		cacheLifetime.roles = map[string]*cacheLease{}
	}
	cacheLifetime.roles[role] = l
	success := false
	defer func() {
		if !success {
			cacheLifetime.failed = true
		}
	}()
	volume, _ := windows.UTF16PtrFromString(profile[:3])
	h, e := windows.CreateFile(volume, directoryShareAccess(windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES), windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return "", errLease
	}
	if _, e = l.holdObject(h, sid, true, false); e != nil {
		return "", e
	}
	for i, p := range append(append([]string{}, components...), ".orbwin-private", "runtime", specs.directory) {
		h, e = relative(windows.Handle(l.held[len(l.held)-1].file.Fd()), p, true)
		if e != nil {
			return "", e
		}
		if _, e = l.holdObject(h, sid, true, i >= len(components)); e != nil {
			return "", e
		}
	}
	parent := windows.Handle(l.held[len(l.held)-1].file.Fd())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, f := range specs.files {
		dir := parent
		if f.subdir != "" {
			h, e = relative(parent, f.subdir, true)
			if e != nil {
				return "", e
			}
			o, x := l.holdObject(h, sid, true, true)
			if x != nil {
				return "", x
			}
			dir = windows.Handle(o.file.Fd())
		}
		h, e = relative(dir, f.name, false)
		if e != nil {
			return "", e
		}
		o, x := l.holdObject(h, sid, false, true)
		if x != nil {
			return "", x
		}
		sha, _, x := digestFile(ctx, o.file, f.limit)
		if x != nil || sha != f.sha {
			return "", errLease
		}
		if o.recheck() != nil {
			return "", errLease
		}
	}
	for _, o := range l.held {
		if o.recheck() != nil {
			return "", errLease
		}
	}
	l.path = filepath.Join(profile, ".orbwin-private", "runtime", specs.directory, specs.files[0].name)
	success = true
	return l.path, nil
}

// A TOKEN_GROUPS result has a DWORD count before any variable SID pointers.
func noRestrictingSids(groups []uint64, returned uint32) bool {
	return len(groups) != 0 && returned >= 4 && uint64(returned) <= uint64(len(groups))*8 && *(*uint32)(unsafe.Pointer(&groups[0])) == 0
}

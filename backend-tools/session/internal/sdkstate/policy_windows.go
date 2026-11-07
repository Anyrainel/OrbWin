//go:build windows

package sdkstate

// Native object-policy/snapshot primitives are copied from the reviewed
// internal/sdkself lease implementation; general ancestor policy is unchanged.
import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"strings"
	"unsafe"
)

var errLease = ErrState

type object struct {
	file               *os.File
	id, sdSHA          string
	directory, private bool
	sid                string
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

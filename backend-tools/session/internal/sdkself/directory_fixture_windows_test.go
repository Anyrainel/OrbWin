//go:build windows && orbwin_sdkself_directory_fixture

package sdkself

// Closed fresh-directory qualification only. No Lease/Current/peer/bundle or
// execution admission is constructed; TestMain NEVER calls m.Run.
import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

type sdkselfDirectoryFixtureRole struct {
	Role, NativeSnapshot, PolicySHA     string
	GrantedAccess                       uint32
	HasTraverse, NoList, DeleteRejected bool
	DeleteStatus                        uint32
	DeleteError                         string
}
type sdkselfDirectoryFixtureResult struct {
	Schema                                int
	Kind, Artifact                        string
	Ordinary, MetadataControlAdmitsDelete bool
	Roles                                 []sdkselfDirectoryFixtureRole
	Cleanup                               []string
	Failure                               string
	Success                               bool
}

var sdkselfDirectoryFixtureCustody = struct {
	sync.Mutex
	caps [][]windows.Handle
}{}

func TestMain(_ *testing.M) {
	r := sdkselfDirectoryFixtureResult{Schema: 1, Kind: "sdkself-relative-directory-traverse-sharing-only"}
	if len(os.Args) != 1 {
		r.Failure = "closed no-argument fixture"
	} else if err := sdkselfDirectoryFixtureProbe(&r); err != nil {
		r.Failure = err.Error()
	}
	r.Success = r.Failure == ""
	_ = json.NewEncoder(os.Stdout).Encode(r)
	if !r.Success {
		os.Exit(1)
	}
	os.Exit(0)
}

func sdkselfDirectoryFixtureProbe(out *sdkselfDirectoryFixtureResult) (ret error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	sid, _, _, err := processFacts(windows.CurrentProcess())
	if err != nil || ordinary() != nil {
		return errLease
	}
	out.Ordinary = true
	var caps []windows.Handle
	retain := func() {
		sdkselfDirectoryFixtureCustody.Lock()
		sdkselfDirectoryFixtureCustody.caps = append(sdkselfDirectoryFixtureCustody.caps, append([]windows.Handle(nil), caps...))
		sdkselfDirectoryFixtureCustody.Unlock()
	}
	finished := false
	defer func() {
		if !finished || ret != nil {
			retain()
			return
		}
		for n := len(caps) - 1; n >= 0; n-- {
			if caps[n] == 0 {
				continue
			}
			h := caps[n]
			caps[n] = 0
			err := windows.CloseHandle(h)
			out.Cleanup = append(out.Cleanup, fmt.Sprintf("close:%v", err))
			if err != nil {
				ret = err
				caps = append(caps, h)
				retain()
				return
			}
		}
	}()
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	out.Artifact = filepath.Join(os.TempDir(), "orbwin-sdkself-dirs-"+hex.EncodeToString(nonce[:]))
	sd, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;OICI;FA;;;" + sid + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	create := func(path string) error {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		err = windows.CreateDirectory(name, &sa)
		runtime.KeepAlive(sd)
		return err
	}
	if err = create(out.Artifact); err != nil {
		return errors.New("fresh artifact unavailable; no overwrite")
	}
	rootName, _ := windows.UTF16PtrFromString(out.Artifact)
	root, err := windows.CreateFile(rootName, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES|windows.FILE_TRAVERSE|windows.SYNCHRONIZE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if root != 0 && root != windows.InvalidHandle {
		caps = append(caps, root)
	}
	if err != nil {
		return err
	}
	if _, err = snapshot(root, true); err != nil {
		return err
	}
	if _, err = objectPolicy(root, sid, true, true); err != nil {
		return err
	}
	parent := root
	path := out.Artifact
	for _, role := range []string{".orbwin-private", "runtime", "orbwin-session-" + strings.Repeat("a", 64), "lan"} {
		path = filepath.Join(path, role)
		if err = create(path); err != nil {
			return err
		}
		h, err := relative(parent, role, true) // EXACT production directory helper
		if h != 0 {
			caps = append(caps, h)
		}
		if err != nil {
			return err
		}
		record := sdkselfDirectoryFixtureRole{Role: role}
		record.NativeSnapshot, err = snapshot(h, true)
		if err != nil {
			return err
		}
		record.PolicySHA, err = objectPolicy(h, sid, true, true)
		if err != nil {
			return err
		}
		var basic [14]uint32
		var length uint32
		status, _, _ := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryObject").Call(uintptr(h), 0, uintptr(unsafe.Pointer(&basic)), unsafe.Sizeof(basic), uintptr(unsafe.Pointer(&length)))
		if status != 0 {
			return errLease
		}
		record.GrantedAccess = basic[1]
		record.HasTraverse = basic[1]&windows.FILE_TRAVERSE != 0
		record.NoList = basic[1]&windows.FILE_LIST_DIRECTORY == 0
		probe, probeErr := sdkselfDirectoryFixtureOpen(parent, role, windows.DELETE|windows.READ_CONTROL|windows.SYNCHRONIZE)
		if probe != 0 {
			caps = append(caps, probe)
		}
		if probeErr != nil {
			record.DeleteError = probeErr.Error()
			if value, ok := probeErr.(windows.NTStatus); ok {
				record.DeleteStatus = uint32(value)
			}
		}
		record.DeleteRejected = probe == 0 && probeErr == windows.STATUS_SHARING_VIOLATION
		out.Roles = append(out.Roles, record)
		if !record.HasTraverse || !record.NoList || !record.DeleteRejected {
			return errors.New("actual SDKself directory sharing predicate failed")
		}
		parent = h
	}
	// Prove the fresh ACL permits DELETE: metadata-only/no-SHARE_DELETE is a
	// deliberately weak observation control, not an exclusion capability.
	if err = create(filepath.Join(out.Artifact, "metadata-control")); err != nil {
		return err
	}
	meta, err := sdkselfDirectoryFixtureOpenShare(root, "metadata-control", windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE)
	if meta != 0 {
		caps = append(caps, meta)
	}
	if err != nil {
		return err
	}
	probe, err := sdkselfDirectoryFixtureOpen(root, "metadata-control", windows.DELETE|windows.READ_CONTROL|windows.SYNCHRONIZE)
	if probe != 0 {
		caps = append(caps, probe)
	}
	if err != nil || probe == 0 {
		return errors.New("metadata control did not admit DELETE")
	}
	out.MetadataControlAdmitsDelete = true
	finished = true
	return nil
}
func sdkselfDirectoryFixtureOpen(parent windows.Handle, part string, access uint32) (windows.Handle, error) {
	return sdkselfDirectoryFixtureOpenShare(parent, part, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE)
}
func sdkselfDirectoryFixtureOpenShare(parent windows.Handle, part string, access, share uint32) (windows.Handle, error) {
	text, err := windows.NewNTUnicodeString(part)
	if err != nil {
		return 0, err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: text, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&h, access, &oa, &status, nil, 0, share, windows.FILE_OPEN, windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	return h, err
}

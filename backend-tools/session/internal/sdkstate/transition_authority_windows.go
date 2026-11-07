//go:build windows

package sdkstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"io"
	"net"
	"orbwin.local/session/internal/sdkself"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"
)

type nativeColdAuthority struct {
	mu                    sync.Mutex
	grant                 transitionGrant
	self                  *sdkself.Lease
	parent                windows.Handle
	parentToken           windows.Token
	parentImage           windows.Handle
	parentCode            []object
	supervisorMu          sync.RWMutex // publication to concurrent runtime checks
	pendingSupervisor     *transitionPendingSupervisor
	logicalSupervisor     *transitionPendingSupervisor // supervisorMu; actual accepted holder
	control               windows.Handle
	controlIO             *transitionControlIO
	controlTaken          bool
	files                 []*os.File
	processes             []windows.Handle
	tokens                []windows.Token
	legacy, controller    windows.Handle
	pipe                  net.Listener
	home, profile         string
	sid                   string
	session               uint32
	closed, used, unknown bool
	finalFile             *os.File
	finalEvent            windows.Handle
	finalConsumed         bool
	denial                transitionDenialRecord
}

// ConsumeColdRetirement consumes ONLY an actual inherited fixed-role file.
// The live selected rdd parent is the authenticated retirement/launch owner;
// the record's process tuples and digests are correlation, not authority by JSON.
// Parent MUST retain actual old SDK generation/ingress/launcher exclusion and
// guest retirement throughout this grant. No ordinary API manufactures it.
func ConsumeColdRetirement(ctx context.Context, raw uintptr) (out *ColdRetirement, result error) {
	if !bounded(ctx) || raw == 0 || raw == ^uintptr(0) {
		return nil, ErrState
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &nativeColdAuthority{}
	ok := false
	defer func() {
		if !ok {
			a.unknown = true
			out = &ColdRetirement{&coldRetirementState{scope: a.grant.Scope, native: a}}
			result = ErrRecoveryRequired
		}
	}()
	var err error
	a.sid, a.session, err = current()
	if err != nil {
		return nil, ErrState
	}
	h := windows.Handle(raw)
	if transitionInherited(h) != nil || objectPrivate(h, a.sid, false) != nil {
		return nil, ErrState
	}
	f := os.NewFile(raw, "SDK inherited cold-transition authority")
	a.files = append(a.files, f)
	info, err := f.Stat()
	if err != nil || info.Size() < 1 || info.Size() > 64<<10 {
		return nil, ErrState
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, ErrState
	}
	data, err := io.ReadAll(io.LimitReader(f, 64<<10+1))
	if err != nil {
		return nil, ErrState
	}
	a.grant, err = parseTransitionGrant(data, uint64(raw), a.sid, a.session)
	if err != nil {
		return nil, ErrState
	}
	g := a.grant
	// KnownFolder uses this actual own-process token, with its documented access.
	var token windows.Token
	err = windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_IMPERSONATE, &token)
	if err != nil {
		return nil, ErrState
	}
	defer token.Close()
	a.profile, err = token.KnownFolderPath(windows.FOLDERID_Profile, 0)
	if err != nil {
		return nil, ErrState
	}
	app, err := token.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return nil, ErrState
	}
	a.home = filepath.Join(app, "OrbWinData", "home")
	path, err := transitionPath(h)
	if err != nil || !strings.EqualFold(path, filepath.Join(a.profile, ".orbwin-private", "services.retirement-"+g.Scope.Transaction+".grant")) {
		return nil, ErrState
	}
	a.self, err = sdkself.Hold(ctx)
	if err != nil || a.self == nil {
		return nil, ErrState
	}
	identity, err := a.self.Identity()
	if err != nil || identity.ImageSHA256 != hex.EncodeToString(g.Scope.SDKImage[:]) {
		return nil, ErrState
	}
	a.parent, err = windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, g.Parent.PID)
	if err != nil || transitionProcessMatch(a.parent, g.Parent, false) != nil || transitionParent(g.Parent.PID) != nil {
		return nil, ErrState
	}
	a.parentToken, err = a.holdProcessToken(a.parent, 0)
	if err != nil {
		return nil, ErrState
	}
	parentPath, err := transitionProcessPath(a.parent)
	if err != nil || !strings.EqualFold(parentPath, filepath.Join(a.profile, ".orbwin-private", "runtime", "rdd-"+hex.EncodeToString(g.Parent.Image[:]), "rdd.exe")) {
		return nil, ErrState
	}
	if a.holdParentImage(ctx) != nil {
		return nil, ErrState
	}
	a.legacy = windows.Handle(g.LegacyRoot)
	a.controller = windows.Handle(g.Controller)
	for _, entry := range []struct {
		h    windows.Handle
		name string
		dir  bool
	}{{a.legacy, filepath.Join(a.home, ".rdorbwin", "local-services"), true}, {a.controller, filepath.Join(a.home, ".rdorbwin", "local-services", "controller.lock"), false}} {
		if transitionInherited(entry.h) != nil {
			return nil, ErrState
		}
		p, e := transitionPath(entry.h)
		if e != nil || !strings.EqualFold(p, entry.name) || legacyTransitionObject(entry.h, a.sid, entry.dir) != nil {
			return nil, ErrState
		}
		a.files = append(a.files, os.NewFile(uintptr(entry.h), "held legacy SDK object"))
	}
	// The parent has durably fenced the supported old RW OPEN_ALWAYS path,
	// positively closed its only RW object, and inherited a readonly/share0 lock.
	// READONLY itself grants no cleanup or signing authority.
	if a.consumeDenial() != nil {
		return nil, ErrState
	}
	probe, e := ntOpen(a.legacy, "controller.lock", false, windows.FILE_OPEN, nil, true, false, false)
	if e != windows.STATUS_SHARING_VIOLATION && e != windows.STATUS_ACCESS_DENIED {
		if e == nil {
			a.files = append(a.files, os.NewFile(uintptr(probe), "unexpected competing SDK lock"))
		}
		return nil, ErrState
	}
	for _, old := range g.Processes {
		process := windows.Handle(old.Process)
		oldToken := windows.Token(old.Token)
		oldImage := windows.Handle(old.Image)
		if transitionInherited(process) != nil || transitionInherited(windows.Handle(oldToken)) != nil || transitionInherited(oldImage) != nil {
			return nil, ErrState
		}
		a.processes = append(a.processes, process)
		a.tokens = append(a.tokens, oldToken)
		a.files = append(a.files, os.NewFile(uintptr(oldImage), "held retired SDK image"))
		if _, err := a.holdProcessToken(process, oldToken); err != nil {
			return nil, ErrState
		}
		user, e := oldToken.GetTokenUser()
		var oldSession, tokenSize uint32
		tokenErr := windows.GetTokenInformation(oldToken, windows.TokenSessionId, (*byte)(unsafe.Pointer(&oldSession)), 4, &tokenSize)
		processPath, pathErr := transitionProcessPath(process)
		heldPath, heldErr := transitionPath(oldImage)
		if e != nil || user.User.Sid.String() != a.sid || tokenErr != nil || tokenSize != 4 || oldSession != a.session || old.Identity.Session != a.session || pathErr != nil || heldErr != nil || !strings.EqualFold(processPath, heldPath) || transitionProcessMatch(process, old.Identity, true) != nil || transitionImageHash(ctx, oldImage, old.Identity.Image) != nil {
			return nil, ErrState
		}
	}
	nameHash := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(a.home)) + "\x00" + g.Scope.Installation))
	if a.consumeControl() != nil {
		return nil, ErrRecoveryRequired
	}
	a.pipe, err = winio.ListenPipe(`\\.\pipe\OrbWinLocalServices-`+hex.EncodeToString(nameHash[:16]), &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + a.sid + ")", InputBufferSize: 8196, OutputBufferSize: 64 << 10})
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	if a.check(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	if a.holdFinalSlot(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	ok = true
	return &ColdRetirement{&coldRetirementState{scope: g.Scope, native: a}}, nil
}

func objectPrivate(h windows.Handle, sid string, dir bool) error {
	_, err := objectPolicy(h, sid, dir, true)
	if err != nil {
		return err
	}
	_, err = snapshot(h, dir)
	return err
}
func legacyTransitionObject(h windows.Handle, sid string, dir bool) error {
	if _, err := snapshot(h, dir); err != nil {
		return err
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return ErrState
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || owner.String() != sid {
		return ErrState
	}
	return nil
}
func transitionInherited(h windows.Handle) error {
	if h == 0 || h == windows.InvalidHandle {
		return ErrState
	}
	var flags uint32
	query := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation")
	result, _, _ := query.Call(uintptr(h), uintptr(unsafe.Pointer(&flags)))
	if result == 0 || flags&windows.HANDLE_FLAG_INHERIT == 0 || windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, 0) != nil {
		return ErrState
	}
	return nil
}
func transitionPath(h windows.Handle) (string, error) {
	var buf [2048]uint16
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil || n == 0 || n >= uint32(len(buf)) {
		return "", ErrState
	}
	return strings.TrimPrefix(windows.UTF16ToString(buf[:n]), `\\?\`), nil
}
func transitionProcessPath(h windows.Handle) (string, error) {
	var buf [2048]uint16
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil || n == 0 || n >= uint32(len(buf)) {
		return "", ErrState
	}
	return windows.UTF16ToString(buf[:n]), nil
}
func transitionProcessMatch(h windows.Handle, p transitionProcess, exited bool) error {
	var created, exit, kernel, user windows.Filetime
	var pid uint32
	getPID := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessId")
	value, _, _ := getPID.Call(uintptr(h))
	pid = uint32(value)
	if value == 0 || pid != p.PID || windows.GetProcessTimes(h, &created, &exit, &kernel, &user) != nil || uint64(created.HighDateTime)<<32|uint64(created.LowDateTime) != p.Creation {
		return ErrState
	}
	state, err := windows.WaitForSingleObject(h, 0)
	if err != nil || exited && state != uint32(windows.WAIT_OBJECT_0) || !exited && state != uint32(windows.WAIT_TIMEOUT) {
		return ErrState
	}
	return nil
}
func transitionImageHash(ctx context.Context, h windows.Handle, want [32]byte) error {
	if want == [32]byte{} {
		return ErrState
	}
	if _, err := snapshot(h, false); err != nil {
		return err
	}
	if _, err := windows.Seek(h, 0, io.SeekStart); err != nil {
		return err
	}
	hash := sha256.New()
	buf := make([]byte, 64<<10)
	defer clear(buf)
	var total uint64
	for {
		if !bounded(ctx) {
			return ErrState
		}
		var n uint32
		e := windows.ReadFile(h, buf, &n, nil)
		total += uint64(n)
		if total > 512<<20 {
			return ErrState
		}
		if n > 0 {
			_, _ = hash.Write(buf[:n])
		}
		if e == windows.ERROR_HANDLE_EOF || e == nil && n == 0 {
			break
		}
		if e != nil {
			return ErrState
		}
	}
	returnError := ErrState
	var actual [32]byte
	copy(actual[:], hash.Sum(nil))
	if actual == want {
		returnError = nil
	}
	return returnError
}
func transitionDataAccess(h windows.Handle) error {
	var access uint32
	var status windows.IO_STATUS_BLOCK
	call := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationFile")
	code, _, _ := call.Call(uintptr(h), uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&access)), 4, 8)
	if code != 0 || access&(windows.FILE_READ_DATA|windows.FILE_WRITE_DATA) != (windows.FILE_READ_DATA|windows.FILE_WRITE_DATA) {
		return ErrState
	}
	return nil
}
func (a *nativeColdAuthority) check(ctx context.Context) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	sid, session, identityErr := current()
	if identityErr != nil || sid != a.sid || session != a.session {
		return ErrRecoveryRequired
	}
	if !bounded(ctx) || a.closed || a.unknown || a.self == nil || a.pipe == nil || a.parent == 0 || ordinaryTransitionToken(a.parentToken, a.sid, a.session) != nil || legacyTransitionObject(a.controller, a.sid, false) != nil || legacyTransitionObject(a.legacy, a.sid, true) != nil {
		return ErrRecoveryRequired
	}
	if a.recheckDenial() != nil || a.recheckSupervisorOrigin() != nil || a.recheckPendingSupervisor() != nil {
		return ErrRecoveryRequired
	}
	identity, err := a.self.Identity()
	if err != nil || identity.ImageSHA256 != hex.EncodeToString(a.grant.Scope.SDKImage[:]) || transitionParent(a.grant.Parent.PID) != nil {
		return ErrRecoveryRequired
	}
	for i, p := range a.processes {
		if transitionProcessMatch(p, a.grant.Processes[i].Identity, true) != nil {
			return ErrRecoveryRequired
		}
	}
	return nil
}

// The grant must come from the actual launching rdd generation, not merely a
// live process with the same private image. The held process handle supplies
// creation identity; this independent snapshot supplies the kernel parent link.
func transitionParent(expected uint32) error {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ErrState
	}
	defer windows.CloseHandle(h)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(h, &entry)
	for err == nil {
		if entry.ProcessID == windows.GetCurrentProcessId() {
			if entry.ParentProcessID == expected {
				return nil
			}
			return ErrState
		}
		err = windows.Process32Next(h, &entry)
	}
	return ErrState
}
func (a *nativeColdAuthority) close() error { return ErrRecoveryRequired } // unknown/pending owner is inspection-only; no silent release

// TOKEN_STATISTICS is the Windows SDK winnt.h structure (also available in the
// pinned MinGW winnt.h). TokenId identifies the actual token object; merely
// supplying any token with the same SID/session does not bind it to a process.
type transitionTokenStatistics struct {
	TokenID, AuthenticationID                                                 windows.LUID
	Expiration                                                                int64
	Type, Impersonation, DynamicCharged, DynamicAvailable, Groups, Privileges uint32
	Modified                                                                  windows.LUID
}

func transitionTokenID(token windows.Token) (windows.LUID, error) {
	var facts transitionTokenStatistics
	var size uint32
	if windows.GetTokenInformation(token, windows.TokenStatistics, (*byte)(unsafe.Pointer(&facts)), uint32(unsafe.Sizeof(facts)), &size) != nil || size != uint32(unsafe.Sizeof(facts)) || facts.Type != 1 || facts.TokenID == (windows.LUID{}) {
		return windows.LUID{}, ErrState
	}
	return facts.TokenID, nil
}
func ordinaryTransitionToken(token windows.Token, sid string, session uint32) error {
	if token == 0 {
		return ErrState
	}
	user, err := token.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil || user.User.Sid.String() != sid {
		return ErrState
	}
	for _, query := range []struct{ class, want uint32 }{{windows.TokenElevation, 0}, {29, 0}, {windows.TokenSessionId, session}} {
		var value, size uint32
		if windows.GetTokenInformation(token, query.class, (*byte)(unsafe.Pointer(&value)), 4, &size) != nil || size != 4 || value != query.want {
			return ErrState
		}
	}
	restricted, err := token.IsRestricted()
	if err != nil || restricted {
		return ErrState
	}
	return nil
}
func (a *nativeColdAuthority) holdProcessToken(process windows.Handle, supplied windows.Token) (windows.Token, error) {
	a.tokens = append(a.tokens, 0)
	index := len(a.tokens) - 1
	err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &a.tokens[index])
	actual := a.tokens[index] // retain partial native output before examining error
	if err != nil || actual == 0 {
		return actual, ErrState
	}
	if ordinaryTransitionToken(actual, a.sid, a.session) != nil {
		return actual, ErrState
	}
	id, err := transitionTokenID(actual)
	if err != nil {
		return actual, err
	}
	if supplied != 0 {
		other, err := transitionTokenID(supplied)
		if err != nil || id != other || ordinaryTransitionToken(supplied, a.sid, a.session) != nil {
			return actual, ErrState
		}
	}
	return actual, nil
}

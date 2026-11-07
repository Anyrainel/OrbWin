//go:build windows

package sdkself

import (
	"context"
	"io"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Physical fixed-leaf resources ONLY; this type cannot issue a Root, source,
// cohort, SDK Ready or retirement grant. The SDKstate authority layer owns those
// admissions and must retain/verify native cohort custody before calling Resume.
var nativeLANPhysicalCustody = struct {
	sync.Mutex
	owners map[*NativeLANLeaf]struct{}
}{owners: map[*NativeLANLeaf]struct{}{}}

type NativeLANLeaf struct {
	mu                                                         sync.Mutex
	claim                                                      *NativeLANClaim
	lease                                                      *Lease
	process, thread, query, image, input, stderr               windows.Handle
	token                                                      windows.Token
	stdout, writer                                             *os.File
	expected                                                   Identity
	creation                                                   uint64
	createAttempted, resumeAttempted, resumed, joined, unknown bool
	closed                                                     bool
	closeErr                                                   error
	unknownCloses                                              []windows.Handle
	unknownFiles                                               []*os.File
}

// StartNativeLANObservation prepares ONLY a suspended fixed same-SDK physical
// leaf. It grants no source/cohort/Root admission and never resumes it itself.
func StartNativeLANObservation(ctx context.Context, lease *Lease, claim *NativeLANClaim) (*NativeLANLeaf, error) {
	if claim == nil || ctx == nil {
		return nil, ErrColdLeaf
	}
	if ctx.Err() != nil {
		return nil, nativeLANNoBirthFor(claim, ctx.Err())
	}
	if lease == nil || lease.leaseState == nil {
		return nil, nativeLANNoBirthFor(claim, ErrColdLeaf)
	}
	n := &NativeLANLeaf{claim: claim, lease: lease}
	nativeLANPhysicalCustody.Lock()
	nativeLANPhysicalCustody.owners[n] = struct{}{}
	nativeLANPhysicalCustody.Unlock()
	completed := false
	defer func() {
		if !completed {
			n.unknown = true
		}
	}()
	fail := func(cause error) (*NativeLANLeaf, error) {
		if !n.createAttempted && n.closeAllocated() == nil {
			nativeLANPhysicalCustody.Lock()
			delete(nativeLANPhysicalCustody.owners, n)
			nativeLANPhysicalCustody.Unlock()
			completed = true
			return nil, nativeLANNoBirthFor(claim, cause)
		}
		n.unknown = true
		completed = true
		return n, ErrColdLeaf
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.recheckLocked() != nil || lease.identity.PID != windows.GetCurrentProcessId() || lease.caller != nil {
		return fail(ErrColdLeaf)
	}
	n.expected = lease.identity
	var member uint32
	status, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&member)))
	if status == 0 || member != 1 {
		return fail(ErrColdLeaf)
	}
	// The existing native lease already excludes code writes/deletes through
	// its real profile/cache ancestors; reopen only this SAME fixed manager to
	// add class44 observation rights, never an arbitrary image execution role.
	path, err := windows.UTF16PtrFromString(lease.path)
	if err != nil {
		return fail(err)
	}
	n.image, err = windows.CreateFile(path, windows.FILE_READ_DATA|windows.FILE_EXECUTE|windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return fail(err)
	}
	actual, err := snapshot(n.image, false)
	if err != nil {
		return fail(err)
	}
	manager := ""
	for _, held := range lease.held {
		if !held.directory {
			manager = held.id
			break
		}
	}
	if manager == "" || actual != manager {
		return fail(ErrColdLeaf)
	}
	n.stdout, n.writer, err = os.Pipe()
	if err != nil {
		return fail(err)
	}
	if windows.SetHandleInformation(windows.Handle(n.writer.Fd()), windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT) != nil {
		return fail(ErrColdLeaf)
	}
	n.input, err = coldLANNull(windows.GENERIC_READ)
	if err != nil {
		return fail(err)
	}
	n.stderr, err = coldLANNull(windows.GENERIC_WRITE)
	if err != nil {
		return fail(err)
	}
	for _, h := range []windows.Handle{n.input, n.stderr} {
		if windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT) != nil {
			return fail(ErrColdLeaf)
		}
	}
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return fail(err)
	}
	defer attrs.Delete()
	handles := []windows.Handle{n.input, windows.Handle(n.writer.Fd()), n.stderr}
	if handles[0] == handles[1] || handles[0] == handles[2] || handles[1] == handles[2] || attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])) != nil {
		return fail(ErrColdLeaf)
	}
	root, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return fail(err)
	}
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return fail(err)
	}
	wd, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return fail(err)
	}
	envText := "SystemRoot=" + root + "\x00WINDIR=" + root + "\x00GOMAXPROCS=2\x00\x00"
	env := utf16.Encode([]rune(envText))
	line, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{lease.path, "lan-network-observation"}))
	if err != nil {
		return fail(err)
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES | windows.STARTF_USESHOWWINDOW
	si.ShowWindow = windows.SW_HIDE
	si.StdInput = n.input
	si.StdOutput = windows.Handle(n.writer.Fd())
	si.StdErr = n.stderr
	if ctx.Err() != nil {
		return fail(ctx.Err())
	}
	if ctx.Err() != nil {
		return fail(ctx.Err())
	}
	n.createAttempted = true // any CreateProcess ambiguity can never be no-birth
	var pi windows.ProcessInformation
	err = windows.CreateProcess(path, line, nil, nil, true, windows.CREATE_SUSPENDED|windows.CREATE_NO_WINDOW|windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT, &env[0], wd, &si.StartupInfo, &pi)
	n.process, n.thread = pi.Process, pi.Thread
	runtime.KeepAlive(handles)
	runtime.KeepAlive(env)
	if err != nil || n.process == 0 || n.thread == 0 {
		return fail(ErrColdLeaf)
	}
	var made, ended, kernel, user windows.Filetime
	if windows.GetProcessTimes(n.process, &made, &ended, &kernel, &user) != nil {
		return fail(ErrColdLeaf)
	}
	n.creation = uint64(made.HighDateTime)<<32 | uint64(made.LowDateTime)
	parentCreation, _ := strconv.ParseUint(lease.identity.CreationTime, 10, 64)
	if n.creation <= parentCreation {
		return fail(ErrColdLeaf)
	}
	var basic windows.PROCESS_BASIC_INFORMATION
	var length uint32
	if windows.NtQueryInformationProcess(n.process, 0, unsafe.Pointer(&basic), uint32(unsafe.Sizeof(basic)), &length) != nil || length != uint32(unsafe.Sizeof(basic)) || uint32(basic.UniqueProcessId) != pi.ProcessId || uint32(basic.InheritedFromUniqueProcessId) != lease.identity.PID {
		return fail(ErrColdLeaf)
	}
	if windows.OpenProcessToken(n.process, windows.TOKEN_QUERY, &n.token) != nil {
		return fail(ErrColdLeaf)
	}
	userToken, err := n.token.GetTokenUser()
	if err != nil || userToken.User.Sid.String() != lease.sid {
		return fail(ErrColdLeaf)
	}
	var elevated, session uint32
	if windows.GetTokenInformation(n.token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &length) != nil || length != 4 || elevated != 0 || windows.GetTokenInformation(n.token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4, &length) != nil || length != 4 || session != lease.identity.Session {
		return fail(ErrColdLeaf)
	}
	argument := n.image
	status, _, _ = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationProcess").Call(uintptr(n.process), 44, uintptr(unsafe.Pointer(&argument)), unsafe.Sizeof(argument), uintptr(unsafe.Pointer(&length)))
	if status != 0 || argument != n.image {
		return fail(ErrColdLeaf)
	}
	if windows.DuplicateHandle(windows.CurrentProcess(), n.process, windows.CurrentProcess(), &n.query, windows.PROCESS_QUERY_INFORMATION|windows.SYNCHRONIZE, false, 0) != nil {
		return fail(ErrColdLeaf)
	}
	var list struct {
		Assigned, Count uint32
		PIDs            [256]uintptr
	}
	if windows.QueryInformationJobObject(0, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&list)), uint32(unsafe.Sizeof(list)), nil) != nil || list.Count > 256 {
		return fail(ErrColdLeaf)
	}
	found := false
	for _, pid := range list.PIDs[:list.Count] {
		if uint32(pid) == pi.ProcessId {
			found = true
		}
	}
	if !found {
		return fail(ErrColdLeaf)
	}
	// Membership is independently rechecked by the actual SDKstate/Rdd cohort
	// adapter using its SAME owned job before this factory may Resume.
	writer := n.writer
	n.writer = nil
	writerHandle := windows.Handle(writer.Fd())
	if err = writer.Close(); err != nil {
		n.unknownFiles = append(n.unknownFiles, writer)
		n.unknownCloses = append(n.unknownCloses, writerHandle)
		n.unknown = true
		return n, ErrColdLeaf
	}
	completed = true
	return n, nil
}

func coldLANNull(access uint32) (windows.Handle, error) {
	text, err := windows.NewNTUnicodeString(`\Device\Null`)
	if err != nil {
		return 0, err
	}
	oa := windows.OBJECT_ATTRIBUTES{ObjectName: text, Attributes: windows.OBJ_CASE_INSENSITIVE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&h, access|windows.SYNCHRONIZE, &oa, &status, nil, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	return h, err
}

func (n *NativeLANLeaf) Resume(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if ctx == nil || n.unknown || n.resumeAttempted || !n.createAttempted || n.process == 0 || n.thread == 0 {
		return ErrColdLeaf
	}
	if err := ctx.Err(); err != nil {
		return &nativeLANNoResume{n.claim, err}
	}
	n.resumeAttempted = true
	previous, err := windows.ResumeThread(n.thread)
	if err != nil || previous != 1 {
		n.unknown = true
		return ErrColdLeaf
	}
	n.resumed = true
	return nil
}

func (n *NativeLANLeaf) NeverResumed() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return !n.unknown && n.createAttempted && n.process != 0 && n.thread != 0 && !n.resumeAttempted && !n.resumed
}

func (n *NativeLANLeaf) Collect(ctx context.Context) (payload []byte, queryErr, nativeErr error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.unknown || n.process == 0 || n.stdout == nil {
		return nil, nil, ErrColdLeaf
	}
	type readResult struct {
		data []byte
		err  error
	}
	readDone := make(chan readResult, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(n.stdout, (320<<10)+1))
		readDone <- readResult{data, err}
	}()
	stopping := false
	ownerCtx := ctx
	for {
		state, err := windows.WaitForSingleObject(n.process, 20)
		if err != nil {
			n.unknown = true
			return nil, nil, ErrColdLeaf
		}
		if state == uint32(windows.WAIT_OBJECT_0) {
			break
		}
		if state != uint32(windows.WAIT_TIMEOUT) {
			n.unknown = true
			return nil, nil, ErrColdLeaf
		}
		if ownerCtx.Err() != nil {
			if stopping {
				n.unknown = true
				return nil, nil, ErrColdLeaf
			}
			queryErr = ctx.Err()
			stopping = true
			if windows.TerminateProcess(n.process, 125) != nil {
				// Natural exit may win the timeout/termination race. Only actual
				// signal of this SAME retained process resolves that case; no
				// second termination or PID-based substitute is attempted.
				state, waitErr := windows.WaitForSingleObject(n.process, 0)
				if waitErr == nil && state == uint32(windows.WAIT_OBJECT_0) {
					break
				}
				n.unknown = true
				return nil, nil, ErrColdLeaf
			}
			var stopCancel context.CancelFunc
			ownerCtx, stopCancel = context.WithTimeout(context.Background(), 2*time.Second)
			defer stopCancel()
		}
	}
	var exit uint32
	var made, ended, kernel, user windows.Filetime
	if windows.GetExitCodeProcess(n.process, &exit) != nil || windows.GetProcessTimes(n.process, &made, &ended, &kernel, &user) != nil || uint64(made.HighDateTime)<<32|uint64(made.LowDateTime) != n.creation {
		n.unknown = true
		return nil, nil, ErrColdLeaf
	}
	if exit != 0 && queryErr == nil {
		queryErr = ErrColdLeaf
	}
	ioCtx, ioCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer ioCancel()
	select {
	case read := <-readDone:
		payload = read.data
		if read.err != nil || len(payload) > 320<<10 {
			queryErr = ErrColdLeaf
			payload = nil
		}
	case <-ioCtx.Done():
		n.unknown = true
		return nil, nil, ErrColdLeaf
	}
	n.joined = true
	return payload, queryErr, nil
}

func (n *NativeLANLeaf) closeAllocated() error {
	if n.unknown {
		return ErrColdLeaf
	}
	for _, file := range []**os.File{&n.writer, &n.stdout} {
		if *file != nil {
			f := *file
			*file = nil
			h := windows.Handle(f.Fd())
			if f.Close() != nil {
				n.unknownFiles = append(n.unknownFiles, f)
				n.unknownCloses = append(n.unknownCloses, h)
				n.unknown = true
				return ErrColdLeaf
			}
		}
	}
	for _, ptr := range []*windows.Handle{&n.thread, &n.query, &n.process, &n.image, &n.input, &n.stderr} {
		if *ptr != 0 {
			h := *ptr
			*ptr = 0
			if windows.CloseHandle(h) != nil {
				n.unknownCloses = append(n.unknownCloses, h)
				n.unknown = true
				return ErrColdLeaf
			}
		}
	}
	if n.token != 0 {
		token := n.token
		n.token = 0
		if token.Close() != nil {
			n.unknownCloses = append(n.unknownCloses, windows.Handle(token))
			n.unknown = true
			return ErrColdLeaf
		}
	}
	return nil
}
func (n *NativeLANLeaf) CloseKnown(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return n.closeErr
	}
	if ctx == nil || ctx.Err() != nil || !n.joined || n.unknown {
		return ErrColdLeaf
	}
	n.closed = true
	n.closeErr = ErrColdLeaf
	if n.closeAllocated() != nil {
		return ErrColdLeaf
	}
	n.closeErr = nil
	nativeLANPhysicalCustody.Lock()
	delete(nativeLANPhysicalCustody.owners, n)
	nativeLANPhysicalCustody.Unlock()
	return nil
}

// Actual cached physical completion only; no Root/cohort/source grant.
func (n *NativeLANLeaf) KnownClosed() bool {
	if n == nil {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.closed && n.joined && !n.unknown && n.closeErr == nil && n.process == 0 && n.thread == 0 && n.query == 0 && n.token == 0 && n.image == 0 && n.input == 0 && n.stderr == 0 && n.stdout == nil && n.writer == nil
}

func (n *NativeLANLeaf) KnownJoined() bool {
	if n == nil {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.joined && !n.unknown && n.process != 0 && n.query != 0
}

// Native trusted adapter access only: read/query/synchronize handle owned by
// this actual leaf, never a path/PID/native-handle constructor. The cohort must
// duplicate/retain it before returning RetainBeforeResume; borrowed value dies
// only after its actual known stopped/checked-close sequence.
func (n *NativeLANLeaf) NativeCohortProcess() (windows.Handle, uint32, uint64, error) {
	if n == nil {
		return 0, 0, 0, ErrColdLeaf
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.unknown || n.closed || n.query == 0 || n.process == 0 {
		return 0, 0, 0, ErrColdLeaf
	}
	pid, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessId").Call(uintptr(n.process))
	if pid == 0 {
		return 0, 0, 0, ErrColdLeaf
	}
	return n.query, uint32(pid), n.creation, nil
}

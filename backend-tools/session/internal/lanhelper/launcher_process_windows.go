//go:build windows && amd64

package lanhelper

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This is the ordinary, owned launcher ONLY, never the elevated helper. A
// post-Create error returns this nonnil owner so Stop/Wait cannot be abandoned.
type ordinaryLauncher struct {
	process, job windows.Handle
	pid          uint32
	creation     string
	assigned     bool
	stop         sync.Once
	done         chan struct{}
	mu           sync.Mutex
	result       error
}

func (launcher *ordinaryLauncher) Stop() {
	launcher.stop.Do(func() {
		if launcher.assigned {
			_ = windows.TerminateJobObject(launcher.job, 125)
		} else {
			_ = windows.TerminateProcess(launcher.process, 125)
		}
	})
}
func (launcher *ordinaryLauncher) Wait(ctx context.Context) error {
	if ctx == nil {
		return errors.New("launcher wait context required")
	}
	select {
	case <-launcher.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (launcher *ordinaryLauncher) release() error {
	select {
	case <-launcher.done:
	default:
		return errors.New("cannot release unjoined ordinary launcher")
	}
	var err error
	if launcher.job != 0 {
		err = windows.CloseHandle(launcher.job)
		launcher.job = 0
	}
	if launcher.process != 0 {
		err = errors.Join(err, windows.CloseHandle(launcher.process))
		launcher.process = 0
	}
	return err
}

func startOrdinaryLauncher(ctx context.Context, authority *AuthorizedInstallation, transaction string) (launcher *ordinaryLauncher, err error) {
	if ctx == nil || authority == nil || authority.manager == nil || authority.worker == nil || !recordID(transaction) {
		return nil, errors.New("signed native ordinary launcher owner required")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS | windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION
	limits.BasicLimitInformation.ActiveProcessLimit = 1
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	program, err := windows.UTF16PtrFromString(authority.manager.path)
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	command, _ := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{authority.manager.path, "lan-elevation-launcher", authority.installation, transaction}))
	system, err := windows.GetSystemDirectory()
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	directory, _ := windows.UTF16PtrFromString(system)
	windowsDirectory, err := windows.GetWindowsDirectory()
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	environment := utf16.Encode([]rune(strings.Join([]string{"SystemRoot=" + windowsDirectory, "WINDIR=" + windowsDirectory, "GOMAXPROCS=2"}, "\x00") + "\x00\x00"))
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var process windows.ProcessInformation
	if err = windows.CreateProcess(program, command, nil, nil, false, windows.CREATE_SUSPENDED|windows.CREATE_NO_WINDOW|windows.CREATE_UNICODE_ENVIRONMENT, &environment[0], directory, &startup, &process); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	launcher = &ordinaryLauncher{process: process.Process, job: job, pid: process.ProcessId, done: make(chan struct{})}
	defer windows.CloseHandle(process.Thread)
	go func() {
		state, e := windows.WaitForSingleObject(process.Process, windows.INFINITE)
		if e != nil || state != uint32(windows.WAIT_OBJECT_0) {
			launcher.mu.Lock()
			launcher.result = errors.New("ordinary launcher exit remains unknown")
			launcher.mu.Unlock()
			return
		}
		close(launcher.done)
	}()
	if err = windows.AssignProcessToJobObject(job, process.Process); err != nil {
		launcher.Stop()
		return launcher, errors.New("ordinary launcher job assignment failed")
	}
	launcher.assigned = true
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(process.Process, &created, &exited, &kernel, &user); err != nil {
		launcher.Stop()
		return launcher, err
	}
	launcher.creation = strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10)
	sid, session, err := nativeToken(process.Process, false)
	if err != nil || sid != authority.manager.identity.UserSID || session != authority.manager.session {
		launcher.Stop()
		return launcher, errors.New("ordinary launcher native token/session differs")
	}
	if err = ctx.Err(); err != nil {
		launcher.Stop()
		return launcher, err
	}
	if count, e := windows.ResumeThread(process.Thread); e != nil || count != 1 {
		launcher.Stop()
		return launcher, errors.New("ordinary launcher resume failed")
	}
	return launcher, nil
}

// Duplicates FROM the retained original ordinary child. No target handle is
// created in SDK by an unacknowledged remote DuplicateHandle operation.
func (launcher *ordinaryLauncher) duplicateHelper(frame launchFrame) (child *elevatedProcess, err error) {
	if frame.Kind != "handle" || frame.validate() != nil {
		return nil, errors.New("native launcher handle frame required")
	}
	value, _ := strconv.ParseUint(frame.SourceHandle, 10, 64)
	var target windows.Handle
	if err = windows.DuplicateHandle(launcher.process, windows.Handle(value), windows.CurrentProcess(), &target, windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, 0); err != nil {
		return nil, errors.New("exact launcher helper handle duplication failed")
	}
	keep := false
	defer func() {
		if !keep {
			windows.CloseHandle(target)
		}
	}()
	pid, err := windows.GetProcessId(target)
	if err != nil || pid != frame.PID {
		return nil, errors.New("duplicated helper process identity differs")
	}
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(target, &created, &exited, &kernel, &user); err != nil || strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10) != frame.Creation {
		return nil, errors.New("duplicated helper creation identity differs")
	}
	child = retainElevatedProcess(target)
	child.pid = pid
	child.creation = frame.Creation
	keep = true
	return child, nil
}

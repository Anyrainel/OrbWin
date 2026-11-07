//go:build windows

package main

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"
)

const clientAttributeJobList = 0x0002000d
const clientAttributeHandleList = 0x00020002

type nativeClientIO struct {
	file    *os.File
	done    chan error
	stdin   bool
	start   func()
	started bool
}
type nativeClientTree struct {
	registry     *clientOwnership
	mu           sync.Mutex
	handles      map[windows.Handle]func() error
	job, process windows.Handle
	ios          []*nativeClientIO
	outputMu     sync.Mutex
	ioFault      chan error
}

func (t *nativeClientTree) own(h windows.Handle, closeHandle func() error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.registry.retain(uintptr(h))
	t.handles[h] = closeHandle
}
func (t *nativeClientTree) closeOne(h windows.Handle) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	closeHandle, ok := t.handles[h]
	if !ok {
		return nil
	}
	if err := t.registry.close(uintptr(h), closeHandle); err != nil {
		return err
	}
	delete(t.handles, h)
	return nil
}
func (t *nativeClientTree) close() error {
	t.mu.Lock()
	handles := make([]windows.Handle, 0, len(t.handles))
	for h := range t.handles {
		handles = append(handles, h)
	}
	t.mu.Unlock()
	var err error
	for _, h := range handles {
		err = errors.Join(err, t.closeOne(h))
	}
	return err
}
func (t *nativeClientTree) duplicate(h windows.Handle) (windows.Handle, error) {
	var inherited windows.Handle
	p := windows.CurrentProcess()
	if err := windows.DuplicateHandle(p, h, p, &inherited, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return 0, err
	}
	t.own(inherited, func() error { return windows.CloseHandle(inherited) })
	return inherited, nil
}
func (t *nativeClientTree) stream(value any, input bool) (windows.Handle, error) {
	if value == nil {
		flags := os.O_WRONLY
		if input {
			flags = os.O_RDONLY
		}
		file, err := os.OpenFile(os.DevNull, flags, 0)
		if err != nil {
			return 0, err
		}
		handle := windows.Handle(file.Fd())
		t.own(handle, file.Close)
		inherited, e := t.duplicate(handle)
		return inherited, errors.Join(e, t.closeOne(handle))
	}
	if file, ok := value.(*os.File); ok {
		return t.duplicate(windows.Handle(file.Fd()))
	}
	var read, write windows.Handle
	// Fixed 64KiB kernel buffers; only the child endpoint is inheritable.
	if err := windows.CreatePipe(&read, &write, nil, 64<<10); err != nil {
		return 0, err
	}
	t.own(read, func() error { return windows.CloseHandle(read) })
	t.own(write, func() error { return windows.CloseHandle(write) })
	parent, child := read, write
	if input {
		parent, child = write, read
	}
	inherited, err := t.duplicate(child)
	if err != nil {
		return 0, err
	}
	if err = t.closeOne(child); err != nil {
		return 0, err
	}
	// Transfer ownership of the parent handle to os.File, retaining the same
	// registry entry. No unsafe os.Process representation or PID reopen exists.
	file := os.NewFile(uintptr(parent), "orbwin-client-io")
	t.mu.Lock()
	t.handles[parent] = file.Close
	t.mu.Unlock()
	item := &nativeClientIO{file: file, done: make(chan error, 1), stdin: input}
	t.ios = append(t.ios, item)
	if input {
		reader, ok := value.(io.Reader)
		if !ok {
			return 0, errors.New("client stdin is not readable")
		}
		item.start = func() {
			_, err := io.CopyBuffer(file, reader, make([]byte, 32<<10))
			err = errors.Join(err, t.closeOne(parent))
			if err != nil {
				_ = t.terminate()
				select {
				case t.ioFault <- err:
				default:
				}
			}
			item.done <- err
		}
	} else {
		writer, ok := value.(io.Writer)
		if !ok {
			return 0, errors.New("client output is not writable")
		}
		item.start = func() {
			_, err := io.CopyBuffer(clientLockedWriter{tree: t, writer: writer}, file, make([]byte, 32<<10))
			if err != nil {
				_ = t.terminate()
				select {
				case t.ioFault <- err:
				default:
				}
			}
			item.done <- err
		}
	}
	return inherited, nil
}

type clientLockedWriter struct {
	tree   *nativeClientTree
	writer io.Writer
}

func (w clientLockedWriter) Write(p []byte) (int, error) {
	w.tree.outputMu.Lock()
	defer w.tree.outputMu.Unlock()
	return w.writer.Write(p)
}

func createNativeClient(ctx context.Context, cmd *exec.Cmd, registry *clientOwnership) (result *nativeClientTree, resultErr error) {
	return createNativeClientAdmitted(ctx, cmd, registry, nil)
}

func createNativeClientAdmitted(ctx context.Context, cmd *exec.Cmd, registry *clientOwnership, beforeResume func(uintptr) error) (result *nativeClientTree, resultErr error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if cmd == nil || cmd.Process != nil || cmd.Path == "" || !filepath.IsAbs(cmd.Path) || len(cmd.Args) == 0 || len(cmd.ExtraFiles) != 0 {
		return nil, errors.New("closed absolute transient client command required")
	}
	if cmd.Err != nil {
		return nil, cmd.Err
	}
	if a := cmd.SysProcAttr; a != nil && (a.CreationFlags != 0 || a.Token != 0 || a.ParentProcess != 0 || len(a.AdditionalInheritedHandles) != 0 || a.NoInheritHandles || a.CmdLine != "" || a.ProcessAttributes != nil || a.ThreadAttributes != nil) {
		return nil, errors.New("unsupported transient process attributes; no dispatch")
	}
	t := &nativeClientTree{registry: registry, handles: map[windows.Handle]func() error{}, ioFault: make(chan error, 1)}
	// Before CreateProcess, no writer exists. Partial IO remains owned and joined;
	// uncertain cleanup latches the registry rather than permitting another run.
	defer func() {
		if resultErr != nil && t.process == 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			closeErr := t.close()
			_, joinErr := t.finishIO(ctx)
			if joinErr != nil || closeErr != nil {
				registry.markUnknown()
				registry.retainOwner(t)
			}
			resultErr = errors.Join(resultErr, joinErr, closeErr)
		}
	}()
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	t.job = job
	t.own(job, func() error { return windows.CloseHandle(job) })
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return nil, err
	}
	handles := make([]windows.Handle, 3)
	values := []any{cmd.Stdin, cmd.Stdout, cmd.Stderr}
	for i, value := range values {
		handles[i], err = t.stream(value, i == 0)
		if err != nil {
			return nil, err
		}
	}
	attrs, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil, err
	}
	defer attrs.Delete()
	jobs := []windows.Handle{job}
	if err = attrs.Update(clientAttributeJobList, unsafe.Pointer(&jobs[0]), unsafe.Sizeof(job)); err != nil {
		return nil, err
	}
	if err = attrs.Update(clientAttributeHandleList, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, err
	}
	si := windows.StartupInfoEx{}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = handles[0], handles[1], handles[2]
	si.ProcThreadAttributeList = attrs.List()
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.HideWindow {
		si.Flags |= windows.STARTF_USESHOWWINDOW
		si.ShowWindow = windows.SW_HIDE
	}
	application, err := windows.UTF16PtrFromString(cmd.Path)
	if err != nil {
		return nil, err
	}
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(cmd.Args))
	if err != nil {
		return nil, err
	}
	var directory *uint16
	if cmd.Dir != "" {
		directory, err = windows.UTF16PtrFromString(cmd.Dir)
		if err != nil {
			return nil, err
		}
	}
	env := cmd.Environ()
	for _, entry := range env {
		if strings.ContainsRune(entry, 0) {
			return nil, errors.New("client environment contains NUL")
		}
	}
	block, err := clientEnvironmentBlock(env)
	if err != nil {
		return nil, err
	}
	pi := windows.ProcessInformation{}
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	err = windows.CreateProcess(application, command, nil, nil, true, flags, &block[0], directory, &si.StartupInfo, &pi)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(jobs)
	runtime.KeepAlive(block)
	runtime.KeepAlive(attrs)
	if err != nil {
		return nil, err
	}
	t.process = pi.Process
	t.own(pi.Process, func() error { return windows.CloseHandle(pi.Process) })
	t.own(pi.Thread, func() error { return windows.CloseHandle(pi.Thread) })
	for _, item := range t.ios {
		item.started = true
		go item.start()
	}
	for _, handle := range handles {
		if e := t.closeOne(handle); e != nil {
			err = errors.Join(err, e)
		}
	}
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil {
		resume := func() error {
			count, e := windows.ResumeThread(pi.Thread)
			if e == nil && count != 1 {
				return errors.New("client initial thread suspension differs")
			}
			return e
		}
		if beforeResume != nil {
			err = writerBeforeFirstResume(ctx, func() error { return beforeResume(uintptr(pi.Process)) }, resume)
		} else if ctx.Err() != nil {
			err = ctx.Err()
		} else {
			err = resume()
		}
	}
	err = errors.Join(err, t.closeOne(pi.Thread))
	if err != nil {
		_ = t.terminate()
	}
	return t, err
}
func clientEnvironmentBlock(env []string) ([]uint16, error) {
	var block []uint16
	for _, entry := range env {
		encoded, err := windows.UTF16FromString(entry)
		if err != nil {
			return nil, err
		}
		block = append(block, encoded...)
		if len(block) > 1<<19 {
			return nil, errors.New("client environment exceeds 1MiB bound")
		}
	}
	block = append(block, 0)
	if len(env) == 0 {
		block = append(block, 0)
	}
	return block, nil
}
func (t *nativeClientTree) waitRoot(ctx context.Context) (uint32, error) {
	for {
		status, err := windows.WaitForSingleObject(t.process, 20)
		if err != nil {
			return 0, err
		}
		if status == windows.WAIT_OBJECT_0 {
			var code uint32
			err = windows.GetExitCodeProcess(t.process, &code)
			return code, err
		}
		if status != uint32(windows.WAIT_TIMEOUT) {
			return 0, errors.New("unexpected client process wait status")
		}
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		select {
		case err := <-t.ioFault:
			return 0, err
		default:
		}
	}
}
func (t *nativeClientTree) terminate() error {
	if t.job == 0 {
		return nil
	}
	return windows.TerminateJobObject(t.job, 125)
}
func (t *nativeClientTree) empty() (bool, error) {
	var a struct {
		User, Kernel, PeriodUser, PeriodKernel int64
		Faults, Total, Active, Terminated      uint32
	}
	err := windows.QueryInformationJobObject(t.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&a)), uint32(unsafe.Sizeof(a)), nil)
	return a.Active == 0, err
}
func (t *nativeClientTree) finishIO(ctx context.Context) (error, error) {
	// Closing parent stdin unblocks native writes on child exit. An arbitrary
	// caller Reader/Writer may still block: no positive IO join/token then exists.
	for _, item := range t.ios {
		if item.stdin {
			_ = t.closeOne(windows.Handle(item.file.Fd()))
		}
	}
	var copyErr error
	for _, item := range t.ios {
		if !item.started {
			continue
		}
		select {
		case err := <-item.done:
			copyErr = errors.Join(copyErr, err)
		case <-ctx.Done():
			return copyErr, ctx.Err()
		}
	}
	return copyErr, nil
}

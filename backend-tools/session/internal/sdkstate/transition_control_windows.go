//go:build windows

package sdkstate

import (
	"context"
	"golang.org/x/sys/windows"
	"unsafe"
)

// ColdControl borrows the original grant-authenticated lifecycle pipe. No path,
// JSON, raw-handle constructor or Close is exported; native final owner closes
// only after all actual runtime/server/control operations are positively joined.
type ColdControl struct {
	authority *nativeColdAuthority
	io        *transitionControlIO
}

func (a *nativeColdAuthority) consumeControl() error {
	h := windows.Handle(a.grant.Control)
	a.control = h // Retain the raw owner even on partial native admission.
	if transitionInherited(h) != nil {
		return ErrState
	}
	kind, err := windows.GetFileType(h)
	if err != nil || kind != windows.FILE_TYPE_PIPE {
		return ErrState
	}
	if _, err := objectPolicy(h, a.sid, false, true); err != nil {
		return ErrState
	}
	var clientPID, flags, output, input, instances uint32
	if windows.GetNamedPipeClientProcessId(h, &clientPID) != nil || clientPID != a.grant.Parent.PID || windows.GetNamedPipeInfo(h, &flags, &output, &input, &instances) != nil || flags&windows.PIPE_SERVER_END == 0 || flags&windows.PIPE_TYPE_MESSAGE != 0 || instances != 1 {
		return ErrState
	}
	var mode uint32
	var status windows.IO_STATUS_BLOCK
	code, _, _ := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationFile").Call(uintptr(h), uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&mode)), 4, 16)
	if code != 0 || mode&(windows.FILE_SYNCHRONOUS_IO_ALERT|windows.FILE_SYNCHRONOUS_IO_NONALERT) != 0 {
		return ErrState
	}
	a.controlIO = &transitionControlIO{pipe: h}
	return nil
}

func (r *ColdRetirement) NativeControl(ctx context.Context) (*ColdControl, error) {
	if r == nil || r.s == nil || !bounded(ctx) {
		return nil, ErrState
	}
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	a, ok := r.s.native.(*nativeColdAuthority)
	if !ok || a == nil {
		return nil, ErrRecoveryRequired
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.controlTaken || a.controlIO == nil || a.check(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	a.controlTaken = true
	return &ColdControl{a, a.controlIO}, nil
}

func (c *ColdControl) Read(ctx context.Context, body []byte) (int, error) {
	if c == nil || c.authority == nil || c.io == nil || !bounded(ctx) || c.authority.check(ctx) != nil {
		return 0, ErrRecoveryRequired
	}
	return c.io.perform(ctx, body, false)
}
func (c *ColdControl) Write(ctx context.Context, body []byte) (int, error) {
	if c == nil || c.authority == nil || c.io == nil || !bounded(ctx) || c.authority.check(ctx) != nil {
		return 0, ErrRecoveryRequired
	}
	return c.io.perform(ctx, body, true)
}

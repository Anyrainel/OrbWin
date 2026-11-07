//go:build windows && amd64

package lanprovider

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"regexp"
	"runtime"
	"unsafe"
)

var ownedNamePattern = regexp.MustCompile(`^OrbWin-LAN-[0-9a-f]{32}-[0-9a-f]{16}$`)

type ownedNativeBackend struct{ declarations traversalDeclarations }

// Only the helper's actual unimpersonated elevated process is admitted. No
// delegated provider rights, supplied token, privilege adjustment or elevation.
func requireOwnedElevation(ctx context.Context) error {
	if ctx == nil {
		return errors.New("bounded LAN operation context required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var token windows.Token
	thread, err := windows.GetCurrentThread()
	if err != nil {
		return errors.New("LAN helper current thread unavailable")
	}
	err = windows.OpenThreadToken(thread, windows.TOKEN_QUERY, true, &token)
	if err == nil {
		_ = token.Close()
		return errors.New("unimpersonated elevated LAN helper required")
	}
	if err != windows.ERROR_NO_TOKEN {
		return errors.New("LAN helper effective token unavailable")
	}
	var elevation, size uint32
	if err = windows.GetTokenInformation(windows.GetCurrentProcessToken(), windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), 4, &size); err != nil || !ownedElevationAllowed(elevation, size, true) {
		return errors.New("actual elevated LAN helper token required")
	}
	return nil
}

func newOwnedNativeBackend(ctx context.Context) (ownedBackend, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := requireOwnedElevation(ctx); err != nil {
		return nil, err
	}
	d, err := readTraversalDeclarations(ctx)
	if err != nil {
		return nil, err
	}
	return &ownedNativeBackend{d}, nil
}
func (b *ownedNativeBackend) Create(ctx context.Context, p ownedPolicy, ready func() error) (ownedCompletion, error) {
	if err := requireOwnedElevation(ctx); err != nil {
		return ownedCompletion{}, err
	}
	if ready == nil || !ownedNamePattern.MatchString(p.receipt.RuleName) {
		return ownedCompletion{}, errors.New("closed LAN create intent required")
	}
	c := ownedDispatchCreate(ctx, p, b.declarations.rule, ready)
	if !c.dispatched || !c.terminal || !c.closed || !c.returnedFullKeys || c.status != 0 {
		return c, errors.New("native LAN Create not positively settled")
	}
	return c, nil
}
func (b *ownedNativeBackend) Read(ctx context.Context, k observedRuleKeys) (ownedDefinition, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := requireOwnedElevation(ctx); err != nil {
		return ownedDefinition{}, err
	}
	return ownedReadDefinition(ctx, k, b.declarations)
}
func (b *ownedNativeBackend) Delete(ctx context.Context, k observedRuleKeys, ready func() error) error {
	if err := requireOwnedElevation(ctx); err != nil {
		return err
	}
	if ready == nil {
		return errors.New("protected LAN removal intent required")
	}
	return ownedDispatchDelete(ctx, k, b.declarations.rule, ready)
}
func (b *ownedNativeBackend) Absent(ctx context.Context, name string) (bool, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := requireOwnedElevation(ctx); err != nil {
		return false, err
	}
	return ownedNameAbsent(ctx, name, b.declarations.rule)
}

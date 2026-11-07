//go:build windows

package networkdiag

import (
	"context"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var winHTTP = windows.NewLazySystemDLL("winhttp.dll")
var getUserProxy = winHTTP.NewProc("WinHttpGetIEProxyConfigForCurrentUser")
var getDefaultProxy = winHTTP.NewProc("WinHttpGetDefaultProxyConfiguration")
var freeGlobal = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree")

type userProxy struct {
	autoDetect         uint32
	pac, proxy, bypass *uint16
}
type defaultProxy struct {
	access        uint32
	proxy, bypass *uint16
}

// Only the first WCHAR is inspected, to distinguish null/empty from present.
// OS-allocated strings never become Go strings or serialization inputs.
func present(value *uint16) bool { return value != nil && *value != 0 }
func release(values ...*uint16) bool {
	ok := true
	for _, value := range values {
		if value != nil {
			result, _, _ := freeGlobal.Call(uintptr(unsafe.Pointer(value)))
			if result != 0 {
				ok = false
			}
		}
	}
	return ok
}
func observeProxy(ctx context.Context) (ProxySnapshot, error) {
	result := ProxySnapshot{Version: 1, CurrentUser: unknownConfiguration("winhttp-current-user-configuration"), WinHTTPDefault: unknownConfiguration("winhttp-default-registry-configuration")}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var user userProxy
	status, _, _ := getUserProxy.Call(uintptr(unsafe.Pointer(&user)))
	if status != 0 {
		result.CurrentUser = configuration(result.CurrentUser.Source, user.autoDetect != 0, present(user.pac), present(user.proxy), present(user.bypass))
	}
	if !release(user.pac, user.proxy, user.bypass) {
		result.CurrentUser = unknownConfiguration(result.CurrentUser.Source)
		return result, errors.New("proxy configuration allocation release failed")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var defaults defaultProxy
	status, _, _ = getDefaultProxy.Call(uintptr(unsafe.Pointer(&defaults)))
	if status != 0 && (defaults.access == 1 || defaults.access == 3) {
		manual := present(defaults.proxy)
		if defaults.access == 1 && !manual || defaults.access == 3 && manual {
			result.WinHTTPDefault = configuration(result.WinHTTPDefault.Source, false, false, manual, present(defaults.bypass))
		}
	}
	if !release(defaults.proxy, defaults.bypass) {
		result.WinHTTPDefault = unknownConfiguration(result.WinHTTPDefault.Source)
		return result, errors.New("proxy configuration allocation release failed")
	}
	return result, ctx.Err()
}

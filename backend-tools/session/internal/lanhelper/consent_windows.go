//go:build windows && amd64

package lanhelper

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
	"orbwin.local/session/internal/lanobserve"
	"orbwin.local/session/internal/lanprofile"
)

// SDK CommCtrl.h wraps both structures in pack(1), including amd64 pointers.
// Default Go structs are WRONG here. Explicit byte offsets are compiled against
// SDK10.0.22621.0 in the source proof; every backing Go object remains pinned.
const taskDialogConfigBytes = 160
const taskDialogButtonBytes = 12

func packedTaskDialog(title, instruction, content, label *uint16, callback uintptr) (config *[taskDialogConfigBytes]byte, button *[taskDialogButtonBytes]byte) {
	config = new([taskDialogConfigBytes]byte)
	button = new([taskDialogButtonBytes]byte)
	binary.LittleEndian.PutUint32(button[0:4], 1000)
	binary.LittleEndian.PutUint64(button[4:12], uint64(uintptr(unsafe.Pointer(label))))
	binary.LittleEndian.PutUint32(config[0:4], taskDialogConfigBytes)
	binary.LittleEndian.PutUint32(config[20:24], 0x800|0x8)
	binary.LittleEndian.PutUint32(config[24:28], 8)
	binary.LittleEndian.PutUint64(config[28:36], uint64(uintptr(unsafe.Pointer(title))))
	binary.LittleEndian.PutUint64(config[36:44], 0xfffc)
	binary.LittleEndian.PutUint64(config[44:52], uint64(uintptr(unsafe.Pointer(instruction))))
	binary.LittleEndian.PutUint64(config[52:60], uint64(uintptr(unsafe.Pointer(content))))
	binary.LittleEndian.PutUint32(config[60:64], 1)
	binary.LittleEndian.PutUint64(config[64:72], uint64(uintptr(unsafe.Pointer(button))))
	binary.LittleEndian.PutUint32(config[72:76], 2)
	binary.LittleEndian.PutUint64(config[140:148], uint64(callback))
	return config, button
}

var taskDialogIndirect = windows.NewLazySystemDLL("comctl32.dll").NewProc("TaskDialogIndirect")
var postDialogMessage = windows.NewLazySystemDLL("user32.dll").NewProc("PostMessageW")

func observeConsentInterface(ctx context.Context, scope lanfirewall.Scope) (lanfirewall.InterfaceBinding, error) {
	interfaces, err := lanobserve.Observe(ctx)
	if err != nil {
		return lanfirewall.InterfaceBinding{}, err
	}
	profiles, err := lanprofile.Observe(ctx, []string{scope.InterfaceGUID})
	if err != nil {
		return lanfirewall.InterfaceBinding{}, err
	}
	return lanfirewall.BindInterface(scope, interfaces, profiles, time.Now())
}

// confirmNativeScope is private to the future one-shot entrypoint. UAC alone
// cannot mint this authority. No request/JSON field can set a consent boolean.
// Refusal/timeout happens before protected provisioning or provider dispatch.
func confirmNativeScope(ctx context.Context, authority *AuthorizedInstallation, record OwnerReview, cleanup *lanlease.Spec) (*scopeConsent, error) {
	if authority == nil || authority.worker == nil || authority.manager == nil {
		return nil, errors.New("signed held installation required for native scope review")
	}
	manager := authority.manager
	if err := manager.checkPipe(manager.pipe); err != nil {
		return nil, err
	}
	if record.Stage.Base.Installation != authority.installation || record.Validate(manager.identity, time.Now()) != nil {
		return nil, errors.New("exact current owner review required")
	}
	if record.Purpose == "activate" && record.Stage.Base.Scope.WorkerSHA256 != authority.assets.Assets[2].SHA256 {
		return nil, errors.New("review worker differs from signed dedicated asset")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	binding, err := consentInterface(ctx, record, cleanup, observeConsentInterface)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if taskDialogIndirect.Find() != nil || postDialogMessage.Find() != nil {
		return nil, errors.New("native exact-scope review is unavailable")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	title, _ := windows.UTF16PtrFromString("OrbWin LAN access")
	instructionText := "Allow this exact LAN port?"
	buttonText := "Allow this port"
	if record.Purpose == "remove" || record.Purpose == "cleanup-remove" {
		instructionText = "Remove this exact OrbWin LAN rule?"
		buttonText = "Remove this rule"
	}
	if record.Purpose == "settle-remove" || record.Purpose == "cleanup-settle" {
		instructionText = "Confirm this OrbWin LAN rule is already absent?"
		buttonText = "Confirm completed removal"
	}
	instruction, _ := windows.UTF16PtrFromString(instructionText)
	scope := record.Stage.Base.Scope
	content, _ := windows.UTF16PtrFromString(fmt.Sprintf("Interface: %s\nGUID: %s / LUID: %s\nAddress: %s:%d (%s)\nProfile: %s\nClients: %s\nContainer: %s:%d\nWorker SHA-256: %s\nReview SHA-256: %s\nExisting lease: %s", binding.Alias, scope.InterfaceGUID, scope.InterfaceLUID, scope.LocalAddress, scope.LocalPort, strings.ToUpper(scope.Protocol), scope.Profile, strings.Join(scope.ClientNetworks, ", "), record.Stage.Publication.ContainerID, record.Stage.Publication.ContainerPort, scope.WorkerSHA256, record.ReviewSHA256, record.LeaseSHA256))
	buttonLabel, _ := windows.UTF16PtrFromString(buttonText)
	deadline, _ := ctx.Deadline()
	callback := syscall.NewCallback(func(hwnd, notification, wParam, lParam, data uintptr) uintptr {
		if notification == 4 && (ctx.Err() != nil || !time.Now().Before(deadline)) {
			postDialogMessage.Call(hwnd, 0x400+102, 2, 0)
		} // TDN_TIMER/TDM_CLICK_BUTTON(IDCANCEL).
		return 0
	})
	config, button := packedTaskDialog(title, instruction, content, buttonLabel, callback)
	var pinned runtime.Pinner
	for _, pointer := range []any{config, button, title, instruction, content, buttonLabel} {
		pinned.Pin(pointer)
	}
	defer pinned.Unpin()
	var selected int32
	pinned.Pin(&selected)
	result, _, _ := taskDialogIndirect.Call(uintptr(unsafe.Pointer(config)), uintptr(unsafe.Pointer(&selected)), 0, 0)
	runtime.KeepAlive(config)
	runtime.KeepAlive(button)
	runtime.KeepAlive(title)
	runtime.KeepAlive(instruction)
	runtime.KeepAlive(content)
	runtime.KeepAlive(buttonLabel)
	if result != 0 || selected != 1000 || ctx.Err() != nil || !time.Now().Before(deadline) {
		return nil, errors.New("native exact-scope review cancelled, timed out or unavailable")
	}
	current, err := consentInterface(ctx, record, cleanup, observeConsentInterface)
	if err != nil || current != binding || record.Validate(manager.identity, time.Now()) != nil || manager.checkPipe(manager.pipe) != nil {
		return nil, errors.New("scope or owner changed during native confirmation; review again")
	}
	return &scopeConsent{authority: authority, reviewSHA: record.ReviewSHA256, purpose: record.Purpose, expires: time.Now().Add(120 * time.Second)}, nil
}

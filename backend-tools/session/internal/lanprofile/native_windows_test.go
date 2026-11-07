//go:build windows

package lanprofile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanobserve"
)

func TestProfileReadOnlyNative(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_LAN_PROFILE") != "1" {
		t.Skip("opt-in native read-only Network List Manager observation")
	}
	// Force Observe through COM's S_FALSE initialization path. On a fresh MTA,
	// a later STA initialization proves the nested reference was balanced.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initial := windows.CoInitializeEx(0, 0)
	if initial != nil && !errors.Is(initial, syscall.Errno(1)) {
		t.Fatal(initial)
	}
	initialized := true
	defer func() {
		if initialized {
			windows.CoUninitialize()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	interfaces, err := lanobserve.Observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	guids := []string{}
	for _, adapter := range interfaces.Adapters {
		guids = append(guids, adapter.GUID)
	}
	snapshot, err := Observe(ctx, guids)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Complete || snapshot.ActivationAvailable || len(snapshot.Profiles) != len(guids) {
		t.Fatalf("profile observation mismatch: %+v", snapshot)
	}
	for _, profile := range snapshot.Profiles {
		if profile.ActivationAvailable {
			t.Fatal("profile authorized LAN exposure")
		}
	}
	data, err := json.Marshal(snapshot)
	if err != nil || len(data) > MaxOutput {
		t.Fatal("unbounded profile result")
	}
	t.Log(string(data))
	windows.CoUninitialize()
	initialized = false
	if initial == nil {
		if err := windows.CoInitializeEx(0, 2); err != nil {
			t.Fatalf("observer leaked its nested MTA initialization: %v", err)
		}
		windows.CoUninitialize()
	}
}

func TestProfileCOMChangedModeStaysUnknownAndPreservesCallerApartment(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initial := windows.CoInitializeEx(0, 2)
	if initial != nil && !errors.Is(initial, syscall.Errno(1)) {
		t.Skipf("test thread has an incompatible apartment: %v", initial)
	}
	initialized := true
	defer func() {
		if initialized {
			windows.CoUninitialize()
		}
	}()
	snapshot, err := Observe(context.Background(), []string{adapterA})
	if err == nil || snapshot.Complete || snapshot.Profiles[0].Category != "unknown" {
		t.Fatal("incompatible COM apartment produced a category")
	}
	again := windows.CoInitializeEx(0, 2)
	if again == nil {
		windows.CoUninitialize()
		initialized = false
		t.Fatal("observer uninitialized the caller's STA after a failed MTA initialization")
	}
	if !errors.Is(again, syscall.Errno(1)) {
		t.Fatal(again)
	}
	windows.CoUninitialize()
}

func TestCOMReadOnlyVTableOffsetsMatchSDKABI(t *testing.T) {
	// A three-slot IUnknown-only prefix would invoke unrelated methods (including
	// setters). Verify the fixed seven-slot dual-interface ABI before native use.
	word := unsafe.Sizeof(uintptr(0))
	if unsafe.Offsetof(managerVTable{}.GetNetworkConnections) != 9*word || unsafe.Offsetof(enumVTable{}.Next) != 8*word || unsafe.Offsetof(connectionVTable{}.GetNetwork) != 7*word || unsafe.Offsetof(connectionVTable{}.IsConnected) != 9*word || unsafe.Offsetof(connectionVTable{}.GetConnectionID) != 11*word || unsafe.Offsetof(connectionVTable{}.GetAdapterID) != 12*word || unsafe.Offsetof(networkVTable{}.GetNetworkID) != 11*word || unsafe.Offsetof(networkVTable{}.IsConnected) != 16*word || unsafe.Offsetof(networkVTable{}.GetCategory) != 18*word {
		t.Fatal("read-only NLM vtable does not match SDK ABI")
	}
}

func TestProfilePreCancelledReadDoesNotAcquireCOM(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot, err := Observe(ctx, []string{adapterA})
	if err == nil || snapshot.Complete || snapshot.Profiles[0].Category != "unknown" {
		t.Fatal("cancelled profile observation produced a category")
	}
}

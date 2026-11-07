//go:build windows && amd64

package lanprovider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
	"unsafe"
)

func TestMIAMD64LayoutMatchesCompiledMicrosoftHeader(t *testing.T) {
	// Compiled-only constants from SDK10.0.22621.0 mi.h (SHA c0c43bbd...),
	// artifacts/qualification/p2-lan-mi-abi-source/layout.c and layout.asm.
	checks := []struct {
		name      string
		got, want uintptr
	}{
		{"application-size", unsafe.Sizeof(applicationHandle{}), 24}, {"application-ft", unsafe.Offsetof(applicationHandle{}.table), 16}, {"session-size", unsafe.Sizeof(sessionHandle{}), 24}, {"session-ft", unsafe.Offsetof(sessionHandle{}.table), 16}, {"operation-size", unsafe.Sizeof(operationHandle{}), 24}, {"operation-ft", unsafe.Offsetof(operationHandle{}.table), 16}, {"options-size", unsafe.Sizeof(optionsHandle{}), 24}, {"options-ft", unsafe.Offsetof(optionsHandle{}.table), 16}, {"class-size", unsafe.Sizeof(classObject{}), 64}, {"class-ft", unsafe.Offsetof(classObject{}.table), 0}, {"value-size", unsafe.Sizeof(miValue{}), 40}, {"value-alignment", unsafe.Alignof(miValue{}), 8}, {"qualifier-size", unsafe.Sizeof(qualifierHandle{}), 24}, {"qualifier-ft", unsafe.Offsetof(qualifierHandle{}.table), 16}, {"parameter-size", unsafe.Sizeof(parameterHandle{}), 24},
		{"app-close", unsafe.Offsetof(applicationTable{}.close), 0}, {"app-new-session", unsafe.Offsetof(applicationTable{}.newSession), 8}, {"app-new-options", unsafe.Offsetof(applicationTable{}.newOptions), 40}, {"session-close", unsafe.Offsetof(sessionTable{}.close), 0}, {"session-get-class", unsafe.Offsetof(sessionTable{}.getClass), 96}, {"op-close", unsafe.Offsetof(operationTable{}.close), 0}, {"op-cancel", unsafe.Offsetof(operationTable{}.cancel), 8}, {"op-get-class", unsafe.Offsetof(operationTable{}.getClass), 40}, {"class-name", unsafe.Offsetof(classTable{}.getName), 0}, {"class-count", unsafe.Offsetof(classTable{}.getCount), 24}, {"class-get-at", unsafe.Offsetof(classTable{}.getAt), 40}, {"class-qualifiers", unsafe.Offsetof(classTable{}.getQualifiers), 48}, {"class-method-count", unsafe.Offsetof(classTable{}.getMethodCount), 56}, {"class-method-at", unsafe.Offsetof(classTable{}.getMethodAt), 64}, {"qualifier-get", unsafe.Offsetof(qualifierTable{}.get), 16}, {"parameter-return", unsafe.Offsetof(parameterTable{}.getReturnType), 0}, {"options-interval", unsafe.Offsetof(optionsTable{}.setInterval), 88}, {"instance-delete", unsafe.Offsetof(instanceTable{}.delete), 16},
		{"interval-size", unsafe.Sizeof(interval{}), 32}, {"interval-seconds", unsafe.Offsetof(interval{}.seconds), 12}, {"interval-microseconds", unsafe.Offsetof(interval{}.microseconds), 16}, {"parameter-ft", unsafe.Offsetof(parameterHandle{}.table), 16}, {"instance-ft", unsafe.Offsetof(instanceObject{}.table), 0},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Fatalf("%s offset/size %d != qualified %d", check.name, check.got, check.want)
		}
	}
}
func TestMIBoundedUTF16RefusesNullUnterminatedAndBrokenSurrogates(t *testing.T) {
	if _, e := stringCopy(nil, 4); e == nil {
		t.Fatal("null accepted")
	}
	for _, units := range [][]uint16{{0xd800, 0}, {0xdc00, 0}, {0xd800, 'x', 0}, {'a', 'b', 'c'}} {
		if _, e := stringCopy(&units[0], len(units)-1); e == nil {
			t.Fatalf("malformed string accepted: %v", units)
		}
	}
	good := []uint16{0xd800, 0xdc00, 0}
	if got, e := stringCopy(&good[0], 2); e != nil || got != "\U00010000" {
		t.Fatalf("valid pair: %q %v", got, e)
	}
}
func TestMIPreCancelledDoesNotLoadOrQueryProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, e := ObserveSchema(ctx)
	if !errors.Is(e, context.Canceled) || got.Complete || got.ActivationAvailable || got.Class != nil {
		t.Fatal("cancelled read acquired native state")
	}
}

// Root must review the exact compiled test executable/hash and run it inside a
// separately enforced hard process deadline. This is never enabled by go test.
func TestMISchemaReadOnlyNative(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_LAN_MI_SCHEMA") != "1" {
		t.Skip("opt-in fixed local MI GetClass only; root review and external process deadline required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snapshot, e := ObserveSchema(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if !snapshot.Complete || snapshot.ActivationAvailable || snapshot.AtomicCreateQualified || snapshot.Class == nil {
		t.Fatal("read-only observation incomplete or claimed activation")
	}
	data, e := json.Marshal(snapshot)
	if e != nil || len(data) > MaxOutput {
		t.Fatal("schema output bound")
	}
	t.Log(string(data))
}

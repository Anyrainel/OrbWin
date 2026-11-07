//go:build windows && amd64

package lanprovider

import (
	"golang.org/x/sys/windows"
	"runtime"
	"testing"
	"unsafe"
)

func TestTypedNativeDecoderCopiesBorrowedArraysAndPreservesNullEmpty(t *testing.T) {
	shorts := []uint16{53, 443}
	var value miValue
	*(*nativeShortArray)(unsafe.Pointer(&value)) = nativeShortArray{data: &shorts[0], count: 2}
	budget := &typedCopyBudget{}
	out, e := decodeNativeTypedValue(19, 0, &value, budget)
	shorts[0] = 80
	runtime.KeepAlive(shorts)
	if e != nil || len(out.shorts) != 2 || out.shorts[0] != 53 {
		t.Fatal("borrowed short array retained instead of copied")
	}
	first, _ := windows.UTF16FromString("private-first")
	second, _ := windows.UTF16FromString("")
	pointers := []*uint16{&first[0], &second[0]}
	value = miValue{}
	*(*nativeStringArray)(unsafe.Pointer(&value)) = nativeStringArray{data: &pointers[0], count: 2}
	budget = &typedCopyBudget{}
	out, e = decodeNativeTypedValue(29, 0, &value, budget)
	first[0] = 'X'
	runtime.KeepAlive(first)
	runtime.KeepAlive(second)
	runtime.KeepAlive(pointers)
	if e != nil || len(out.texts) != 2 || out.texts[0] != "private-first" || out.texts[1] != "" {
		t.Fatal("borrowed strings retained or empty string lost")
	}
	null, e := decodeNativeTypedValue(29, instanceNullFlag, &miValue{}, &typedCopyBudget{})
	empty, e2 := decodeNativeTypedValue(29, 0, &miValue{}, &typedCopyBudget{})
	if e != nil || e2 != nil || null.flags == empty.flags || empty.texts == nil {
		t.Fatal("NULL and empty array collapsed")
	}
}
func TestTypedNativeDecoderRefusesOversizedMalformedAndUnknownValues(t *testing.T) {
	var array miValue
	*(*nativeStringArray)(unsafe.Pointer(&array)) = nativeStringArray{count: 33}
	var malformed miValue
	*(*nativeShortArray)(unsafe.Pointer(&malformed)) = nativeShortArray{count: 1}
	var boolean miValue
	*(*uint8)(unsafe.Pointer(&boolean)) = 2
	var date miValue
	(*nativeDatetimeValue)(unsafe.Pointer(&date)).isTimestamp = 2
	long := make([]uint16, 1026)
	for i := 0; i < 1025; i++ {
		long[i] = 'x'
	}
	var text miValue
	*(**uint16)(unsafe.Pointer(&text)) = &long[0]
	for _, v := range []struct {
		kind, flags uint32
		value       *miValue
	}{{29, 0, &array}, {19, 0, &malformed}, {0, 0, &boolean}, {12, 0, &date}, {13, 0, &text}, {31, instanceNullFlag, &miValue{}}, {13, 0, &miValue{}}} {
		out, e := decodeNativeTypedValue(v.kind, v.flags, v.value, &typedCopyBudget{})
		if e == nil || out.kind != 0 || out.flags != 0 || out.text != "" || len(out.texts) != 0 {
			t.Fatal("malformed/unsupported value accepted or partial value escaped")
		}
	}
	runtime.KeepAlive(long)
}
func TestTypedNativeStringArrayEnforcesFieldAndCumulativePayloadBudget(t *testing.T) {
	units, _ := windows.UTF16FromString("private-payload")
	pointers := make([]*uint16, 32)
	for i := range pointers {
		pointers[i] = &units[0]
	}
	var array miValue
	*(*nativeStringArray)(unsafe.Pointer(&array)) = nativeStringArray{data: &pointers[0], count: 32}
	for _, budget := range []*typedCopyBudget{{field: maxTypedFieldBytes - 17}, {used: maxTypedPayloadBytes - 17}} {
		if _, e := decodeNativeTypedValue(29, 0, &array, budget); e == nil || budget.used > maxTypedPayloadBytes || budget.field > maxTypedFieldBytes {
			t.Fatal("typed array payload escaped bound")
		}
	}
	runtime.KeepAlive(units)
	runtime.KeepAlive(pointers)
}

func TestTypedNativeDatetimeUsesCompiledUnionOffsetAndLogicalFields(t *testing.T) {
	var value miValue
	native := (*nativeDatetimeValue)(unsafe.Pointer(&value))
	native.isTimestamp = 1
	native.parts = [8]uint32{2026, 10, 25, 12, 34, 56, 123000, 0}
	utc := int32(-60)
	native.parts[7] = uint32(utc)
	out, e := decodeNativeTypedValue(12, 0, &value, &typedCopyBudget{})
	if e != nil || !out.date.timestamp || out.date.parts != [7]uint32{2026, 10, 25, 12, 34, 56, 123000} || out.date.utc != -60 {
		t.Fatal("timestamp logical fields/UTC or compiled layout lost")
	}
	native.isTimestamp = 0
	native.parts = [8]uint32{3, 4, 5, 6, 7, 0xdead, 0xbeef, 0xf00d}
	out, e = decodeNativeTypedValue(12, 0, &value, &typedCopyBudget{})
	if e != nil || out.date.timestamp || out.date.parts != [7]uint32{3, 4, 5, 6, 7} || out.date.utc != 0 {
		t.Fatal("interval logical fields confused with padding")
	}
}

func TestTypedNativeDecoderAcceptsExactStringAndArrayElementBounds(t *testing.T) {
	units := make([]uint16, 1025)
	for i := 0; i < 1024; i++ {
		units[i] = 'x'
	}
	var value miValue
	*(**uint16)(unsafe.Pointer(&value)) = &units[0]
	out, e := decodeNativeTypedValue(13, 0, &value, &typedCopyBudget{})
	if e != nil || len(out.text) != 1024 {
		t.Fatal("exact string-unit bound refused")
	}
	shorts := make([]uint16, 32)
	*(*nativeShortArray)(unsafe.Pointer(&value)) = nativeShortArray{data: &shorts[0], count: 32}
	out, e = decodeNativeTypedValue(19, 0, &value, &typedCopyBudget{})
	if e != nil || len(out.shorts) != 32 {
		t.Fatal("exact array-element bound refused")
	}
	runtime.KeepAlive(units)
	runtime.KeepAlive(shorts)
}

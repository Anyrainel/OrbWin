//go:build windows

package main

import "testing"

func TestRestrictingSIDContract(t *testing.T) {
	groups := make([]uint64, 256)
	if !noRestrictingSids(groups, 4) || !noRestrictingSids(groups, 2048) {
		t.Fatal("empty restricting SID result refused")
	}
	for _, n := range []uint32{0, 3, 2049, ^uint32(0)} {
		if noRestrictingSids(groups, n) {
			t.Fatal("malformed native result admitted", n)
		}
	}
	if noRestrictingSids(nil, 4) {
		t.Fatal("missing count admitted")
	}
	groups[0] = 1
	if noRestrictingSids(groups, 4) {
		t.Fatal("restricting SID admitted")
	}
	groups[0] = ^uint64(0)
	if noRestrictingSids(groups, 2048) {
		t.Fatal("invalid nonempty count admitted")
	}
}

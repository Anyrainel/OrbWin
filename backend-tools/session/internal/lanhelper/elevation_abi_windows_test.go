//go:build windows && amd64

package lanhelper

import (
	"testing"
	"unsafe"
)

func TestLANElevationShellExecuteHeaderLayout(t *testing.T) {
	if unsafe.Sizeof(shellExecuteInfo{}) != 112 || unsafe.Offsetof(shellExecuteInfo{}.parent) != 8 || unsafe.Offsetof(shellExecuteInfo{}.verb) != 16 || unsafe.Offsetof(shellExecuteInfo{}.file) != 24 || unsafe.Offsetof(shellExecuteInfo{}.parameters) != 32 || unsafe.Offsetof(shellExecuteInfo{}.directory) != 40 || unsafe.Offsetof(shellExecuteInfo{}.show) != 48 || unsafe.Offsetof(shellExecuteInfo{}.process) != 104 {
		t.Fatal("ShellExecuteExW layout differs from compiled SDK header")
	}
}

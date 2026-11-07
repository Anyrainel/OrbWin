//go:build windows

package sdkstate

import (
	"runtime"
	"testing"
	"unsafe"
)

// ABI-only: no native filesystem or state root is opened. Windows requires
// DWORDLONG alignment even when Go decides to put the backing object on stack.
func TestDirectoryBufferHasNativeDWORDLONGAlignment(t *testing.T) {
	for i := 0; i < 8; i++ {
		buffer := directoryBuffer()
		runtime.GC()
		if len(buffer) != 65536 || cap(buffer) != 65536 || uintptr(unsafe.Pointer(&buffer[0]))%8 != 0 {
			t.Fatal("FILE_ID_BOTH_DIR_INFO buffer violates its native size/alignment contract")
		}
		buffer[len(buffer)-1] = 1
		runtime.KeepAlive(buffer)
	}
}

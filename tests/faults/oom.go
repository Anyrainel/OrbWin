// A disposable, cgroup-limited workload. Touch every page so the qualification
// distinguishes kernel OOM termination from an ordinary failing command.
package main

import (
	"runtime"
	"time"
)

func main() {
	allocation := make([]byte, 128<<20)
	for offset := 0; offset < len(allocation); offset += 4096 {
		allocation[offset] = 1
	}
	time.Sleep(30 * time.Second)
	runtime.KeepAlive(allocation)
}

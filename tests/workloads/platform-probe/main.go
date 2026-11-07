// One-shot, dependency-free execution evidence. This program never changes
// binfmt registrations, guest settings, files or network state.
package main

import (
	"encoding/json"
	"os"
	"runtime"
)

func main() {
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{
		"os": runtime.GOOS, "architecture": runtime.GOARCH, "result": "executed",
	}); err != nil {
		os.Exit(1)
	}
}

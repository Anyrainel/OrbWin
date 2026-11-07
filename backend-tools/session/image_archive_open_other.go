//go:build !windows

package main

import "os"

// Linux builds are for parser tests. Product import is Windows-only; this does
// not claim Windows sharing guarantees on a different host filesystem.
func openImageArchiveFile(path string) (*os.File, error) { return os.Open(path) }

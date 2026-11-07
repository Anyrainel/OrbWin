//go:build !windows

package main

import (
	"errors"
	"os"
)

func transientLease(string, bool) (*os.File, error) {
	return nil, errors.New("transient ownership currently requires the Windows SDK client")
}
func transientBusy(error) bool { return false }

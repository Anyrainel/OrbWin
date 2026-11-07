//go:build !windows

package main

import "errors"

func hostFreeSpace(string) (uint64, error) {
	return 0, errors.New("native file copies currently require Windows local drive paths")
}

//go:build !windows

package main

import "errors"

func runColdServiceArguments(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "local-services-cold" {
		return false, nil
	}
	return true, errors.New("native Windows cold SDK entry required")
}

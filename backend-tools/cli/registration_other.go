//go:build !windows

package main

import "errors"

func verifyRegistration(string) error { return errors.New("OrbWin lifecycle requires Windows") }

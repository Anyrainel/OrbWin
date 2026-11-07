//go:build !windows

package main

import "errors"

func nativeLocations() (string, string, error) {
	return "", "", errors.New("OrbWin CLI requires native Windows cache provenance")
}
func existingCache(string) (string, error) {
	return "", errors.New("OrbWin CLI requires native Windows cache provenance")
}

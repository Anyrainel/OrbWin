//go:build !windows

package main

import "context"

func publishClientSnapshot(context.Context, *clientConfigGuard) (string, error) {
	return "", errClientConfig
}

//go:build !windows

package main

import "context"

func acquirePrivateClientConfig(context.Context) (*clientConfigGuard, error) {
	return nil, errClientConfig
}

func preparePrivateWriterRecovery(context.Context) (*clientWriterRecovery, error) {
	return nil, errClientConfig
}

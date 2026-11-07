//go:build !windows

package main

import (
	"context"
	"os/exec"
)

func runClientWriter(context.Context, *exec.Cmd, clientWriterAdmission) (*retiredClientTree, error) {
	return nil, errClientConfig
}

func runClientCommand(ctx context.Context, cmd *exec.Cmd, _ bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return cmd.Run()
}

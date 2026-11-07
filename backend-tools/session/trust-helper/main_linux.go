//go:build linux

package main

import (
	"context"
	"fmt"
	"orbwin.local/session/internal/wsltrust"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := wsltrust.ServeSystem(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(125)
	}
}

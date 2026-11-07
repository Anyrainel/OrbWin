//go:build windows

package main

import (
	"context"
	"orbwin.local/session/internal/lanworker"
	"os"
)

func main() {
	if len(os.Args) != 1 {
		os.Exit(125)
	}
	if lanworker.RunNative(context.Background()) != nil {
		os.Exit(125)
	}
}

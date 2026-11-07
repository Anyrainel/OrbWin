//go:build windows && amd64

package main

import (
	"context"
	"orbwin.local/session/internal/lanhelper"
	"os"
)

func main() {
	if lanhelper.RunNativeHelper(context.Background(), os.Args[1:]) != nil {
		os.Exit(125)
	}
}

//go:build !windows

package main

import (
	"context"
	"errors"
	"io"
)

func lanInterfaces(context.Context, io.Writer) error {
	return errors.New("LAN interface observation requires Windows")
}

//go:build !windows

package main

import (
	"context"
	"errors"
	"io"
)

func lanProfiles(context.Context, io.Writer) error {
	return errors.New("LAN profile observation requires Windows")
}

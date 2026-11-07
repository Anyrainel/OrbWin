//go:build !windows || !amd64

package lanhelper

import (
	"context"
	"errors"
)

func RunNativeHelper(context.Context, []string) error {
	return errors.New("native LAN helper unsupported on this target")
}

//go:build !windows || !amd64

package lanhelper

import (
	"context"
	"errors"
)

func RunElevationLauncher(context.Context, []string) error {
	return errors.New("native elevation launcher unsupported on this target")
}

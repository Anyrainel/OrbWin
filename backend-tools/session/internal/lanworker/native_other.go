//go:build !windows

package lanworker

import (
	"context"
	"errors"
)

func RunNative(context.Context) error {
	return errors.New("ordinary LAN worker requires reviewed Windows native boundary")
}

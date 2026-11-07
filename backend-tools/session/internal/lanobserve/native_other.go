//go:build !windows

package lanobserve

import (
	"context"
	"errors"
)

func Observe(context.Context) (Snapshot, error) {
	return Snapshot{}, errors.New("LAN interface observation requires Windows IP Helper")
}

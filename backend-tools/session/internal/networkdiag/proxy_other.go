//go:build !windows

package networkdiag

import (
	"context"
	"errors"
)

func observeProxy(ctx context.Context) (ProxySnapshot, error) {
	result := ProxySnapshot{Version: 1, CurrentUser: unknownConfiguration("windows-only"), WinHTTPDefault: unknownConfiguration("windows-only")}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, errors.New("Windows proxy configuration observation is unavailable on this platform")
}

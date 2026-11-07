//go:build !windows

package sdkstate

import "context"

func openNative(context.Context, string, string, string, openRestriction) (backend, error) {
	return nil, ErrState
}

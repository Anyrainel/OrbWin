//go:build !windows || !amd64

package lanprovider

import (
	"context"
	"errors"
)

func openCleanupSource(context.Context, string, string, uintptr) (cleanupSource, error) {
	return nil, errors.New("protected cleanup requires qualified Windows AMD64 helper")
}

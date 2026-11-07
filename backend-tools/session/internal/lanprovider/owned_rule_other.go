//go:build !windows || !amd64

package lanprovider

import (
	"context"
	"errors"
)

func newOwnedNativeBackend(context.Context) (ownedBackend, error) {
	return nil, errors.New("owned LAN rules require qualified Windows AMD64 MI provider")
}

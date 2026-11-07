//go:build !windows || !amd64

package lanprovider

import (
	"context"
	"errors"
)

func ObserveSchema(ctx context.Context) (Snapshot, error) {
	return Snapshot{Version: 1, Source: "unavailable"}, errors.New("native MI schema bridge currently requires Windows amd64")
}

//go:build !windows

package lanprofile

import (
	"context"
	"errors"
	"time"
)

func Observe(ctx context.Context, guids []string) (Snapshot, error) {
	guids = append([]string{}, guids...)
	snapshot, err := unknownSnapshot(guids, time.Now())
	if err != nil {
		return snapshot, err
	}
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}
	return snapshot, errors.New("Windows Network List Manager profile observation is unavailable")
}

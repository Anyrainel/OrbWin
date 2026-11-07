//go:build !windows

package lanhelper

import (
	"context"
	"errors"
)

func ReadProtectedReviewObservation(context.Context, string, string, string) (ProtectedReviewObservation, error) {
	return ProtectedReviewObservation{}, errors.New("native protected LAN review observation unavailable")
}

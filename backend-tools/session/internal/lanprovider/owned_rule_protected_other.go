//go:build !windows || !amd64

package lanprovider

import (
	"context"
	"errors"
	"orbwin.local/session/internal/lanlease"
)

func ReadProtectedRule(context.Context, string, string) (lanlease.RuleAcknowledgement, error) {
	return lanlease.RuleAcknowledgement{}, errors.New("protected native LAN rule observation requires Windows AMD64")
}

func ReadProtectedRemoval(context.Context, string, string) (lanlease.RemovalAcknowledgement, error) {
	return lanlease.RemovalAcknowledgement{}, errors.New("protected native LAN removal observation requires Windows AMD64")
}
func ReadProtectedCleanup(context.Context, string, string) (CleanupObservation, error) {
	return CleanupObservation{}, ErrCleanupPending
}

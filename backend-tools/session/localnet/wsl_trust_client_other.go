//go:build !windows

package localnet

import (
	"errors"
	"orbwin.local/session/internal/wsltrust"
)

func newWSLTrustClient([]byte, string) (WSLTrustClient, error) {
	return nil, errors.New("selected WSL trust requires Windows")
}
func registeredWSLTrustClients() ([]wsltrust.Registration, error) {
	return nil, errors.New("selected WSL trust requires Windows")
}
func newWSLTrustRegistry() (WSLTrustRegistry, error) {
	return nil, errors.New("selected WSL registry requires Windows")
}

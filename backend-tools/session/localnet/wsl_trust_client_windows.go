//go:build windows

package localnet

import "orbwin.local/session/internal/wsltrust"

type currentWSLTrustRegistry struct{ user string }

func (registry *currentWSLTrustRegistry) UserID() string { return registry.user }
func (registry *currentWSLTrustRegistry) Present(id string) (bool, error) {
	return wsltrust.RegistrationPresent(id)
}
func newWSLTrustRegistry() (WSLTrustRegistry, error) {
	user, err := wsltrust.CurrentWindowsUser()
	if err != nil {
		return nil, err
	}
	return &currentWSLTrustRegistry{user: user}, nil
}

func newWSLTrustClient(asset []byte, digest string) (WSLTrustClient, error) {
	return wsltrust.NewHost(asset, digest)
}
func registeredWSLTrustClients() ([]wsltrust.Registration, error) { return wsltrust.Registrations() }

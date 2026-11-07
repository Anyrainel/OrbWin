//go:build !windows || !amd64

package localnet

func validateNativeLANRegistration(RuntimeOptions) error { return ErrLANAdapterUnavailable }
func newNativeLANRegistration(RuntimeOptions) (*LANAdapters, error) {
	return nil, ErrLANAdapterUnavailable
}

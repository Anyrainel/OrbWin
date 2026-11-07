//go:build !windows

package localnet

import "errors"

func ProtectPrivateBundle([]byte, string, string) ([]byte, error) {
	return nil, errors.New("local signing-state protection requires Windows user DPAPI")
}
func UnprotectPrivateBundle([]byte, string, string) ([]byte, error) {
	return nil, errors.New("local signing-state protection requires Windows user DPAPI")
}

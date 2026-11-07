//go:build !windows

package localnet

import "errors"

func OpenCurrentUserRootStore(bool) (RootStore, error) {
	return nil, errors.New("Windows current-user trust requires Windows")
}

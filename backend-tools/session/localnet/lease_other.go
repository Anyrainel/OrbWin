//go:build !windows

package localnet

import (
	"errors"
	"os"
)

func serviceLease(string) (*os.File, error) {
	return nil, errors.New("local-services controller requires Windows process ownership")
}

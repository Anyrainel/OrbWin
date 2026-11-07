//go:build !windows

package localnet

import (
	"errors"
	"os/exec"
)

func prepareReadOnlyLANChild(*exec.Cmd) error {
	return errors.New("native LAN discovery requires Windows")
}

//go:build windows

package localnet

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

func prepareReadOnlyLANChild(command *exec.Cmd) error {
	root, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return err
	}
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	command.Dir = directory
	command.Env = []string{"SystemRoot=" + root, "WINDIR=" + root, "GOMAXPROCS=2"}
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return nil
}

//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

func newBridgeCommand() (*exec.Cmd, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(path, "--bridge")
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | createNoWindow,
		HideWindow:    true,
	}
	return command, nil
}

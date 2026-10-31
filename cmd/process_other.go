//go:build !windows

package main

import (
	"os"
	"os/exec"
)

func newBridgeCommand() (*exec.Cmd, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(path, "--bridge")
	command.Stdin = nil
	command.Stdout = nil
	command.Stderr = nil
	return command, nil
}

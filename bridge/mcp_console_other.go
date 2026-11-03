//go:build !windows

package bridge

import "os/exec"

func configureMCPProcess(cmd *exec.Cmd, setup bool) {}

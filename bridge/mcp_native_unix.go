//go:build !windows

package bridge

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

func mcpProtectKey(clear []byte) ([]byte, error)    { return append([]byte(nil), clear...), nil }
func mcpUnprotectKey(cipher []byte) ([]byte, error) { return append([]byte(nil), cipher...), nil }
func mcpHideProcess(_ *exec.Cmd)                    {}
func mcpDetachProcess(cmd *exec.Cmd)                { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func mcpRunnerExclusive(private string) (func(), error) {
	f, err := os.OpenFile(private+"/runner.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("Tunnel runner already active")
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
func mcpSameProcess(pid int, wantedExe string) bool {
	if pid <= 0 || wantedExe == "" {
		return false
	}
	expected, err := filepath.EvalSymlinks(wantedExe)
	if err != nil {
		return false
	}
	if runtime.GOOS == "linux" {
		actual, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
		if err != nil {
			return false
		}
		return filepath.Clean(actual) == filepath.Clean(expected)
	}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
		if err != nil {
			return false
		}
		command := strings.TrimSpace(string(out))
		return strings.HasPrefix(command, expected+" tunnel-run") || strings.HasPrefix(command, strconv.Quote(expected)+" tunnel-run")
	}
	return false
}

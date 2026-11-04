package bridge

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func syndirelayOwnTunnelCommandLine(command, profile string) bool {
	fields := strings.Fields(command)
	if len(fields) < 3 {
		return false
	}
	executable := strings.Trim(fields[0], "\\\"'")
	name := strings.ToLower(filepath.Base(executable))
	return (name == "tunnel-client" || name == "tunnel-client.exe") &&
		strings.Contains(command, profile) && strings.Contains(command, " run ")
}

// Process presence and control-plane readiness are separate signals.
func syndirelayTunnelProcessRunning(profile string) bool {
	switch runtime.GOOS {
	case "linux":
		entries, err := os.ReadDir("/proc")
		if err != nil {
			return false
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "" || name[0] < '0' || name[0] > '9' {
				continue
			}
			data, err := os.ReadFile(filepath.Join("/proc", name, "cmdline"))
			if err != nil {
				continue
			}
			if syndirelayOwnTunnelCommandLine(strings.ReplaceAll(string(data), "\x00", " "), profile) {
				return true
			}
		}
	case "windows":
		ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
		defer cancel()
		script := "$p=$env:SYNDIRELAY_MCP_PROFILE; $x=@(Get-CimInstance Win32_Process -Filter \"Name='tunnel-client.exe'\" -ErrorAction SilentlyContinue | Where-Object {$_.CommandLine -and $_.CommandLine.Contains($p) -and $_.CommandLine.Contains(' run ')}); if($x.Count -gt 0){'RUNNING'}"
		cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.Env = append(os.Environ(), "SYNDIRELAY_MCP_PROFILE="+profile)
		out, err := cmd.Output()
		return err == nil && strings.Contains(string(out), "RUNNING")
	case "darwin":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "ps", "-axo", "command=").Output()
		if err != nil {
			return false
		}
		for _, line := range strings.Split(string(out), "\n") {
			if syndirelayOwnTunnelCommandLine(line, profile) {
				return true
			}
		}
	}
	return false
}

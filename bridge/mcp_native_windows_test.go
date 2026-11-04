//go:build windows

package bridge

import (
	"encoding/json"
	"os"
	"testing"
)

func TestMCPRunnerIdentitySurvivesReplacedExecutable(t *testing.T) {
	pid := os.Getpid()
	started, err := mcpProcessStartTime(pid)
	if err != nil || started == 0 {
		t.Fatalf("get process creation time: %v, %d", err, started)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !mcpSameProcess(pid, exe, started) {
		t.Fatal("current process should be recognized")
	}
	// The Windows loader may report a *.tmpUpdate image after installing a new
	// release. The stored PID and creation time are immutable for this process.
	if !mcpSameProcess(pid, exe+".tmpUpdate.replaced", started) {
		t.Fatal("running process was lost after EXE replacement")
	}
	if mcpSameProcess(pid, exe, started+1) {
		t.Fatal("reused PID was not rejected")
	}
	if mcpSameProcess(pid, exe, 0) {
		t.Fatal("missing creation time was accepted")
	}
	if mcpSameProcess(0, exe, started) {
		t.Fatal("invalid PID was accepted")
	}
}

func TestMCPConnectionStatusRecognizesActiveRunner(t *testing.T) {
	started, err := mcpProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	p := mcpNativePaths("windows", t.TempDir())
	if err := os.MkdirAll(p.Private, 0700); err != nil {
		t.Fatal(err)
	}
	info := tunnelRunnerInfo{
		PID: os.Getpid(), Token: "0123456789abcdef0123456789abcdef",
		Exe:       `C:\software\Coding\SyndiRelay\bin\syndirelay.exe`,
		StartedAt: started,
	}
	save := func() {
		t.Helper()
		b, e := json.Marshal(info)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(p.Runner, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	save()
	if !mcpRunnerRunning(p) {
		t.Fatal("active Supervisor incorrectly shown as stopped")
	}
	info.StartedAt++
	save()
	if mcpRunnerRunning(p) {
		t.Fatal("different process instance incorrectly shown as running")
	}
}

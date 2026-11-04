package bridge

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativeMCPCredentialsNeverAppearInProcessArguments(t *testing.T) {
	key := "only_synthetic_credential_0123456789"
	if !validRuntimeKey(key) {
		t.Fatal("test key invalid")
	}
	encrypted, err := mcpEncodeKey(key)
	if err != nil {
		t.Fatal(err)
	}
	clear, err := mcpUnprotectKey(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(clear), key) {
		t.Fatal("key roundtrip failed")
	}
	if runtime.GOOS == "windows" && strings.Contains(string(encrypted), key) {
		t.Fatal("Windows DPAPI storage exposed plaintext")
	}
}

func TestNativeMCPRejectUnownedState(t *testing.T) {
	p := nativeMCPPaths{Private: t.TempDir()}
	p.Runner = filepath.Join(p.Private, "runner.json")
	p.Stop = filepath.Join(p.Private, "stop-request")
	if mcpRunnerRunning(p) {
		t.Fatal("missing runner reported running")
	}
	if err := nativeRequestStop(p); err != nil {
		t.Fatal("stopping absent runner should be safe")
	}
	if _, err := os.Stat(p.Stop); !os.IsNotExist(err) {
		t.Fatal("stop-request created for unowned process")
	}
}

func TestNativeMCPProfileID(t *testing.T) {
	d := t.TempDir()
	file := filepath.Join(d, "profile.yaml")
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{"tunnel_id: tunnel_1234567890abcdef1234567890abcdef", true},
		{"tunnel_id: " + mcpExampleID, false},
		{"tunnel_id: tunnel_invalid", false},
		{"tunnel_id: ../../escape", false},
	} {
		if err := os.WriteFile(file, []byte(tc.raw), 0600); err != nil {
			t.Fatal(err)
		}
		if ok := readTunnelID(file) != ""; ok != tc.valid {
			t.Errorf("profile %q valid=%v", tc.raw, ok)
		}
	}
}

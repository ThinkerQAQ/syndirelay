package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeMCPPathsUseApplicationDataDirectory(t *testing.T) {
	for _, osName := range []string{"windows", "linux", "darwin"} {
		dataDir := filepath.Join(t.TempDir(), "Data")
		p := mcpNativePaths(osName, dataDir)
		want := filepath.Join(dataDir, "mcp")
		if p.Private != want {
			t.Errorf("%s: private directory = %q, want %q", osName, p.Private, want)
		}
		if p.Profile != filepath.Join(want, "syndirelay.yaml") || p.Health != filepath.Join(want, "health.url") {
			t.Errorf("%s: unexpected MCP paths: %+v", osName, p)
		}
		if filepath.Dir(p.Key) != want || filepath.Dir(p.Runner) != want || filepath.Dir(p.Revision) != want {
			t.Errorf("%s: private files are not in Data/mcp: %+v", osName, p)
		}
		if osName == "windows" && filepath.Base(p.Key) != "runtime-key.dpapi" {
			t.Errorf("Windows key must use DPAPI, got %q", p.Key)
		}
	}
}

func TestNativeMCPUsesConfigDirOverrideAndIgnoresLegacyProfiles(t *testing.T) {
	root := t.TempDir()
	customData := filepath.Join(root, "SyndiRelay", "Data")
	t.Setenv("SYNDIRELAY_DATA_DIR", customData)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	for _, legacy := range []string{
		filepath.Join(root, "xdg", "tunnel-client", "syndirelay.yaml"),
		filepath.Join(root, "xdg", "syndirelay", "native-mcp", "syndirelay.yaml"),
	} {
		if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(legacy, []byte("tunnel_id: tunnel_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := localMCPPaths()
	if err != nil {
		t.Fatal(err)
	}
	if p.Private != filepath.Join(customData, "mcp") {
		t.Fatalf("MCP path = %q, expected application Data/mcp", p.Private)
	}
	if readTunnelID(p.Profile) != "" {
		t.Fatal("fresh config imported legacy Tunnel profile")
	}
	if _, err := os.Stat(p.Profile); !os.IsNotExist(err) {
		t.Fatalf("new profile unexpectedly exists: %v", err)
	}
	for _, legacy := range []string{
		filepath.Join(root, "xdg", "tunnel-client", "syndirelay.yaml"),
		filepath.Join(root, "xdg", "syndirelay", "native-mcp", "syndirelay.yaml"),
	} {
		b, err := os.ReadFile(legacy)
		if err != nil || !strings.Contains(string(b), "tunnel_aaaaaaaa") {
			t.Fatalf("legacy profile was modified: %v", err)
		}
	}
}

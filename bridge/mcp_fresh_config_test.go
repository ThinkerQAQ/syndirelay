package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeMCPPathsNeverReuseLegacyConfiguration(t *testing.T) {
	for _, tc := range []struct{ osName, home, local, xdg string }{
		{"windows", "C:/Users/example", "C:/Users/example/AppData/Local", ""},
		{"linux", "/home/example", "", "/tmp/example-xdg"},
		{"darwin", "/Users/example", "", "/Users/example/.config"},
	} {
		profile, health := mcpPaths(tc.osName, tc.home, tc.local, tc.xdg)
		if filepath.Dir(profile) != filepath.Dir(health) {
			t.Errorf("%s config and health must share a private namespace", tc.osName)
		}
		if !strings.Contains(strings.ToLower(filepath.ToSlash(profile)), "native") {
			t.Errorf("%s config does not use fresh NativeMCP namespace: %s", tc.osName, profile)
		}
		if strings.Contains(profile, "tunnel-client") {
			t.Errorf("%s still uses script-managed profile path", tc.osName)
		}
	}
}

func TestNativeMCPDoesNotReadOldProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("filesystem isolation test")
	}
	if os.Getenv("CI") == "windows" {
		t.Skip("uses isolated Unix HOME for test")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	xdg := filepath.Join(root, "xdg")
	if err := os.MkdirAll(filepath.Join(xdg, "tunnel-client"), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(xdg, "tunnel-client", "syndirelay.yaml")
	if err := os.WriteFile(legacy, []byte("tunnel_id: tunnel_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	p, _ := mcpPaths("linux", home, "", xdg)
	if readTunnelID(p) != "" {
		t.Fatal("fresh configuration imported old Tunnel profile")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("new profile existed before first save: %v", err)
	}
	if content, _ := os.ReadFile(legacy); !strings.Contains(string(content), "tunnel_aaaaaaaa") {
		t.Fatal("legacy profile was changed")
	}
}

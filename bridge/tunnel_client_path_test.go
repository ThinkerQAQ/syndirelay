package bridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func makeClientFixture(t *testing.T, goos, directory string) string {
	t.Helper()
	name := "tunnel-client"
	if goos == "windows" {
		name += ".exe"
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("synthetic tunnel executable"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTunnelClientIsolationFromGlobalPATH(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			dataDir := t.TempDir()
			globalDir := t.TempDir()
			globalClient := makeClientFixture(t, goos, globalDir)
			t.Setenv("PATH", globalDir)
			if got := findTunnelClient(goos, dataDir); got != "" {
				t.Fatalf("shared client %q should not be discovered from PATH %q", got, globalClient)
			}
			defaultClient := makeClientFixture(t, goos, filepath.Join(dataDir, "bin"))
			if got := findTunnelClient(goos, dataDir); got != defaultClient {
				t.Fatalf("private client detection = %q, want %q", got, defaultClient)
			}
			if strings.Contains(defaultClient, ".local") {
				t.Fatal("private default must never use shared .local/bin")
			}
		})
	}
}

func TestTunnelClientCustomPathValidationAndReset(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			dataDir := t.TempDir()
			expected := tunnelClientTarget(goos, dataDir)
			if got, custom, err := selectedTunnelClientPath(goos, dataDir); err != nil || custom || got != expected {
				t.Fatalf("default path = %q custom=%v err=%v", got, custom, err)
			}
			if _, err := setTunnelClientPath(goos, dataDir, "relative/client"); err == nil {
				t.Fatal("relative client path accepted")
			}
			missing := filepath.Join(t.TempDir(), filepath.Base(expected))
			if _, err := setTunnelClientPath(goos, dataDir, missing); err == nil {
				t.Fatal("nonexistent client accepted")
			}
			customClient := makeClientFixture(t, goos, t.TempDir())
			if _, err := setTunnelClientPath(goos, dataDir, customClient); err != nil {
				t.Fatal(err)
			}
			if got := findTunnelClient(goos, dataDir); got != customClient {
				t.Fatalf("custom path not adopted: %q", got)
			}
			if got, custom, err := selectedTunnelClientPath(goos, dataDir); err != nil || !custom || got != customClient {
				t.Fatalf("persisted path = %q custom=%v err=%v", got, custom, err)
			}
			// A broken custom path is missing, even if a global or default
			// executable exists: never silently switch client identities.
			if err := os.Remove(customClient); err != nil {
				t.Fatal(err)
			}
			_ = makeClientFixture(t, goos, filepath.Join(dataDir, "bin"))
			if got := findTunnelClient(goos, dataDir); got != "" {
				t.Fatalf("broken custom path silently fell back to %q", got)
			}
			if _, err := setTunnelClientPath(goos, dataDir, ""); err != nil {
				t.Fatal(err)
			}
			if got := findTunnelClient(goos, dataDir); got != expected {
				t.Fatalf("reset did not restore private default: %q", got)
			}
			if _, err := os.Stat(tunnelClientPathPreference(dataDir)); !os.IsNotExist(err) {
				t.Fatal("reset must remove custom preference")
			}
		})
	}
}

func TestMCPClientPathManagementAPI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNDIRELAY_DATA_DIR", dir)
	server, err := New("client-test-secret-token")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	// New installations must not use globally installed Tunnel Clients.
	response, err := http.Get(httpServer.URL + "/v1/mcp-connection")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET without bridge token returned %d", response.StatusCode)
	}
	custom := makeClientFixture(t, runtime.GOOS, t.TempDir())
	post := func(path string) int {
		body, err := json.Marshal(mcpConnectionAction{Action: "set-client-path", ClientPath: path})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/mcp-connection", strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("x-thinkerqaq-token", "client-test-secret-token")
		res, err := httpServer.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}
	if status := post("relative/path"); status != http.StatusBadRequest {
		t.Fatalf("invalid path HTTP %d", status)
	}
	if status := post(custom); status != http.StatusOK {
		t.Fatalf("save path HTTP %d", status)
	}
	status, err := server.MCPConnection(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.ClientPath != custom || !status.ClientInstalled || !status.ClientCustom {
		t.Fatalf("custom status: path=%q installed=%v custom=%v", status.ClientPath, status.ClientInstalled, status.ClientCustom)
	}
	if code := post(""); code != http.StatusOK {
		t.Fatalf("reset HTTP %d", code)
	}
	status, err = server.MCPConnection(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.ClientCustom || status.ClientPath != tunnelClientTarget(runtime.GOOS, dir) {
		t.Fatalf("default not restored: custom=%v path=%q", status.ClientCustom, status.ClientPath)
	}
}

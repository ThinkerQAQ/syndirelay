package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeMCPBoundaries(t *testing.T) {
	good := "tunnel_1234567890abcdef1234567890abcdef"
	for _, id := range []string{"tunnel_", "../../private", mcpExampleID, good + ";evil"} {
		if validNativeTunnel(id) {
			t.Fatalf("accepted unsafe Tunnel ID %q", id)
		}
	}
	if !validNativeTunnel(good) {
		t.Fatal("rejected valid Tunnel ID")
	}
	for _, key := range []string{"short", "with spaces malformed credential", "line\nbreaks"} {
		if validRuntimeKey(key) {
			t.Fatalf("accepted unsafe key %q", key)
		}
	}
	if !validRuntimeKey("test_runtime_credential_123456789") {
		t.Fatal("valid test key rejected")
	}
	for _, osName := range []string{"windows", "linux", "darwin"} {
		paths := mcpNativePaths(osName, filepath.Join(t.TempDir(), "Data"))
		if !strings.HasSuffix(paths.Profile, "syndirelay.yaml") || !strings.Contains(paths.Runner, "runner.json") {
			t.Fatalf("%s: %+v", osName, paths)
		}
	}
	root := t.TempDir()
	file := filepath.Join(root, "nested", "private.key")
	if err := privateWrite(file, []byte("not-a-live-key")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil || string(got) != "not-a-live-key" {
		t.Fatalf("atomic private storage %q %v", got, err)
	}
	if tunnelReady(context.Background(), "", filepath.Join(root, "health.url")) {
		t.Fatal("missing client reported ready")
	}
}

func TestMCPManagementAuthenticated(t *testing.T) {
	server, err := New("private-test-token")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, _ := http.NewRequest(method, httpServer.URL+"/v1/mcp-connection", strings.NewReader("{}"))
		response, err := httpServer.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthorized %s returned %d", method, response.StatusCode)
		}
	}
	for _, body := range []string{
		`{"action":"save","tunnel_id":"tunnel_invalid","runtime_api_key":"test_fake_123456789"}`,
		`{"action":"save","tunnel_id":"tunnel_0123456789abcdef0123456789abcdef","runtime_api_key":"test_fake_123456789"}`,
		`{"action":"random-shell-command"}`,
	} {
		req, _ := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/mcp-connection", strings.NewReader(body))
		req.Header.Set("x-thinkerqaq-token", "private-test-token")
		response, err := httpServer.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("unsafe body %s returned %d", body, response.StatusCode)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/mcp-connection", nil)
	req.Header.Set("x-thinkerqaq-token", "private-test-token")
	response, err := httpServer.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET status=%d", response.StatusCode)
	}
}

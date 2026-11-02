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

func TestMCPBoundaries(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Setup-SyndiRelay-MCP.ps1", "Manage-SyndiRelay-MCP.ps1", "manage-mcp.sh", "setup-mcp.sh"} {
		if err := os.WriteFile(filepath.Join(root, "scripts", name), []byte(""), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, action := range []string{"start", "stop", "restart"} {
		cmd, err := mcpCommand("windows", root, action, "")
		if err != nil || cmd.Args[len(cmd.Args)-1] != action {
			t.Fatalf("%s: %v %v", action, cmd, err)
		}
	}
	for _, action := range []string{"delete", "start; whoami", ""} {
		if _, err := mcpCommand("windows", root, action, ""); err == nil {
			t.Fatalf("accepted %q", action)
		}
	}
	for _, id := range []string{"tunnel_", "../secret", "tunnel_0123456789abcdef0123456789abcdef;evil"} {
		if _, err := mcpCommand("windows", root, "setup", id); err == nil {
			t.Fatalf("accepted ID %q", id)
		}
	}
	id := "tunnel_0123456789abcdef0123456789abcdef"
	cmd, err := mcpCommand("windows", root, "setup", id)
	if err != nil || cmd.Args[len(cmd.Args)-2] != "-TunnelId" {
		t.Fatalf("unexpected %v %v", cmd, err)
	}
	if tunnelReady(context.Background(), "", filepath.Join(root, "health.url")) {
		t.Fatal("missing client ready")
	}
	profile, health := mcpPaths("linux", "/home/example", "", "/tmp/config")
	if profile != filepath.Join("/tmp/config", "tunnel-client", "syndirelay.yaml") || health != filepath.Join("/tmp/config", "syndirelay", "mcp", "health.url") {
		t.Fatal(profile, health)
	}
}

func TestMCPManagementAuthenticated(t *testing.T) {
	s, err := New("private-test-token")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, _ := http.NewRequest(method, server.URL+"/v1/mcp-connection", strings.NewReader(`{"action":"install-client"}`))
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s: %d", method, response.StatusCode)
		}
	}
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/mcp-connection", strings.NewReader(`{"action":"setup","tunnel_id":"tunnel_invalid"}`))
	req.Header.Set("x-thinkerqaq-token", "private-test-token")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid setup must not spawn: %d", response.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/v1/mcp-connection", nil)
	req.Header.Set("x-thinkerqaq-token", "private-test-token")
	response, err = server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticated GET: %d", response.StatusCode)
	}
}

package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPSaveCommandDoesNotPutKeyOnCommandLine(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Setup-SyndiRelay-MCP.ps1", "setup-mcp.sh"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0600); err != nil {
			t.Fatal(err)
		}
	}
	id := "tunnel_1234567890abcdef1234567890abcdef"
	for _, goos := range []string{"windows", "linux", "darwin"} {
		cmd, err := mcpSaveCommand(goos, root, id, true, id, true)
		if err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		args := strings.Join(cmd.Args, " ")
		if strings.Contains(args, "MY_SECRET") || strings.Contains(args, "CONTROL_PLANE_API_KEY") {
			t.Fatal("secret in process arguments")
		}
		if !strings.Contains(args, "setup-mcp.sh") && !strings.Contains(args, "Setup-SyndiRelay-MCP.ps1") {
			t.Fatal("incorrect fixed script")
		}
		if goos == "windows" {
			for _, flag := range []string{"-NonInteractive", "-KeyFromStdin", "-SaveOnly", "-ReuseSavedKey", "-ReplaceExisting"} {
				if !strings.Contains(args, flag) {
					t.Fatalf("missing %s in %v", flag, cmd.Args)
				}
			}
		} else {
			env := strings.Join(cmd.Env, " ")
			for _, key := range []string{"SYNDIRELAY_MCP_KEY_STDIN=1", "SYNDIRELAY_MCP_SAVE_ONLY=1", "SYNDIRELAY_MCP_REUSE_KEY=1"} {
				if !strings.Contains(env, key) {
					t.Fatalf("%s: missing %s", goos, key)
				}
			}
		}
	}
	for _, bad := range []string{"tunnel_invalid", "../../../tmp", "tunnel_1234567890abcdef1234567890abcdef;evil"} {
		if _, err := mcpSaveCommand("windows", root, bad, false, "", false); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestSyndirelayOwnedTunnelProcessParsing(t *testing.T) {
	profile := filepath.Join("example", "syndirelay.yaml")
	for _, command := range []string{"tunnel-client run --config " + profile, "tunnel-client.exe run --config " + profile} {
		if !syndirelayOwnTunnelCommandLine(command, profile) {
			t.Fatalf("missed own tunnel %s", command)
		}
	}
	for _, command := range []string{"tunnel-client run --config another.yaml", "tunnel-client doctor --config " + profile, "other-tool run --config " + profile} {
		if syndirelayOwnTunnelCommandLine(command, profile) {
			t.Fatalf("matched foreign command %s", command)
		}
	}
}

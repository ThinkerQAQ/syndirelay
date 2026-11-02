package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Management stays local to the authenticated application, separate from remote MCP tools.
var tunnelIDPattern = regexp.MustCompile(`^tunnel_(?:[a-z0-9]{32}|[a-z0-9]{4}_[a-z0-9]{32})$`)
var profileTunnelID = regexp.MustCompile(`(?m)^\s*tunnel_id:\s*["']?(tunnel_[a-z0-9_]+)`)

type MCPConnectionStatus struct {
	State           string `json:"state"`
	TunnelID        string `json:"tunnel_id"`
	ClientInstalled bool   `json:"client_installed"`
	Configured      bool   `json:"configured"`
	Ready           bool   `json:"ready"`
	LocalConfig     any    `json:"local_config"`
}

func findTunnelClient(goos, home string) string {
	if path, err := exec.LookPath("tunnel-client"); err == nil {
		return path
	}
	path := tunnelClientTarget(goos, home)
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return path
	}
	return ""
}
func mcpPaths(goos, home, local, xdg string) (string, string) {
	if goos == "windows" {
		p := filepath.Join(local, "SyndiRelay", "MCP")
		return filepath.Join(p, "syndirelay.yaml"), filepath.Join(p, "health.url")
	}
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "tunnel-client", "syndirelay.yaml"), filepath.Join(xdg, "syndirelay", "mcp", "health.url")
}
func tunnelReady(ctx context.Context, client, health string) bool {
	if client == "" {
		return false
	}
	if st, err := os.Stat(health); err != nil || st.IsDir() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 1800*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, client, "health", "--url-file", health, "--require-control-plane-poll", "--json")
	configureMCPProcess(cmd, false)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	return cmd.Run() == nil
}
func (s *Server) MCPConnection(ctx context.Context) (MCPConnectionStatus, error) {
	out := MCPConnectionStatus{State: "unconfigured"}
	home, err := os.UserHomeDir()
	if err != nil {
		return out, err
	}
	exe, err := os.Executable()
	if err != nil {
		return out, err
	}
	args := []string{"mcp-stdio"}
	out.LocalConfig = map[string]any{"mcpServers": map[string]any{"syndirelay": map[string]any{"command": exe, "args": args}}}
	profile, health := mcpPaths(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_CONFIG_HOME"))
	client := findTunnelClient(runtime.GOOS, home)
	out.ClientInstalled = client != ""
	if f, err := os.Open(profile); err == nil {
		data, readErr := io.ReadAll(io.LimitReader(f, 128<<10))
		f.Close()
		if readErr == nil {
			m := profileTunnelID.FindSubmatch(data)
			if len(m) == 2 && tunnelIDPattern.Match(m[1]) {
				out.Configured = true
				out.TunnelID = string(m[1])
			}
		}
	}
	if !out.ClientInstalled {
		out.State = "client_missing"
	} else if out.Configured {
		out.State = "stopped_or_unhealthy"
		out.Ready = tunnelReady(ctx, client, health)
		if out.Ready {
			out.State = "ready"
		}
	}
	return out, nil
}

// Only fixed scripts and allowlisted values can become process arguments.
func mcpCommand(goos, root, action, id string) (*exec.Cmd, error) {
	if action == "setup" && !tunnelIDPattern.MatchString(id) {
		return nil, errors.New("invalid MCP action or Tunnel ID")
	}
	if action != "setup" && action != "start" && action != "stop" && action != "restart" {
		return nil, errors.New("invalid MCP action or Tunnel ID")
	}
	script := "manage-mcp.sh"
	if action == "setup" {
		script = "setup-mcp.sh"
	}
	if goos == "windows" {
		script = "Manage-SyndiRelay-MCP.ps1"
		if action == "setup" {
			script = "Setup-SyndiRelay-MCP.ps1"
		}
	}
	path := filepath.Join(root, "scripts", script)
	if st, err := os.Lstat(path); err != nil || !st.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	if goos == "windows" {
		args := []string{"-NoProfile", "-ExecutionPolicy", "Bypass"}
		if action == "setup" {
			args = append(args, "-NoExit")
		}
		args = append(args, "-File", path)
		if action == "setup" {
			args = append(args, "-TunnelId", id)
		} else {
			args = append(args, "-Action", action)
		}
		cmd := exec.Command("powershell.exe", args...)
		configureMCPProcess(cmd, action == "setup")
		return cmd, nil
	}
	if goos != "linux" && goos != "darwin" {
		return nil, errors.New("invalid MCP action or Tunnel ID")
	}
	if action != "setup" {
		return exec.Command("bash", path, action), nil
	}
	// Credentials are entered only into the local terminal.
	shellQuote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	if goos == "darwin" {
		line := "SYNDIRELAY_TUNNEL_ID=" + id + " bash " + shellQuote(path)
		return exec.Command("osascript", "-e", "tell application \"Terminal\" to do script "+strconv.Quote(line)), nil
	}
	for _, terminal := range []string{"x-terminal-emulator", "gnome-terminal"} {
		if bin, err := exec.LookPath(terminal); err == nil {
			sep := "-e"
			if terminal == "gnome-terminal" {
				sep = "--"
			}
			cmd := exec.Command(bin, sep, "bash", path)
			cmd.Env = append(os.Environ(), "SYNDIRELAY_TUNNEL_ID="+id)
			return cmd, nil
		}
	}
	return nil, errors.New("graphical terminal unavailable")
}
func (s *Server) handleMCPConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		status, err := s.MCPConnection(r.Context())
		if err != nil {
			http.Error(w, "MCP status unavailable", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	} else if r.Method == http.MethodPost {
		var in struct {
			Action   string `json:"action"`
			TunnelID string `json:"tunnel_id"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		dec.DisallowUnknownFields()
		if dec.Decode(&in) != nil || dec.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid input", 400)
			return
		}
		if in.Action == "install-client" {
			home, err := os.UserHomeDir()
			if err != nil {
				http.Error(w, "home unavailable", 500)
				return
			}
			_, err = installTunnelClient(r.Context(), tunnelReleaseHTTPClient(), runtime.GOOS, runtime.GOARCH, home)
			if err != nil {
				http.Error(w, "official client install failed; check network and verified release availability", 502)
				return
			}
		} else {
			exe, err := os.Executable()
			if err != nil {
				http.Error(w, "installation unavailable", http.StatusInternalServerError)
				return
			}
			root := filepath.Dir(filepath.Dir(exe))
			cmd, err := mcpCommand(runtime.GOOS, root, in.Action, in.TunnelID)
			if err != nil {
				http.Error(w, "invalid action, missing setup script or local terminal", 400)
				return
			}
			cmd.Dir = root
			if in.Action == "setup" {
				if err = cmd.Start(); err != nil {
					http.Error(w, "cannot open local setup terminal", 500)
					return
				}
				go func() { _ = cmd.Wait() }()
			} else {
				ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
				defer cancel()
				managed := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
				managed.Dir = root
				managed.SysProcAttr = cmd.SysProcAttr
				if err = managed.Run(); err != nil {
					http.Error(w, "service action failed; check local service configuration", 409)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"accepted": true})
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

package bridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Management stays local to the authenticated application, separate from remote MCP tools.
var tunnelIDPattern = regexp.MustCompile(`^tunnel_(?:[a-z0-9]{32}|[a-z0-9]{4}_[a-z0-9]{32})$`)
var profileTunnelID = regexp.MustCompile(`(?m)^\s*tunnel_id:\s*["']?(tunnel_[a-z0-9_]+)`)

type MCPConnectionStatus struct {
	ClientPath        string `json:"client_path"`
	ClientDefaultPath string `json:"client_default_path"`
	ClientCustom      bool   `json:"client_custom"`
	State             string `json:"state"`
	TunnelID          string `json:"tunnel_id"`
	ClientInstalled   bool   `json:"client_installed"`
	Configured        bool   `json:"configured"`
	Ready             bool   `json:"ready"`
	LocalConfig       any    `json:"local_config"`
	RuntimeKeySaved   bool   `json:"runtime_key_saved"`
	Running           bool   `json:"running"`
	SetupState        string `json:"setup_state"`
	Revision          string `json:"revision"`
	RestartRequired   bool   `json:"restart_required"`
	TunnelAutoConnect bool   `json:"tunnel_auto_connect"`
	TunnelConnecting  bool   `json:"tunnel_connecting"`
}

func mcpPaths(dataDir string) (string, string) {
	// Tunnel state belongs to SyndiRelay's existing data directory on every OS.
	// Never read or migrate the obsolete script-managed Tunnel configuration.
	p := filepath.Join(dataDir, "mcp")
	return filepath.Join(p, "syndirelay.yaml"), filepath.Join(p, "health.url")
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
	mcpHideProcess(cmd)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	return cmd.Run() == nil
}
func (s *Server) MCPConnection(ctx context.Context) (MCPConnectionStatus, error) {
	out := MCPConnectionStatus{State: "unconfigured"}
	exe, err := os.Executable()
	if err != nil {
		return out, err
	}
	args := []string{"mcp-stdio"}
	out.LocalConfig = map[string]any{"mcpServers": map[string]any{"syndirelay": map[string]any{"command": exe, "args": args}}}
	privatePaths, err := localMCPPaths()
	if err != nil {
		return out, err
	}
	profile, health := privatePaths.Profile, privatePaths.Health
	dataDir := filepath.Dir(privatePaths.Private)
	out.ClientDefaultPath = tunnelClientTarget(runtime.GOOS, dataDir)
	out.ClientPath, out.ClientCustom, err = selectedTunnelClientPath(runtime.GOOS, dataDir)
	if err != nil {
		return out, err
	}
	client := findTunnelClient(runtime.GOOS, dataDir)
	out.ClientInstalled = client != ""
	out.Running = mcpRunnerRunning(privatePaths)
	out.TunnelAutoConnect = mcpAutoConnectEnabled(privatePaths)
	if state, ok := mcpReadRunnerState(privatePaths); ok {
		out.TunnelConnecting = state.Connecting
	}
	if f, err := os.Open(profile); err == nil {
		data, readErr := io.ReadAll(io.LimitReader(f, 128<<10))
		f.Close()
		if readErr == nil {
			m := profileTunnelID.FindSubmatch(data)
			if len(m) == 2 && validNativeTunnel(string(m[1])) {
				out.Configured = true
				out.TunnelID = string(m[1])
			}
		}
	}
	private := filepath.Dir(health)
	keyName := "runtime.env"
	if runtime.GOOS == "windows" {
		keyName = "runtime-key.dpapi"
	}
	if info, err := os.Stat(filepath.Join(private, keyName)); err == nil && !info.IsDir() && info.Size() > 0 {
		if _, err := mcpReadKey(privatePaths); err == nil {
			out.RuntimeKeySaved = true
		}
	}
	if b, err := os.ReadFile(filepath.Join(private, "setup-state")); err == nil {
		out.SetupState = strings.TrimSpace(string(b))
	}
	if b, err := os.ReadFile(filepath.Join(private, "configuration-revision")); err == nil {
		out.Revision = strings.TrimSpace(string(b))
	}
	if info, err := os.Stat(filepath.Join(private, "restart-required")); err == nil && !info.IsDir() {
		out.RestartRequired = true
	}
	if !out.ClientInstalled {
		out.State = "client_missing"
	} else if out.Configured {
		out.State = "stopped_or_unhealthy"
		if out.Running {
			if out.TunnelConnecting {
				out.State = "starting_or_unhealthy"
			} else {
				out.State = "standby"
			}
		}
		out.Ready = out.Running && out.TunnelConnecting && tunnelReady(ctx, client, health)
		if out.Ready {
			out.State = "ready"
		}
	}
	return out, nil
}

// MCP runtime and credentials are managed by native Go.
func (s *Server) handleMCPConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		status, err := s.MCPConnection(r.Context())
		if err != nil {
			http.Error(w, "MCP status unavailable", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in mcpConnectionAction
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid request", 400)
		return
	}
	switch in.Action {
	case "save":
		s.handleMCPSave(w, r, in)
		return
	case "install-client":
		dataDir, err := ConfigDir()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "client_directory_unavailable", err.Error(), nil)
			return
		}
		if _, err = installTunnelClient(r.Context(), tunnelReleaseHTTPClient(), runtime.GOOS, runtime.GOARCH, dataDir); err != nil {
			writeAPIError(w, http.StatusBadGateway, "client_install_failed", err.Error(), nil)
			return
		}
		if _, err = setTunnelClientPath(runtime.GOOS, dataDir, ""); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "client_path_save_failed", err.Error(), nil)
			return
		}
	case "set-client-path":
		dataDir, err := ConfigDir()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "client_directory_unavailable", err.Error(), nil)
			return
		}
		if _, err = setTunnelClientPath(runtime.GOOS, dataDir, in.ClientPath); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_client_path", err.Error(), nil)
			return
		}
	case "connect", "disconnect":
		if err := mcpControlTunnel(in.Action); err != nil {
			writeAPIError(w, http.StatusConflict, "mcp_tunnel_conflict", "Tunnel 连接操作失败："+err.Error(), nil)
			return
		}
	case "auto-connect-enable", "auto-connect-disable":
		if err := mcpSetAutoConnect(in.Action == "auto-connect-enable"); err != nil {
			http.Error(w, "Tunnel connection preference update failed", 409)
			return
		}
	case "start", "stop", "restart":
		if err := nativeAction(in.Action); err != nil {
			writeAPIError(w, http.StatusConflict, "mcp_service_conflict", "Tunnel 服务操作失败："+err.Error(), nil)
			return
		}
	case "auto-status", "auto-enable", "auto-disable":
		exe, err := os.Executable()
		if err != nil {
			http.Error(w, "local executable unavailable", 500)
			return
		}
		action := strings.TrimPrefix(in.Action, "auto-")
		enabled, err := mcpAutoManage(action, exe)
		if err != nil {
			http.Error(w, "Tunnel startup management failed: "+err.Error(), 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"accepted": true, "auto_start": enabled})
		return
	default:
		http.Error(w, "unsupported MCP action", 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"accepted": true})
}

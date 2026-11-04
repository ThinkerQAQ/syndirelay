package bridge

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type mcpConnectionAction struct {
	Action           string `json:"action"`
	TunnelID         string `json:"tunnel_id"`
	RuntimeAPIKey    string `json:"runtime_api_key"`
	Replace          bool   `json:"replace"`
	PreviousTunnelID string `json:"previous_tunnel_id"`
	ReuseKey         bool   `json:"reuse_key"`
}

var mcpSaveMu sync.Mutex

// Modelled on VidOtter PR #23: fixed scripts only, Runtime API Key via stdin.
func mcpSaveCommand(goos, root, id string, replace bool, previous string, reuse bool) (*exec.Cmd, error) {
	if !tunnelIDPattern.MatchString(id) || (replace && !tunnelIDPattern.MatchString(previous)) {
		return nil, errors.New("invalid Tunnel ID")
	}
	if goos == "windows" {
		script := filepath.Join(root, "scripts", "Setup-SyndiRelay-MCP.ps1")
		if info, err := os.Stat(script); err != nil || !info.Mode().IsRegular() {
			return nil, os.ErrNotExist
		}
		args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script, "-TunnelId", id, "-KeyFromStdin", "-SaveOnly"}
		if replace {
			args = append(args, "-ReplaceExisting", "-ExpectedTunnelId", previous)
		}
		if reuse {
			args = append(args, "-ReuseSavedKey")
		}
		cmd := exec.Command("powershell.exe", args...)
		configureMCPProcess(cmd, false)
		return cmd, nil
	}
	if goos == "linux" || goos == "darwin" {
		script := filepath.Join(root, "scripts", "setup-mcp.sh")
		if info, err := os.Stat(script); err != nil || !info.Mode().IsRegular() {
			return nil, os.ErrNotExist
		}
		cmd := exec.Command("bash", script)
		cmd.Env = append(os.Environ(), "SYNDIRELAY_TUNNEL_ID="+id, "SYNDIRELAY_MCP_KEY_STDIN=1", "SYNDIRELAY_MCP_SAVE_ONLY=1")
		if replace {
			cmd.Env = append(cmd.Env, "SYNDIRELAY_REPLACE_TUNNEL=1", "SYNDIRELAY_EXPECTED_TUNNEL_ID="+previous)
		}
		if reuse {
			cmd.Env = append(cmd.Env, "SYNDIRELAY_MCP_REUSE_KEY=1")
		}
		return cmd, nil
	}
	return nil, errors.New("unsupported OS")
}

func (s *Server) handleMCPSave(w http.ResponseWriter, r *http.Request, in mcpConnectionAction) {
	if !tunnelIDPattern.MatchString(in.TunnelID) {
		http.Error(w, "invalid Tunnel ID", http.StatusBadRequest)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, "home unavailable", 500)
		return
	}
	profile, health := mcpPaths(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_CONFIG_HOME"))
	var oldID string
	if data, err := os.ReadFile(profile); err == nil {
		match := profileTunnelID.FindSubmatch(data)
		if len(match) == 2 && tunnelIDPattern.Match(match[1]) {
			oldID = string(match[1])
		}
		if oldID == "" {
			http.Error(w, "existing profile is invalid", 409)
			return
		}
	}
	if oldID != "" && (!in.Replace || in.PreviousTunnelID != oldID) {
		http.Error(w, "saved Tunnel ID changed; refresh before replacing", 409)
		return
	}
	if oldID == "" && in.Replace {
		http.Error(w, "no saved configuration to replace", 409)
		return
	}
	key := strings.TrimSpace(in.RuntimeAPIKey)
	if len(key) > 512 || strings.ContainsAny(key, "\r\n\x00") || (key != "" && len(key) < 16) || (in.ReuseKey && key != "") || (!in.ReuseKey && key == "") {
		http.Error(w, "invalid Runtime API Key", 400)
		return
	}
	if in.ReuseKey {
		name := "runtime.env"
		if runtime.GOOS == "windows" {
			name = "runtime-key.dpapi"
		}
		info, err := os.Stat(filepath.Join(filepath.Dir(health), name))
		if oldID == "" || err != nil || info.IsDir() || info.Size() == 0 {
			http.Error(w, "saved Runtime API Key unavailable", 409)
			return
		}
	}
	if findTunnelClient(runtime.GOOS, home) == "" {
		http.Error(w, "official Tunnel client missing", 409)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		http.Error(w, "installation unavailable", 500)
		return
	}
	root := InstallationRoot(exe)
	cmd, err := mcpSaveCommand(runtime.GOOS, root, in.TunnelID, in.Replace, in.PreviousTunnelID, in.ReuseKey)
	if err != nil {
		http.Error(w, "MCP setup script unavailable", 409)
		return
	}
	if !mcpSaveMu.TryLock() {
		http.Error(w, "configuration already in progress", 409)
		return
	}
	state := filepath.Join(filepath.Dir(health), "setup-state")
	if err := os.MkdirAll(filepath.Dir(state), 0700); err != nil {
		mcpSaveMu.Unlock()
		http.Error(w, "cannot create private configuration directory", 500)
		return
	}
	if err := os.WriteFile(state, []byte("running"), 0600); err != nil {
		mcpSaveMu.Unlock()
		http.Error(w, "cannot save setup state", 500)
		return
	}
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(key + "\n")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = os.WriteFile(state, []byte("failed"), 0600)
		mcpSaveMu.Unlock()
		http.Error(w, "unable to start background setup", 500)
		return
	}
	go func() {
		defer mcpSaveMu.Unlock()
		result := "succeeded"
		if cmd.Wait() != nil {
			result = "failed"
		}
		cmd.Stdin = nil
		_ = os.WriteFile(state, []byte(result), 0600)
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]bool{"accepted": true})
}

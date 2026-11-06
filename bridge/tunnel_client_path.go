package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The user-selected executable path is installation-specific. In particular,
// no PATH or shared ~/.local/bin fallback is allowed.
func tunnelClientPathPreference(dataDir string) string {
	return filepath.Join(dataDir, "mcp", "client-path")
}

func selectedTunnelClientPath(goos, dataDir string) (string, bool, error) {
	defaultPath := tunnelClientTarget(goos, dataDir)
	data, err := os.ReadFile(tunnelClientPathPreference(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return defaultPath, false, nil
	}
	if err != nil {
		return "", false, err
	}
	path := strings.TrimSpace(string(data))
	if path == "" {
		return defaultPath, false, nil
	}
	if len(path) > 4096 || !filepath.IsAbs(path) {
		return "", false, errors.New("saved Tunnel Client path is not an absolute path")
	}
	return filepath.Clean(path), true, nil
}

func tunnelClientExecutableExists(goos, path string) bool {
	if path == "" {
		return false
	}
	expectedName := "tunnel-client"
	if goos == "windows" {
		expectedName += ".exe"
	}
	name := filepath.Base(path)
	if goos == "windows" {
		if !strings.EqualFold(name, expectedName) {
			return false
		}
	} else if name != expectedName {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	// Windows filesystems do not expose Unix executable mode bits. This also
	// allows platform-fixture tests to exercise Unix archive layouts on Windows.
	return runtime.GOOS == "windows" || info.Mode().Perm()&0111 != 0
}

func findTunnelClient(goos, dataDir string) string {
	path, _, err := selectedTunnelClientPath(goos, dataDir)
	if err != nil || !tunnelClientExecutableExists(goos, path) {
		return ""
	}
	return path
}

// setTunnelClientPath validates the executable before persisting a custom
// location. An empty value restores the private installation default.
// A running Tunnel supervisor adopts changes only after Restart.
func setTunnelClientPath(goos, dataDir, selected string) (string, error) {
	tunnelClientInstallMu.Lock()
	defer tunnelClientInstallMu.Unlock()
	path := strings.TrimSpace(selected)
	pref := tunnelClientPathPreference(dataDir)
	if path == "" {
		if err := os.Remove(pref); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return tunnelClientTarget(goos, dataDir), nil
	}
	if len(path) > 4096 || !filepath.IsAbs(path) {
		return "", errors.New("Tunnel Client path must be an absolute file path")
	}
	path = filepath.Clean(path)
	if !tunnelClientExecutableExists(goos, path) {
		return "", fmt.Errorf("Tunnel Client executable is missing, invalid, or not executable: %s", path)
	}
	if path == tunnelClientTarget(goos, dataDir) {
		if err := os.Remove(pref); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return path, nil
	}
	if err := privateWrite(pref, []byte(path+"\n")); err != nil {
		return "", err
	}
	return path, nil
}

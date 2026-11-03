//go:build darwin

package bridge

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func mcpAutoManage(action, executable string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	unit := filepath.Join(home, "Library", "LaunchAgents", "com.thinkerqaq.syndirelay.tunnel.plist")
	data, err := os.ReadFile(unit)
	exists := err == nil
	if exists && !bytes.Contains(data, []byte(escapePlist(executable))) {
		return false, errors.New("another installation owns Tunnel LaunchAgent")
	}
	uid := strconv.Itoa(os.Getuid())
	label := "gui/" + uid + "/com.thinkerqaq.syndirelay.tunnel"
	switch action {
	case "status":
		return exists, nil
	case "enable":
		if !filepath.IsAbs(executable) || strings.ContainsAny(executable, "\r\n") {
			return false, errors.New("invalid executable path")
		}
		content := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.thinkerqaq.syndirelay.tunnel</string>
<key>ProgramArguments</key><array><string>` + escapePlist(executable) + `</string><string>tunnel-run</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
</dict></plist>`
		if err = privateWrite(unit, []byte(content)); err != nil {
			return false, err
		}
		// The preference is stored by the LaunchAgent itself; enable does not
		// start a Tunnel immediately.
		return true, nil
	case "disable":
		if !exists {
			return false, nil
		}
		// Stop only the task entry; running Tunnel stays controlled separately.
		_ = exec.Command("launchctl", "bootout", label).Run()
		if err = os.Remove(unit); err != nil {
			return false, err
		}
		return false, nil
	default:
		return false, errors.New("unsupported auto-start action")
	}
}
func escapePlist(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return r.Replace(s)
}

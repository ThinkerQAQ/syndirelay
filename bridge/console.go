package bridge

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"

	blogextension "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/extension"
)

func consoleHostAllowed(host string) bool {
	name, port, err := net.SplitHostPort(host)
	if err != nil || port != "32145" {
		return false
	}
	return name == "127.0.0.1" || name == "localhost"
}

// The console serves static, compiled public UI assets. Privileged operations
// go through the installed browser Extension's authenticated bridge transport;
// ordinary HTTP requests cannot acquire the native Bridge token.
func (s *Server) serveConsole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !consoleHostAllowed(r.Host) {
		http.Error(w, "invalid console host", http.StatusForbidden)
		return
	}
	// Reject speculative subresource fetches initiated by other websites.
	if strings.EqualFold(r.Header.Get("sec-fetch-site"), "cross-site") &&
		!strings.EqualFold(r.Header.Get("sec-fetch-mode"), "navigate") {
		http.Error(w, "cross-site console resources are forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' https:; "+
			"connect-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	if r.URL.Path == "/console" || r.URL.Path == "/console/" {
		http.Redirect(w, r, "/console/popup/popup.html", http.StatusFound)
		return
	}
	if r.URL.Path == "/console/popup/popup.html" {
		data, err := blogextension.Assets.ReadFile("popup/popup.html")
		if err != nil {
			http.Error(w, "console unavailable", http.StatusInternalServerError)
			return
		}
		random := make([]byte, 24)
		if _, err := rand.Read(random); err != nil {
			http.Error(w, "console nonce unavailable", http.StatusInternalServerError)
			return
		}
		nonce := base64.RawStdEncoding.EncodeToString(random)
		s := fmt.Sprintf("<meta name=\"blogctl-style-nonce\" content=\"%s\">", nonce)
		html := strings.Replace(string(data), "</head>", s+"\n</head>", 1)
		w.Header().Set("Content-Security-Policy",
			fmt.Sprintf("default-src 'none'; script-src 'self'; style-src 'self' 'nonce-%s'; img-src 'self' https:; "+
				"connect-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'", nonce))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
		return
	}
	subtree, err := fs.Sub(blogextension.Assets, ".")
	if err != nil {
		http.Error(w, "console unavailable", 500)
		return
	}
	http.StripPrefix("/console/", http.FileServer(http.FS(subtree))).ServeHTTP(w, r)
}

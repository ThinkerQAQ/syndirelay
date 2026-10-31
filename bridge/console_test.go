package bridge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConsoleServesEmbeddedSharedFeatureWithoutExposingBridgeToken(t *testing.T) {
	s := &Server{}
	request := httptest.NewRequest("GET", "http://127.0.0.1:32145/console/popup/popup.html", nil)
	response := httptest.NewRecorder()
	s.serveConsole(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "id=\"sharedDraftWorkspace\"") {
		t.Fatal("shared Feature UI missing")
	}
	if strings.Contains(response.Body.String(), "x-thinkerqaq-token") {
		t.Fatal("Bridge token exposed")
	}
	csp := response.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("CSP missing")
	}
	if strings.Contains(csp, "unsafe-inline") {
		t.Fatal("unsafe-inline must stay forbidden")
	}
	marker := "name=\"blogctl-style-nonce\" content=\""
	html := response.Body.String()
	start := strings.Index(html, marker)
	if start < 0 {
		t.Fatal("CodeMirror nonce meta missing")
	}
	remain := html[start+len(marker):]
	finish := strings.Index(remain, "\"")
	if finish < 0 || finish < 20 {
		t.Fatal("invalid randomized nonce")
	}
	nonce := remain[:finish]
	if !strings.Contains(csp, "'nonce-"+nonce+"'") {
		t.Fatal("CSP nonce mismatch")
	}
}
func TestConsoleBlocksForgedHostsCrossSiteResourceRequestsAndWrites(t *testing.T) {
	s := &Server{}
	requests := []struct{ host, method, path, site, mode string }{
		{"evil.example:32145", "GET", "/console/popup/popup.html", "", ""},
		{"127.0.0.1:32145", "POST", "/console/popup/popup.html", "", ""},
		{"127.0.0.1:32145", "GET", "/console/popup/popup.js", "cross-site", "no-cors"},
	}
	for _, item := range requests {
		req := httptest.NewRequest(item.method, "http://127.0.0.1:32145"+item.path, nil)
		req.Host = item.host
		if item.site != "" {
			req.Header.Set("Sec-Fetch-Site", item.site)
		}
		if item.mode != "" {
			req.Header.Set("Sec-Fetch-Mode", item.mode)
		}
		rr := httptest.NewRecorder()
		s.serveConsole(rr, req)
		if rr.Code < 400 {
			t.Fatalf("%+v accepted: %d", item, rr.Code)
		}
	}
}

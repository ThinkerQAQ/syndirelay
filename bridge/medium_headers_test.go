package bridge

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMediumRequestsAdvertiseJSONResponses(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, mediumOrigin+"/p/example/deltas", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	setMediumHeaders(req, platformSession{}, mediumOrigin+"/p/example/edit")
	if got := req.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Medium deltas request must accept JSON: %q", got)
	}
	if got := req.Header.Get("content-type"); got != "application/json" {
		t.Fatalf("Medium delta must send JSON: %q", got)
	}
}

func TestMediumEdgeClientHintsMatchBrowserFamily(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, mediumOrigin+"/_/upload?source=6", nil)
	if err != nil {
		t.Fatal(err)
	}
	setMediumHeaders(req, platformSession{UserAgent: "Mozilla/5.0 Chrome/155.0.0.0 Safari/537.36 Edg/155.0.0.0"}, mediumOrigin+"/p/draft-id/edit")
	got := req.Header.Get("sec-ch-ua")
	if !strings.Contains(got, "Microsoft Edge") || strings.Contains(got, "Google Chrome") {
		t.Fatalf("Edge UA must not advertise Chrome brand: %q", got)
	}
}
func TestMediumHTMLGatewayResponseIsSafelySummarized(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, mediumOrigin+"/_/upload", nil)
	if err != nil {
		t.Fatal(err)
	}
	html := "<!DOCTYPE html><html><title>Service Unavailable</title><body>SENSITIVE-HTML-PAYLOAD</body></html>"
	res := mediumResponse(req, http.StatusGatewayTimeout, html, http.Header{"Content-Type": {"text/html; charset=utf-8"}})
	var payload any
	err = decodeMediumResponse(res, &payload)
	if err == nil || !strings.Contains(err.Error(), "504") ||
		!strings.Contains(err.Error(), "HTML error page") ||
		strings.Contains(err.Error(), "SENSITIVE-HTML-PAYLOAD") {
		t.Fatalf("gateway response must be status-only, not HTML dump: %v", err)
	}
}

// Both platforms share the configured standard net/http transport.
// This regression test prevents a browser-fingerprint transport from returning.
func TestMediumAnd51ctoUseStandardBridgeHTTPTransport(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"medium", "51cto"} {
		server.mu.Lock()
		server.sessions[platform] = platformSession{ExpiresAt: time.Now().Add(time.Hour)}
		server.mu.Unlock()
		_, client, err := (bridgeNativePublisher{server: server}).publisherSession(platform)
		if err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
		if client != server.httpClient {
			t.Fatalf("%s: publisher must reuse configured Bridge HTTP client", platform)
		}
		if _, ok := client.Transport.(*http.Transport); !ok {
			t.Fatalf("%s: expected Go standard HTTP transport, got %T", platform, client.Transport)
		}
	}
}

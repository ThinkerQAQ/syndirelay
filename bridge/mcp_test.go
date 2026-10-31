package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	blogapp "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/app"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func mcpRequest(t *testing.T, ts *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req, err := http.NewRequest(method, ts.URL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("x-thinkerqaq-token", "test-token")
	req.Header.Set("content-type", "application/json")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestMCPSessionRefreshUsesExistingExtensionSession(t *testing.T) {
	t.Setenv("BLOGCTL_DATA_DIR", t.TempDir())
	server, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	r := mcpRequest(t, ts, "POST", "/v1/mcp/session-refresh", map[string]any{"platforms": []string{"medium", "devto"}})
	if r.StatusCode != 200 {
		t.Fatalf("request status = %d", r.StatusCode)
	}
	r.Body.Close()
	r = mcpRequest(t, ts, "GET", "/v1/mcp/session-refresh", nil)
	var pending struct {
		Pending []string `json:"pending"`
	}
	if err := json.NewDecoder(r.Body).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if len(pending.Pending) != 1 || pending.Pending[0] != "medium" {
		t.Fatalf("pending = %#v", pending.Pending)
	}
	requestBody, _ := json.Marshal(map[string]any{"cookies": []map[string]string{{"name": "sid", "value": "not-visible-to-mcp"}}, "userAgent": "Mozilla/5.0"})
	req, _ := http.NewRequest("POST", ts.URL+"/v1/sessions/medium", bytes.NewReader(requestBody))
	setExtensionAuth(req, "test-token")
	req.Header.Set("content-type", "application/json")
	r, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatalf("extension session status = %d", r.StatusCode)
	}
	r = mcpRequest(t, ts, "GET", "/v1/mcp/session-refresh", nil)
	defer r.Body.Close()
	pending.Pending = nil
	_ = json.NewDecoder(r.Body).Decode(&pending)
	if len(pending.Pending) != 0 {
		t.Fatalf("session refresh not acknowledged: %v", pending.Pending)
	}
	r = mcpRequest(t, ts, "GET", "/v1/mcp/platforms", nil)
	defer r.Body.Close()
	var text map[string]any
	_ = json.NewDecoder(r.Body).Decode(&text)
	if _, contains := text["cookies"]; contains {
		t.Fatal("secret leaked into MCP response")
	}
}
func TestMCPArticleReadSafePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BLOGCTL_DATA_DIR", t.TempDir())
	target := filepath.Join(dir, "src", "content", "articles", "en", "test.md")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("# English version"), 0600); err != nil {
		t.Fatal(err)
	}
	server, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	server.config.ContentRoot = dir
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	r := mcpRequest(t, ts, "GET", "/v1/mcp/article/test?language=en", nil)
	if r.StatusCode != 200 {
		t.Fatalf("read status %d", r.StatusCode)
	}
	var result struct {
		Markdown string `json:"markdown"`
	}
	_ = json.NewDecoder(r.Body).Decode(&result)
	r.Body.Close()
	if result.Markdown != "# English version" {
		t.Fatalf("markdown = %q", result.Markdown)
	}
	for _, path := range []string{"/v1/mcp/article/../secret?language=en", "/v1/mcp/article/%2e%2e/secret?language=en", "/v1/mcp/article/test?language=../"} {
		r := mcpRequest(t, ts, "GET", path, nil)
		r.Body.Close()
		if r.StatusCode == 200 {
			t.Errorf("accepted unsafe article: %s", path)
		}
	}
	// Without the local Bridge token no article should be returned.
	req, _ := http.NewRequest("GET", ts.URL+"/v1/mcp/article/test?language=en", nil)
	r, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 401 {
		t.Fatalf("unauthorized = %d", r.StatusCode)
	}
}
func TestMCPIdempotentPublishingJobs(t *testing.T) {
	t.Setenv("BLOGCTL_DATA_DIR", t.TempDir())
	server, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	var runs atomic.Int32
	server.syncRunner = func(_ context.Context, _ bridgeConfig, _ syncRequest, _ func(blogapp.SyncEvent)) (string, error) {
		runs.Add(1)
		return "done", nil
	}
	request := syncRequest{Article: "test", Platforms: []string{"devto"}, Operation: "create", RequestID: "mcp-test-request-0001"}
	first, err := server.startSyncJob(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := server.startSyncJob(request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("duplicate job IDs: %s %s", first.ID, second.ID)
	}
	changed := request
	changed.Article = "changed"
	if _, err = server.startSyncJob(changed); err == nil {
		t.Fatal("reusing request ID with changed payload must fail")
	}
	deadline := time.After(time.Second)
	for {
		if runs.Load() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("job never executed")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	time.Sleep(25 * time.Millisecond)
	if runs.Load() != 1 {
		t.Fatalf("duplicate executed %d times", runs.Load())
	}
	// The request ID is restored from the original jobs.json payload after
	// restarting the Bridge, so a process restart cannot blindly create again.
	expiry := time.Now().Add(time.Second)
	for {
		server.mu.Lock()
		state := server.jobs[first.ID].State
		server.mu.Unlock()
		if state == "completed" {
			break
		}
		if time.Now().After(expiry) {
			t.Fatal("job never completed")
		}
		time.Sleep(time.Millisecond)
	}
	recovered, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	recovered.syncRunner = func(_ context.Context, _ bridgeConfig, _ syncRequest, _ func(blogapp.SyncEvent)) (string, error) {
		runs.Add(1)
		return "unexpected", nil
	}
	restored, err := recovered.startSyncJob(request)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID != first.ID {
		t.Fatalf("recovered id = %s, expected %s", restored.ID, first.ID)
	}
	if runs.Load() != 1 {
		t.Fatal("recovered request triggered a duplicate publish")
	}
}

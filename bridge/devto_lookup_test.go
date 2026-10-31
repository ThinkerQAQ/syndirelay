package bridge

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevtoArticleSearchReadsAccountArticles(t *testing.T) {
	root := t.TempDir()
	articles := filepath.Join(root, "src", "content", "articles")
	if err := os.MkdirAll(articles, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articles, "example.md"), []byte("---\ntitle: Example\nstatus: published\n---\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	server.config.ContentRoot = root
	server.config.PublicationBindingsPath = filepath.Join(t.TempDir(), "publications.json")
	server.config.DevtoAPIKey = "test-api-key"
	server.httpClient = &http.Client{Transport: cnBlogsBindingTransport(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/articles/me/all" || request.URL.Query().Get("page") != "1" {
			t.Fatalf("unexpected DEV.to request: %s %s", request.Method, request.URL.String())
		}
		if request.Header.Get("api-key") != "test-api-key" {
			t.Fatal("DEV.to API key was not sent")
		}
		body := `[ {"id":42,"title":"Example","url":"https://dev.to/user/example","canonical_url":"https://thinkerqaq.github.io/en/articles/example/","published":false}, {"id":43,"title":"Other","url":"https://dev.to/user/other","published":true} ]`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header), Request: request}, nil
	})}
	request := httptest.NewRequest(http.MethodPost, "/v1/devto/articles/search?article=example", nil)
	setExtensionAuth(request, "token")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "test-api-key") {
		t.Fatal("search response exposed API key")
	}
	var decoded struct {
		Candidates []devtoArticleCandidate `json:"candidates"`
		Truncated  bool                    `json:"truncated"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Candidates) != 1 || decoded.Candidates[0].ID != 42 || decoded.Truncated {
		t.Fatalf("unexpected DEV.to candidates: %#v", decoded)
	}
}

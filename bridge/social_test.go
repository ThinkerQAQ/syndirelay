package bridge

import (
	"encoding/json"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/social"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSocialAPIRequiresBridgeTokenEvenForDraftListing(t *testing.T) {
	t.Setenv("BLOGCTL_DATA_DIR", t.TempDir())
	s := &Server{token: "test-token"}
	for _, tc := range []struct {
		path, token string
		want        int
	}{
		{"/v1/social/drafts", "", http.StatusUnauthorized},
		{"/v1/social/drafts", "wrong", http.StatusUnauthorized},
		{"/v1/social/drafts", "test-token", http.StatusOK},
		{"/v1/social/drafts/../bad", "test-token", http.StatusBadRequest},
	} {
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.token != "" {
			r.Header.Set("x-thinkerqaq-token", tc.token)
		}
		w := httptest.NewRecorder()
		s.handleSocial(w, r, strings.Split(strings.Trim(tc.path, "/"), "/"))
		if w.Code != tc.want {
			t.Fatalf("%s token=%q: got %d want %d: %s", tc.path, tc.token, w.Code, tc.want, w.Body.String())
		}
	}
}

func TestSocialBridgeReturnsExtensionCompatibleSuccessEnvelope(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BLOGCTL_DATA_DIR", dir)
	input := social.DraftInput{
		ID:     "hello-scott-20261017",
		Source: social.Source{Kind: "series", Slug: "concurrency-programming", Language: "en", URL: "https://thinkerqaq.com/en/series/concurrency-programming/"},
		Posts:  map[string]social.Post{"linkedin": {Text: "Hello from Scott"}},
	}
	draft, err := social.Create(dir, input)
	if err != nil {
		t.Fatal(err)
	}
	bridge := &Server{token: "test-token"}
	request := func(method, path, body string) map[string]any {
		t.Helper()
		r := httptest.NewRequest(method, "http://127.0.0.1:32145"+path, strings.NewReader(body))
		r.Header.Set("x-thinkerqaq-token", "test-token")
		w := httptest.NewRecorder()
		bridge.handleSocial(w, r, strings.Split(strings.Trim(path, "/"), "/"))
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s: HTTP %d: %s", method, path, w.Code, w.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["ok"] != true {
			t.Fatalf("%s %s: missing ok=true: %s", method, path, w.Body.String())
		}
		return result
	}
	listed := request(http.MethodGet, "/v1/social/drafts", "")
	drafts, ok := listed["drafts"].([]any)
	if !ok || len(drafts) != 1 {
		t.Fatalf("draft listing: %+v", listed)
	}
	loaded := request(http.MethodGet, "/v1/social/drafts/"+draft.ID, "")
	if loaded["draft"].(map[string]any)["id"] != draft.ID {
		t.Fatalf("draft lookup: %+v", loaded)
	}
	changed := request(http.MethodPut, "/v1/social/drafts/"+draft.ID,
		`{"revision":"`+draft.Revision+`","posts":{"linkedin":{"text":"Updated hello from Scott"}}}`)
	remote := changed["draft"].(map[string]any)
	if remote["approvedRevision"] != nil || remote["revision"] == draft.Revision {
		t.Fatalf("editing must invalidate the content revision: %+v", changed)
	}
}

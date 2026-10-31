package bridge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Medium inventory is account-scoped and read-only. Both drafts and
// published stories already use the publisher's existing listPosts flow.
func TestMediumRemoteInventoryIncludesDraftsAndPublished(t *testing.T) {
	server, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	server.httpClient = &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != "/_/graphql" {
			t.Fatalf("unexpected Medium request %s %s", req.Method, req.URL)
			return nil, nil
		}
		operation := req.Header.Get("graphql-operation")
		calls = append(calls, operation)
		if !strings.Contains(req.Header.Get("cookie"), "sid=test-session") {
			t.Fatal("missing Medium browser session cookie")
		}
		switch operation {
		case "BlogCTLMediumDraftsQuery":
			return mediumResponse(req, http.StatusOK, `[{"data":{"viewer":{"id":"user","username":"ThinkerQAQ","latestPostsConnection":{"postPreviews":[{"postId":"abcde1234","post":{"id":"abcde1234","title":"Concurrency draft","isPublished":false}}]}}}}]`, nil), nil
		case "BlogCTLMediumPublishedQuery":
			return mediumResponse(req, http.StatusOK, `[{"data":{"viewer":{"id":"user","username":"ThinkerQAQ","latestPostsConnection":{"postPreviews":[{"postId":"ff1234aaaa","post":{"id":"ff1234aaaa","title":"Published article","mediumUrl":"https://medium.com/@ThinkerQAQ/published-ff1234aaaa","isPublished":true}}]}}}}]`, nil), nil
		default:
			t.Fatalf("unrecognized Medium query %q", operation)
			return nil, nil
		}
	})}
	handler := server.Handler()
	body := []byte(`{"cookies":[{"name":"sid","value":"test-session","domain":"medium.com","path":"/","secure":true}],"userAgent":"Mozilla/5.0"}`)
	syncReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/medium", bytes.NewReader(body))
	syncReq.Header.Set("content-type", "application/json")
	setExtensionAuth(syncReq, "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, syncReq)
	if recorder.Code != http.StatusOK {
		t.Fatalf("session sync status=%d: %s", recorder.Code, recorder.Body)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/remote/inventory?platform=medium", nil)
	setExtensionAuth(req, "test-token")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("inventory status=%d: %s", recorder.Code, recorder.Body)
	}
	var payload struct {
		Platform string                `json:"platform"`
		Account  string                `json:"account"`
		Items    []remoteInventoryPost `json:"items"`
		Partial  string                `json:"partial"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Platform != "medium" || payload.Account != "ThinkerQAQ" || payload.Partial != "" {
		t.Fatalf("metadata=%+v", payload)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("unexpected inventory: %+v", payload.Items)
	}
	want := []remoteInventoryPost{
		{ID: "abcde1234", Title: "Concurrency draft", URL: "https://medium.com/p/abcde1234/edit", Published: false},
		{ID: "ff1234aaaa", Title: "Published article", URL: "https://medium.com/@ThinkerQAQ/published-ff1234aaaa", Published: true},
	}
	for i, item := range payload.Items {
		if item != want[i] {
			t.Fatalf("item[%d]=%+v want %+v", i, item, want[i])
		}
	}
	if len(calls) != 2 || calls[0] != "BlogCTLMediumDraftsQuery" || calls[1] != "BlogCTLMediumPublishedQuery" {
		t.Fatalf("GraphQL calls=%v", calls)
	}
}

func TestMediumRemoteInventoryNeedsExtensionAuthAndSession(t *testing.T) {
	server, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	unauth := httptest.NewRequest(http.MethodPost, "/v1/remote/inventory?platform=medium", nil)
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, unauth)
	if out.Code == http.StatusOK {
		t.Fatal("unauthorized read returned 200")
	}
	authorized := httptest.NewRequest(http.MethodPost, "/v1/remote/inventory?platform=medium", nil)
	setExtensionAuth(authorized, "test-token")
	out = httptest.NewRecorder()
	handler.ServeHTTP(out, authorized)
	if out.Code == http.StatusOK || !strings.Contains(out.Body.String(), "session_required") {
		t.Fatalf("missing Medium browser session must fail: status=%d body=%s", out.Code, out.Body)
	}
}

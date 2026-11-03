package publisher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExplicitHashnodeUpdateUsesSelectedDraftWithoutRecreate(t *testing.T) {
	editor := &hashnodeTestEditor{version: 100}
	server := httptest.NewServer(http.HandlerFunc(editor.serve))
	defer server.Close()
	service := Service{HTTPClient: &http.Client{Transport: rewriteHashnodeHost{base: http.DefaultTransport, target: server.URL}}}
	session := Session{Cookies: []BrowserCookie{{Name: "test_session", Value: "dummy", Domain: "hashnode.com"}},
		CookieHostSuffixes: []string{"hashnode.com"}}
	input := DraftInput{Slug: "qa", Title: "New heading", Markdown: "E2E_VERSION_UPDATED"}
	got, e := service.RunExplicitDraft(context.Background(), "hashnode", session, input, "update",
		ExplicitTarget{ID: "draft-1", State: "draft", URL: "https://hashnode.com/draft/draft-1"}, false)
	if e != nil {
		t.Fatal(e)
	}
	if got.Kind != "draft-updated" || got.ID != "draft-1" || editor.creates != 0 || editor.updates != 2 || editor.body != input.Markdown {
		t.Fatalf("result %+v creates %d updates %d body %q", got, editor.creates, editor.updates, editor.body)
	}
}

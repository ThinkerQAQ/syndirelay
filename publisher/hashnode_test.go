package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type hashnodeTestEditor struct {
	mu            sync.Mutex
	creates       int
	updates       int
	failFirstEdit bool
	body          string
	title         string
	version       int64
}

func (e *hashnodeTestEditor) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/publications":
		fmt.Fprint(w, `{"success":true,"publications":[{"_id":"publication-1"}]}`)
	case r.Method == http.MethodPost && r.URL.Path == "/api/drafts":
		e.creates++
		fmt.Fprint(w, `{"success":true,"draftId":"draft-1"}`)
	case r.Method == http.MethodGet && r.URL.Path == "/api/drafts/draft-1":
		fmt.Fprintf(w, `{"success":true,"draft":{"_id":"draft-1","publication":"publication-1","dateUpdated":%d}}`, e.version)
	case r.Method == http.MethodPost && r.URL.Path == "/api/drafts/draft-1/lock":
		fmt.Fprint(w, `{"success":true,"locked":true}`)
	case r.Method == http.MethodDelete && r.URL.Path == "/api/drafts/draft-1/lock":
		fmt.Fprint(w, `{"success":true,"released":true}`)
	case r.Method == http.MethodPut && r.URL.Path == "/api/drafts/draft-1":
		if e.failFirstEdit {
			e.failFirstEdit = false
			http.Error(w, "temporarily unavailable", http.StatusInternalServerError)
			return
		}
		var fields map[string]any
		if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if int64(fields["baseDateUpdated"].(float64)) != e.version {
			http.Error(w, "stale revision", 409)
			return
		}
		if len(fields["editSessionId"].(string)) != 36 {
			http.Error(w, "bad edit session", 400)
			return
		}
		if title, ok := fields["title"].(string); ok {
			e.title = title
		}
		if body, ok := fields["contentMarkdown"].(string); ok {
			e.body = body
		}
		e.version++
		e.updates++
		fmt.Fprintf(w, `{"success":true,"draftId":"draft-1","dateUpdated":%d}`, e.version)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func TestHashnodeDraftCreationThenUpdateUsesSameRemoteID(t *testing.T) {
	editor := &hashnodeTestEditor{version: 100}
	server := httptest.NewServer(http.HandlerFunc(editor.serve))
	defer server.Close()
	adapter, err := newHashnodeAdapter(server.Client(), Session{
		Cookies: []BrowserCookie{{Name: "test", Value: "value", Domain: "localhost"}},
	}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if auth, err := adapter.CheckAuth(ctx); err != nil || !auth.Authenticated {
		t.Fatalf("auth = %+v, %v", auth, err)
	}
	created, err := adapter.CreateDraft(ctx, DraftInput{})
	if err != nil || created.ID != "draft-1" || !created.Created {
		t.Fatalf("create %+v, %v", created, err)
	}
	input := DraftInput{Title: "English title", Markdown: "# English body", NativeCanonicalURL: "https://thinkerqaq.com/en/articles/example/"}
	updated, err := adapter.UpdateDraft(ctx, DraftRef{ID: created.ID, URL: created.URL}, input)
	if err != nil || updated.ID != created.ID || !updated.Updated {
		t.Fatalf("update %+v, %v", updated, err)
	}
	if editor.creates != 1 || editor.updates != 3 || editor.title != input.Title || editor.body != input.Markdown {
		t.Fatalf("editor state creates=%d updates=%d title=%q body=%q", editor.creates, editor.updates, editor.title, editor.body)
	}
}

func TestHashnodeDraftBindingSurvivesEditorFailureWithoutDuplicate(t *testing.T) {
	editor := &hashnodeTestEditor{version: 200, failFirstEdit: true}
	server := httptest.NewServer(http.HandlerFunc(editor.serve))
	defer server.Close()
	service := Service{HTTPClient: server.Client(), PublicationPath: t.TempDir() + "/publications.json"}
	// Service authenticates with the production origin. Rewrite only the host
	// in transport for this local test, preserving the actual HTTP behavior.
	base := service.HTTPClient
	service.HTTPClient = &http.Client{Transport: rewriteHashnodeHost{base: base.Transport, target: server.URL}}
	session := Session{Cookies: []BrowserCookie{{Name: "test_session", Value: "dummy", Domain: "hashnode.com"}}, CookieHostSuffixes: []string{"hashnode.com"}}
	input := DraftInput{Slug: "example", Title: "Example", Markdown: "# Body", ContentHash: "new-hash"}
	if _, err := service.CreateOrUpdateDraftInput(context.Background(), "hashnode", session, "", input, false); err == nil {
		t.Fatal("first editor update should fail")
	}
	state, _, err := LoadPublicationState(service.PublicationPath, input.Slug, "hashnode")
	if err != nil || state.RemoteDraftID != "draft-1" || state.DraftHash != "" {
		t.Fatalf("incomplete draft was not persisted: %+v %v", state, err)
	}
	result, err := service.CreateOrUpdateDraftInput(context.Background(), "hashnode", session, "", input, false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || editor.creates != 1 {
		t.Fatalf("retry must reuse draft: result=%+v creates=%d", result, editor.creates)
	}
	state, _, err = LoadPublicationState(service.PublicationPath, input.Slug, "hashnode")
	if err != nil || state.DraftHash != input.ContentHash {
		t.Fatalf("draft hash not saved: %+v %v", state, err)
	}
}

type rewriteHashnodeHost struct {
	base   http.RoundTripper
	target string
}

func (h rewriteHashnodeHost) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "hashnode.com" {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(h.target, "http://")
		return http.DefaultTransport.RoundTrip(clone)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestHashnodeUploadRejectsUnknownPresignHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/upload/image" {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("content-type", "application/json")
		fmt.Fprint(w, `{"success":true,"presignedUrl":"https://example.org/upload","cdnUrl":"https://cdn.hashnode.com/cover.png"}`)
	}))
	defer server.Close()
	h := &hashnodeAdapter{origin: server.URL, client: server.Client(), uploadClient: server.Client()}
	if _, err := h.uploadImage(context.Background(), RehostImage{Payload: []byte("fake-png"), ContentType: "image/png"}); err == nil {
		t.Fatal("upload must reject untrusted presigned host before forwarding image bytes")
	}
}

type uploadRoundTrip func(*http.Request) (*http.Response, error)

func (f uploadRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestHashnodeNativeImageUploadDoesNotForwardBrowserCookies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/upload/image" || req.Method != http.MethodPost {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("content-type", "application/json")
		fmt.Fprint(w, `{"success":true,"presignedUrl":"https://hashnode-images.example.us.r2.cloudflarestorage.com/upload.png?opaque=1","cdnUrl":"https://cdn.hashnode.com/images/upload.png"}`)
	}))
	defer server.Close()
	var sent int
	upload := &http.Client{Transport: uploadRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPut || req.URL.Hostname() != "hashnode-images.example.us.r2.cloudflarestorage.com" {
			t.Errorf("unexpected upload endpoint: %s %s", req.Method, req.URL.Hostname())
		}
		if got := req.Header.Get("Cookie"); got != "" {
			t.Error("Hashnode browser cookies must not be sent to R2")
		}
		if req.Header.Get("content-type") != "image/png" {
			t.Error("content-type must be preserved")
		}
		sent++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	h := &hashnodeAdapter{origin: server.URL, client: server.Client(), uploadClient: upload}
	result, err := h.uploadImage(context.Background(), RehostImage{Payload: []byte("png-bytes"), ContentType: "image/png"})
	if err != nil || sent != 1 || result != "https://cdn.hashnode.com/images/upload.png" {
		t.Fatalf("result=%s uploaded=%d err=%v", result, sent, err)
	}
}

func TestHashnodeListPostsReadsBothTabsAndEditorURLs(t *testing.T) {
	var draftsQueried, publishedQueried int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/api/publications":
			fmt.Fprint(w, `{"success":true,"publications":[{"_id":"publication-1","username":"thinkerqaq"}]}`)
		case "/api/drafts":
			if r.URL.Query().Get("publicationId") != "publication-1" || r.URL.Query().Get("limit") != "50" {
				t.Errorf("invalid list query: %s", r.URL.RawQuery)
			}
			switch r.URL.Query().Get("tab") {
			case "drafts":
				draftsQueried++
				fmt.Fprint(w, `{"success":true,"drafts":[{"_id":"draft-1","title":"Draft title"}],"hasMore":false}`)
			case "published":
				publishedQueried++
				fmt.Fprint(w, `{"success":true,"published":[{"_id":"post-1","title":"Public title","slug":"public-title"}],"hasMore":false}`)
			default:
				http.Error(w, "invalid tab", 400)
			}
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer server.Close()
	adapter, err := newHashnodeAdapter(server.Client(), Session{Cookies: []BrowserCookie{{Name: "test", Value: "value", Domain: "localhost"}}}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	user, posts, partial, err := adapter.listPosts(context.Background(), "")
	if err != nil || partial || user != "thinkerqaq" {
		t.Fatalf("user=%q posts=%v partial=%v err=%v", user, posts, partial, err)
	}
	if draftsQueried != 1 || publishedQueried != 1 || len(posts) != 2 || posts[0].ID != "draft-1" || posts[0].URL != "https://hashnode.com/draft/draft-1" || posts[1].ID != "post-1" || posts[1].URL != "https://thinkerqaq.hashnode.dev/public-title" || !posts[1].Published {
		t.Fatalf("unexpected Hashnode inventory: %+v", posts)
	}
}

func TestHashnodeListPostsFailsClosedWhenDraftsAreTruncated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if r.URL.Path == "/api/publications" {
			fmt.Fprint(w, `{"success":true,"publications":[{"_id":"p","username":"thinkerqaq"}]}`)
			return
		}
		if r.URL.Query().Get("tab") == "drafts" {
			fmt.Fprint(w, `{"success":true,"drafts":[],"hasMore":true}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"published":[],"hasMore":false}`)
	}))
	defer server.Close()
	a, err := newHashnodeAdapter(server.Client(), Session{Cookies: []BrowserCookie{{Name: "session", Value: "v", Domain: "localhost"}}}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _, incomplete, err := a.listPosts(context.Background(), "")
	if err != nil || !incomplete {
		t.Fatalf("missing incomplete detection: incomplete=%v error=%v", incomplete, err)
	}
}

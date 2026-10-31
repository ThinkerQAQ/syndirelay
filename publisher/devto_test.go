package publisher

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func devtoTestInput() DraftInput {
	return DraftInput{
		Slug: "example", Title: "Example", Description: "Description",
		Markdown: "Body\n", ContentHash: "hash-1", Tags: []string{"Go", "Concurrency"},
		CoverImageURL:      "https://thinkerqaq.github.io/media/cover.png",
		NativeCanonicalURL: "https://thinkerqaq.github.io/en/articles/example/",
		Published:          false,
	}
}

func TestDEVToCreateDiscoversCanonicalArticleAndUpdatesInsteadOfDuplicating(t *testing.T) {
	var mu sync.Mutex
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		methods = append(methods, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.Header.Get("api-key") != "secret" {
			http.Error(w, "missing key", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/articles/me/all":
			_ = json.NewEncoder(w).Encode([]devtoArticle{{
				ID: 42, Title: "Example",
				CanonicalURL: "https://thinkerqaq.github.io/en/articles/example/?utm_source=old#fragment",
				URL:          "https://dev.to/thinker/example-42",
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/articles/42":
			_ = json.NewEncoder(w).Encode(devtoArticle{
				ID: 42, Title: "Example", Description: "Old", BodyMarkdown: "Old body",
				CanonicalURL: "https://thinkerqaq.github.io/en/articles/example/",
				TagList:      []string{"go"}, URL: "https://dev.to/thinker/example-42",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/articles/42":
			var body struct {
				Article devtoPayload `json:"article"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Article.BodyMarkdown != "Body\n" || body.Article.Tags != "go,concurrency" {
				t.Fatalf("payload = %#v", body.Article)
			}
			_ = json.NewEncoder(w).Encode(devtoArticle{ID: 42, URL: "https://dev.to/thinker/example-42"})
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()

	adapter, err := newDEVToAdapter(server.Client(), Session{APIKey: "secret"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.CreateDraft(context.Background(), devtoTestInput())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.Created || result.ID != "42" {
		t.Fatalf("result = %#v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, method := range methods {
		if strings.HasPrefix(method, http.MethodPost+" ") {
			t.Fatalf("unexpected duplicate create: %v", methods)
		}
	}
}

func TestDEVToSkipsUnchangedChangedOnlyDraft(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/articles/me/all":
			_ = json.NewEncoder(w).Encode([]devtoArticle{{
				ID: 42, Title: "Example", Description: "Description", BodyMarkdown: "Body",
				CanonicalURL: "https://thinkerqaq.github.io/en/articles/example/",
				TagList:      []string{"go", "concurrency"}, URL: "https://dev.to/thinker/example-42",
			}})
		default:
			http.Error(w, "unexpected mutation", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	adapter, err := newDEVToAdapter(server.Client(), Session{APIKey: "secret"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	input := devtoTestInput()
	input.ChangedOnly = true
	result, err := adapter.CreateDraft(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped || requests != 1 {
		t.Fatalf("result = %#v requests=%d", result, requests)
	}
}

func TestDEVToCreatesWhenCanonicalDoesNotExist(t *testing.T) {
	posted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/articles/me/all":
			_ = json.NewEncoder(w).Encode([]devtoArticle{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/articles":
			posted = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(devtoArticle{ID: 99, URL: "https://dev.to/thinker/example-99"})
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()

	adapter, _ := newDEVToAdapter(server.Client(), Session{APIKey: "secret"}, server.URL)
	result, err := adapter.CreateDraft(context.Background(), devtoTestInput())
	if err != nil {
		t.Fatal(err)
	}
	if !posted || !result.Created || result.ID != "99" {
		t.Fatalf("result = %#v posted=%v", result, posted)
	}
}

func TestDEVToPublishDraftUsesOfficialArticleAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/articles/42" {
			http.Error(w, "unexpected", http.StatusNotFound)
			return
		}
		if r.Header.Get("api-key") != "secret" || r.Header.Get("accept") != devtoAccept {
			t.Fatalf("headers = %#v", r.Header)
		}
		var body struct {
			Article map[string]any `json:"article"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Article["published"] != true || len(body.Article) != 1 {
			t.Fatalf("publish payload = %#v", body.Article)
		}
		_ = json.NewEncoder(w).Encode(devtoArticle{
			ID: 42, Published: true, URL: "https://dev.to/thinker/example-42",
		})
	}))
	defer server.Close()

	adapter, err := newDEVToAdapter(server.Client(), Session{APIKey: "secret"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.PublishDraft(context.Background(), DraftRef{ID: "42"}, DraftInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "42" || result.URL != "https://dev.to/thinker/example-42" {
		t.Fatalf("result = %#v", result)
	}
}

func TestDEVToNativeImageUploadUsesCapturedBrowserFlow(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "assets", "mermaid")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "diagram.png"), []byte("diagram-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	dashboardHits := 0
	uploads := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/dashboard":
			dashboardHits++
			if !strings.Contains(r.Header.Get("cookie"), "_forem_session=session-value") {
				t.Fatalf("browser cookie = %q", r.Header.Get("cookie"))
			}
			_, _ = io.WriteString(w, `<form><input type="hidden" name="authenticity_token" value="csrf-token" /></form>`)
		case r.Method == http.MethodGet && r.URL.Path == "/cover.png":
			w.Header().Set("content-type", "image/png")
			_, _ = w.Write([]byte("cover-bytes"))
		case r.Method == http.MethodPost && r.URL.Path == "/image_uploads":
			uploads++
			if r.Header.Get("x-csrf-token") != "csrf-token" {
				t.Fatalf("csrf header = %q", r.Header.Get("x-csrf-token"))
			}
			if !strings.Contains(r.Header.Get("cookie"), "_forem_session=session-value") {
				t.Fatalf("upload cookie = %q", r.Header.Get("cookie"))
			}
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("authenticity_token") != "csrf-token" {
				t.Fatalf("authenticity_token = %q", r.FormValue("authenticity_token"))
			}
			file, _, err := r.FormFile("image[]")
			if err != nil {
				t.Fatal(err)
			}
			_ = file.Close()
			target := "https://dev-to-uploads.s3.us-east-2.amazonaws.com/uploads/articles/body.png"
			if uploads == 2 {
				target = "https://dev-to-uploads.s3.us-east-2.amazonaws.com/uploads/articles/cover.png"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"links": []string{target}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/articles/me/all":
			if r.Header.Get("api-key") != "secret" {
				t.Fatal("API key missing")
			}
			_ = json.NewEncoder(w).Encode([]devtoArticle{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/articles":
			var body struct {
				Article devtoPayload `json:"article"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(body.Article.BodyMarkdown, "https://dev-to-uploads.s3.us-east-2.amazonaws.com/uploads/articles/body.png") {
				t.Fatalf("body markdown = %q", body.Article.BodyMarkdown)
			}
			if body.Article.MainImage != "https://dev-to-uploads.s3.us-east-2.amazonaws.com/uploads/articles/cover.png" {
				t.Fatalf("main image = %q", body.Article.MainImage)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(devtoArticle{ID: 99, URL: "https://dev.to/thinker/example-99"})
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()

	input := devtoTestInput()
	input.DistributionRoot = root
	input.SourceDir = root
	input.Markdown = "![diagram](blogctl-asset://mermaid/diagram)"
	input.CoverImageURL = server.URL + "/cover.png"
	input.Assets = []PublishingAsset{{
		Kind: "mermaid", ID: "diagram", ObjectKey: "generated/mermaid/diagram.png",
		PublicURL: "https://assets.example/generated/mermaid/diagram.png",
		Source:    "blogctl-asset://mermaid/diagram",
	}}

	adapter, err := newDEVToAdapter(server.Client(), Session{
		APIKey:    "secret",
		Cookies:   []BrowserCookie{{Name: "_forem_session", Value: "session-value", Domain: "dev.to", Path: "/", Secure: true}},
		UserAgent: "BlogCTL-Test-UA",
	}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.CreateDraft(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || uploads != 2 || dashboardHits != 1 {
		t.Fatalf("result=%#v uploads=%d dashboardHits=%d", result, uploads, dashboardHits)
	}
}

func TestDEVToSaveRefusesToEditPublishedArticle(t *testing.T) {
	puts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/articles/me/all":
			_ = json.NewEncoder(w).Encode([]devtoArticle{{
				ID: 42, Title: "Example", CanonicalURL: "https://thinkerqaq.github.io/en/articles/example/",
				URL: "https://dev.to/thinker/example-42", PublishedAt: "2026-09-23T05:00:00Z",
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/articles/42":
			_ = json.NewEncoder(w).Encode(devtoArticle{
				ID: 42, Title: "Example", Description: "Old", BodyMarkdown: "Old body",
				CanonicalURL: "https://thinkerqaq.github.io/en/articles/example/",
				URL:          "https://dev.to/thinker/example-42", PublishedAt: "2026-09-23T05:00:00Z",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/articles/42":
			puts++
			http.Error(w, "must not update public article", http.StatusInternalServerError)
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()

	adapter, _ := newDEVToAdapter(server.Client(), Session{APIKey: "secret"}, server.URL)
	_, err := adapter.CreateDraft(context.Background(), devtoTestInput())
	if err == nil || !IsKind(err, ErrValidation) {
		t.Fatalf("error = %v, want validation error", err)
	}
	if puts != 0 {
		t.Fatalf("public DEV.to article was updated %d time(s)", puts)
	}
}

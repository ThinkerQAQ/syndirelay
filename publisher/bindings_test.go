package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseCNBlogsPostReference(t *testing.T) {
	for reference, want := range map[string]string{
		"42": "42",
		"https://www.cnblogs.com/ThinkerQAQ/p/42.html":  "42",
		"https://i.cnblogs.com/articles/edit;postId=42": "42",
		"https://i.cnblogs.com/posts/edit;postId=42":    "42",
	} {
		got, err := ParseCNBlogsPostReference(reference)
		if err != nil || got != want {
			t.Fatalf("reference %q = %q, %v", reference, got, err)
		}
	}
	for _, reference := range []string{"http://www.cnblogs.com/a/p/42", "https://evil.example/p/42", "https://i.cnblogs.com/other?postId=42", "abc"} {
		if _, err := ParseCNBlogsPostReference(reference); err == nil {
			t.Fatalf("accepted %q", reference)
		}
	}
}

func TestCNBlogsLookupUsesEditorListAndVerifiesDetail(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/api/user":
			return jsonResponse(request, 200, `{"loginName":"ThinkerQAQ"}`, nil), nil
		case "/api/posts/list":
			if request.URL.Query().Get("search") != "并发编程" {
				t.Fatal("search query was not sent")
			}
			return jsonResponse(request, 200, `{"postList":[{"id":42,"title":"并发编程","url":"https://www.cnblogs.com/ThinkerQAQ/p/42","isPublished":true,"dateUpdated":"2026-09-19T15:07:00"}],"postsCount":1}`, nil), nil
		case "/api/posts/42":
			return jsonResponse(request, 200, `{"blogPost":{"id":42,"title":"并发编程","url":"https://www.cnblogs.com/ThinkerQAQ/p/42","isPublished":true,"author":"ThinkerQAQ","dateUpdated":"2026-09-19T15:07:00"}}`, nil), nil
		default:
			t.Fatalf("unexpected request %s", request.URL.String())
			return nil, nil
		}
	})}
	account, posts, err := CNBlogsSearchPosts(context.Background(), client, cnBlogsSession(), "并发编程")
	if err != nil || account != "ThinkerQAQ" || len(posts) != 1 || !posts[0].Published || posts[0].ID != "42" {
		t.Fatalf("search = %q, %#v, %v", account, posts, err)
	}
	account, post, err := CNBlogsGetPost(context.Background(), client, cnBlogsSession(), "42")
	if err != nil || account != "ThinkerQAQ" || post.ID != "42" {
		t.Fatalf("detail = %q, %#v, %v", account, post, err)
	}
}

func TestPublicationBindingStateDeleteOnlyClearsSelectedSlot(t *testing.T) {
	path := testPublicationPath(t)
	if err := SavePublicationBinding(path, PublicationBinding{
		Slug: "example", Platform: "cnblogs",
		RemoteDraftID: "52", DraftURL: "https://i.cnblogs.com/articles/edit;postId=52",
		PublishedRemoteID: "42", PublishedURL: "https://www.cnblogs.com/ThinkerQAQ/p/42",
	}); err != nil {
		t.Fatal(err)
	}
	if err := DeletePublicationBindingState(path, "example", "cnblogs", "published", "99"); err == nil {
		t.Fatal("stale ID removed current publication state")
	}
	if err := DeletePublicationBindingState(path, "example", "cnblogs", "published", "42"); err != nil {
		t.Fatal(err)
	}
	binding, found, err := LoadPublicationBinding(path, "example", "cnblogs")
	if err != nil || !found || binding.PublishedRemoteID != "" || binding.RemoteDraftID != "52" {
		t.Fatalf("publication after published delete = %#v, %v, %v", binding, found, err)
	}
	if err := DeletePublicationBindingState(path, "example", "cnblogs", "draft", "52"); err != nil {
		t.Fatal(err)
	}
	_, found, err = LoadPublicationBinding(path, "example", "cnblogs")
	if err != nil || found {
		t.Fatalf("publication should be removed after final slot delete: %v, %v", found, err)
	}
}

func TestDurablePublicationWritesPreserveOtherPlatforms(t *testing.T) {
	path := testPublicationPath(t)
	if err := SavePublicationBinding(path, PublicationBinding{
		Slug: "example", Platform: "cnblogs", PublishedRemoteID: "42",
		PublishedURL: "https://www.cnblogs.com/ThinkerQAQ/p/42", Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)
	if err := SavePublicationDraftResult(path, "example", "juejin", "hash-1", DraftResult{
		ID: "draft-1", URL: "https://juejin.cn/editor/drafts/draft-1", Created: true,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := SavePublicationPublishResult(path, "example", "juejin", "hash-1", PublishResult{
		ID: "post-1", URL: "https://juejin.cn/post/post-1",
	}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	cnblogs, found, err := LoadPublicationBinding(path, "example", "cnblogs")
	if err != nil || !found || cnblogs.PublishedRemoteID != "42" {
		t.Fatalf("CNBlogs publication = %#v, %v, %v", cnblogs, found, err)
	}
	state, _, err := LoadPublicationState(path, "example", "juejin")
	if err != nil {
		t.Fatal(err)
	}
	if state.RemoteDraftID != "" || state.DraftURL != "" || state.DraftHash != "" ||
		state.PublishedRemoteID != "post-1" || state.PublishedURL != "https://juejin.cn/post/post-1" || state.PublishedHash != "hash-1" {
		t.Fatalf("publication state = %#v", state)
	}
}

func TestPublishResultRequiresStableRemoteIdentity(t *testing.T) {
	path := testPublicationPath(t)
	now := time.Date(2026, 9, 23, 8, 30, 0, 0, time.UTC)
	if err := SavePublicationDraftResult(path, "example", "devto", "hash-1", DraftResult{
		ID: "42", URL: "https://dev.to/dashboard/edit/42", Created: true,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := SavePublicationPublishResult(path, "example", "devto", "hash-1", PublishResult{
		URL: "https://dev.to/thinker/example-42",
	}, now.Add(time.Minute)); err == nil || !strings.Contains(err.Error(), "without remote id and URL") {
		t.Fatalf("missing published id error = %v", err)
	}
	binding, found, err := LoadPublicationBinding(path, "example", "devto")
	if err != nil || !found {
		t.Fatalf("binding = %#v found=%v err=%v", binding, found, err)
	}
	if binding.PublishedURL != "" || binding.RemoteDraftID != "42" {
		t.Fatalf("failed publish write mutated binding: %#v", binding)
	}
}

func TestSuccessfulPublishEndsDraftLifecycle(t *testing.T) {
	path := testPublicationPath(t)
	now := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	if err := SavePublicationDraftResult(path, "example", "devto", "hash-1", DraftResult{
		ID: "42", URL: "https://dev.to/dashboard/edit/42", Created: true,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := SavePublicationPublishResult(path, "example", "devto", "hash-1", PublishResult{
		ID: "42", URL: "https://dev.to/thinker/example-42",
	}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	binding, found, err := LoadPublicationBinding(path, "example", "devto")
	if err != nil || !found {
		t.Fatalf("binding = %#v, found=%v, err=%v", binding, found, err)
	}
	if binding.RemoteDraftID != "" || binding.DraftURL != "" || binding.DraftHash != "" || binding.DraftSyncedAt != "" {
		t.Fatalf("published binding retained stale draft state: %#v", binding)
	}
	if binding.PublishedRemoteID != "42" || binding.PublishedURL != "https://dev.to/thinker/example-42" {
		t.Fatalf("published state = %#v", binding)
	}
}

func TestPublicationPendingFieldsAreDurable(t *testing.T) {
	path := testPublicationPath(t)
	now := time.Date(2026, 9, 22, 11, 0, 0, 0, time.UTC)
	if err := SavePublicationDraftResult(path, "example", "medium", "hash-medium", DraftResult{
		ID: "post-1", URL: "https://medium.com/p/post-1/edit", Created: true,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := SavePublicationPendingFields(path, "example", "medium", []string{"canonical", "tags", "coverImage", "tags", ""}); err != nil {
		t.Fatal(err)
	}

	records, err := ListPublicationRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	if got := strings.Join(records[0].PendingFields, ","); got != "canonical,tags,coverImage" {
		t.Fatalf("pending fields = %q", got)
	}

	remaining, err := ResolvePublicationPendingFields(path, "example", "medium", []string{"tags"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(remaining, ","); got != "canonical,coverImage" {
		t.Fatalf("remaining pending fields = %q", got)
	}
}

func TestBindingsV2LoadsDurablePublicationState(t *testing.T) {
	path := testPublicationPath(t)
	raw := `{
  "version": 2,
  "cnblogs": [],
  "publications": [
    {
      "slug": "example",
      "platform": "juejin",
      "remoteDraftId": "draft-2",
      "draftUrl": "https://juejin.cn/editor/drafts/draft-2",
      "draftHash": "hash-2",
      "publishedUrl": "https://juejin.cn/post/post-2",
      "publishedHash": "hash-2"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	state, source, err := LoadPublicationState(path, "example", "juejin")
	if err != nil {
		t.Fatal(err)
	}
	if source != path {
		t.Fatalf("source = %q", source)
	}
	if state.RemoteDraftID != "draft-2" || state.DraftHash != "hash-2" ||
		state.PublishedURL != "https://juejin.cn/post/post-2" || state.PublishedHash != "hash-2" {
		t.Fatalf("state = %#v", state)
	}
}

func TestPublicationFileWritesOnlyUnifiedSchema(t *testing.T) {
	path := testPublicationPath(t)
	if err := SavePublicationBinding(path, PublicationBinding{
		Slug: "example", Platform: "cnblogs",
		PublishedRemoteID: "42", PublishedURL: "https://www.cnblogs.com/ThinkerQAQ/p/42",
	}); err != nil {
		t.Fatal(err)
	}
	if err := SavePublicationDraftResult(path, "example", "juejin", "hash-j", DraftResult{
		ID: "draft-j", URL: "https://juejin.cn/editor/drafts/draft-j", Created: true,
	}, time.Date(2026, 9, 23, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["version"] != float64(bindingFileVersion) {
		t.Fatalf("version = %#v", decoded["version"])
	}
	if _, exists := decoded["cnblogs"]; exists {
		t.Fatalf("unexpected cnblogs top-level state was written: %s", raw)
	}
	if _, exists := decoded["unbound"]; exists {
		t.Fatalf("unexpected unbound state was written: %s", raw)
	}
	publications, ok := decoded["publications"].([]any)
	if !ok || len(publications) != 2 {
		t.Fatalf("publications = %#v", decoded["publications"])
	}
}

func TestPublicationFileRejectsUnsupportedVersion(t *testing.T) {
	path := testPublicationPath(t)
	if err := os.WriteFile(path, []byte(`{"version":1,"publications":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadPublicationBinding(path, "example", "juejin"); err == nil || !strings.Contains(err.Error(), "unsupported bindings version: 1") {
		t.Fatalf("err = %v, want explicit version rejection", err)
	}
}

func TestPublicationResultWritesRejectDuplicateRemoteIDs(t *testing.T) {
	path := testPublicationPath(t)
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	if err := SavePublicationDraftResult(path, "article-a", "juejin", "hash-a", DraftResult{
		ID: "remote-42", URL: "https://juejin.cn/editor/drafts/remote-42", Created: true,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := SavePublicationDraftResult(path, "article-b", "juejin", "hash-b", DraftResult{
		ID: "remote-42", URL: "https://juejin.cn/editor/drafts/remote-42", Created: true,
	}, now); err == nil || !strings.Contains(err.Error(), "already bound to article-a") {
		t.Fatalf("duplicate draft id error = %v", err)
	}
	if err := SavePublicationDraftResult(path, "article-b", "juejin", "hash-b", DraftResult{
		ID: "draft-b", URL: "https://juejin.cn/editor/drafts/draft-b", Created: true,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := SavePublicationPublishResult(path, "article-b", "juejin", "hash-b", PublishResult{
		ID: "remote-42", URL: "https://juejin.cn/post/remote-42",
	}, now.Add(time.Minute)); err == nil || !strings.Contains(err.Error(), "already bound to article-a") {
		t.Fatalf("duplicate published id error = %v", err)
	}
	binding, found, err := LoadPublicationBinding(path, "article-b", "juejin")
	if err != nil || !found {
		t.Fatalf("binding = %#v found=%v err=%v", binding, found, err)
	}
	if binding.PublishedRemoteID != "" || binding.RemoteDraftID != "draft-b" {
		t.Fatalf("failed duplicate write mutated binding: %#v", binding)
	}
}

func TestConcurrentPublicationWritesDoNotLoseRecords(t *testing.T) {
	path := testPublicationPath(t)
	const count = 32
	var wait sync.WaitGroup
	errs := make(chan error, count)

	for index := 0; index < count; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- SavePublicationDraftResult(
				path,
				fmt.Sprintf("article-%02d", index),
				"juejin",
				fmt.Sprintf("hash-%02d", index),
				DraftResult{
					ID:      fmt.Sprintf("draft-%02d", index),
					URL:     fmt.Sprintf("https://juejin.cn/editor/drafts/draft-%02d", index),
					Created: true,
				},
				time.Date(2026, 9, 23, 8, 0, index, 0, time.UTC),
			)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	records, err := ListPublicationRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != count {
		t.Fatalf("records = %d, want %d", len(records), count)
	}
}

func TestPublicationPathCanLiveOutsideContentRepository(t *testing.T) {
	contentRoot := t.TempDir()
	path := filepath.Join(t.TempDir(), "BlogCTL", "publications.json")
	if strings.HasPrefix(path, contentRoot) {
		t.Fatal("test setup must keep publication state outside the content repository")
	}
	if err := SavePublicationBinding(path, PublicationBinding{
		Slug: "example", Platform: "cnblogs", PublishedRemoteID: "42",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

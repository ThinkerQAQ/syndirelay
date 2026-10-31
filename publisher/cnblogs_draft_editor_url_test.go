package publisher

import (
	"path/filepath"
	"testing"
)

func TestCNBlogsDraftEditorURLAndRemoteCandidate(t *testing.T) {
	const expected = "https://i.cnblogs.com/posts/edit;postId=23247130"
	if got := CNBlogsDraftEditorURL("23247130"); got != expected {
		t.Fatalf("editor URL = %q; expected %q", got, expected)
	}
	draft := cnBlogsPostFromMap(map[string]any{"id": float64(23247130), "url": "https://i.cnblogs.com/posts/preview;postId=23247130", "isPublished": false})
	if draft.URL != expected {
		t.Fatalf("draft preview link was not replaced: %q", draft.URL)
	}
	pub := cnBlogsPostFromMap(map[string]any{"id": float64(42), "url": "https://www.cnblogs.com/ThinkerQAQ/p/42", "isPublished": true})
	if pub.URL != "https://www.cnblogs.com/ThinkerQAQ/p/42" {
		t.Fatalf("published article URL was modified: %q", pub.URL)
	}
}

func TestCNBlogsExistingDraftBindingLinksNormalizeWithoutMigrating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "publications.json")
	err := SavePublicationBinding(path, PublicationBinding{
		Slug: "example", Platform: "cnblogs", RemoteDraftID: "23247130",
		DraftURL:          "https://i.cnblogs.com/articles/edit;postId=23247130",
		PublishedRemoteID: "42", PublishedURL: "https://www.cnblogs.com/ThinkerQAQ/p/42",
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := ListPublicationRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].DraftURL != CNBlogsDraftEditorURL("23247130") ||
		records[0].PublishedURL != "https://www.cnblogs.com/ThinkerQAQ/p/42" {
		t.Fatalf("record links: %+v", records)
	}
	links, err := LoadArticleLinks(path, "example")
	if err != nil {
		t.Fatal(err)
	}
	if links["cnblogs"].DraftURL != CNBlogsDraftEditorURL("23247130") {
		t.Fatalf("article links still open preview: %+v", links["cnblogs"])
	}
	original, exists, err := LoadPublicationBinding(path, "example", "cnblogs")
	if err != nil || !exists || original.DraftURL != "https://i.cnblogs.com/articles/edit;postId=23247130" {
		t.Fatalf("existing saved data must remain unchanged: %+v, %v", original, err)
	}
}

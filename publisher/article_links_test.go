package publisher

import (
	"path/filepath"
	"testing"
)

func TestLoadArticleLinksReadsLocalReferencesOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "publications.json")
	for _, binding := range []PublicationBinding{
		{
			Slug: "example", Platform: "juejin",
			RemoteDraftID: "123", DraftURL: "https://juejin.cn/editor/drafts/123",
		},
		{
			Slug: "example", Platform: "csdn",
			RemoteDraftID: "456", DraftURL: "https://editor.csdn.net/md?articleId=456",
		},
	} {
		if err := SavePublicationBinding(root, binding); err != nil {
			t.Fatal(err)
		}
	}
	links, err := LoadArticleLinks(root, "example")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links["juejin"].RemoteID != "123" || links["csdn"].RemoteID != "456" {
		t.Fatalf("links = %#v", links)
	}
	missing, err := LoadArticleLinks(root, "missing")
	if err != nil || len(missing) != 0 {
		t.Fatalf("missing = %#v, error = %v", missing, err)
	}
}

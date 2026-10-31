package publisher

import (
	"path/filepath"
	"testing"
)

func TestOSChinaDraftEditorURLExactAccountAndDraftID(t *testing.T) {
	const want = "https://my.oschina.net/u/2360403/blog/ai-write/draft/3328466"
	if got := OSChinaDraftEditorURL("2360403", "3328466"); got != want {
		t.Fatalf("editor link: got %q want %q", got, want)
	}
	if got := OSChinaDraftURL("2360403", "3328466", "https://my.oschina.net/shengkunz/blog/write/draft/3328466"); got != want {
		t.Fatalf("legacy preview link retained: %q", got)
	}
	if got := OSChinaDraftURL("", "3328466", want); got != want {
		t.Fatalf("numeric author was not recovered from canonical URL: %q", got)
	}
	if got := OSChinaDraftEditorURL("shengkunz", "3328466"); got != "" {
		t.Fatalf("username must not be used as numeric creator ID: %q", got)
	}
}

func TestOSChinaDraftLinksNormalizeOnReadWithoutChangingPublishedArticles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "publications.json")
	old := "https://my.oschina.net/shengkunz/blog/write/draft/3328466"
	published := "https://my.oschina.net/u/2360403/blog/19763618"
	if err := SavePublicationBinding(path, PublicationBinding{
		Slug: "example", Platform: "oschina", Account: "2360403",
		RemoteDraftID: "3328466", DraftURL: old,
		PublishedRemoteID: "19763618", PublishedURL: published,
	}); err != nil {
		t.Fatal(err)
	}
	records, err := ListPublicationRecords(path)
	if err != nil || len(records) != 1 {
		t.Fatalf("read publication records: %+v %v", records, err)
	}
	want := OSChinaDraftEditorURL("2360403", "3328466")
	if records[0].DraftURL != want || records[0].PublishedURL != published {
		t.Fatalf("incorrect record URLs: %+v", records[0])
	}
	links, err := LoadArticleLinks(path, "example")
	if err != nil || links["oschina"].DraftURL != want || links["oschina"].PublishedURL != published {
		t.Fatalf("incorrect article links: %+v %v", links, err)
	}
	original, found, err := LoadPublicationBinding(path, "example", "oschina")
	if err != nil || !found || original.DraftURL != old {
		t.Fatalf("read-only normalization must preserve original Data: %+v %v", original, err)
	}
}

package publisher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testPublicationPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "publications.json")
}

func TestPublicationStateIsCreatedAndOwnedByGo(t *testing.T) {
	path := testPublicationPath(t)
	state, statePath, err := LoadPublicationState(path, "example", "juejin")
	if err != nil {
		t.Fatal(err)
	}
	if state != (PublicationState{}) {
		t.Fatalf("initial state = %#v", state)
	}
	if statePath != path {
		t.Fatalf("state path = %q, want %q", statePath, path)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state read unexpectedly created durable file: %v", err)
	}

	if err := SavePublicationDraftResult(path, "example", "juejin", "hash-1", DraftResult{
		ID: "draft-1", URL: "https://juejin.cn/editor/drafts/draft-1", Created: true,
	}, testTime()); err != nil {
		t.Fatal(err)
	}
	state, _, err = LoadPublicationState(path, "example", "juejin")
	if err != nil {
		t.Fatal(err)
	}
	if state.RemoteDraftID != "draft-1" || state.DraftHash != "hash-1" {
		t.Fatalf("saved state = %#v", state)
	}
}

func testTime() time.Time {
	return time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
}

func TestListPublicationRecords(t *testing.T) {
	path := testPublicationPath(t)
	if err := SavePublicationDraftResult(path, "example", "juejin", "hash-1", DraftResult{
		ID: "draft-1", URL: "https://juejin.cn/editor/drafts/draft-1", Created: true,
	}, testTime()); err != nil {
		t.Fatal(err)
	}
	if err := SavePublicationPublishResult(path, "example", "juejin", "hash-1", PublishResult{
		ID: "post-1", URL: "https://juejin.cn/post/post-1",
	}, testTime().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	records, err := ListPublicationRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v", records)
	}
	record := records[0]
	if record.Article != "example" || record.Platform != "juejin" || record.RemoteID != "" || record.PublishedRemoteID != "post-1" {
		t.Fatalf("record identity = %#v", record)
	}
	if record.PublishedURL != "https://juejin.cn/post/post-1" || record.UpdatedAt != "2026-09-21T01:00:00Z" {
		t.Fatalf("record publication = %#v", record)
	}
}

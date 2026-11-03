package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func Test51CTOCachedRemoteIDsOnlyFromSuccessfulOwnTaskEvents(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "content", "articles")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "test.md"), []byte("---\ntitle: QA Article\nstatus: published\n---\ntext"), 0600); err != nil {
		t.Fatal(err)
	}
	jobs := []*durableTaskJob{
		{Article: "test", Events: []syncJobEvent{
			{Platform: "51cto", Result: "draft-created", TargetID: "123", URL: "https://blog.51cto.com/blogger/draft/123"},
			{Platform: "51cto", Result: "draft-updated", TargetID: "123"},
			{Platform: "51cto", Result: "target-failed", TargetID: "222"},
			{Platform: "devto", Result: "draft-created", TargetID: "333"},
			{Platform: "51cto", Result: "draft-created", TargetID: "999", URL: "https://evil.invalid/999"},
		}},
		{Article: "../secrets", Events: []syncJobEvent{{Platform: "51cto", Result: "draft-created", TargetID: "666"}}},
		{Article: "test", DryRun: true, Events: []syncJobEvent{{Platform: "51cto", Result: "draft-created", TargetID: "777"}}},
	}
	got := cached51CTODraftPosts(jobs, root)
	if len(got) != 1 || got[0].ID != "123" || got[0].Title != "QA Article" ||
		got[0].Published || got[0].URL != "https://blog.51cto.com/blogger/draft/123" {
		t.Fatalf("unsafe cached remote drafts: %+v", got)
	}
}

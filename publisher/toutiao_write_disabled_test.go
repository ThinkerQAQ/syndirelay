package publisher

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
)

// Public publishing a new article is not enabled by restoring draft saves.
// Updates of published articles must have a verified binding before network IO.
func TestToutiaoUnsafeWritesStillFailBeforeHTTP(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		t.Fatalf("unsafe operation made a remote HTTP request: %s", r.URL)
		return nil, nil
	})}
	service := Service{HTTPClient: client, PublicationPath: filepath.Join(t.TempDir(), "publications.json")}
	session := toutiaoTestSession()
	input := DraftInput{Slug: "concurrency-series-06-atomic-implementation", Title: "test", Markdown: "test", ContentHash: "sha"}
	_, publishErr := service.PublishDraftInput(context.Background(), "toutiao", session, "", input)
	_, _, updateErr := service.UpdateToutiaoPublishedInput(context.Background(), session, "", input)
	if !IsKind(publishErr, ErrValidation) || !IsKind(updateErr, ErrValidation) || calls != 0 {
		t.Fatalf("unsafe operations should fail without HTTP: publish=%v update=%v requests=%d", publishErr, updateErr, calls)
	}
}

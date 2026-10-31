package bridge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

func TestPublicationPendingResolveUpdatesOnlyDurableState(t *testing.T) {
	root := t.TempDir()
	publicationPath := filepath.Join(t.TempDir(), "publications.json")
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	if err := publisher.SavePublicationDraftResult(publicationPath, "example", "medium", "hash", publisher.DraftResult{
		ID: "post-1", URL: "https://medium.com/p/post-1/edit", Created: true,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := publisher.SavePublicationPendingFields(publicationPath, "example", "medium", []string{"canonical", "tags"}); err != nil {
		t.Fatal(err)
	}

	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	server.config.ContentRoot = root
	server.config.PublicationBindingsPath = publicationPath

	body, _ := json.Marshal(map[string]any{"fields": []string{"tags"}})
	request := httptest.NewRequest(http.MethodPost, "/v1/publications/pending/resolve?article=example&platform=medium", bytes.NewReader(body))
	setExtensionAuth(request, "token")
	request.Header.Set("content-type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var decoded struct {
		PendingFields []string `json:"pendingFields"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.PendingFields) != 1 || decoded.PendingFields[0] != "canonical" {
		t.Fatalf("pending = %#v", decoded.PendingFields)
	}

	records, err := publisher.ListPublicationRecords(publicationPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(records[0].PendingFields) != 1 || records[0].PendingFields[0] != "canonical" {
		t.Fatalf("records = %#v", records)
	}
	if records[0].RemoteID != "post-1" || records[0].DraftURL != "https://medium.com/p/post-1/edit" {
		t.Fatalf("publication identity changed: %#v", records[0])
	}
}

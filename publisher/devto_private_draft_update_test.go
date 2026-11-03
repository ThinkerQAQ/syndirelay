package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDEVToPrivateDraftUsesAuthenticatedOwnerListWhenPublicReadIs404(t *testing.T) {
	var writes, reads, ownLists int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/articles/4963204":
			reads++
			http.Error(w, "not found", http.StatusNotFound)
		case r.Method == "GET" && r.URL.Path == "/api/articles/me/all":
			ownLists++
			// DEV.to private draft list metadata may not include body_markdown.
			fmt.Fprint(w, `[{"id":4963204,"title":"Private draft","url":"https://dev.to/thinker/hidden","published":false}]`)
		case r.Method == "PUT" && r.URL.Path == "/api/articles/4963204":
			writes++
			var payload struct {
				Article devtoPayload `json:"article"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload.Article.BodyMarkdown != "UPDATED BODY\n" || payload.Article.Published {
				t.Fatalf("wrong draft payload %#v", payload.Article)
			}
			fmt.Fprint(w, `{"id":4963204,"url":"https://dev.to/thinker/hidden"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	a, err := newDEVToAdapter(server.Client(), Session{APIKey: "test-key"}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.UpdateDraft(context.Background(), DraftRef{ID: "4963204"}, DraftInput{Title: "Private draft", Markdown: "UPDATED BODY"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.ID != "4963204" || reads != 1 || ownLists != 1 || writes != 1 {
		t.Fatalf("result %+v; reads %d lists %d writes %d", result, reads, ownLists, writes)
	}
}

func TestDEVToPrivateDraft404MustNotUpdateUnknownDraft(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/articles/989":
			http.NotFound(w, r)
		case r.Method == "GET" && r.URL.Path == "/api/articles/me/all":
			fmt.Fprint(w, `[{"id":988,"title":"someone else's draft"}]`)
		default:
			writes++
			w.WriteHeader(http.StatusConflict)
		}
	}))
	defer server.Close()
	a, e := newDEVToAdapter(server.Client(), Session{APIKey: "test"}, server.URL)
	if e != nil {
		t.Fatal(e)
	}
	_, e = a.UpdateDraft(context.Background(), DraftRef{ID: "989"}, DraftInput{Title: "target", Markdown: "UPDATE"})
	if e == nil || !strings.Contains(e.Error(), "404") {
		t.Fatalf("missing failure: %v", e)
	}
	if writes != 0 {
		t.Fatal("unexpected remote mutation")
	}
}

package buffer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateDraftNeverPublishes(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Error("authorization missing")
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"createPost":{"__typename":"PostActionSuccess","post":{"id":"one","text":"hello","status":"draft","channelId":"ch"}}}}`))
	}))
	defer server.Close()
	client := Client{APIKey: "test", Endpoint: server.URL}
	got, err := client.CreateDraft(context.Background(), PostPayload{ChannelID: "ch", Text: "hello", ImageURL: "https://example.com/cover.png"})
	if err != nil || got.Status != "draft" {
		t.Fatalf("post=%+v err=%v", got, err)
	}
	in := request["variables"].(map[string]any)["input"].(map[string]any)
	if in["saveToDraft"] != true || in["mode"] != "addToQueue" {
		t.Fatal("unsafe draft mode")
	}
	if len(in["assets"].([]any)) != 1 {
		t.Fatal("image missing")
	}
}
func TestGraphErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"unauthorized"}]}`))
	}))
	defer server.Close()
	c := Client{APIKey: "test", Endpoint: server.URL}
	_, err := c.Post(context.Background(), "post")
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("error=%v", err)
	}
}
func TestTypedMutationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"createPost":{"__typename":"MutationError","message":"Not allowed"}}}`))
	}))
	defer server.Close()
	_, err := (Client{APIKey: "test", Endpoint: server.URL}).CreateDraft(context.Background(), PostPayload{ChannelID: "one", Text: "hello"})
	if err == nil || !strings.Contains(err.Error(), "Not allowed") {
		t.Fatalf("error=%v", err)
	}
}

func TestPublishExistingDraftDoesNotCreateAnotherPost(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"editPost":{"__typename":"PostActionSuccess","post":{"id":"existing","text":"hello","status":"sending","channelId":"linkedin-channel"}}}}`))
	}))
	defer server.Close()
	client := Client{APIKey: "test", Endpoint: server.URL}
	post, err := client.PublishDraft(context.Background(), "existing", "hello")
	if err != nil || post.Status != "sending" {
		t.Fatalf("post=%+v error=%v", post, err)
	}
	if !strings.Contains(request["query"].(string), "editPost(input:") {
		t.Fatal("publication must edit the same remote draft rather than create a new one")
	}
	input := request["variables"].(map[string]any)["input"].(map[string]any)
	if input["id"] != "existing" || input["text"] != "hello" || input["mode"] != "shareNow" || input["saveToDraft"] != false {
		t.Fatalf("unexpected publishing input: %+v", input)
	}
}

func TestPublishDraftRefusesEmptyContentBeforeHTTPRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	client := Client{APIKey: "test", Endpoint: server.URL}
	_, err := client.PublishDraft(context.Background(), "existing", " ")
	if err == nil || calls != 0 {
		t.Fatalf("unexpected publication attempt: err=%v calls=%d", err, calls)
	}
}

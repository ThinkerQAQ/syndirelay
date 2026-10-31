package bridge

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

type cnBlogsBindingTransport func(*http.Request) (*http.Response, error)

func (f cnBlogsBindingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func bindingJSON(request *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(body)), Request: request}
}

func TestCNBlogsBindingSearchAndManualVerification(t *testing.T) {
	root := t.TempDir()
	articles := filepath.Join(root, "src", "content", "articles")
	if err := os.MkdirAll(articles, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articles, "example.md"), []byte("---\ntitle: 并发编程\nstatus: published\n---\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	server.config.ContentRoot = root
	publicationPath := filepath.Join(t.TempDir(), "publications.json")
	server.config.PublicationBindingsPath = publicationPath
	server.sessions["cnblogs"] = platformSession{RequestCookieHeader: "login=test-secret", UserAgent: "test", ExpiresAt: time.Now().Add(time.Minute)}
	remoteUpdatedAt := "baseline"
	server.httpClient = &http.Client{Transport: cnBlogsBindingTransport(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			t.Fatal("lookup wrote to CNBlogs")
		}
		switch request.URL.Path {
		case "/api/user":
			return bindingJSON(request, `{"loginName":"ThinkerQAQ"}`), nil
		case "/api/posts/list":
			if request.URL.Query().Get("search") != "并发编程" {
				t.Fatal("wrong lookup query")
			}
			return bindingJSON(request, `{"postList":[{"id":42,"title":"并发编程","url":"https://www.cnblogs.com/ThinkerQAQ/p/42","isPublished":true,"dateUpdated":"baseline"}],"postsCount":1}`), nil
		case "/api/posts/42":
			return bindingJSON(request, `{"blogPost":{"id":42,"title":"并发编程","url":"https://www.cnblogs.com/ThinkerQAQ/p/42","isPublished":true,"author":"ThinkerQAQ","dateUpdated":"`+remoteUpdatedAt+`"}}`), nil
		default:
			t.Fatalf("unexpected endpoint %s", request.URL.Path)
			return nil, nil
		}
	})}
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()
	post := func(path string, body []byte) map[string]any {
		request, _ := http.NewRequest(http.MethodPost, handler.URL+path, bytes.NewReader(body))
		setExtensionAuth(request, "token")
		response, err := handler.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			raw, _ := io.ReadAll(response.Body)
			t.Fatalf("status %d: %s", response.StatusCode, raw)
		}
		var decoded map[string]any
		if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	search := post("/v1/cnblogs/binding/search?article=example", nil)
	if len(search["candidates"].([]any)) != 1 {
		t.Fatalf("search = %#v", search)
	}
	bound := post("/v1/cnblogs/binding?article=example", []byte(`{"reference":"https://www.cnblogs.com/ThinkerQAQ/p/42"}`))
	binding := bound["binding"].(map[string]any)
	if binding["postId"] != "42" || binding["state"] != "published" || binding["remoteUpdatedAt"] != "baseline" {
		t.Fatalf("binding = %#v", binding)
	}
	if _, err := os.Stat(publicationPath); err != nil {
		t.Fatal(err)
	}
	verified := post("/v1/cnblogs/binding/verify?article=example", nil)
	if verified["found"] != true || verified["post"].(map[string]any)["id"] != "42" {
		t.Fatalf("verified = %#v", verified)
	}
	if err := publisher.SavePublicationBinding(publicationPath, publisher.PublicationBinding{Slug: "example", Platform: "cnblogs", Account: "ThinkerQAQ", PublishedRemoteID: "42", PublishedURL: "https://www.cnblogs.com/ThinkerQAQ/p/42", RemoteUpdatedAt: "baseline", PublishedHash: "old-hash"}); err != nil {
		t.Fatal(err)
	}
	remoteUpdatedAt = "changed-remotely"
	bound = post("/v1/cnblogs/binding?article=example", []byte(`{"reference":"42","replace":true}`))
	binding = bound["binding"].(map[string]any)
	if binding["remoteUpdatedAt"] != "changed-remotely" || binding["lastPushedHash"] != nil {
		t.Fatalf("reverified binding retained stale local hash: %#v", binding)
	}
}

func TestCNBlogsBindingDeleteOnlyRemovesSelectedLocalSlot(t *testing.T) {
	root := t.TempDir()
	articles := filepath.Join(root, "src", "content", "articles")
	if err := os.MkdirAll(articles, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articles, "example.md"), []byte("---\ntitle: Example\nstatus: published\n---\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	publicationPath := filepath.Join(t.TempDir(), "publications.json")
	if err := publisher.SavePublicationBinding(publicationPath, publisher.PublicationBinding{
		Slug: "example", Platform: "cnblogs",
		RemoteDraftID: "52", DraftURL: "https://i.cnblogs.com/articles/edit;postId=52",
		PublishedRemoteID: "42", PublishedURL: "https://www.cnblogs.com/ThinkerQAQ/p/42",
	}); err != nil {
		t.Fatal(err)
	}
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	server.config.ContentRoot = root
	server.config.PublicationBindingsPath = publicationPath
	request := httptest.NewRequest(http.MethodDelete, "/v1/cnblogs/binding?article=example", bytes.NewBufferString(`{"state":"draft","postId":"52"}`))
	setExtensionAuth(request, "token")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	binding, found, err := publisher.LoadPublicationBinding(publicationPath, "example", "cnblogs")
	if err != nil || !found || binding.RemoteDraftID != "" || binding.PublishedRemoteID != "42" {
		t.Fatalf("publication = %#v, %v, %v", binding, found, err)
	}
}

package publisher

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func osChinaSession() Session {
	return Session{
		UserAgent: "BlogCTL-Test-UA",
		Cookies: []BrowserCookie{{
			Name: "oscid", Value: "secret", Domain: ".oschina.net", Path: "/", Secure: true,
		}},
	}
}

func TestOSChinaUpdateDraftUsesCapturedDraftField(t *testing.T) {
	var payload map[string]any
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/oschinapi/user/myDetails":
			return jsonResponse(request, 200, `{"success":true,"result":{"userId":2360403,"userVo":{"name":"ThinkerQAQ"}}}`, nil), nil
		case "/oschinapi/api/draft/save_draft":
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `{"success":true,"code":200,"result":{"id":3322149}}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewOSChinaAdapter(client, osChinaSession())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.UpdateDraft(context.Background(), DraftRef{ID: "3322149"}, DraftInput{
		Title: "test", Markdown: "body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.ID != "3322149" {
		t.Fatalf("result = %#v", result)
	}
	if valueString(payload["draft"]) != "3322149" {
		t.Fatalf("payload = %#v", payload)
	}
	if _, exists := payload["id"]; exists {
		t.Fatalf("payload unexpectedly contains id: %#v", payload)
	}
	if _, exists := payload["draftId"]; exists {
		t.Fatalf("payload unexpectedly contains draftId: %#v", payload)
	}
}

func TestOSChinaListPostsUsesDraftAndPublishedLists(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/oschinapi/user/myDetails":
			return jsonResponse(request, 200, `{"success":true,"result":{"userId":2360403,"userVo":{"name":"ThinkerQAQ"}}}`, nil), nil
		case "/oschinapi/api/draft/list":
			return jsonResponse(request, 200, `{
				"success":true,
				"result":{"records":[
					{"id":3322149,"title":"并发编程（四）：互斥锁的实现 · ThinkerQAQ"}
				],"current":1,"pages":1}
			}`, nil), nil
		case "/oschinapi/blog/my/web":
			return jsonResponse(request, 200, `{
				"success":true,
				"result":{"records":[
					{"id":19763618,"title":"并发编程（三）：互斥锁——语言层的原子性、可见性与有序性 · ThinkerQAQ"}
				],"current":1,"pages":1}
			}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	account, posts, err := OSChinaListPosts(context.Background(), client, osChinaSession())
	if err != nil {
		t.Fatal(err)
	}
	if account != "2360403" {
		t.Fatalf("account = %q", account)
	}
	if len(posts) != 2 {
		t.Fatalf("posts = %#v", posts)
	}
	if posts[0].Published || !strings.Contains(posts[0].URL, "/blog/ai-write/draft/3322149") {
		t.Fatalf("draft = %#v", posts[0])
	}
	if !posts[1].Published || !strings.HasSuffix(posts[1].URL, "/blog/19763618") {
		t.Fatalf("published = %#v", posts[1])
	}
}

func TestOSChinaTitleMatchesPlatformSuffix(t *testing.T) {
	if !OSChinaTitleMatches(
		"并发编程（四）：互斥锁的实现",
		"并发编程（四）：互斥锁的实现 · ThinkerQAQ",
	) {
		t.Fatal("expected suffix title to match")
	}
	if OSChinaTitleMatches("并发编程（四）", "并发编程（三）") {
		t.Fatal("unexpected title match")
	}
}

func TestOSChinaPublishRehostsCompilerAssets(t *testing.T) {
	root := t.TempDir()
	assetDir := filepath.Join(root, "assets", "mermaid")
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "asset-1.png"), []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}

	publishedContent := ""
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/oschinapi/user/myDetails":
			return jsonResponse(request, 200, `{"success":true,"result":{"userId":2360403,"userVo":{"name":"ThinkerQAQ"}}}`, nil), nil
		case "/oschinapi/ai/creation/project/uploadDetail":
			return jsonResponse(request, 200, `{"success":true,"result":"https://oscimg.oschina.net/diagram.png"}`, nil), nil
		case "/oschinapi/blog_catalog/list_by_user":
			return jsonResponse(request, 200, `{"success":true,"result":[{"id":1,"blogCount":1}]}`, nil), nil
		case "/oschinapi/blog/web/add":
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			publishedContent = valueString(payload["content"])
			return jsonResponse(request, 200, `{"success":true,"code":200,"result":19763618}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewOSChinaAdapter(client, osChinaSession())
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.PublishDraft(context.Background(), DraftRef{ID: "3322149"}, DraftInput{
		Title:            "Example",
		Markdown:         "![diagram](blogctl-asset://mermaid/asset-1)",
		DistributionRoot: root,
		Assets: []PublishingAsset{{
			Kind: "mermaid", ID: "asset-1",
			Source: "blogctl-asset://mermaid/asset-1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(publishedContent, "blogctl-asset://") {
		t.Fatalf("internal asset leaked into publish payload: %q", publishedContent)
	}
	if !strings.Contains(publishedContent, "https://oscimg.oschina.net/diagram.png") {
		t.Fatalf("publish payload did not use hosted image: %q", publishedContent)
	}
}

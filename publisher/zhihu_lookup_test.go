package publisher

import (
	"context"
	"net/http"
	"testing"
)

func TestZhihuListPostsUsesDraftAndPublishedLists(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Path == "/api/v4/me":
			if request.URL.Query().Get("include") == "url_token,name,id" {
				return jsonResponse(request, 200, `{"id":"member-id","url_token":"thinkQAQ","name":"ThinkerQAQ"}`, nil), nil
			}
			return jsonResponse(request, 200, `{"id":"member-id","name":"ThinkerQAQ"}`, nil), nil
		case request.URL.Path == "/api/v4/articles/my_drafts":
			if request.Header.Get("x-zse-93") != zhihuZSE93 || request.Header.Get("x-zse-96") == "" {
				t.Fatalf("draft-list request is missing Zhihu signature headers: %#v", request.Header)
			}
			return jsonResponse(request, 200, `{
				"paging":{"is_end":true},
				"data":[
					{"url_token":"2085475357956875797","title":"并发编程（四）：互斥锁的实现——从 Runtime 到 CPU · ThinkerQAQ"},
					{"url_token":"2084743256093488777","title":"并发编程（三）：互斥锁——语言层的原子性、可见性与有序性 · ThinkerQAQ"}
				]
			}`, nil), nil
		case request.URL.Path == "/api/v4/members/thinkQAQ/articles":
			if request.Header.Get("x-zse-93") != zhihuZSE93 || request.Header.Get("x-zse-96") == "" {
				t.Fatalf("published-list request is missing Zhihu signature headers: %#v", request.Header)
			}
			return jsonResponse(request, 200, `{
				"paging":{"is_end":true},
				"data":[
					{"id":"2084737186444976575","title":"并发编程（三）：互斥锁——语言层的原子性、可见性与有序性 · ThinkerQAQ","url":"http://zhuanlan.zhihu.com/p/2084737186444976575"}
				]
			}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	account, posts, err := ZhihuListPosts(context.Background(), client, Session{
		UserAgent: "BlogCTL-Test-UA",
		Cookies: []BrowserCookie{
			{Name: "z_c0", Value: "secret", Domain: ".zhihu.com", Path: "/", Secure: true},
			{Name: "d_c0", Value: "test-dc0", Domain: ".zhihu.com", Path: "/", Secure: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if account != "thinkQAQ" {
		t.Fatalf("account = %q", account)
	}
	if len(posts) != 3 {
		t.Fatalf("posts = %#v", posts)
	}
	if posts[0].Published || posts[0].ID != "2085475357956875797" {
		t.Fatalf("draft = %#v", posts[0])
	}
	if !posts[2].Published || posts[2].URL != "https://zhuanlan.zhihu.com/p/2084737186444976575" {
		t.Fatalf("published = %#v", posts[2])
	}
}

func TestZhihuTitleMatchesPlatformSuffix(t *testing.T) {
	if !ZhihuTitleMatches(
		"并发编程（四）：互斥锁的实现——从 Runtime 到 CPU",
		"并发编程（四）：互斥锁的实现——从 Runtime 到 CPU · ThinkerQAQ",
	) {
		t.Fatal("expected suffix title to match")
	}
	if ZhihuTitleMatches("并发编程（四）", "并发编程（三）") {
		t.Fatal("unexpected title match")
	}
}

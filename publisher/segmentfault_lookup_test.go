package publisher

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSegmentFaultListPostsUsesListPagesAndLocalTitles(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Path == "/user/settings":
			return jsonResponse(request, 200, `<a href="/u/thinkerqaq">ThinkerQAQ</a>`, nil), nil
		case request.URL.Path == "/user/draft" && request.URL.Query().Get("page") == "":
			return jsonResponse(request, 200, `
				<a href="/write?draftId=1220000048305659">并发编程（四）：互斥锁的实现</a>
				<a href="/write?draftId=1220000048305000"><span>Other draft</span></a>
				<a href="/user/draft?page=2">2</a>
			`, nil), nil
		case request.URL.Path == "/user/draft" && request.URL.Query().Get("page") == "2":
			return jsonResponse(request, 200, `
				<a href="https://segmentfault.com/write?freshman=1&amp;draftId=1220000048304000">Older draft</a>
			`, nil), nil
		case request.URL.Path == "/u/thinkerqaq/articles":
			return jsonResponse(request, 200, `
				<a href="/a/1190000048305689">并发编程（四）：互斥锁的实现 · ThinkerQAQ</a>
				<a href="/a/1190000048304331"><strong>并发编程（三）：语言层互斥锁</strong></a>
				<a href="/a/1190000048305689/revision">not an article-list link</a>
			`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	account, posts, err := SegmentFaultListPosts(context.Background(), client, segmentFaultSession())
	if err != nil {
		t.Fatal(err)
	}
	if account != "thinkerqaq" {
		t.Fatalf("account = %q", account)
	}
	if len(posts) != 5 {
		t.Fatalf("posts = %#v", posts)
	}
	if posts[0].ID != "1220000048305659" || posts[0].Published {
		t.Fatalf("first draft = %#v", posts[0])
	}
	if posts[3].ID != "1190000048305689" || !posts[3].Published {
		t.Fatalf("first published = %#v", posts[3])
	}
}

func TestSegmentFaultTitleMatchesPlatformSuffix(t *testing.T) {
	if !SegmentFaultTitleMatches(
		"并发编程（四）：互斥锁的实现",
		"并发编程（四）：互斥锁的实现 · ThinkerQAQ",
	) {
		t.Fatal("expected suffix title to match")
	}
	if SegmentFaultTitleMatches("并发编程（四）", "并发编程（三）") {
		t.Fatal("unexpected title match")
	}
}

func TestParseSegmentFaultPostsDeduplicatesListLinks(t *testing.T) {
	raw := []byte(`
		<a href="/a/1190000048305689">Title &amp; More</a>
		<a href="https://segmentfault.com/a/1190000048305689#comments">Title &amp; More</a>
	`)
	posts := parseSegmentFaultPosts(raw, true)
	if len(posts) != 1 {
		t.Fatalf("posts = %#v", posts)
	}
	if posts[0].Title != "Title & More" || !strings.HasSuffix(posts[0].URL, "/a/1190000048305689") {
		t.Fatalf("post = %#v", posts[0])
	}
}

package publisher

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func toutiaoTestSession() Session {
	return Session{UserAgent: "BlogCTL-Test", Cookies: []BrowserCookie{
		{Name: "sessionid", Value: "private", Domain: ".toutiao.com", Path: "/", Secure: true},
	}}
}

func TestToutiaoListPostsReadsCreatorDraftsAndPagedPublishedArticles(t *testing.T) {
	publishedPages := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/mp/agw/media/get_media_info":
			return jsonResponse(request, 200, `{"data":{"user":{"id":"creator-1","screen_name":"example"}}}`, nil), nil
		case "/mp/agw/creator_center/draft_list":
			query := request.URL.Query()
			if query.Get("type") != "0" || query.Get("app_id") != "1231" {
				t.Fatalf("draft request params: %v", query)
			}
			return jsonResponse(request, 200, `{"code":0,"draft_list":[{"gid":"11","title":"并发教程（一）","draft_type":2},{"gid":"0","title":""}]}`, nil), nil
		case "/mp/agw/creator_center/list/v2":
			publishedPages++
			query := request.URL.Query()
			if query.Get("status") != "2" || query.Get("type") != "0" || query.Get("app_id") != "1231" {
				t.Fatalf("published request params: %v", query)
			}
			if publishedPages == 1 {
				if query.Has("cursor") {
					t.Fatal("first page unexpectedly has cursor")
				}
				return jsonResponse(request, 200, `{"code":0,"has_more":true,"end_cursor":123,"contents":[{"article_attr":{"gid":"22","title":"并发教程（二）","status":2}},{"article_attr":{"gid":"12","title":"审核中","status":1}}]}`, nil), nil
			}
			if query.Get("cursor") != "123" {
				t.Fatalf("second page cursor: %v", query)
			}
			return jsonResponse(request, 200, `{"code":0,"has_more":false,"end_cursor":456,"contents":[{"article_attr":{"gid":"33","title":"并发教程（三）","status":2}},{"article_attr":{"gid":"22","title":"重复文章","status":2}}]}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL)
			return nil, nil
		}
	})}
	account, posts, err := ToutiaoListPosts(context.Background(), client, toutiaoTestSession())
	if err != nil {
		t.Fatal(err)
	}
	if account != "creator-1" || len(posts) != 3 || publishedPages != 2 {
		t.Fatalf("account=%s posts=%+v pages=%d", account, posts, publishedPages)
	}
	if posts[0].Published || posts[0].ID != "11" || !strings.Contains(posts[0].URL, "pgc_id=11") {
		t.Fatalf("draft: %+v", posts[0])
	}
	if !posts[1].Published || !posts[2].Published || posts[2].URL != "https://www.toutiao.com/article/33/" {
		t.Fatalf("published: %+v", posts)
	}
}

func TestToutiaoListPostsRejectsStalledCursor(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/mp/agw/media/get_media_info":
			return jsonResponse(request, 200, `{"data":{"user":{"id":"1"}}}`, nil), nil
		case "/mp/agw/creator_center/draft_list":
			return jsonResponse(request, 200, `{"code":0,"draft_list":[]}`, nil), nil
		case "/mp/agw/creator_center/list/v2":
			return jsonResponse(request, 200, `{"code":0,"has_more":true,"end_cursor":0,"contents":[{"article_attr":{"gid":"33","title":"文章","status":2}}]}`, nil), nil
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
			return nil, nil
		}
	})}
	_, _, err := ToutiaoListPosts(context.Background(), client, toutiaoTestSession())
	if err == nil || !strings.Contains(err.Error(), "pagination did not advance") {
		t.Fatalf("expected pagination error, got %v", err)
	}
}

func TestToutiaoTitleMatchesTruncatedTitle(t *testing.T) {
	title := "这是一篇非常长的并发编程文章标题，用来验证头条三十个汉字上限的文章关联功能是否正常"
	if !ToutiaoTitleMatches(title, truncateToutiaoTitle(title)) {
		t.Fatal("truncated title must match article created by the publisher")
	}
	if ToutiaoTitleMatches("完全不同的标题", "随机的其他文章") {
		t.Fatal("unrelated title must not match")
	}
}

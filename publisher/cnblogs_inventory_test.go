package publisher

import (
	"context"
	"net/http"
	"testing"
)

func TestCNBlogsInventoryDeduplicatesOverlappingPageZeroAndOne(t *testing.T) {
	pages := []string{}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/user":
			return jsonResponse(req, http.StatusOK, `{"loginName":"ThinkerQAQ"}`, nil), nil
		case "/api/posts/list":
			page := req.URL.Query().Get("p")
			pages = append(pages, page)
			switch page {
			case "0", "1":
				return jsonResponse(req, http.StatusOK,
					`{"postList":[{"id":42,"title":"Alpha"},{"id":43,"title":"Beta"}],"postsCount":3}`, nil), nil
			case "2":
				return jsonResponse(req, http.StatusOK,
					`{"postList":[{"id":44,"title":"Gamma"}],"postsCount":3}`, nil), nil
			}
		}
		t.Errorf("unexpected request: %s", req.URL.String())
		return jsonResponse(req, http.StatusNotFound, `{}`, nil), nil
	})}
	_, posts, err := CNBlogsSearchPosts(context.Background(), client, cnBlogsSession(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 3 || posts[0].ID != "42" || posts[1].ID != "43" || posts[2].ID != "44" {
		t.Fatalf("duplicate or missing CNBlogs IDs: %#v", posts)
	}
	if len(pages) != 3 || pages[0] != "0" || pages[1] != "1" || pages[2] != "2" {
		t.Fatalf("overlap should not prematurely terminate pagination: %v", pages)
	}
}

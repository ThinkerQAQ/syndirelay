package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestJuejinListAllPostsIncludesUnpublishedDraftsFromHARSchema(t *testing.T) {
	const userID = "user-123"
	var draftPages []int
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/user_api/v1/user/get":
			return jsonResponse(req, 200, `{"data":{"user_id":"user-123","user_name":"tester"}}`, nil), nil
		case "/user_api/v1/sys/token":
			return jsonResponse(req, 200, "", map[string]string{"x-ware-csrf-token": "0,csrf-draft-list,1,success,x"}), nil
		case "/content_api/v1/article/query_list":
			return jsonResponse(req, 200,
				`{"err_no":0,"data":[{"article_id":"published-42","article_info":{"article_id":"published-42","draft_id":"draft-previous","title":"已发布文章"}}],"has_more":false}`, nil), nil
		case "/content_api/v1/article_draft/list_by_user":
			if req.Method != "POST" || req.Header.Get("x-secsdk-csrf-token") != "csrf-draft-list" {
				t.Fatalf("unexpected draft auth or method: %s, CSRF present=%v",
					req.Method, req.Header.Get("x-secsdk-csrf-token") != "")
			}
			if req.URL.Query().Get("aid") != "2608" || req.URL.Query().Get("spider") != "0" {
				t.Fatalf("unexpected query fields")
			}
			var request struct {
				Keyword  string `json:"keyword"`
				PageSize int    `json:"page_size"`
				PageNo   int    `json:"page_no"`
			}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Keyword != "" || request.PageSize != 10 {
				t.Fatalf("draft request=%+v", request)
			}
			draftPages = append(draftPages, request.PageNo)
			if request.PageNo == 1 {
				// Browser HAR contains article_id even on an unsubmitted status=0
				// draft: the editor must use 'id', not 'article_id'.
				batch := ""
				for i := 0; i < 10; i++ {
					if i > 0 {
						batch += ","
					}
					batch += fmt.Sprintf(`{"id":"draft-%02d","article_id":"stale-post-%02d","user_id":"user-123","status":0,"title":"测试草稿 %d"}`, i, i, i)
				}
				return jsonResponse(req, 200, `{"err_no":0,"count":11,"data":[`+batch+`]}`, nil), nil
			}
			if request.PageNo == 2 {
				return jsonResponse(req, 200, `{"err_no":0,"count":11,"data":[{"id":"draft-10","article_id":"post-10","user_id":"user-123","status":0,"title":"未发布文章"}]}`, nil), nil
			}
			t.Fatalf("unnecessary draft page %d", request.PageNo)
			return nil, nil
		default:
			t.Fatalf("unexpected request: %s", req.URL.Path)
			return nil, nil
		}
	})}
	account, posts, truncated, err := JuejinListAllPosts(context.Background(), client, juejinSession())
	if err != nil {
		t.Fatal(err)
	}
	if account != userID || truncated || len(posts) != 12 {
		t.Fatalf("account=%q truncated=%v count=%d", account, truncated, len(posts))
	}
	if len(draftPages) != 2 || draftPages[0] != 1 || draftPages[1] != 2 {
		t.Fatalf("draft pages=%v", draftPages)
	}
	if posts[0].ID != "draft-00" || posts[0].Published || posts[0].DraftID != "draft-00" ||
		posts[0].URL != "https://juejin.cn/editor/drafts/draft-00" {
		t.Fatalf("draft identity was confused with article_id: %+v", posts[0])
	}
	if posts[11].ID != "published-42" || !posts[11].Published {
		t.Fatalf("published item missing: %+v", posts[11])
	}
}

func TestJuejinDraftListRejectsWrongAccountAndAPIError(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"wrong-account", `{"err_no":0,"count":1,"data":[{"id":"foreign-draft","user_id":"someone-else","title":"Other","status":0}]}`},
		{"upstream-error", `{"err_no":7001,"err_msg":"draft list unavailable","data":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/user_api/v1/user/get":
					return jsonResponse(req, 200, `{"data":{"user_id":"user-123"}}`, nil), nil
				case "/user_api/v1/sys/token":
					return jsonResponse(req, 200, "", map[string]string{"x-ware-csrf-token": "0,csrf-draft-list,1,success,x"}), nil
				case "/content_api/v1/article/query_list":
					return jsonResponse(req, 200, `{"err_no":0,"data":[],"has_more":false}`, nil), nil
				case "/content_api/v1/article_draft/list_by_user":
					return jsonResponse(req, 200, tc.body, nil), nil
				default:
					t.Fatalf("unexpected path %s", req.URL.Path)
					return nil, nil
				}
			})}
			_, _, _, err := JuejinListAllPosts(context.Background(), client, juejinSession())
			if err == nil {
				t.Fatal("unsafe/unavailable draft list was accepted")
			}
		})
	}
}

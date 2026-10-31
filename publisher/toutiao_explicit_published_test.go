package publisher

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestToutiaoExplicitPublishedUpdateChecksVersionAndPreservesIdentity(t *testing.T) {
	for _, version := range []string{"", "122", "123"} {
		t.Run("version_"+version, func(t *testing.T) {
			writes := 0
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/mp/agw/media/get_media_info":
					return jsonResponse(req, 200, `{"data":{"user":{"id":"owner"}}}`, nil), nil
				case "/mp/agw/creator_center/list/v2":
					return jsonResponse(req, 200, `{"code":0,"has_more":false,"contents":[{"article_attr":{"gid":"42","title":"Article","status":2,"modify_time":123}}]}`, nil), nil
				case "/mp/agw/article/publish":
					writes++
					if err := req.ParseForm(); err != nil {
						t.Fatal(err)
					}
					if req.PostForm.Get("pgc_id") != "42" || req.PostForm.Get("save") != "1" {
						t.Fatal("update did not retain the published identity")
					}
					html := req.PostForm.Get("content")
					if strings.Contains(html, `href="#`) || !strings.Contains(html, `href="https://go.dev/ref/mem#tmp_1"`) {
						t.Fatalf("invalid reference adaptation: %s", html)
					}
					return jsonResponse(req, 200, `{"code":0,"err_no":0,"data":{"pgc_id":"42"}}`, nil), nil
				default:
					t.Fatalf("unexpected endpoint %s", req.URL)
					return nil, nil
				}
			})}
			got, err := (Service{HTTPClient: client}).RunExplicitDraft(context.Background(), "toutiao", toutiaoTestSession(), DraftInput{Title: "Article", HTML: `<p>见<a href="#part"><strong>第一节</strong></a>和<a href="https://go.dev/ref/mem#tmp_1">官方资料</a></p>`}, "update", ExplicitTarget{ID: "42", State: "published", UpdatedAt: version}, false)
			if version == "123" {
				if err != nil || writes != 1 || got.ID != "42" || got.Kind != "published-updated" {
					t.Fatalf("update: %+v writes=%d err=%v", got, writes, err)
				}
			} else if err == nil || writes != 0 {
				t.Fatalf("stale update wrote: %d err=%v", writes, err)
			}
		})
	}
}

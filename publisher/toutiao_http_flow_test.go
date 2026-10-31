package publisher

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestToutiaoNativeHTTPAuthorizationPreflightAndUnboundDraft(t *testing.T) {
	const mediaID = "1234567890123456"
	var calls []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/mp/agw/media/get_media_info":
			return jsonResponse(r, 200, `{"code":0,"data":{"media":{"id_str":"1234567890123456"},"user":{"id":"9876543210123456","screen_name":"synthetic"}}}`, nil), nil
		case "/spice/image":
			if r.Method != "HEAD" || r.Header.Get("x-secsdk-csrf-request") != "1" {
				t.Fatal("missing native HTTP CSRF preflight")
			}
			return jsonResponse(r, 200, "", map[string]string{"x-ware-csrf-token": "0,synthetic-ephemeral-csrf-123,90000,ignored,ignored"}), nil
		case "/mp/agw/article/publish":
			if r.Method != "POST" || r.Header.Get("x-secsdk-csrf-token") != "synthetic-ephemeral-csrf-123" {
				t.Fatal("draft POST missing short-lived CSRF")
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.PostForm.Get("save") != "0" || r.PostForm.Has("pgc_id") {
				t.Fatal("unbound article draft must omit pgc_id, with save=0")
			}
			if r.PostForm.Get("article_ad_type") != "3" || !r.PostForm.Has("customer_nick_name") {
				t.Fatal("initial draft form fields do not match HAR")
			}
			body := r.PostForm.Get("content")
			if strings.Count(body, "<li>") != 3 || strings.Contains(body, `href="#`) ||
				!strings.Contains(body, "　1.1 子章节") {
				t.Fatalf("native HTTP submission did not normalize the nested TOC: %s", body)
			}
			id := r.PostForm.Get("title_id")
			segments := strings.Split(id, "_")
			if len(segments) != 2 || len(segments[0]) != 13 || segments[1] != mediaID {
				t.Fatalf("media-scoped title ID absent: length=%d", len(id))
			}
			return jsonResponse(r, 200, `{"code":0,"err_no":0,"data":{"pgc_id":"7777777777777777777"}}`, nil), nil
		default:
			t.Fatalf("unexpected outbound call %s", r.URL)
			return nil, nil
		}
	})}
	adapterValue, err := NewToutiaoAdapter(client, toutiaoTestSession())
	if err != nil {
		t.Fatal(err)
	}
	adapter := adapterValue.(*toutiaoAdapter)
	auth, err := adapter.CheckAuth(context.Background())
	if err != nil || !auth.Authenticated {
		t.Fatalf("auth failed %v", err)
	}
	result, err := adapter.CreateDraft(context.Background(), DraftInput{Title: "示例文章", Markdown: "示例正文", HTML: `<h2>目录</h2><ul><li><a href="#1">1. 主章节</a><ul><li><a href="#11">1.1 子章节</a></li></ul></li><li><a href="#2">2. 其他章节</a></li></ul><p>正文</p>`})
	if err != nil || !result.Created || result.ID != "7777777777777777777" {
		t.Fatalf("draft response incorrect: created=%v err=%v", result.Created, err)
	}
	if got := strings.Join(calls, ","); got != "GET /mp/agw/media/get_media_info,HEAD /spice/image,POST /mp/agw/article/publish" {
		t.Fatalf("native HTTP order wrong: %s", got)
	}
	if adapter.session.RequestHeaders["x-secsdk-csrf-token"] == "synthetic-ephemeral-csrf-123" {
		t.Fatal("the short-lived CSRF credential must be restored after publishing")
	}
}

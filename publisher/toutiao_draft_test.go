package publisher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestToutiaoDraftCreateAndUpdateUseCapturedSaveMode(t *testing.T) {
	const existingID = "7694632080231186986"
	var saves []url.Values
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/spice/image":
			if req.Method != "HEAD" {
				t.Fatal("CSRF preflight must use HEAD")
			}
			return jsonResponse(req, 200, "", map[string]string{"x-ware-csrf-token": "0,synthetic-fresh-CSRF-token-123,90000,any,extra"}), nil
		case "/mp/agw/creator_center/draft_list":
			return jsonResponse(req, 200, `{"code":0,"draft_list":[{"gid":"7694632080231186986","title":"原始草稿"}]}`, nil), nil
		case "/mp/agw/article/publish":
			if req.Header.Get("x-secsdk-csrf-token") != "synthetic-fresh-CSRF-token-123" {
				t.Fatal("publisher did not use freshly issued CSRF token")
			}
			if req.URL.Query().Get("aid") != "1231" {
				t.Fatalf("missing creator aid: %v", req.URL.Query())
			}
			if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded;charset=UTF-8" {
				t.Fatalf("content-type: %q", req.Header.Get("Content-Type"))
			}
			if err := req.ParseForm(); err != nil {
				t.Fatal(err)
			}
			saves = append(saves, req.PostForm)
			return jsonResponse(req, 200, `{"code":0,"err_no":0,"message":"保存成功","data":{"pgc_id":"7694632080231186986"}}`, nil), nil
		default:
			t.Fatalf("unexpected endpoint: %s", req.URL)
			return nil, nil
		}
	})}
	adapter, err := NewToutiaoAdapter(client, toutiaoTestSession())
	if err != nil {
		t.Fatal(err)
	}
	// Authentication normally populates mediaID from creator account info.
	adapter.(*toutiaoAdapter).mediaID = "1234567890123456"
	create, err := adapter.CreateDraft(context.Background(), DraftInput{Title: "创建草稿", Markdown: "新正文"})
	if err != nil {
		t.Fatal(err)
	}
	if !create.Created || create.ID != existingID {
		t.Fatalf("create=%+v", create)
	}
	update, err := adapter.UpdateDraft(context.Background(), DraftRef{ID: existingID}, DraftInput{Title: "更新草稿", Markdown: "修改正文"})
	if err != nil {
		t.Fatal(err)
	}
	if !update.Updated || update.ID != existingID {
		t.Fatalf("update=%+v", update)
	}
	if len(saves) != 2 {
		t.Fatalf("writes=%d", len(saves))
	}
	for i, v := range saves {
		if v.Get("save") != "0" || v.Get("source") != "29" || v.Get("draft_form_data") != `{"coverType":2}` {
			t.Fatalf("write %d form: save=%s source=%s draft-form=%s", i, v.Get("save"), v.Get("source"), v.Get("draft_form_data"))
		}
		if v.Get("extra") == "" || v.Get("content") == "" || v.Get("search_creation_info") == "" {
			t.Fatalf("write %d lacks creator editor fields", i)
		}
	}
	if _, ok := saves[0]["pgc_id"]; ok {
		t.Fatalf("creating a new draft unexpectedly carries pgc_id")
	}
	if saves[0].Get("article_ad_type") != "3" || saves[0].Get("customer_nick_name") != "" {
		t.Fatal("initial draft form does not match captured creator settings")
	}
	if _, exists := saves[0]["customer_nick_name"]; !exists {
		t.Fatal("initial draft must include an empty customer_nick_name")
	}
	if saves[1].Get("article_ad_type") != "3" || saves[1].Get("article_type") != "0" {
		t.Fatal("existing draft must use the editor's captured reopened-draft form")
	}
	id := saves[0].Get("title_id")
	parts := strings.Split(id, "_")
	if len(parts) != 2 || len(parts[0]) != 13 || parts[1] != "1234567890123456" {
		t.Fatalf("initial draft title_id must use timestamp_mediaId: received length=%d", len(id))
	}
	if titleID, ok := saves[1]["title_id"]; !ok || len(titleID) != 1 || titleID[0] != "" {
		t.Fatal("reopened editor sends an explicitly empty title_id")
	}
	if saves[1].Get("pgc_id") != existingID {
		t.Fatalf("updating draft must preserve ID, got %s", saves[1].Get("pgc_id"))
	}
}

func TestToutiaoUpdateDraftRefusesUnknownOrPublishedID(t *testing.T) {
	publishRequests := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/mp/agw/creator_center/draft_list":
			return jsonResponse(req, 200, `{"code":0,"draft_list":[{"gid":"111","title":"草稿"}]}`, nil), nil
		case "/mp/agw/article/publish":
			publishRequests++
			t.Fatal("unknown draft ID must not submit a publish or draft form")
			return nil, nil
		default:
			t.Fatalf("unexpected endpoint: %s", req.URL.Path)
			return nil, nil
		}
	})}
	adapterValue, _ := NewToutiaoAdapter(client, toutiaoTestSession())
	_, err := adapterValue.UpdateDraft(context.Background(), DraftRef{ID: "999"}, DraftInput{Title: "可能已发布", Markdown: "正文"})
	if err == nil || !IsKind(err, ErrValidation) {
		t.Fatalf("missing safe validation: %v", err)
	}
	if publishRequests != 0 {
		t.Fatalf("unexpected form submissions: %d", publishRequests)
	}
}

func TestToutiaoUpdateDraftRejectsChangedRemoteID(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/mp/agw/creator_center/draft_list":
			return jsonResponse(req, 200, `{"code":0,"draft_list":[{"gid":"111","title":"草稿"}]}`, nil), nil
		case "/mp/agw/article/publish":
			return jsonResponse(req, 200, `{"code":0,"err_no":0,"data":{"pgc_id":"222"}}`, nil), nil
		default:
			t.Fatalf("unexpected endpoint: %s", req.URL.Path)
			return nil, nil
		}
	})}
	adapterValue, _ := NewToutiaoAdapter(client, toutiaoTestSession())
	_, err := adapterValue.UpdateDraft(context.Background(), DraftRef{ID: "111"}, DraftInput{Title: "草稿", Markdown: "正文"})
	if err == nil || !strings.Contains(err.Error(), "different article ID") {
		t.Fatalf("expected remote ID mismatch, got %v", err)
	}
}

func TestToutiaoPublishIsNotDraftSave(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/mp/agw/creator_center/draft_list" {
			return jsonResponse(req, 200, `{"code":0,"draft_list":[{"gid":"123","title":"发布"}]}`, nil), nil
		}
		if req.URL.Path != "/mp/agw/article/publish" {
			t.Fatalf("unexpected endpoint %s", req.URL.Path)
		}
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if req.PostForm.Get("save") != "1" || req.PostForm.Get("pgc_id") != "123" {
			t.Fatalf("explicit publish must use separate mode; save=%s ID=%s", req.PostForm.Get("save"), req.PostForm.Get("pgc_id"))
		}
		return jsonResponse(req, 200, `{"code":0,"err_no":0,"data":{"pgc_id":"123"}}`, nil), nil
	})}
	adapterValue, _ := NewToutiaoAdapter(client, toutiaoTestSession())
	result, err := adapterValue.PublishDraft(context.Background(), DraftRef{ID: "123"}, DraftInput{Title: "发布", Markdown: "正文"})
	if err != nil || result.URL != "https://www.toutiao.com/article/123/" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestToutiaoUpdateDraftPreservesExistingCoverMetadata(t *testing.T) {
	const originalCovers = `[{"url":"https://example.invalid/cover.jpg","thumb_width":750,"thumb_height":422}]`
	metadata, err := json.Marshal(map[string]string{"pgc_feed_covers": originalCovers})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"code":       0,
		"draft_list": []map[string]any{{"gid": "111", "title": "existing", "graphic_extra": string(metadata)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/mp/agw/creator_center/draft_list":
			return jsonResponse(req, 200, string(payload), nil), nil
		case "/mp/agw/article/publish":
			if err := req.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if req.PostForm.Get("pgc_feed_covers") != originalCovers {
				t.Fatalf("lost existing draft cover metadata: %q", req.PostForm.Get("pgc_feed_covers"))
			}
			return jsonResponse(req, 200, `{"code":0,"err_no":0,"data":{"pgc_id":"111"}}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s", req.URL)
			return nil, nil
		}
	})}
	adapterValue, _ := NewToutiaoAdapter(client, toutiaoTestSession())
	_, err = adapterValue.UpdateDraft(context.Background(), DraftRef{ID: "111"}, DraftInput{Title: "changed", Markdown: "changed body"})
	if err != nil {
		t.Fatal(err)
	}
}

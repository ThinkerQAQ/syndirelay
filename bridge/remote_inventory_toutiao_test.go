package bridge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToutiaoRemoteInventoryReadsDraftsAndPublishedWithoutWrites(t *testing.T) {
	server, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	server.httpClient = &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("inventory wrote %s", req.URL)
		}
		var body string
		switch req.URL.Path {
		case "/mp/agw/media/get_media_info":
			body = `{"data":{"user":{"id":"creator-1","screen_name":"example"}}}`
		case "/mp/agw/creator_center/draft_list":
			body = `{"code":0,"draft_list":[{"gid":"11","title":"Draft"}]}`
		case "/mp/agw/creator_center/list/v2":
			body = `{"code":0,"has_more":false,"contents":[{"article_attr":{"gid":"22","title":"Published","status":2}}]}`
		default:
			t.Fatalf("unexpected request: %s", req.URL)
		}
		return mediumResponse(req, http.StatusOK, body, nil), nil
	})}
	sync := httptest.NewRequest(http.MethodPost, "/v1/sessions/toutiao", bytes.NewBufferString(`{"cookies":[{"name":"sessionid","value":"test-session","domain":".toutiao.com","path":"/","secure":true}],"userAgent":"Test"}`))
	setExtensionAuth(sync, "test-token")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, sync)
	if rec.Code != http.StatusOK {
		t.Fatalf("session: %d %s", rec.Code, rec.Body)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/remote/inventory?platform=toutiao", nil)
	setExtensionAuth(req, "test-token")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("inventory: %d %s", rec.Code, rec.Body)
	}
	var result struct {
		Account string                `json:"account"`
		Items   []remoteInventoryPost `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Account != "creator-1" || len(result.Items) != 2 || result.Items[0].ID != "11" || result.Items[0].Published || result.Items[1].ID != "22" || !result.Items[1].Published {
		t.Fatalf("inventory: %+v", result)
	}
}

package publisher

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestToutiaoDraftFailureRetainsOnlyNonSecretBusinessCodes(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/spice/image":
			return jsonResponse(r, 200, "", map[string]string{"x-ware-csrf-token": "0,synthetic-csrf-secret-123,90000,extra"}), nil
		case "/mp/agw/article/publish":
			return jsonResponse(r, 200, `{"code":7050,"err_no":30001,"message":"保存失败"}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, nil
		}
	})}
	adapterValue, err := NewToutiaoAdapter(client, toutiaoTestSession())
	if err != nil {
		t.Fatal(err)
	}
	adapter := adapterValue.(*toutiaoAdapter)
	adapter.mediaID = "1234567890123456"
	_, err = adapter.CreateDraft(context.Background(), DraftInput{Title: "test", Markdown: "text"})
	if err == nil {
		t.Fatal("failed draft must not report success")
	}
	msg := err.Error()
	if !strings.Contains(msg, "code=7050") || !strings.Contains(msg, "err_no=30001") ||
		!strings.Contains(msg, "保存失败") || strings.Contains(msg, "synthetic-csrf-secret") {
		t.Fatalf("business error must be actionable and sanitized: %s", msg)
	}
}

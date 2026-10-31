package publisher

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
)

func TestToutiaoUpdatePublishedRequiresVerifiedBindingBeforeRemoteRequests(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		t.Fatalf("unverified Toutiao publishing must not make remote HTTP requests: %s", r.URL)
		return nil, nil
	})}
	_, _, err := (Service{HTTPClient: client, PublicationPath: filepath.Join(t.TempDir(), "publications.json")}).
		UpdateToutiaoPublishedInput(context.Background(), toutiaoTestSession(), "", DraftInput{
			Slug: "concurrency-06", Title: "正文", Markdown: "正文",
		})
	if !IsKind(err, ErrValidation) || calls != 0 {
		t.Fatalf("expected fail-closed published update: err=%v calls=%d", err, calls)
	}
}

func TestToutiaoUpdatePublishedRejectsMismatchedBaselineAndAccount(t *testing.T) {
	for _, tc := range []struct{ name, account, modified string }{
		{"stale revision", "creator-1", "old"},
		{"different account", "another-creator", "1791545110"},
		{"missing baseline", "creator-1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "publications.json")
			err := SavePublicationBinding(path, PublicationBinding{
				Slug: "article", Platform: "toutiao", PublishedRemoteID: "777", PublishedURL: "https://www.toutiao.com/article/777/",
				Account: tc.account, RemoteUpdatedAt: tc.modified,
			})
			if err != nil {
				t.Fatal(err)
			}
			requests := 0
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/mp/agw/media/get_media_info":
					return jsonResponse(r, 200, `{"code":0,"data":{"user":{"id":"creator-1"}}}`, nil), nil
				case "/mp/agw/creator_center/list/v2":
					return jsonResponse(r, 200, `{"code":0,"contents":[{"article_attr":{"gid":"777","title":"old","status":2,"modify_time":1791545110}}]}`, nil), nil
				case "/mp/agw/article/publish":
					requests++
					t.Fatal("unsafe republish attempt")
					return nil, nil
				default:
					t.Fatalf("unexpected request: %s", r.URL)
					return nil, nil
				}
			})}
			_, _, err = (Service{HTTPClient: client, PublicationPath: path}).UpdateToutiaoPublishedInput(context.Background(), toutiaoTestSession(), "", DraftInput{Slug: "article", Title: "new", Markdown: "new", ContentHash: "new"})
			if err == nil || !IsKind(err, ErrValidation) || requests != 0 {
				t.Fatalf("expected rejection err=%v requests=%d", err, requests)
			}
		})
	}
}

func TestToutiaoUpdatePublishedCannotBecomeDraftSave(t *testing.T) {
	published := PublicationState{PublishedRemoteID: "777", PublishedURL: "https://www.toutiao.com/article/777/"}
	if mayRecreateMissingDraft("toutiao", published) {
		t.Fatal("missing draft must not silently recreate a published article")
	}
	if v := toutiaoArticleValues(DraftInput{Title: "hello"}, "<p>body</p>", "777", "", false); v.Get("save") != "0" {
		t.Fatal("draft mutation must keep save=0")
	}
	if v := toutiaoArticleValues(DraftInput{Title: "hello"}, "<p>body</p>", "777", "", true); v.Get("save") != "1" {
		t.Fatal("explicit publish must keep save=1")
	}
}

// A mock success deliberately exercises the explicit save=1 existing-ID path.
// Never use a real published article in integration verification.
func TestToutiaoPublishedUpdatePreservesIDAndInvalidatesBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "publications.json")
	if err := SavePublicationBinding(path, PublicationBinding{
		Slug: "article", Platform: "toutiao", PublishedRemoteID: "777",
		PublishedURL: "https://www.toutiao.com/article/777/",
		Account:      "creator-1", RemoteUpdatedAt: "1791545110",
	}); err != nil {
		t.Fatal(err)
	}
	writes := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/mp/agw/media/get_media_info":
			return jsonResponse(r, 200, `{"code":0,"data":{"media":{"id_str":"1234567890123456"},"user":{"id":"creator-1"}}}`, nil), nil
		case "/mp/agw/creator_center/list/v2":
			return jsonResponse(r, 200, `{"code":0,"contents":[{"article_attr":{"gid":"777","title":"original","status":2,"modify_time":1791545110}}]}`, nil), nil
		case "/spice/image":
			if r.Method != http.MethodHead {
				t.Fatal("expected HEAD CSRF preflight")
			}
			return jsonResponse(r, 200, "", map[string]string{"x-ware-csrf-token": "0,synthetic-csrf-credential,90000,extra"}), nil
		case "/mp/agw/article/publish":
			writes++
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.PostForm.Get("save") != "1" || r.PostForm.Get("pgc_id") != "777" {
				t.Fatalf("explicit update must use existing public ID: %v", r.PostForm)
			}
			if r.Header.Get("x-secsdk-csrf-token") != "synthetic-csrf-credential" {
				t.Fatal("expected fresh HTTP CSRF token")
			}
			return jsonResponse(r, 200, `{"code":0,"err_no":0,"data":{"pgc_id":"777"}}`, nil), nil
		default:
			t.Fatalf("unexpected HTTP request: %s", r.URL)
			return nil, nil
		}
	})}
	service := Service{HTTPClient: client, PublicationPath: path}
	result, skipped, err := service.UpdateToutiaoPublishedInput(context.Background(), toutiaoTestSession(), "", DraftInput{
		Slug: "article", Title: "updated", Markdown: "updated body", ContentHash: "new-hash",
	})
	if err != nil || skipped || writes != 1 || result.ID != "777" {
		t.Fatalf("result=%+v skipped=%t err=%v writes=%d", result, skipped, err, writes)
	}
	binding, exists, err := LoadPublicationBinding(path, "article", "toutiao")
	if err != nil || !exists || binding.RemoteUpdatedAt != "" || binding.VerifiedAt != "" ||
		binding.PublishedRemoteID != "777" || binding.PublishedHash != "new-hash" {
		t.Fatalf("baseline must be invalidated pending re-verification: %+v err=%v", binding, err)
	}
}

package publisher

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestToutiaoNativeProbeReadOnlyDoesNotWrite(t *testing.T) {
	var requests []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/mp/agw/media/get_media_info":
			return jsonResponse(r, 200, `{"code":0,"data":{"media":{"id_str":"1234567890123456"},"user":{"id":"9876543210123456"}}}`, nil), nil
		case "/spice/image":
			return jsonResponse(r, 200, "", map[string]string{"x-ware-csrf-token": "0,synthetic-CSRF-credential,120000,extra,extra"}), nil
		default:
			t.Fatalf("unexpected request %s", r.URL)
			return nil, nil
		}
	})}
	result, err := ProbeToutiaoHTTP(context.Background(), client, toutiaoTestSession(), false)
	if err != nil || !result.Authenticated || !result.CSRFReady || result.CreatedDraftID != "" {
		t.Fatalf("probe read-only result %+v, err=%v", result, err)
	}
	if strings.Join(requests, ",") != "GET /mp/agw/media/get_media_info,HEAD /spice/image" {
		t.Fatalf("unexpected read-only API flow %v", requests)
	}
}

func TestToutiaoNativeProbeCreatesOnlyPrivateDraftAndVerifiesInventory(t *testing.T) {
	var requestPaths []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestPaths = append(requestPaths, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/mp/agw/media/get_media_info":
			return jsonResponse(r, 200, `{"code":0,"data":{"media":{"id_str":"1234567890123456"},"user":{"id":"9876543210123456"}}}`, nil), nil
		case "/spice/image":
			return jsonResponse(r, 200, "", map[string]string{"x-ware-csrf-token": "0,synthetic-CSRF-credential,120000,extra,extra"}), nil
		case "/mp/agw/article/publish":
			if r.Method != "POST" {
				t.Fatal("draft must use POST")
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.PostForm.Get("save") != "0" || r.PostForm.Has("pgc_id") {
				t.Fatal("test must be a new private draft without a bound id")
			}
			return jsonResponse(r, 200, `{"code":0,"err_no":0,"data":{"pgc_id":"12345"}}`, nil), nil
		case "/mp/agw/creator_center/draft_list":
			return jsonResponse(r, 200, `{"code":0,"draft_list":[{"gid":"12345","title":"BlogCTL 纯接口测试草稿"}]}`, nil), nil
		default:
			t.Fatalf("unexpected request %s", r.URL)
			return nil, nil
		}
	})}
	result, err := ProbeToutiaoHTTP(context.Background(), client, toutiaoTestSession(), true)
	if err != nil || !result.DraftVerified || result.CreatedDraftID != "12345" {
		t.Fatalf("probe result %+v, err=%v", result, err)
	}
	if got := strings.Join(requestPaths, ","); got != "GET /mp/agw/media/get_media_info,HEAD /spice/image,POST /mp/agw/article/publish,GET /mp/agw/creator_center/draft_list" {
		t.Fatalf("unexpected request sequence %s", got)
	}
}

func TestToutiaoNativeProbeStopsAfterRejectedPreflight(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jsonResponse(r, 200, `{"code":0,"data":{"media":{"id_str":"1234567890123456"},"user":{"id":"9876543210123456"}}}`, nil), nil
		}
		if calls == 2 {
			return jsonResponse(r, 403, "", nil), nil
		}
		t.Fatal("unexpected write after CSRF preflight failure")
		return nil, nil
	})}
	result, err := ProbeToutiaoHTTP(context.Background(), client, toutiaoTestSession(), true)
	if err == nil || result.CreatedDraftID != "" || calls != 2 {
		t.Fatalf("expected preflight failure before any draft write, result=%+v, calls=%d", result, calls)
	}
}

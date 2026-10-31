package publisher

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func segmentFaultSession() Session {
	return Session{
		UserAgent: "BlogCTL-Test-UA",
		Cookies: []BrowserCookie{{
			Name: "sf-session", Value: "secret", Domain: ".segmentfault.com", Path: "/", Secure: true,
		}},
	}
}

func TestSegmentFaultCreateDraftUsesPostAndEmptyObjectID(t *testing.T) {
	var payload map[string]any
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/write":
			return jsonResponse(request, 200, `<script>serverData":{"Token":"sf-token"}</script>`, nil), nil
		case "/gateway/tags":
			if request.URL.Query().Get("keyv") != segmentFaultKeyVersion {
				t.Fatalf("keyv = %q", request.URL.Query().Get("keyv"))
			}
			ivd := request.URL.Query().Get("ivd")
			decoded, decodeErr := base64.StdEncoding.DecodeString(ivd)
			if decodeErr != nil || len(decoded) != 256 {
				t.Fatalf("ivd is not a 2048-bit RSA ciphertext: len=%d err=%v", len(decoded), decodeErr)
			}
			return jsonResponse(request, 200, `{"rows":{"backend":[{"id":101,"name":"Go"}]}}`, nil), nil
		case "/gateway/draft":
			if request.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", request.Method)
			}
			if request.Header.Get("token") != "sf-token" || request.Header.Get("authorization") != "Bearer sf-token" {
				t.Fatalf("auth headers = token:%q authorization:%q", request.Header.Get("token"), request.Header.Get("authorization"))
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `[0,{"id":"draft-1"}]`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewSegmentFaultAdapter(client, segmentFaultSession())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.CreateDraft(context.Background(), DraftInput{
		Title: "Example", Markdown: "Body", Tags: []string{"Go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "draft-1" || !result.Created || result.URL != "https://segmentfault.com/write?draftId=draft-1" {
		t.Fatalf("result = %#v", result)
	}
	if payload["object_id"] != "" {
		t.Fatalf("create payload = %#v", payload)
	}
	if _, exists := payload["id"]; exists {
		t.Fatalf("create payload unexpectedly contains id: %#v", payload)
	}
}

func TestSegmentFaultUpdateDraftUsesPutResourceEndpoint(t *testing.T) {
	var payload map[string]any
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/write":
			return jsonResponse(request, 200, `window.g_initialProps = {"global":{"sessionInfo":{"key":"legacy-token"}}};
	</script>`, nil), nil
		case "/gateway/tags":
			if request.URL.Query().Get("keyv") != segmentFaultKeyVersion {
				t.Fatalf("keyv = %q", request.URL.Query().Get("keyv"))
			}
			ivd := request.URL.Query().Get("ivd")
			decoded, decodeErr := base64.StdEncoding.DecodeString(ivd)
			if decodeErr != nil || len(decoded) != 256 {
				t.Fatalf("ivd is not a 2048-bit RSA ciphertext: len=%d err=%v", len(decoded), decodeErr)
			}
			return jsonResponse(request, 200, `{"rows":{"backend":[{"id":101,"name":"Go"}]}}`, nil), nil
		case "/gateway/draft/draft-2":
			if request.Method != http.MethodPut {
				t.Fatalf("method = %s, want PUT", request.Method)
			}
			if request.Header.Get("token") != "legacy-token" || request.Header.Get("authorization") != "Bearer legacy-token" {
				t.Fatalf("auth headers = %#v", request.Header)
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    request,
			}, nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewSegmentFaultAdapter(client, segmentFaultSession())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.UpdateDraft(context.Background(), DraftRef{ID: "draft-2"}, DraftInput{
		Title: "Updated", Markdown: "Changed body", Tags: []string{"Go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "draft-2" || !result.Updated {
		t.Fatalf("result = %#v", result)
	}
	if payload["id"] != "draft-2" {
		t.Fatalf("update payload = %#v", payload)
	}
	if payload["object_id"] != "" {
		t.Fatalf("update payload object_id = %#v, want empty string", payload["object_id"])
	}
	if payload["language"] != "" || payload["cover"] != "" {
		t.Fatalf("update payload optional fields = language:%#v cover:%#v", payload["language"], payload["cover"])
	}
}

func TestSegmentFaultUpdateDraftMapsMissingRemoteDraft(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/write":
			return jsonResponse(request, 200, `serverData":{"Token":"sf-token"}`, nil), nil
		case "/gateway/tags":
			return jsonResponse(request, 200, `{"rows":{"backend":[{"id":101,"name":"Go"}]}}`, nil), nil
		case "/gateway/draft/missing":
			return jsonResponse(request, http.StatusNotFound, `{"message":"draft not found"}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL)
			return nil, nil
		}
	})}
	adapter, err := NewSegmentFaultAdapter(client, segmentFaultSession())
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.UpdateDraft(context.Background(), DraftRef{ID: "missing"}, DraftInput{Title: "x", Markdown: "x", Tags: []string{"Go"}})
	if err == nil || !IsKind(err, ErrRemoteDraftMissing) {
		t.Fatalf("error = %v, want remote-draft-not-found", err)
	}
}

func TestSegmentFaultPublishUsesCapturedGatewayArticleContract(t *testing.T) {
	const draftID = "1220000048321382"
	const articleID = "1190000048322882"
	const blogID int64 = 1200000048304547

	editorHTML := `<html><body>
<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"initialState":{"editor":{"detail":{
"blogs":[{"id":1200000048304547,"name":"并发编程实战","slug":"concurrency"}],
"draft":{"id":1220000048321382,"title":"Example","text":"Remote body","cover":null,"tags":[{"id":101,"name":"go"}]}
}}}}}}</script>
<script>window.serverData={"Token":"sf-token"}</script>
</body></html>`

	var publishPayload map[string]any
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/write":
			if request.URL.Query().Get("draftId") != draftID {
				t.Fatalf("draftId = %q", request.URL.Query().Get("draftId"))
			}
			return jsonResponse(request, 200, editorHTML, nil), nil
		case "/gateway/article":
			if request.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", request.Method)
			}
			if request.Header.Get("token") != "sf-token" || request.Header.Get("authorization") != "Bearer sf-token" {
				t.Fatalf("auth headers = %#v", request.Header)
			}
			if request.Header.Get("referer") != "https://segmentfault.com/write?draftId="+draftID {
				t.Fatalf("referer = %q", request.Header.Get("referer"))
			}
			if err := json.NewDecoder(request.Body).Decode(&publishPayload); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, http.StatusCreated, `{"data":{"id":1190000048322882},"msg":""}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewSegmentFaultAdapter(client, segmentFaultSession())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.PublishDraft(context.Background(), DraftRef{ID: draftID}, DraftInput{
		Title: "Example", Markdown: "Local body", Tags: []string{"go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != articleID || result.URL != "https://segmentfault.com/a/"+articleID {
		t.Fatalf("result = %#v", result)
	}
	if got := int64(publishPayload["draft_id"].(float64)); got != 1220000048321382 {
		t.Fatalf("draft_id = %d", got)
	}
	if got := int64(publishPayload["blog_id"].(float64)); got != blogID {
		t.Fatalf("blog_id = %d", got)
	}
	if publishPayload["title"] != "Example" || publishPayload["text"] != "Remote body" {
		t.Fatalf("publish payload did not use remote draft: %#v", publishPayload)
	}
	tags, ok := publishPayload["tags"].([]any)
	if !ok || len(tags) != 1 || int64(tags[0].(float64)) != 101 {
		t.Fatalf("tags = %#v", publishPayload["tags"])
	}
	if got := int(publishPayload["type"].(float64)); got != 1 {
		t.Fatalf("type = %d", got)
	}
}

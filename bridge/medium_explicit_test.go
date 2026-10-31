package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	blogcompiler "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/compiler"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

// Model persisted paragraph fields from the deltas accepted by the mock.
func mediumMockDraftReadback(req *http.Request, postID string, deltas []map[string]any) *http.Response {
	paragraphs := make([]map[string]any, 0, len(deltas))
	for _, delta := range deltas {
		if paragraph, ok := delta["paragraph"].(map[string]any); ok {
			paragraphs = append(paragraphs, paragraph)
		}
	}
	raw, _ := json.Marshal([]any{map[string]any{
		"data": map[string]any{"postResult": map[string]any{
			"id":      postID,
			"content": map[string]any{"bodyModel": map[string]any{"paragraphs": paragraphs}},
		}},
	}})
	return mediumResponse(req, http.StatusOK, string(raw), nil)
}

func TestMediumExplicitCreateUsesNativeBridgeAndLeavesUnpublished(t *testing.T) {
	calls := []string{}
	var saved []map[string]any
	client := &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		if req.Header.Get("x-xsrf-token") != "test-xsrf" {
			t.Fatalf("missing Medium XSRF header")
		}
		switch req.URL.Path {
		case "/new-story":
			return mediumResponse(req, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"id":"1e645140212b"}}}`, nil), nil
		case "/_/graphql":
			return mediumMockDraftReadback(req, "1e645140212b", saved), nil
		case "/p/1e645140212b/deltas":
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["id"] != "1e645140212b" {
				t.Fatalf("unexpected delta target: %#v", body)
			}
			for _, row := range body["deltas"].([]any) {
				saved = append(saved, row.(map[string]any))
			}
			return mediumResponse(req, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"latestRev":1}}}`, nil), nil
		default:
			t.Fatalf("unexpected Medium operation: %s", req.URL.String())
			return nil, nil
		}
	})}
	raw, err := json.Marshal(mediumDraft{Title: "Concurrency Programming", Deltas: []map[string]any{
		{"type": 1, "paragraph": map[string]any{"type": 1, "text": "Body"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runMediumExplicit(context.Background(),
		publisher.Session{Cookies: []publisher.BrowserCookie{{Name: "xsrf", Value: "test-xsrf", Domain: "medium.com"}}},
		client, blogcompiler.CompiledArticle{Payload: raw, Title: "Concurrency Programming"},
		publisher.DraftInput{}, t.TempDir(), "create", explicitTaskTarget{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "1e645140212b" || result.URL != "https://medium.com/p/1e645140212b/edit" || result.Kind != "draft-created" {
		t.Fatalf("Medium result: %+v", result)
	}
	if got := strings.Join(calls, ","); got != "POST /new-story,POST /p/1e645140212b/deltas,POST /_/graphql" {
		t.Fatalf("unexpected requests: %s", got)
	}
}

func TestMediumExplicitPublishedTargetAllowedForUpdateWithoutPublishing(t *testing.T) {
	client := &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected remote request: %s", req.URL.String())
		return nil, nil
	})}
	_, err := runMediumExplicit(context.Background(), publisher.Session{}, client,
		blogcompiler.CompiledArticle{}, publisher.DraftInput{}, t.TempDir(),
		"update", explicitTaskTarget{Platform: "medium", ID: "1e645140212b", State: "published"}, false)
	if err == nil || !strings.Contains(err.Error(), "missing transport payload") {
		t.Fatalf("published Update must pass the target validation and require a compiler payload: %v", err)
	}
	_, err = runMediumExplicit(context.Background(), publisher.Session{}, client,
		blogcompiler.CompiledArticle{}, publisher.DraftInput{}, t.TempDir(),
		"update", explicitTaskTarget{Platform: "medium", ID: "1e645140212b", State: "published"}, true)
	if err == nil || !strings.Contains(err.Error(), "manual Save and publish") {
		t.Fatalf("published Update must reject automatic publication: %v", err)
	}
}

func TestMediumAndToutiaoExplicitRequestsAreAccepted(t *testing.T) {
	for _, op := range []string{"create", "update", "publish-draft"} {
		req := syncRequest{Article: "concurrency-series-00", Platforms: []string{"medium"}, Operation: op}
		if op != "create" {
			req.Targets = []explicitTaskTarget{{Platform: "medium", ID: "1e645140212b", State: "draft", URL: "https://medium.com/p/1e645140212b/edit"}}
		}
		if _, err := normalizeSyncRequest(req); err != nil {
			t.Fatalf("Medium %s rejected: %v", op, err)
		}
	}
	req := syncRequest{Article: "concurrency-series-00", Platforms: []string{"toutiao"}, Operation: "create"}
	if _, err := normalizeSyncRequest(req); err != nil {
		t.Fatalf("Toutiao create rejected: %v", err)
	}
}

// Explicit Create never reuses a remote draft; explicit Update requires a
// draft ID selected by the user in the update workflow.
func TestMediumExplicitCreateAndUpdateStaySeparate(t *testing.T) {
	client := &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected network request for invalid explicit operation: %s", req.URL.Path)
		return nil, nil
	})}
	compiled := blogcompiler.CompiledArticle{Payload: []byte(`{"title":"semantic test","deltas":[]}`), Title: "semantic test"}
	for _, tc := range []struct {
		op            string
		target        explicitTaskTarget
		errorContains string
	}{
		{"create", explicitTaskTarget{Platform: "medium", ID: "65eb8f90268b", State: "draft"}, "must not specify an existing remote post"},
		{"update", explicitTaskTarget{Platform: "medium", State: "draft"}, "requires a selected draft ID"},
	} {
		_, err := runMediumExplicit(context.Background(), publisher.Session{}, client, compiled, publisher.DraftInput{}, t.TempDir(), tc.op, tc.target, false)
		if err == nil || !strings.Contains(err.Error(), tc.errorContains) {
			t.Fatalf("%s should reject invalid target: %v", tc.op, err)
		}
	}
}

// Exercise the full explicit Create path, including a remote story ID, a
// downloaded PNG, browser-native upload callback and Medium image/code deltas.
// This is an offline integration test: it does not write a real Medium draft.
func TestMediumExplicitCreateWithBrowserNativeCoverAndCode(t *testing.T) {
	const newID = "456789abcdef"
	var saved []map[string]any
	var pngBuffer bytes.Buffer
	if err := png.Encode(&pngBuffer, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	steps := []string{}
	client := &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/new-story":
			steps = append(steps, "create")
			return mediumResponse(req, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"id":"456789abcdef"}}}`, nil), nil
		case "/cover.png":
			steps = append(steps, "download")
			return mediumResponse(req, http.StatusOK, pngBuffer.String(), http.Header{"Content-Type": {"image/png"}}), nil
		case "/_/upload":
			t.Fatal("native image must not go through Go tls-client")
			return nil, nil
		case "/_/graphql":
			steps = append(steps, "verify")
			return mediumMockDraftReadback(req, newID, saved), nil
		case "/p/456789abcdef/deltas":
			steps = append(steps, "save")
			var payload struct {
				ID     string `json:"id"`
				Deltas []struct {
					Paragraph struct {
						Type     int            `json:"type"`
						Metadata map[string]any `json:"metadata"`
					} `json:"paragraph"`
				} `json:"deltas"`
			}
			data, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatal(err)
			}
			var full map[string]any
			if err := json.Unmarshal(data, &full); err != nil {
				t.Fatal(err)
			}
			for _, row := range full["deltas"].([]any) {
				saved = append(saved, row.(map[string]any))
			}
			if payload.ID != newID {
				t.Fatalf("saved to unexpected draft: %q", payload.ID)
			}
			var imageOK, codeOK bool
			for _, d := range payload.Deltas {
				if d.Paragraph.Type == 8 {
					codeOK = true
				}
				if d.Paragraph.Type != 4 {
					continue
				}
				imageOK = true
				meta := d.Paragraph.Metadata
				if len(meta) != 3 || meta["id"] != "1*browser-native" ||
					meta["originalWidth"] != float64(800) || meta["originalHeight"] != float64(600) {
					t.Fatalf("native image metadata mismatch: %#v", meta)
				}
			}
			if !imageOK || !codeOK {
				t.Fatalf("missing image or native code block: image=%v code=%v", imageOK, codeOK)
			}
			return mediumResponse(req, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"latestRev":1}}}`, nil), nil
		default:
			t.Fatalf("unexpected HTTP request %s %s", req.Method, req.URL.Path)
			return nil, nil
		}
	})}
	serialized, err := json.Marshal(mediumDraft{
		Title: "Browser native Medium create",
		Deltas: []map[string]any{
			{"type": 1, "paragraph": map[string]any{"type": 4, "text": "", "markups": []any{}}, "image": map[string]any{"url": "https://asset.example/cover.png"}},
			{"type": 1, "paragraph": map[string]any{"type": 8, "text": "func main() { println(42) }"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	upload := func(_ context.Context, draftID string, image publisher.RehostImage) (mediumUploadResult, error) {
		if draftID != newID || image.ContentType != "image/png" || len(image.Payload) == 0 {
			t.Fatalf("wrong browser upload task: draft=%q, mime=%q, bytes=%d", draftID, image.ContentType, len(image.Payload))
		}
		steps = append(steps, "browser-upload")
		return mediumUploadResult{FileID: "1*browser-native", Width: 800, Height: 600}, nil
	}
	result, err := runMediumExplicit(context.Background(),
		publisher.Session{Cookies: []publisher.BrowserCookie{{Name: "xsrf", Value: "test-only", Domain: "medium.com"}}},
		client, blogcompiler.CompiledArticle{Payload: serialized, Title: "Browser native Medium create"},
		publisher.DraftInput{}, t.TempDir(), "create", explicitTaskTarget{}, false, upload)
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != newID || result.Kind != "draft-created" {
		t.Fatalf("result: %#v", result)
	}
	if got := strings.Join(steps, ","); got != "create,download,browser-upload,save,verify" {
		t.Fatalf("native create flow out of order: %s", got)
	}
}

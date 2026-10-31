package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	blogcompiler "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/compiler"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

func TestMediumDraftVerificationRejectsMissingImagesAndCorruptCode(t *testing.T) {
	deltas := []map[string]any{
		{"paragraph": map[string]any{"type": 3, "text": "Title"}},
		{"paragraph": map[string]any{"type": float64(8), "text": "foo()\n  bar()"}},
		{"paragraph": map[string]any{"type": float64(4), "text": "", "metadata": map[string]any{
			"id": "1*image.png", "originalWidth": float64(1400), "originalHeight": float64(800),
		}}},
	}
	want := []mediumSavedParagraph{{Type: 3, Text: "Title"}, {Type: 8, Text: "foo()\n  bar()"}, {Type: 4}}
	want[2].Metadata.ID = "1*image.png"
	want[2].Metadata.OriginalWidth = 1400
	want[2].Metadata.OriginalHeight = 800
	if err := verifyMediumParagraphs(deltas, want); err != nil {
		t.Fatalf("valid body: %v", err)
	}
	if err := verifyMediumParagraphs(deltas, want[:2]); err == nil || !strings.Contains(err.Error(), "paragraphs") {
		t.Fatalf("missing picture accepted: %v", err)
	}
	badCode := append([]mediumSavedParagraph(nil), want...)
	badCode[1].Text = "foo() bar()"
	if err := verifyMediumParagraphs(deltas, badCode); err == nil || !strings.Contains(err.Error(), "text differs") {
		t.Fatalf("corrupt code accepted: %v", err)
	}
	badImage := append([]mediumSavedParagraph(nil), want...)
	badImage[2].Metadata.ID = "1*wrong.png"
	if err := verifyMediumParagraphs(deltas, badImage); err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("incorrect image accepted: %v", err)
	}
}

func TestMediumDraftReadbackHandlesXSSIAndGraphQLErrors(t *testing.T) {
	raw, _ := json.Marshal([]any{map[string]any{"data": map[string]any{"postResult": map[string]any{
		"id": "abcd1234", "content": map[string]any{"bodyModel": map[string]any{
			"paragraphs": []any{map[string]any{"type": 8, "text": "line\n  indented"}},
		}},
	}}}})
	client := &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/_/graphql" {
			t.Fatalf("unexpected request: %s", req.URL)
		}
		return mediumResponse(req, http.StatusOK, "])}while(1);</x>"+string(raw), nil), nil
	})}
	got, err := (mediumClient{httpClient: client}).readDraftParagraphs(context.Background(), platformSession{}, "abcd1234")
	if err != nil || len(got) != 1 || got[0].Type != 8 || got[0].Text != "line\n  indented" {
		t.Fatalf("result=%#v err=%v", got, err)
	}
	client.Transport = mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return mediumResponse(req, http.StatusOK, "[{\"errors\":[{\"message\":\"Unsupported field\"}]}]", nil), nil
	})
	if _, err := (mediumClient{httpClient: client}).readDraftParagraphs(context.Background(), platformSession{}, "abcd1234"); err == nil {
		t.Fatal("GraphQL rejection must fail closed")
	}
}

func TestMediumIncompleteCreateKeepsDraftIDAndProgress(t *testing.T) {
	stages := []string{}
	client := &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/new-story":
			stages = append(stages, "create")
			return mediumResponse(req, http.StatusOK, "])}while(1);</x>{\"success\":true,\"payload\":{\"value\":{\"id\":\"abcdef123456\"}}}", nil), nil
		case "/picture.png":
			stages = append(stages, "download")
			return mediumResponse(req, http.StatusOK, "valid-binary-placeholder", http.Header{"Content-Type": {"image/png"}}), nil
		default:
			t.Fatalf("image failure must not write body: %s", req.URL)
			return nil, nil
		}
	})}
	draft := mediumDraft{Title: "Only draft", Deltas: []map[string]any{
		{"type": 1, "paragraph": map[string]any{"type": 4, "text": ""}, "image": map[string]any{"url": "https://medium.com/picture.png"}},
	}}
	payload, _ := json.Marshal(draft)
	var allocated string
	result, err := runMediumExplicitWithProgress(context.Background(),
		publisher.Session{Cookies: []publisher.BrowserCookie{{Name: "xsrf", Value: "fake", Domain: "medium.com"}}},
		client, blogcompiler.CompiledArticle{Payload: payload, Title: "Only draft"}, publisher.DraftInput{}, t.TempDir(), "create",
		explicitTaskTarget{}, false, func(id string) { allocated = id; stages = append(stages, "checkpoint") },
		func(_ context.Context, id string, _ publisher.RehostImage) (mediumUploadResult, error) {
			if allocated != id {
				t.Fatal("ID checkpoint must precede image upload")
			}
			stages = append(stages, "upload")
			return mediumUploadResult{}, errors.New("simulated upload failure")
		})
	if err == nil || !strings.Contains(err.Error(), "simulated upload failure") {
		t.Fatalf("partial failure expected: %v", err)
	}
	if result.ID != allocated || result.URL != "https://medium.com/p/abcdef123456/edit" {
		t.Fatalf("draft reference lost: %+v", result)
	}
	if got := strings.Join(stages, ","); got != "create,checkpoint,download,upload" {
		t.Fatalf("unexpected order: %s", got)
	}
}

func TestMediumCreateRejectsPublishAfterWithoutNetwork(t *testing.T) {
	client := &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("draft-only guard must run before Medium network: %s", req.URL)
		return nil, nil
	})}
	_, err := runMediumExplicit(context.Background(), publisher.Session{}, client,
		blogcompiler.CompiledArticle{}, publisher.DraftInput{}, t.TempDir(), "create", explicitTaskTarget{}, true)
	if err == nil || !strings.Contains(err.Error(), "manually") {
		t.Fatalf("Medium must remain draft-only: %v", err)
	}
}

func TestMediumCreateReadbackMismatchPreservesRemoteDraft(t *testing.T) {
	client := &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/new-story":
			return mediumResponse(req, http.StatusOK, "])}while(1);</x>{\"success\":true,\"payload\":{\"value\":{\"id\":\"1111aaaabbbb\"}}}", nil), nil
		case "/p/1111aaaabbbb/deltas":
			return mediumResponse(req, http.StatusOK, "])}while(1);</x>{\"success\":true,\"payload\":{\"value\":{\"latestRev\":1}}}", nil), nil
		case "/_/graphql":
			// Server accepted the Delta request but returned no persisted paragraphs.
			return mediumMockDraftReadback(req, "1111aaaabbbb", nil), nil
		default:
			t.Fatalf("unexpected Medium operation: %s", req.URL)
			return nil, nil
		}
	})}
	draft := mediumDraft{Title: "Check draft", Deltas: []map[string]any{
		{"type": 1, "paragraph": map[string]any{"type": 8, "text": "hello\n  world"}},
	}}
	raw, _ := json.Marshal(draft)
	result, err := runMediumExplicit(context.Background(),
		publisher.Session{Cookies: []publisher.BrowserCookie{{Name: "xsrf", Value: "test", Domain: "medium.com"}}},
		client, blogcompiler.CompiledArticle{Payload: raw, Title: "Check draft"},
		publisher.DraftInput{}, t.TempDir(), "create", explicitTaskTarget{}, false)
	if err == nil || !strings.Contains(err.Error(), "remote content verification failed") {
		t.Fatalf("unverified draft must fail: %v", err)
	}
	if result.ID != "1111aaaabbbb" || result.URL != "https://medium.com/p/1111aaaabbbb/edit" {
		t.Fatalf("lost partial draft reference: %+v", result)
	}
}

func TestMediumBrowserReadbackAcceptsRealGraphQLEnums(t *testing.T) {
	client := &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/_/graphql" {
			t.Fatalf("unexpected endpoint: %s", req.URL)
			return nil, nil
		}
		if req.Header.Get("graphql-operation") != "BlogCTLMediumDraftVerification" {
			t.Fatal("expected existing readback query")
		}
		return mediumResponse(req, http.StatusOK, `[{"data":{"postResult":{"id":"abc12345","content":{"bodyModel":{"paragraphs":[{"type":"H3","text":"Title"},{"type":"P","text":"Body"},{"type":"IMG","text":"","metadata":{"id":"1*persistedimage","originalWidth":1100,"originalHeight":650}},{"type":"P","text":""},{"type":"PRE","text":"foo()\n  bar()"}]}}}}}]`, nil), nil
	})}
	got, err := (mediumClient{httpClient: client}).readDraftParagraphs(context.Background(), platformSession{}, "abc12345")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got[0].Type != 3 || got[1].Type != 1 || got[2].Type != 4 || got[3].Type != 1 || got[4].Type != 8 {
		t.Fatalf("unexpected Medium enum mapping: %+v", got)
	}
	draft := mediumDraft{Title: "Title", BrowserPlan: &mediumBrowserDraftPlan{
		Deltas: []map[string]any{
			{"paragraph": map[string]any{"type": 1, "text": "Body"}},
			{"paragraph": map[string]any{"type": 1, "text": "[BLOGCTL_IMAGE_001]"}},
			{"paragraph": map[string]any{"type": 8, "text": "foo()\n  bar()"}},
		},
		Images: []mediumBrowserDraftImage{{Marker: "[BLOGCTL_IMAGE_001]"}},
	}}
	if err := verifyMediumBrowserSavedParagraphs(draft, got); err != nil {
		t.Fatalf("real Media readback must pass: %v", err)
	}
	badText := append([]mediumSavedParagraph(nil), got...)
	badText[4].Text = "foo() bar()"
	if err := verifyMediumBrowserSavedParagraphs(draft, badText); err == nil {
		t.Fatal("corrupted PRE content accepted")
	}
	badImage := append([]mediumSavedParagraph(nil), got...)
	badImage[2].Metadata.ID = "invalid-image"
	if err := verifyMediumBrowserSavedParagraphs(draft, badImage); err == nil {
		t.Fatal("image without Medium CDN metadata accepted")
	}
	if err := verifyMediumBrowserSavedParagraphs(draft, got[:4]); err == nil {
		t.Fatal("missing final paragraph accepted")
	}
	if err := verifyMediumBrowserSavedParagraphs(draft, append(got, got[2])); err == nil {
		t.Fatal("extra IMG accepted")
	}
	stillMarker := append([]mediumSavedParagraph(nil), got...)
	stillMarker[3].Text = "[BLOGCTL_IMAGE_001]"
	if err := verifyMediumBrowserSavedParagraphs(draft, stillMarker); err == nil {
		t.Fatal("remaining image placeholder accepted")
	}
}

func TestMediumBrowserVerifyAcceptsServerPersistenceAfterDelayedBadge(t *testing.T) {
	draft := mediumDraft{Title: "Title", Deltas: []map[string]any{{"type": 1, "paragraph": map[string]any{"type": 1, "text": "[BLOGCTL_IMAGE_001]"}}},
		BrowserPlan: &mediumBrowserDraftPlan{
			Deltas: []map[string]any{{"type": 1, "paragraph": map[string]any{"type": 1, "text": "[BLOGCTL_IMAGE_001]"}}},
			Images: []mediumBrowserDraftImage{{Marker: "[BLOGCTL_IMAGE_001]", URL: "https://medium.com/cover.png"}},
		},
	}
	calls := []string{}
	client := &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.Path)
		switch req.URL.Path {
		case "/cover.png":
			return mediumResponse(req, http.StatusOK, "image-bytes", http.Header{"Content-Type": {"image/png"}}), nil
		case "/_/graphql":
			return mediumResponse(req, http.StatusOK, `[{"data":{"postResult":{"id":"abc12345","content":{"bodyModel":{"paragraphs":[{"type":"H3","text":"Title"},{"type":"IMG","text":"","metadata":{"id":"1*savedcover","originalWidth":1200,"originalHeight":720}},{"type":"P","text":""}]}}}}}]`, nil), nil
		default:
			t.Fatalf("unexpected browser readback call: %s", req.URL)
			return nil, nil
		}
	})}
	medium := mediumClient{httpClient: client,
		browserInsert: func(_ context.Context, _, _ string, _ publisher.RehostImage) error {
			calls = append(calls, "browser-insert")
			return nil
		},
		browserVerify: func(_ context.Context, _ string, _ int) error {
			calls = append(calls, "browser-verify")
			return errors.New("Medium browser upload failed: Medium draft autosave verification failed: Medium editor has not confirmed autosave")
		},
	}
	_, err := medium.completeDraftBrowserImages(context.Background(), platformSession{}, draft, publisher.DraftInput{}, "abc12345", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(calls, ","), "browser-verify,/_/graphql") {
		t.Fatalf("expected server-authoritative fallback, got %v", calls)
	}
}

func TestMediumBrowserVerifyDoesNotBypassBrokenImages(t *testing.T) {
	medium := mediumClient{browserInsert: func(context.Context, string, string, publisher.RehostImage) error { return nil }, browserVerify: func(context.Context, string, int) error { return errors.New("Medium editor still has 1 missing image") }}
	draft := mediumDraft{Title: "Title", Deltas: []map[string]any{{"paragraph": map[string]any{"type": 1, "text": "body"}}}, BrowserPlan: &mediumBrowserDraftPlan{Deltas: []map[string]any{{"paragraph": map[string]any{"type": 1, "text": "body"}}}}}
	_, err := medium.completeDraftBrowserImages(context.Background(), platformSession{}, draft, publisher.DraftInput{}, "abc12345", "", false)
	if err == nil || !strings.Contains(err.Error(), "1 missing image") {
		t.Fatalf("must preserve safety failure, got %v", err)
	}
}

func TestMediumFullReplacementDiscardAllOldDocumentStyles(t *testing.T) {
	body := []map[string]any{
		{"type": 1, "index": 8, "paragraph": map[string]any{"type": 8, "text": "go func() {\n\tcount++\n}", "markups": []any{}}},
		{"type": 1, "index": 9, "paragraph": map[string]any{"type": 1, "text": "Atomicity", "markups": []any{map[string]any{"type": 1, "start": 0, "end": 9}}}},
	}
	deltas, err := mediumFullReplacementDeltas(3, "Fresh compiler title", body)
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 6 {
		t.Fatalf("wrong replacement size: %d", len(deltas))
	}
	for i := 0; i < 3; i++ {
		if typ, ok := mediumDeltaNumber(deltas[i]["type"]); !ok || typ != 2 || deltas[i]["index"] != 2-i {
			t.Fatalf("old paragraph %d not deleted in reverse order: %+v", i, deltas[i])
		}
	}
	title := deltas[3]["paragraph"].(map[string]any)
	if deltas[3]["index"] != 0 || title["type"] != 3 || title["text"] != "Fresh compiler title" {
		t.Fatalf("source title must be recreated: %+v", deltas[3])
	}
	for i, b := range body {
		p := deltas[i+4]["paragraph"].(map[string]any)
		if deltas[i+4]["index"] != i+1 || p["text"] != b["paragraph"].(map[string]any)["text"] {
			t.Fatalf("fresh body %d is missing: %+v", i, deltas[i+4])
		}
	}
	deltas[4]["paragraph"].(map[string]any)["text"] = "changed"
	if body[0]["paragraph"].(map[string]any)["text"] != "go func() {\n\tcount++\n}" {
		t.Fatal("replacement changed the compiler source")
	}
	for _, tc := range []struct {
		count int
		title string
	}{{0, "valid"}, {3, " "}} {
		if _, err := mediumFullReplacementDeltas(tc.count, tc.title, body); err == nil {
			t.Fatalf("invalid replacement accepted: %+v", tc)
		}
	}
}

func TestMediumSavedInlineStyleVerificationRejectsDroppedOrShiftedMarkup(t *testing.T) {
	raw := []byte(`[{"type":"STRONG","start":0,"end":9},{"type":"CODE","start":12,"end":20},{"type":"A","start":24,"end":28}]`)
	var actual []mediumSavedMarkup
	if err := json.Unmarshal(raw, &actual); err != nil {
		t.Fatal(err)
	}
	expected := []any{
		map[string]any{"type": 10, "start": 12, "end": 20},
		map[string]any{"type": 3, "start": 24, "end": 28},
		map[string]any{"type": 1, "start": 0, "end": 9},
	}
	if err := verifyMediumSavedMarkups(expected, actual); err != nil {
		t.Fatalf("unordered real saved styles should match: %v", err)
	}
	bad := append([]mediumSavedMarkup(nil), actual...)
	bad[1].Start++
	if err := verifyMediumSavedMarkups(expected, bad); err == nil {
		t.Fatal("moved inline code range was accepted")
	}
	if err := verifyMediumSavedMarkups(expected, actual[:2]); err == nil {
		t.Fatal("dropped link formatting was accepted")
	}
	badType := []byte(`[{"type":"UNKNOWN","start":0,"end":9}]`)
	if err := json.Unmarshal(badType, &actual); err == nil {
		t.Fatal("unknown saved markup type accepted")
	}
}

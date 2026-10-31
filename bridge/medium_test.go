package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	blogapp "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/app"
	blogcompiler "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/compiler"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

type mediumRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn mediumRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func mediumResponse(request *http.Request, status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{
		StatusCode: status,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func TestFilterMediumCookies(t *testing.T) {
	got := filterMediumCookies([]browserCookie{
		{Name: "sid", Value: "secret"},
		{Name: "xsrf", Value: "token"},
		{Name: "unrelated", Value: "drop"},
	})
	if len(got) != 2 || got["sid"] != "secret" || got["xsrf"] != "token" {
		t.Fatalf("cookies = %#v", got)
	}
}

func TestMediumSessionCookieHeaderMergesBrowserCookies(t *testing.T) {
	header := mediumSessionCookieHeader(platformSession{
		RequestCookieHeader: "sid=raw-sid; xsrf=raw-xsrf",
		Cookies: map[string]string{
			"sid":          "map-sid",
			"cf_clearance": "clearance-token",
			"_cfuvid":      "visitor-token",
		},
	})

	for name, expected := range map[string]string{
		"sid":          "raw-sid",
		"xsrf":         "raw-xsrf",
		"cf_clearance": "clearance-token",
		"_cfuvid":      "visitor-token",
	} {
		if got := mediumRequestCookieValue(header, name); got != expected {
			t.Fatalf("cookie %s = %q, want %q; header names = %v", name, got, expected, cookieHeaderNames(header))
		}
	}
	if strings.Contains(header, "map-sid") {
		t.Fatalf("request cookie should win over browser cookie; header names = %v", cookieHeaderNames(header))
	}
}

func TestMediumUploadRetriesRejectedPNGAsJPEG(t *testing.T) {
	var pngPayload bytes.Buffer
	pngImage := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pngImage.Set(0, 0, color.RGBA{R: 40, G: 80, B: 120, A: 255})
	if err := png.Encode(&pngPayload, pngImage); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	client := mediumClient{httpClient: &http.Client{Transport: mediumRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		attempts++
		reader, err := request.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		if attempts == 1 {
			if part.Header.Get("Content-Type") != "image/png" {
				t.Fatalf("first content type = %q", part.Header.Get("Content-Type"))
			}
			return mediumResponse(request, http.StatusUnsupportedMediaType, `{"success":false,"error":"unsupported"}`, nil), nil
		}
		if part.Header.Get("Content-Type") != "image/jpeg" || len(payload) < 2 || payload[0] != 0xff || payload[1] != 0xd8 {
			t.Fatalf("JPEG retry = type %q prefix %x", part.Header.Get("Content-Type"), payload[:min(2, len(payload))])
		}
		return mediumResponse(request, http.StatusOK,
			`])}while(1);</x>{"success":true,"payload":{"value":{"fileId":"1*retry.jpg","imgWidth":1,"imgHeight":1}}}`, nil), nil
	})}}
	result, err := client.uploadImage(context.Background(), platformSession{
		Cookies: map[string]string{"xsrf": "token"}, UserAgent: "BlogCTL-Test",
	}, "test-draft", publisher.RehostImage{Source: "cover.png", Payload: pngPayload.Bytes(), ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || result.FileID != "1*retry.jpg" {
		t.Fatalf("attempts=%d result=%#v", attempts, result)
	}
}

func TestStripMediumXSSI(t *testing.T) {
	for _, input := range []string{
		`])}while(1);</x>{"success":true}`,
		`])}while(1);</x>
{"success":true}`,
		`)]}'while(1);</x>
{"success":true}`,
	} {
		if got := stripMediumXSSI(input); got != `{"success":true}` {
			t.Fatalf("stripMediumXSSI(%q) = %q", input, got)
		}
	}
	plain := `{"success":true}`
	if got := stripMediumXSSI(plain); got != plain {
		t.Fatalf("plain response changed to %q", got)
	}
}

func TestMediumCreateDraftUsesCurrentNewStoryFlow(t *testing.T) {
	calls := []string{}
	client := &http.Client{Transport: mediumRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls = append(calls, request.Method+" "+request.URL.Path)
		if request.Header.Get("cookie") != "sid=sid-value; uid=uid-value; xsrf=xsrf-value" {
			t.Fatalf("cookie = %q", request.Header.Get("cookie"))
		}
		if request.Header.Get("x-requested-with") != "XMLHttpRequest" {
			t.Fatalf("x-requested-with = %q", request.Header.Get("x-requested-with"))
		}
		if request.Header.Get("x-obvious-cid") != "web" {
			t.Fatalf("x-obvious-cid = %q", request.Header.Get("x-obvious-cid"))
		}
		if request.Header.Get("x-xsrf-token") != "xsrf-value" {
			t.Fatalf("xsrf = %q", request.Header.Get("x-xsrf-token"))
		}

		switch request.URL.Path {
		case "/new-story":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["baseRev"] != float64(-1) || body["coverless"] != true || body["visibility"] != float64(0) {
				t.Fatalf("new-story body = %#v", body)
			}
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"id":"post-123","mediumUrl":""}}}`,
				nil), nil
		case "/p/post-123/deltas":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["id"] != "post-123" || body["baseRev"] != float64(-1) {
				t.Fatalf("delta body = %#v", body)
			}
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true}`, nil), nil
		default:
			t.Fatalf("unexpected Medium request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	result, err := (mediumClient{httpClient: client}).createDraft(context.Background(), platformSession{
		Cookies: map[string]string{
			"sid": "sid-value", "uid": "uid-value", "xsrf": "xsrf-value",
		},
		UserAgent: "BlogCTL-Test-UA",
	}, mediumDraft{
		Title:  "Title",
		Deltas: []map[string]any{{"type": 1, "paragraph": map[string]any{"type": 1, "text": "Body"}}},
	}, publisher.DraftInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result["postId"] != "post-123" || result["draftUrl"] != "https://medium.com/p/post-123/edit" {
		t.Fatalf("result = %#v", result)
	}
	if strings.Join(calls, ",") != "POST /new-story,POST /p/post-123/deltas" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestMediumPrimesMissingXSRFBeforeWrite(t *testing.T) {
	client := &http.Client{Transport: mediumRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/":
			headers := make(http.Header)
			headers.Add("Set-Cookie", "xsrf=issued-token; Domain=.medium.com; Path=/; Secure")
			return mediumResponse(request, http.StatusOK, "ok", headers), nil
		case "/new-story":
			if request.Header.Get("x-xsrf-token") != "issued-token" {
				t.Fatalf("primed xsrf = %q", request.Header.Get("x-xsrf-token"))
			}
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"id":"post-xsrf"}}}`, nil), nil
		case "/p/post-xsrf/deltas":
			if request.Header.Get("x-xsrf-token") != "issued-token" {
				t.Fatalf("delta xsrf = %q", request.Header.Get("x-xsrf-token"))
			}
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL.String())
			return nil, nil
		}
	})}

	_, err := (mediumClient{httpClient: client}).createDraft(context.Background(), platformSession{
		Cookies: map[string]string{"sid": "sid-value", "uid": "uid-value"},
	}, mediumDraft{Title: "Title", Deltas: []map[string]any{}}, publisher.DraftInput{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMediumDraftCoverImageJSON(t *testing.T) {
	draft := mediumDraft{
		Title:  "Cover test",
		Deltas: []map[string]any{},
		CoverImage: &mediumCoverImage{
			URL: "https://thinkerqaq.github.io/media/articles/test/cover.png",
			Alt: "Test cover",
		},
	}
	payload, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "\"coverImage\"") {
		t.Fatalf("expected coverImage in JSON: %s", payload)
	}
	if !strings.Contains(string(payload), "cover.png") {
		t.Fatalf("expected cover URL in JSON: %s", payload)
	}
}

func installMediumBridgeSession(server *Server) {
	server.sessions["medium"] = platformSession{
		Cookies: map[string]string{
			"sid": "sid-value", "uid": "uid-value", "xsrf": "xsrf-value",
		},
		BrowserCookies: []browserCookie{
			{Name: "sid", Value: "sid-value", Domain: ".medium.com", Path: "/"},
			{Name: "uid", Value: "uid-value", Domain: ".medium.com", Path: "/"},
			{Name: "xsrf", Value: "xsrf-value", Domain: ".medium.com", Path: "/"},
		},
		UserAgent: "BlogCTL-Test-UA",
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

func compiledMediumArticle(t *testing.T, hash string) blogcompiler.CompiledArticle {
	t.Helper()
	payload, err := json.Marshal(mediumDraft{
		Title:        "Medium title",
		Deltas:       []map[string]any{{"type": 1, "paragraph": map[string]any{"type": 1, "text": "Body"}}},
		CanonicalURL: "https://thinkerqaq.github.io/articles/example/",
		Tags:         []string{"go", "concurrency"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return blogcompiler.CompiledArticle{
		Version: 1, Slug: "example", Platform: "medium",
		Title: "Medium title", Markdown: "Body", HTML: "<p>Body</p>",
		Language: "en", ContentHash: hash, SourceDir: "/tmp/articles",
		Payload: payload, FallbackHTML: "<p>Fallback body</p>",
	}
}

func TestBridgeNativePublisherCreatesMediumDraftAndRecordsState(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	installMediumBridgeSession(server)
	server.httpClient = &http.Client{Transport: mediumRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/new-story":
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"id":"post-unified","mediumUrl":""}}}`, nil), nil
		case "/p/post-unified/deltas":
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true}`, nil), nil
		default:
			t.Fatalf("unexpected Medium request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	contentRoot := t.TempDir()
	distributionRoot := filepath.Join(t.TempDir(), "distribution")
	publicationPath := filepath.Join(t.TempDir(), "publications.json")
	server.config.DistributionRoot = distributionRoot
	server.config.PublicationBindingsPath = publicationPath
	compiled := compiledMediumArticle(t, "hash-medium")
	result, err := (bridgeNativePublisher{server: server}).CreateOrUpdateDraft(context.Background(), blogapp.NativeDraftRequest{
		Article: "example", Platform: "medium", ContentRoot: contentRoot, DistributionRoot: distributionRoot, Compiled: compiled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Result != "draft-created" || result.URL != "https://medium.com/p/post-unified/edit" {
		t.Fatalf("result = %#v", result)
	}

	state, _, err := publisher.LoadPublicationState(publicationPath, "example", "medium")
	if err != nil {
		t.Fatal(err)
	}
	if state.RemoteDraftID != "post-unified" || state.DraftURL != result.URL || state.DraftHash != "hash-medium" {
		t.Fatalf("state = %#v", state)
	}
	records, err := publisher.ListPublicationRecords(publicationPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || strings.Join(records[0].PendingFields, ",") != "canonical,tags" {
		t.Fatalf("publication inventory = %#v", records)
	}
	if !strings.Contains(result.Message, "canonical") || !strings.Contains(result.Message, "tags") {
		t.Fatalf("result message = %q", result.Message)
	}

	fallbackPath, err := mediumFallbackPath(distributionRoot, "example")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(fallbackPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != compiled.FallbackHTML {
		t.Fatalf("fallback = %q", raw)
	}
}

func TestBridgeNativePublisherSkipsUnchangedMediumDraft(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	installMediumBridgeSession(server)
	server.httpClient = &http.Client{Transport: mediumRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatalf("unchanged Medium draft should not call network: %s", request.URL.String())
		return nil, nil
	})}

	contentRoot := t.TempDir()
	distributionRoot := filepath.Join(t.TempDir(), "distribution")
	publicationPath := filepath.Join(t.TempDir(), "publications.json")
	server.config.DistributionRoot = distributionRoot
	server.config.PublicationBindingsPath = publicationPath
	if err := publisher.SavePublicationDraftResult(publicationPath, "example", "medium", "same-hash", publisher.DraftResult{
		ID: "post-existing", URL: "https://medium.com/p/post-existing/edit", Created: true,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}

	result, err := (bridgeNativePublisher{server: server}).CreateOrUpdateDraft(context.Background(), blogapp.NativeDraftRequest{
		Article: "example", Platform: "medium", ContentRoot: contentRoot, DistributionRoot: distributionRoot,
		ChangedOnly: false, Compiled: compiledMediumArticle(t, "same-hash"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Result != "skipped" || result.URL != "https://medium.com/p/post-existing/edit" {
		t.Fatalf("result = %#v", result)
	}
}

func TestBridgeNativePublisherUploadsMediumBodyImage(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	installMediumBridgeSession(server)

	contentRoot := t.TempDir()
	distributionRoot := filepath.Join(t.TempDir(), "distribution")
	publicationPath := filepath.Join(t.TempDir(), "publications.json")
	server.config.DistributionRoot = distributionRoot
	server.config.PublicationBindingsPath = publicationPath
	assetDir := filepath.Join(distributionRoot, "assets", "mermaid")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "asset-1.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	uploaded := false
	server.httpClient = &http.Client{Transport: mediumRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/new-story":
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"id":"post-image","mediumUrl":""}}}`, nil), nil
		case "/_/upload":
			uploaded = true
			reader, err := request.MultipartReader()
			if err != nil {
				t.Fatal(err)
			}
			part, err := reader.NextPart()
			if err != nil {
				t.Fatal(err)
			}
			if part.FormName() != "uploadedFile" || part.FileName() != "image.png" ||
				part.Header.Get("Content-Type") != "image/png" {
				t.Fatalf("Medium upload part = name %q file %q type %q", part.FormName(), part.FileName(), part.Header.Get("Content-Type"))
			}
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"fileId":"1*medium.png","imgWidth":1200,"imgHeight":800}}}`, nil), nil
		case "/p/post-image/deltas":
			var requestBody struct {
				Deltas []struct {
					Paragraph struct {
						Type     int            `:"type"`
						Layout   int            `:"layout"`
						Metadata map[string]any `:"metadata"`
					} `:"paragraph"`
				} `:"deltas"`
			}
			if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
				t.Fatal(err)
			}
			foundImage := false
			for _, delta := range requestBody.Deltas {
				if delta.Paragraph.Type != 4 {
					continue
				}
				foundImage = true
				meta := delta.Paragraph.Metadata
				if delta.Paragraph.Layout != 1 || len(meta) != 3 ||
					meta["id"] != "1*medium.png" ||
					meta["originalWidth"] != float64(1200) ||
					meta["originalHeight"] != float64(800) {
					t.Fatalf("image metadata differs from browser's saved Delta: %+v", delta.Paragraph)
				}
			}
			if !foundImage {
				t.Fatal("no native image paragraph was sent")
			}
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"latestRev":4}}}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	payload, err := json.Marshal(mediumDraft{
		Title: "Medium image",
		Deltas: []map[string]any{{
			"type":      1,
			"paragraph": map[string]any{"type": 4, "text": "", "markups": []any{}, "layout": 1, "metadata": map[string]any{}},
			"image":     map[string]any{"url": "blogctl-asset://mermaid/asset-1", "alt": "diagram"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	compiled := compiledMediumArticle(t, "image-hash")
	compiled.Payload = payload
	compiled.Assets = []blogcompiler.Asset{{
		Kind: "mermaid", ID: "asset-1", ObjectKey: "generated/mermaid/asset-1.png",
		PublicURL: "https://assets.example/generated/mermaid/asset-1.png",
		Source:    "blogctl-asset://mermaid/asset-1",
	}}

	result, err := (bridgeNativePublisher{server: server}).CreateOrUpdateDraft(context.Background(), blogapp.NativeDraftRequest{
		Article: "example", Platform: "medium", ContentRoot: contentRoot, DistributionRoot: distributionRoot, Compiled: compiled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !uploaded || result.Result != "draft-created" {
		t.Fatalf("uploaded=%v result=%#v", uploaded, result)
	}
}

func TestBridgeNativePublisherUpdatesChangedMediumDraft(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	installMediumBridgeSession(server)
	var newParagraphs []map[string]any
	server.httpClient = &http.Client{Transport: mediumRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/p/post-existing/notes":
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"post":{"id":"post-existing","latestRev":9,"firstPublishedAt":0,"uniqueSlug":"","mediumUrl":"","creator":{"username":"ThinkerQAQ"}}}}`, nil), nil
		case "/_/graphql":
			if request.Header.Get("graphql-operation") == "BlogCTLMediumDraftVerification" {
				return mediumMockDraftReadback(request, "post-existing", newParagraphs), nil
			}
			return mediumResponse(request, http.StatusOK,
				`[{"data":{"postResult":{"content":{"bodyModel":{"paragraphs":[{"name":"title"},{"name":"body"}]}}}}}]`, nil), nil
		case "/p/post-existing/deltas":
			var body struct {
				BaseRev int              `json:"baseRev"`
				Deltas  []map[string]any `json:"deltas"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.BaseRev != 9 || len(body.Deltas) < 2 {
				t.Fatalf("update body = %#v", body)
			}
			if len(body.Deltas) != 4 || body.Deltas[1]["type"] != float64(2) || body.Deltas[1]["index"] != float64(0) ||
				body.Deltas[2]["type"] != float64(1) || body.Deltas[2]["index"] != float64(0) {
				t.Fatalf("update must discard old title and replace the entire document: %#v", body.Deltas)
			}
			newParagraphs = body.Deltas[2:]
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"latestRev":13}}}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	contentRoot := t.TempDir()
	distributionRoot := filepath.Join(t.TempDir(), "distribution")
	publicationPath := filepath.Join(t.TempDir(), "publications.json")
	server.config.DistributionRoot = distributionRoot
	server.config.PublicationBindingsPath = publicationPath
	if err := publisher.SavePublicationDraftResult(publicationPath, "example", "medium", "old-hash", publisher.DraftResult{
		ID: "post-existing", URL: "https://medium.com/p/post-existing/edit", Created: true,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}

	result, err := (bridgeNativePublisher{server: server}).CreateOrUpdateDraft(context.Background(), blogapp.NativeDraftRequest{
		Article: "example", Platform: "medium", ContentRoot: contentRoot, DistributionRoot: distributionRoot,
		ChangedOnly: false, Compiled: compiledMediumArticle(t, "new-hash"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Result != "updated" || result.URL != "https://medium.com/p/post-existing/edit" {
		t.Fatalf("result = %#v", result)
	}
}

func TestBridgeNativePublisherRefusesToSavePublishedMediumStory(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	installMediumBridgeSession(server)
	deltaWrites := 0
	server.httpClient = &http.Client{Transport: mediumRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/p/post-existing/notes":
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"post":{"id":"post-existing","latestRev":9,"firstPublishedAt":1758600000000,"uniqueSlug":"published-post","mediumUrl":"https://medium.com/@ThinkerQAQ/published-post","creator":{"username":"ThinkerQAQ"}}}}`, nil), nil
		case "/p/post-existing/deltas":
			deltaWrites++
			t.Fatal("draft save must not write deltas to an already-published Medium story")
			return nil, nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	contentRoot := t.TempDir()
	distributionRoot := filepath.Join(t.TempDir(), "distribution")
	publicationPath := filepath.Join(t.TempDir(), "publications.json")
	server.config.DistributionRoot = distributionRoot
	server.config.PublicationBindingsPath = publicationPath
	if err := publisher.SavePublicationBinding(publicationPath, publisher.PublicationBinding{
		Slug: "example", Platform: "medium",
		PublishedRemoteID: "post-existing", PublishedURL: "https://medium.com/@ThinkerQAQ/published-post",
		PublishedHash: "old-hash",
	}); err != nil {
		t.Fatal(err)
	}

	_, err = (bridgeNativePublisher{server: server}).CreateOrUpdateDraft(context.Background(), blogapp.NativeDraftRequest{
		Article: "example", Platform: "medium", ContentRoot: contentRoot, DistributionRoot: distributionRoot,
		ChangedOnly: false, Compiled: compiledMediumArticle(t, "new-hash"),
	})
	if err == nil || !strings.Contains(err.Error(), "already published") {
		t.Fatalf("error = %v, want published-story validation failure", err)
	}
	if deltaWrites != 0 {
		t.Fatalf("published Medium story received %d delta write(s)", deltaWrites)
	}
}

func TestBridgeNativePublisherPublishesMediumDraft(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	installMediumBridgeSession(server)
	server.httpClient = &http.Client{Transport: mediumRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/p/post-existing/notes":
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"post":{"id":"post-existing","latestRev":12,"firstPublishedAt":0,"uniqueSlug":"","mediumUrl":"","creator":{"username":"ThinkerQAQ"}}}}`, nil), nil
		case "/_/graphql":
			return mediumResponse(request, http.StatusOK,
				`[{"data":{"postResult":{"id":"post-existing","title":"Why count++ Breaks Under Concurrency: Atomicity, Visibility & Ordering — Concurrency Programming (1)","previewContent":{"subtitle":"A hardware-first explanation of CPU caches, store buffers, atomic instructions, cache coherence, and memory fences."}}}}]`, nil), nil
		case "/p/post-existing/publish":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["latestRev"] != float64(12) ||
				body["title"] != "Why count++ Breaks Under Concurrency: Atomicity, Visibility & Ordering — Concurrency Programming (1)" ||
				body["subtitle"] != "A hardware-first explanation of CPU caches, store buffers, atomic instructions, cache coherence, and memory fences." {
				t.Fatalf("publish body = %#v", body)
			}
			return mediumResponse(request, http.StatusOK,
				`])}while(1);</x>{"success":true,"payload":{"value":{"id":"post-existing","uniqueSlug":"medium-title-post-existing","mediumUrl":"https://medium.com/@ThinkerQAQ/medium-title-post-existing","creator":{"username":"ThinkerQAQ"}}}}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	contentRoot := t.TempDir()
	distributionRoot := filepath.Join(t.TempDir(), "distribution")
	publicationPath := filepath.Join(t.TempDir(), "publications.json")
	server.config.DistributionRoot = distributionRoot
	server.config.PublicationBindingsPath = publicationPath
	if err := publisher.SavePublicationDraftResult(publicationPath, "example", "medium", "hash-medium", publisher.DraftResult{
		ID: "post-existing", URL: "https://medium.com/p/post-existing/edit", Created: true,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}

	result, err := (bridgeNativePublisher{server: server}).PublishDraft(context.Background(), blogapp.NativePublishRequest{
		Article: "example", Platform: "medium", ContentRoot: contentRoot, DistributionRoot: distributionRoot, Compiled: compiledMediumArticle(t, "hash-medium"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Result != "published" || result.URL != "https://medium.com/@ThinkerQAQ/medium-title-post-existing" {
		t.Fatalf("result = %#v", result)
	}
}

func TestParseMediumStoryLinksFromCapturedLists(t *testing.T) {
	drafts := parseMediumStoryLinks(`
		<a href="https://medium.com/p/1e645140212b/edit?source=your_stories_outbox">TEest</a>
		<a href="https://medium.com/p/6e2fff4d49cd/edit?source=your_stories_outbox">Concurrency Programming (0): The Problem Space and Scope</a>
	`, false)
	if len(drafts) != 2 || drafts[0].ID != "1e645140212b" || drafts[0].Published {
		t.Fatalf("drafts = %#v", drafts)
	}

	published := parseMediumStoryLinks(`
		<a href="https://medium.com/@ThinkerQAQ/why-count-breaks-under-concurrency-atomicity-visibility-ordering-concurrency-programming-1-bce5e98fe815?source=your_stories_outbox">Why count++ Breaks Under Concurrency: Atomicity, Visibility &amp; Ordering — Concurrency Programming…</a>
	`, true)
	if len(published) != 1 || published[0].ID != "bce5e98fe815" || !published[0].Published {
		t.Fatalf("published = %#v", published)
	}
}

func TestMediumListPostsUsesGraphQLWithoutStoriesPage(t *testing.T) {
	calls := []string{}
	client := &http.Client{Transport: mediumRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls = append(calls, request.URL.RequestURI())
		if request.URL.Path != "/_/graphql" {
			t.Fatalf("unexpected Medium lookup request: %s", request.URL.String())
		}
		if request.Header.Get("graphql-operation") == "BlogCTLMediumPublishedQuery" {
			return mediumResponse(request, http.StatusOK, `[{"data":{"viewer":{"id":"user","username":"ThinkerQAQ","latestPostsConnection":{"postPreviews":[{"postId":"bce5e98fe815","post":{"id":"bce5e98fe815","title":"Published title","mediumUrl":"https://medium.com/@ThinkerQAQ/example-bce5e98fe815","uniqueSlug":"example-bce5e98fe815","isPublished":true}}]}}}}]`, nil), nil
		}
		return mediumResponse(request, http.StatusOK, `[{"data":{"viewer":{"id":"user","username":"ThinkerQAQ","latestPostsConnection":{"postPreviews":[{"postId":"1e645140212b","post":{"id":"1e645140212b","title":"Draft title","mediumUrl":"","uniqueSlug":"","isPublished":false}}]}}}}]`, nil), nil
	})}

	account, posts, err := (mediumClient{httpClient: client}).listPosts(context.Background(), platformSession{
		RequestCookieHeader: "sid=session",
		UserAgent:           "UA",
	})
	if err != nil {
		t.Fatal(err)
	}
	if account != "ThinkerQAQ" {
		t.Fatalf("account = %q", account)
	}
	if len(posts) != 2 || posts[0].ID != "1e645140212b" || posts[1].ID != "bce5e98fe815" {
		t.Fatalf("posts = %#v", posts)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestMediumTitleMatchesPublishedListTruncation(t *testing.T) {
	local := "Why count++ Breaks Under Concurrency: Atomicity, Visibility & Ordering — Concurrency Programming (1)"
	remote := "Why count++ Breaks Under Concurrency: Atomicity, Visibility & Ordering — Concurrency Programming…"
	if !mediumTitleMatches(local, remote) {
		t.Fatalf("expected %q to match %q", local, remote)
	}
}

func TestMediumCanonicalMatchesTrackedFooterLink(t *testing.T) {
	canonical := "https://thinkerqaq.github.io/en/articles/concurrency-series-01-hardware/"
	tracked := "https://thinkerqaq.github.io/en/articles/concurrency-series-01-hardware/?utm_source=medium&utm_medium=referral&utm_campaign=article_syndication"
	redirect := "https://medium.com/r/?url=https%3A%2F%2Fthinkerqaq.github.io%2Fen%2Farticles%2Fconcurrency-series-01-hardware%2F%3Futm_source%3Dmedium%26utm_medium%3Dreferral%26utm_campaign%3Darticle_syndication"

	if !mediumCanonicalMatches(tracked, canonical) {
		t.Fatal("tracked canonical URL should match source canonical")
	}
	if !mediumCanonicalMatches(redirect, canonical) {
		t.Fatal("Medium redirect URL should match source canonical")
	}
	if mediumCanonicalMatches("https://thinkerqaq.github.io/en/articles/other/", canonical) {
		t.Fatal("unrelated canonical URL must not match")
	}
}

// A gateway timeout is not a media format error. Do not waste another 10s
// sending JPEG to the same unavailable proxy/upstream endpoint.
func TestMediumDoesNotRetryGatewayFailureAsJPEG(t *testing.T) {
	calls := 0
	client := mediumClient{httpClient: &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return mediumResponse(req, http.StatusGatewayTimeout, "{\"error\":\"upstream timeout\"}", nil), nil
	})}}
	var pngBuffer bytes.Buffer
	sample := image.NewRGBA(image.Rect(0, 0, 3, 3))
	if err := png.Encode(&pngBuffer, sample); err != nil {
		t.Fatal(err)
	}
	_, err := client.uploadImage(context.Background(), platformSession{Cookies: map[string]string{"xsrf": "test"}}, "test-draft",
		publisher.RehostImage{Payload: pngBuffer.Bytes(), ContentType: "image/png"})
	if !mediumUploadGatewayFailure(err) {
		t.Fatalf("expected typed gateway failure: %v", err)
	}
	if calls != 1 {
		t.Fatalf("gateway error must not trigger JPEG retry, got %d uploads", calls)
	}
}

func TestMediumImageGatewayFailureDoesNotWriteBrokenDraft(t *testing.T) {
	var created, saved, uploaded int
	client := mediumClient{httpClient: &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/_/upload":
			uploaded++
			return mediumResponse(req, http.StatusGatewayTimeout, "gateway unavailable", nil), nil
		case "/new-story":
			created++
			return mediumResponse(req, http.StatusOK, `])}while(1);</x>{"success":true,"payload":{"value":{"id":"new-post"}}}`, nil), nil
		case "/p/new-post/deltas":
			saved++
			return mediumResponse(req, http.StatusOK, `])}while(1);</x>{"success":true,"payload":{"value":{"latestRev":1}}}`, nil), nil
		default:
			t.Fatalf("unexpected endpoint %q", req.URL.Path)
			return nil, nil
		}
	})}}
	var pngImage bytes.Buffer
	if err := png.Encode(&pngImage, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	assetRoot := t.TempDir()
	assetDir := filepath.Join(assetRoot, "assets", "mermaid")
	if err := os.MkdirAll(assetDir, 0700); err != nil {
		t.Fatal(err)
	}
	pngFile := filepath.Join(assetDir, "test.png")
	if err := os.WriteFile(pngFile, pngImage.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	draft := mediumDraft{Title: "Demo", Deltas: []map[string]any{{
		"type": 1, "paragraph": map[string]any{"type": 4, "text": ""},
		"image": map[string]any{"url": "blogctl-asset://mermaid/test", "alt": "must remain an image"},
	}}}
	result, err := client.createDraft(context.Background(), platformSession{Cookies: map[string]string{"xsrf": "test"}}, draft,
		publisher.DraftInput{DistributionRoot: assetRoot, Assets: []publisher.PublishingAsset{{Kind: "mermaid", ID: "test", Source: "blogctl-asset://mermaid/test"}}})
	if err == nil || !strings.Contains(err.Error(), "refusing to replace image") {
		t.Fatalf("must reject lossy image replacement: %v", err)
	}
	if uploaded != 1 || created != 1 || saved != 0 {
		t.Fatalf("should not save broken image paragraphs: upload=%d created=%d saved=%d", uploaded, created, saved)
	}
	if result["postId"] != "new-post" {
		t.Fatalf("lost recoverable draft ID: %+v", result)
	}
}

// Non-secret HTTP details from a successful Medium browser HAR: POST
// /_/upload?source=6 with a draft Referer, multipart file, and Accept */*.
func TestMediumUploadMatchesSuccessfulBrowserHAR(t *testing.T) {
	requestCount := 0
	fake := mediumClient{httpClient: &http.Client{Transport: mediumRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		if req.URL.Host != "medium.com" || req.URL.Path != "/_/upload" ||
			req.URL.Query().Get("source") != "6" || req.Method != http.MethodPost {
			t.Fatalf("upload endpoint differs from HAR: %s %s", req.Method, req.URL.String())
		}
		if got := req.Header.Get("Referer"); got != "https://medium.com/p/test-existing/edit" {
			t.Fatalf("upload referer differs from HAR: %s", got)
		}
		if req.Header.Get("Accept") != "*/*" ||
			req.Header.Get("Cache-Control") != "no-cache" ||
			req.Header.Get("Pragma") != "no-cache" ||
			req.Header.Get("Dnt") != "1" ||
			req.Header.Get("x-requested-with") != "" || req.Header.Get("x-client-date") != "" {
			t.Fatalf("upload headers differ from HAR")
		}
		if req.Header.Get("x-obvious-cid") != "web" ||
			req.Header.Get("x-xsrf-token") != "example-xsrf" {
			t.Fatalf("missing expected Medium upload headers")
		}
		multipart, err := req.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		part, err := multipart.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		if part.FormName() != "uploadedFile" || part.Header.Get("content-type") != "image/png" {
			t.Fatalf("multipart differs from HAR: %s %s", part.FormName(), part.Header.Get("content-type"))
		}
		return mediumResponse(req, http.StatusOK,
			"])}while(1);</x>{\"success\":true,\"payload\":{\"value\":{\"fileId\":\"1*example.png\",\"imgWidth\":1200,\"imgHeight\":800}}}", nil), nil
	})}}
	var sample bytes.Buffer
	if err := png.Encode(&sample, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	result, err := fake.uploadImageOnce(context.Background(), platformSession{
		Cookies: map[string]string{"xsrf": "example-xsrf"}, UserAgent: "Mozilla/5.0 Chrome/155.0.0.0",
	}, "test-existing", publisher.RehostImage{Source: "diagram.png", Payload: sample.Bytes(), ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 1 || result.FileID != "1*example.png" || result.Width != 1200 || result.Height != 800 {
		t.Fatalf("upload result: %+v requests=%d", result, requestCount)
	}
}

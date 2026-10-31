package publisher

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCNBlogsPayloadMatchesBrowserDraftContract locks cnBlogsPayload to the
// real browser editor contract captured from https://i.cnblogs.com/api/posts
// (see resources/i.cnblogs.com.har). postType must be 1, the standard blog
// post the CNBlogs editor submits — not the stale value 2 this adapter used
// to emit, which the browser never sends.
func TestCNBlogsPayloadMatchesBrowserDraftContract(t *testing.T) {
	input := DraftInput{Title: "test"}
	payload := cnBlogsPayload("", input, "test", false)

	for field, want := range map[string]any{
		"postType":                 1,
		"usingEditorId":            5,
		"isMarkdown":               true,
		"isDraft":                  true,
		"isPublished":              false,
		"isAigc":                   false,
		"inSiteHome":               true,
		"displayOnHomePage":        true,
		"includeInMainSyndication": true,
		"blogId":                   0,
		"canChangeCreatedTime":     false,
	} {
		if payload[field] != want {
			t.Fatalf("%s = %v, want %v", field, payload[field], want)
		}
	}
	if payload["id"] != nil {
		t.Fatalf("new draft id = %v, want nil", payload["id"])
	}
	for _, field := range []string{"url", "categoryIds", "categories", "blogTeamIds", "description", "tags", "dateUpdated", "author", "autoDesc"} {
		if payload[field] != nil {
			t.Fatalf("%s = %#v, want nil for a browser-created draft", field, payload[field])
		}
	}
	if got := payload["collectionIds"].([]any); len(got) != 0 {
		t.Fatalf("collectionIds = %#v, want empty array", got)
	}
	if value, ok := payload["datePublished"].(string); !ok {
		t.Fatalf("datePublished = %#v, want string", payload["datePublished"])
	} else if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		t.Fatalf("datePublished = %q: %v", value, err)
	}
}

func cnBlogsSession() Session {
	expiry := float64(time.Now().Add(time.Hour).Unix())
	return Session{
		UserAgent: "BlogCTL-Test-UA",
		Cookies: []BrowserCookie{
			{Name: "XSRF-TOKEN", Value: "test-xsrf", Domain: "i.cnblogs.com", Path: "/", Secure: true, HTTPOnly: true, ExpirationDate: &expiry},
			{Name: ".Cnblogs.Account.Identifier", Value: "secret", Domain: ".cnblogs.com", Path: "/", Secure: true, ExpirationDate: &expiry},
		},
	}
}

func cnBlogsAuthClient(t *testing.T, status int, body string) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/user" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
		}
		return jsonResponse(request, status, body, nil), nil
	})}
}

// TestCNBlogsCheckAuthDecidesFromAPIUser locks auth to GET /api/user: 200 with a
// non-empty loginName means logged in; empty loginName or 401/403 produce an
// auth-expired error carrying the underlying status for diagnostics.
func TestCNBlogsCheckAuthDecidesFromAPIUser(t *testing.T) {
	adapter, err := NewCNBlogsAdapter(cnBlogsAuthClient(t, 200, `{"loginName":"ThinkerQAQ","displayName":"TK"}`), cnBlogsSession())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.CheckAuth(context.Background())
	if err != nil {
		t.Fatalf("CheckAuth error: %v", err)
	}
	if !result.Authenticated || result.UserID != "ThinkerQAQ" {
		t.Fatalf("result = %#v, want authenticated ThinkerQAQ", result)
	}

	for name, tc := range map[string]struct {
		status int
		body   string
	}{
		"empty loginName": {200, `{"loginName":"","displayName":""}`},
		"unauthorized":    {401, `{"errors":["Unauthorized"],"type":1}`},
		"forbidden":       {403, `{"errors":["Forbidden"]}`},
	} {
		t.Run(name, func(t *testing.T) {
			adapter, err := NewCNBlogsAdapter(cnBlogsAuthClient(t, tc.status, tc.body), cnBlogsSession())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.CheckAuth(context.Background()); err == nil || !IsKind(err, ErrAuthExpired) {
				t.Fatalf("err = %v, want auth-expired", err)
			}
		})
	}
}

func TestCNBlogsUsesCapturedBrowserCookieHeaderForAPI(t *testing.T) {
	const captured = ".Cnblogs.AspNetCore.Cookies=login; .CNBlogsCookie=legacy"
	session := Session{UserAgent: "BlogCTL-Test-UA", RequestCookieHeader: captured}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/api/user" || request.Header.Get("Cookie") != captured {
			t.Fatal("CNBlogs auth request did not contain the captured browser Cookie header")
		}
		return jsonResponse(request, http.StatusOK, `{"loginName":"ThinkerQAQ"}`, nil), nil
	})}
	adapter, err := NewCNBlogsAdapter(client, session)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CheckAuth(context.Background()); err != nil {
		t.Fatal(err)
	}
	uploadRequest, err := adapter.(*cnBlogsAdapter).request(context.Background(), http.MethodGet, "https://upload.cnblogs.com/v2/images/cors-upload", nil)
	if err != nil {
		t.Fatal(err)
	}
	if uploadRequest.Header.Get("Cookie") != "" {
		t.Fatal("CNBlogs editor Cookie header was sent to the upload host")
	}
}

func TestCNBlogsUsesHostScopedCapturedCookieHeaderForUpload(t *testing.T) {
	const editorCookie = ".Cnblogs.AspNetCore.Cookies=editor-login"
	const uploadCookie = ".Cnblogs.Upload.Cookies=upload-login"
	session := Session{
		UserAgent:           "BlogCTL-Test-UA",
		RequestCookieHeader: editorCookie,
		RequestCookieHeaders: map[string]string{
			"upload.cnblogs.com": uploadCookie,
		},
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return jsonResponse(request, http.StatusOK, `{}`, nil), nil
	})}
	adapter, err := NewCNBlogsAdapter(client, session)
	if err != nil {
		t.Fatal(err)
	}
	uploadRequest, err := adapter.(*cnBlogsAdapter).request(context.Background(), http.MethodGet, "https://upload.cnblogs.com/v2/images/cors-upload", nil)
	if err != nil {
		t.Fatal(err)
	}
	if uploadRequest.Header.Get("Cookie") != uploadCookie {
		t.Fatalf("upload Cookie = %q, want host-scoped captured Cookie", uploadRequest.Header.Get("Cookie"))
	}
	editorRequest, err := adapter.(*cnBlogsAdapter).request(context.Background(), http.MethodGet, "https://i.cnblogs.com/api/user", nil)
	if err != nil {
		t.Fatal(err)
	}
	if editorRequest.Header.Get("Cookie") != editorCookie {
		t.Fatalf("editor Cookie = %q, want editor captured Cookie", editorRequest.Header.Get("Cookie"))
	}
}

// TestCNBlogsCheckAuthRejectsInvalidJSON ensures a 200 with non-JSON body is an
// upstream error, never a false "logged in".
func TestCNBlogsCheckAuthRejectsInvalidJSON(t *testing.T) {
	adapter, err := NewCNBlogsAdapter(cnBlogsAuthClient(t, 200, `not-json`), cnBlogsSession())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CheckAuth(context.Background()); err == nil || !IsKind(err, ErrUpstream) {
		t.Fatalf("err = %v, want upstream error", err)
	}
}

// TestCNBlogsUpdatePayloadPreservesServerFields locks the update payload to the
// server's blogPost: server-owned fields survive while only the edited
// title/body/description/draft-state are overwritten.
func TestCNBlogsUpdatePayloadPreservesServerFields(t *testing.T) {
	base := map[string]any{
		"id":                float64(23036002),
		"url":               "https://www.cnblogs.com/ThinkerQAQ/p/23036002",
		"datePublished":     "2026-09-19T10:15:00",
		"dateUpdated":       "2026-09-19T15:07:00",
		"blogId":            float64(824919),
		"author":            "ThinkerQAQ",
		"autoDesc":          "test",
		"displayOnHomePage": true,
		"usingEditorId":     nil,
	}
	input := DraftInput{Title: "new title", Description: "new desc", Markdown: "new body"}
	payload := cnBlogsUpdatePayload("23036002", input, "new body", false, base)

	for field, want := range map[string]any{
		"id":            float64(23036002),
		"url":           "https://www.cnblogs.com/ThinkerQAQ/p/23036002",
		"datePublished": "2026-09-19T10:15:00",
		"dateUpdated":   "2026-09-19T15:07:00",
		"blogId":        float64(824919),
		"author":        "ThinkerQAQ",
		"autoDesc":      "test",
	} {
		if payload[field] != want {
			t.Fatalf("%s = %v, want %v", field, payload[field], want)
		}
	}
	for field, want := range map[string]any{
		"title":         "new title",
		"postBody":      "new body",
		"description":   "new desc",
		"isPublished":   false,
		"isDraft":       true,
		"usingEditorId": 5,
	} {
		if payload[field] != want {
			t.Fatalf("%s = %v, want %v", field, payload[field], want)
		}
	}
	if payload["displayOnHomePage"] != true {
		t.Fatalf("displayOnHomePage = %v, want true (preserved)", payload["displayOnHomePage"])
	}
}

func TestCNBlogsPublishedUpdateMatchesCapturedRepublishContract(t *testing.T) {
	base := map[string]any{
		"id":                       float64(23039631),
		"postType":                 float64(1),
		"accessPermission":         float64(0),
		"title":                    "并发编程（三）",
		"url":                      "https://www.cnblogs.com/ThinkerQAQ/p/23039631",
		"postBody":                 "old body",
		"categoryIds":              []any{},
		"categories":               nil,
		"collectionIds":            []any{float64(44484)},
		"inSiteCandidate":          false,
		"inSiteHome":               true,
		"siteCategoryId":           float64(106876),
		"blogTeamIds":              []any{},
		"isPublished":              true,
		"displayOnHomePage":        true,
		"isAllowComments":          true,
		"includeInMainSyndication": false,
		"isPinned":                 false,
		"showBodyWhenPinned":       false,
		"isOnlyForRegisterUser":    false,
		"isUpdateDateAdded":        false,
		"description":              "",
		"featuredImage":            nil,
		"tags":                     []any{},
		"publishAt":                nil,
		"datePublished":            "2026-09-19T12:08:00.000Z",
		"dateUpdated":              "2026-09-19T20:10:00",
		"isMarkdown":               true,
		"isDraft":                  false,
		"isAigc":                   false,
		"autoDesc":                 "existing auto description",
		"blogId":                   float64(824919),
		"author":                   "ThinkerQAQ",
		"usingEditorId":            nil,
		"sourceUrl":                nil,
	}
	input := DraftInput{
		Title:       "并发编程（三）：互斥锁——语言层的原子性、可见性与有序性 · ThinkerQAQ",
		Description: "",
	}
	payload := cnBlogsUpdatePayload("23039631", input, "updated body", true, base)

	// The 2026-09-21 capture successfully updated an already-published post
	// by POSTing the same id with isPublished=true and isDraft=false.
	for field, want := range map[string]any{
		"id":                       float64(23039631),
		"url":                      "https://www.cnblogs.com/ThinkerQAQ/p/23039631",
		"isPublished":              true,
		"isDraft":                  false,
		"inSiteHome":               true,
		"includeInMainSyndication": false,
		"datePublished":            "2026-09-19T12:08:00.000Z",
		"dateUpdated":              "2026-09-19T20:10:00",
		"blogId":                   float64(824919),
		"author":                   "ThinkerQAQ",
		"usingEditorId":            5,
	} {
		if payload[field] != want {
			t.Fatalf("%s = %v, want %v", field, payload[field], want)
		}
	}
	if payload["title"] != input.Title || payload["postBody"] != "updated body" {
		t.Fatalf("updated title/body = %v / %v", payload["title"], payload["postBody"])
	}
	if got := payload["collectionIds"].([]any); len(got) != 1 || got[0] != float64(44484) {
		t.Fatalf("collectionIds = %#v, want preserved capture value", got)
	}
}

// TestCNBlogsUpdateDraftFetchesThenPostsServerFields verifies the update path
// GETs the existing post and echoes server fields into the save request.
func TestCNBlogsSaveUsesCapturedEditorSessionHeader(t *testing.T) {
	var sessionID string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/posts/edit":
			return jsonResponse(request, 200, "", nil), nil
		case request.Method == http.MethodPost && request.URL.Path == "/api/posts":
			sessionID = request.Header.Get("sessionId")
			return jsonResponse(request, 200, `{"id":23070000}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewCNBlogsAdapter(client, cnBlogsSession())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CreateDraft(context.Background(), DraftInput{Title: "new draft", Markdown: "body"}); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(sessionID, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Fatalf("sessionId = %q, want UUID-shaped editor session id", sessionID)
	}
}

func TestCNBlogsUpdateDraftFetchesThenPostsServerFields(t *testing.T) {
	var posted map[string]any
	fetchCalled := false
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/posts/edit":
			return jsonResponse(request, 200, "", nil), nil
		case request.Method == http.MethodGet && request.URL.Path == "/api/posts/23036002":
			fetchCalled = true
			return jsonResponse(request, 200, `{"blogPost":{"id":23036002,"url":"https://www.cnblogs.com/ThinkerQAQ/p/23036002","postBody":"old","datePublished":"2026-09-19T10:15:00","dateUpdated":"2026-09-19T15:07:00","blogId":824919,"author":"ThinkerQAQ","autoDesc":"test","usingEditorId":null}}`, nil), nil
		case request.Method == http.MethodPost && request.URL.Path == "/api/posts":
			if err := json.NewDecoder(request.Body).Decode(&posted); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `{"id":23036002}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewCNBlogsAdapter(client, cnBlogsSession())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.UpdateDraft(context.Background(), DraftRef{ID: "23036002"}, DraftInput{Title: "updated", Markdown: "updated body"}); err != nil {
		t.Fatal(err)
	}
	if !fetchCalled {
		t.Fatal("expected GET /api/posts/{id} before update")
	}
	for field, want := range map[string]any{
		"url":           "https://www.cnblogs.com/ThinkerQAQ/p/23036002",
		"datePublished": "2026-09-19T10:15:00",
		"dateUpdated":   "2026-09-19T15:07:00",
		"blogId":        float64(824919),
		"author":        "ThinkerQAQ",
		"autoDesc":      "test",
	} {
		if posted[field] != want {
			t.Fatalf("post %s = %v, want %v", field, posted[field], want)
		}
	}
	if posted["title"] != "updated" || posted["postBody"] != "updated body" {
		t.Fatalf("post title/body = %v/%v, want updated", posted["title"], posted["postBody"])
	}
}

func imageResponse(request *http.Request, status int, contentType string, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func multipartRequestParts(t *testing.T, request *http.Request) (map[string]string, string, string, []byte) {
	t.Helper()
	reader, err := request.MultipartReader()
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{}
	fileField := ""
	filename := ""
	var filePayload []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		if part.FileName() != "" {
			fileField = part.FormName()
			filename = part.FileName()
			filePayload = payload
			continue
		}
		fields[part.FormName()] = string(payload)
	}
	if filename == "" {
		t.Fatal("multipart request did not contain a file")
	}
	return fields, fileField, filename, filePayload
}

func TestCNBlogsImageUploadUsesV2BrowserContract(t *testing.T) {
	const source = "https://assets.example.com/diagram.png"
	const uploaded = "https://img2024.cnblogs.com/blog/3466743/202609/diagram.png"
	const xsrf = "capture-token"
	v2Called := false

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Hostname() {
		case "assets.example.com":
			return imageResponse(request, http.StatusOK, "image/png", "png-bytes"), nil
		case "upload.cnblogs.com":
			if request.URL.Path != "/v2/images/cors-upload" {
				t.Fatalf("unexpected upload path: %s", request.URL.Path)
			}
			v2Called = true
			if request.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", request.Method)
			}
			if request.Header.Get("accept") != "application/json, text/plain, */*" {
				t.Fatalf("accept = %q", request.Header.Get("accept"))
			}
			if request.Header.Get("origin") != cnBlogsOrigin || request.Header.Get("referer") != cnBlogsOrigin+"/" {
				t.Fatalf("origin/referer = %q/%q", request.Header.Get("origin"), request.Header.Get("referer"))
			}
			if request.Header.Get("x-xsrf-token") != xsrf {
				t.Fatalf("x-xsrf-token = %q, want capture token", request.Header.Get("x-xsrf-token"))
			}
			fields, field, filename, payload := multipartRequestParts(t, request)
			if field != "image" || filename != "image.png" || string(payload) != "png-bytes" {
				t.Fatalf("multipart file = %q %q %q", field, filename, string(payload))
			}
			if fields["app"] != "blog" || fields["uploadType"] != "Select" {
				t.Fatalf("multipart fields = %#v, want app=blog and uploadType=Select", fields)
			}
			return jsonResponse(request, http.StatusOK, `{"success":true,"message":"`+uploaded+`"}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	session := cnBlogsSession()
	session.Cookies = session.Cookies[1:]
	session.RequestCookieHeader = "XSRF-TOKEN=" + xsrf + "; .CNBlogsCookie=login"
	adapter, err := NewCNBlogsAdapter(client, session)
	if err != nil {
		t.Fatal(err)
	}
	target, err := adapter.(*cnBlogsAdapter).uploadImage(context.Background(), RehostImage{Source: source, Payload: []byte("png-bytes"), ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	if !v2Called || target != uploaded {
		t.Fatalf("target = %q, called = %v", target, v2Called)
	}
}

func TestCNBlogsKeepsRemoteR2ImageWhenCNBlogsUploadIsUnavailable(t *testing.T) {
	const source = "https://pub-example.r2.dev/publishing/mermaid/diagram.png"
	markdown := "before\n\n![diagram](" + source + ")\n\nafter"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Hostname() {
		case "pub-example.r2.dev":
			return imageResponse(request, http.StatusOK, "image/png", "png-bytes"), nil
		case "upload.cnblogs.com":
			return jsonResponse(request, http.StatusServiceUnavailable, `{"message":"temporary unavailable"}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewCNBlogsAdapter(client, cnBlogsSession())
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.(*cnBlogsAdapter).prepareMarkdown(context.Background(), DraftInput{Markdown: markdown})
	if err != nil {
		t.Fatal(err)
	}
	if got != markdown {
		t.Fatalf("markdown changed despite upload outage:\n%s", got)
	}
}

func TestCNBlogsPublishRehostsCompilerAssets(t *testing.T) {
	root := t.TempDir()
	assetDir := filepath.Join(root, "assets", "mermaid")
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "asset-1.png"), []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}

	publishedBody := ""
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/v2/images/cors-upload":
			return jsonResponse(request, 200, `{"url":"https://img2026.cnblogs.com/blog/diagram.png"}`, nil), nil
		case "/api/posts/42":
			return jsonResponse(request, 200, `{"blogPost":{
				"id":42,
				"title":"Example",
				"postBody":"old",
				"url":"https://www.cnblogs.com/ThinkerQAQ/p/42",
				"isPublished":false,
				"isDraft":true,
				"author":"ThinkerQAQ",
				"blogId":824919,
				"datePublished":"2026-09-23T00:00:00.000Z",
				"dateUpdated":"2026-09-23T00:00:00.000Z"
			}}`, nil), nil
		case "/api/posts":
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			publishedBody = valueString(payload["postBody"])
			return jsonResponse(request, 200, `{"id":42,"url":"https://www.cnblogs.com/ThinkerQAQ/p/42.html"}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewCNBlogsAdapter(client, cnBlogsSession())
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.PublishDraft(context.Background(), DraftRef{ID: "42"}, DraftInput{
		Title:            "Example",
		Markdown:         "![diagram](blogctl-asset://mermaid/asset-1)",
		DistributionRoot: root,
		Assets: []PublishingAsset{{
			Kind: "mermaid", ID: "asset-1",
			Source: "blogctl-asset://mermaid/asset-1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(publishedBody, "blogctl-asset://") {
		t.Fatalf("internal asset leaked into publish payload: %q", publishedBody)
	}
	if !strings.Contains(publishedBody, "https://img2026.cnblogs.com/blog/diagram.png") {
		t.Fatalf("publish payload did not use hosted image: %q", publishedBody)
	}
}

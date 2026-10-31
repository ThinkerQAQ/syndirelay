package publisher

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(request *http.Request, status int, body string, headers map[string]string) *http.Response {
	header := make(http.Header)
	header.Set("content-type", "application/json")
	for name, value := range headers {
		header.Set(name, value)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func juejinSession() Session {
	expiry := float64(time.Now().Add(time.Hour).Unix())
	return Session{
		UserAgent: "BlogCTL-Test-UA",
		Cookies: []BrowserCookie{{
			Name: "sessionid", Value: "secret-session", Domain: ".juejin.cn", Path: "/",
			Secure: true, HTTPOnly: true, ExpirationDate: &expiry,
		}},
	}
}

func TestJuejinCreateDraftUsesBrowserSessionAndCSRF(t *testing.T) {
	var createBody map[string]any
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/user_api/v1/user/get":
			if !strings.Contains(request.Header.Get("cookie"), "sessionid=secret-session") {
				t.Fatalf("auth cookie = %q", request.Header.Get("cookie"))
			}
			return jsonResponse(request, 200, `{"data":{"user_id":"u1","user_name":"tester"}}`, nil), nil
		case "/user_api/v1/sys/token":
			if request.Method != http.MethodHead {
				t.Fatalf("csrf method = %s", request.Method)
			}
			return jsonResponse(request, 200, "", map[string]string{
				"x-ware-csrf-token": "0,csrf-value,86370000,success,session",
			}), nil
		case "/content_api/v1/article_draft/create":
			if request.Method != http.MethodPost {
				t.Fatalf("create method = %s", request.Method)
			}
			if request.Header.Get("origin") != juejinOrigin || request.Header.Get("referer") != juejinOrigin+"/" {
				t.Fatalf("origin/referer = %q / %q", request.Header.Get("origin"), request.Header.Get("referer"))
			}
			if request.Header.Get("x-secsdk-csrf-token") != "csrf-value" {
				t.Fatalf("csrf header = %q", request.Header.Get("x-secsdk-csrf-token"))
			}
			if err := json.NewDecoder(request.Body).Decode(&createBody); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `{"err_no":0,"data":{"id":"draft-123"}}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewJuejinAdapter(client, juejinSession())
	if err != nil {
		t.Fatal(err)
	}
	auth, err := adapter.CheckAuth(context.Background())
	if err != nil || !auth.Authenticated || auth.UserID != "u1" {
		t.Fatalf("auth = %#v, err = %v", auth, err)
	}
	result, err := adapter.CreateDraft(context.Background(), DraftInput{
		Title: "Title", Description: "Description", Markdown: "# Body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "draft-123" || result.URL != "https://juejin.cn/editor/drafts/draft-123" || !result.Created {
		t.Fatalf("result = %#v", result)
	}
	if createBody["title"] != "Title" || createBody["mark_content"] != "# Body" || createBody["edit_type"] != float64(10) {
		t.Fatalf("create body = %#v", createBody)
	}
}

func TestJuejinUpdateDraftPreservesCapturedRemoteFields(t *testing.T) {
	var updateBody map[string]any
	detailCalls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/user_api/v1/sys/token":
			return jsonResponse(request, 200, "", map[string]string{"x-ware-csrf-token": "0,csrf,1,success,x"}), nil
		case "/content_api/v1/article_draft/detail":
			detailCalls++
			return jsonResponse(request, 200, `{
				"err_no":0,
				"data":{
					"draft_id":"existing",
					"article_draft":{
						"id":"existing",
						"article_id":"published-1",
						"category_id":"6809637769959178254",
						"tag_ids":[6809640408797167623,6809640445233070094],
						"link_url":"",
						"cover_image":"cover",
						"is_gfw":0,
						"is_english":0,
						"is_original":1,
						"edit_type":10,
						"theme_ids":[7275231252674773028],
						"pics":[{"pic_url":"https://example.com/p.png","pic_uri":"uri/p.png"}]
					},
					"columns":[{"column":{"column_id":"7684228650583015430"}}],
					"theme_list":[{"theme":{"theme_id":"7275231252674773028"}}]
				}
			}`, nil), nil
		case "/content_api/v1/article_draft/update":
			if err := json.NewDecoder(request.Body).Decode(&updateBody); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `{"err_no":0,"data":{}}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL.String())
			return nil, nil
		}
	})}
	adapter, err := NewJuejinAdapter(client, juejinSession())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.UpdateDraft(context.Background(), DraftRef{ID: "existing"}, DraftInput{
		Title: "Updated", Description: "Summary", Markdown: "Body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.ID != "existing" || detailCalls != 1 {
		t.Fatalf("result = %#v, detailCalls = %d", result, detailCalls)
	}
	if updateBody["id"] != "existing" ||
		updateBody["category_id"] != "6809637769959178254" ||
		updateBody["cover_image"] != "cover" ||
		updateBody["is_original"] != float64(1) {
		t.Fatalf("update body = %#v", updateBody)
	}
	tags := updateBody["tag_ids"].([]any)
	if len(tags) != 2 || tags[0] != "6809640408797167623" || tags[1] != "6809640445233070094" {
		t.Fatalf("tag_ids = %#v", tags)
	}
	themes := updateBody["theme_ids"].([]any)
	if len(themes) != 1 || themes[0] != "7275231252674773028" {
		t.Fatalf("theme_ids = %#v", themes)
	}
	pics := updateBody["pics"].([]any)
	if len(pics) != 1 {
		t.Fatalf("pics = %#v", pics)
	}
}

func TestJuejinPublishUsesCapturedColumnsThemesAndRepublishesExistingArticle(t *testing.T) {
	var publishBody map[string]any
	publishCalls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/user_api/v1/sys/token":
			return jsonResponse(request, 200, "", map[string]string{"x-ware-csrf-token": "0,csrf,1,success,x"}), nil
		case "/content_api/v1/article_draft/detail":
			return jsonResponse(request, 200, `{
				"err_no":0,
				"data":{
					"draft_id":"7686808742222774291",
					"article_draft":{
						"id":"7686808742222774291",
						"article_id":"7686846675643547711",
						"category_id":"6809637769959178254",
						"tag_ids":[6809640408797167623,6809640445233070094],
						"is_original":1,
						"edit_type":10,
						"theme_ids":[7275231252674773028]
					},
					"columns":[{"column":{"column_id":"7684228650583015430"}}],
					"theme_list":[{"theme":{"theme_id":"7275231252674773028"}}]
				}
			}`, nil), nil
		case "/content_api/v1/article/publish":
			publishCalls++
			if err := json.NewDecoder(request.Body).Decode(&publishBody); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `{"err_no":0,"err_msg":"success","data":{"article_id":"7686846675643547711"}}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewJuejinAdapter(client, juejinSession())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.PublishDraft(context.Background(), DraftRef{ID: "7686808742222774291"}, DraftInput{})
	if err != nil {
		t.Fatal(err)
	}
	if publishCalls != 1 {
		t.Fatalf("publishCalls = %d, want 1 even when article_id already exists", publishCalls)
	}
	if result.URL != "https://juejin.cn/post/7686846675643547711" {
		t.Fatalf("result = %#v", result)
	}
	if publishBody["draft_id"] != "7686808742222774291" || publishBody["sync_to_org"] != false {
		t.Fatalf("publish body = %#v", publishBody)
	}
	columns := publishBody["column_ids"].([]any)
	if len(columns) != 1 || columns[0] != "7684228650583015430" {
		t.Fatalf("column_ids = %#v", columns)
	}
	themes := publishBody["theme_ids"].([]any)
	if len(themes) != 1 || themes[0] != "7275231252674773028" {
		t.Fatalf("theme_ids = %#v", themes)
	}
}

func TestJuejinPublishRepairsMissingCategoryAndTags(t *testing.T) {
	detailCalls := 0
	var updateBody map[string]any
	var publishBody map[string]any
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/user_api/v1/sys/token":
			return jsonResponse(request, 200, "", map[string]string{"x-ware-csrf-token": "0,csrf,1,success,x"}), nil
		case "/content_api/v1/article_draft/detail":
			detailCalls++
			category := "0"
			tags := "[]"
			if detailCalls > 1 {
				category = "6809637769959178254"
				tags = "[6809640408797167623]"
			}
			return jsonResponse(request, 200, `{"err_no":0,"data":{"draft_id":"draft-missing","article_draft":{"id":"draft-missing","article_id":"0","category_id":"`+category+`","tag_ids":`+tags+`,"edit_type":10}}}`, nil), nil
		case "/tag_api/v1/query_category_list":
			return jsonResponse(request, 200, `{"err_no":0,"data":{"categories":[{"category_id":"6809637769959178254","category_name":"后端"}]}}`, nil), nil
		case "/recommend_api/v1/tag/recommend/search":
			return jsonResponse(request, 200, `{"err_no":0,"data":[{"tag_id":"6809640408797167623","tag_name":"Java"}]}`, nil), nil
		case "/content_api/v1/article_draft/update":
			if err := json.NewDecoder(request.Body).Decode(&updateBody); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `{"err_no":0,"data":{"id":"draft-missing"}}`, nil), nil
		case "/content_api/v1/article/publish":
			if err := json.NewDecoder(request.Body).Decode(&publishBody); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `{"err_no":0,"err_msg":"success","data":{"article_id":"published-123"}}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}
	adapterValue, err := NewJuejinAdapter(client, juejinSession())
	if err != nil {
		t.Fatal(err)
	}
	adapter := adapterValue.(*juejinAdapter)
	result, err := adapter.PublishDraft(context.Background(), DraftRef{ID: "draft-missing"}, DraftInput{
		Title: "Java 并发编程", Description: "mutex runtime", Markdown: "# Body", Tags: []string{"Java"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if detailCalls != 2 {
		t.Fatalf("detailCalls = %d, want 2", detailCalls)
	}
	if updateBody["category_id"] != "6809637769959178254" {
		t.Fatalf("update category = %#v", updateBody["category_id"])
	}
	tagIDs, _ := updateBody["tag_ids"].([]any)
	if len(tagIDs) == 0 || tagIDs[0] != "6809640408797167623" {
		t.Fatalf("update tag_ids = %#v", updateBody["tag_ids"])
	}
	if publishBody["draft_id"] != "draft-missing" {
		t.Fatalf("publish body = %#v", publishBody)
	}
	if result.ID != "published-123" || result.URL != "https://juejin.cn/post/published-123" {
		t.Fatalf("result = %#v", result)
	}
}

func TestJuejinListPostsUsesPublishedListAndKeepsDraftID(t *testing.T) {
	var queryBody map[string]any
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/user_api/v1/user/get":
			return jsonResponse(request, 200, `{"data":{"user_id":"4333630801325609","user_name":"ThinkerQAQ_"}}`, nil), nil
		case "/user_api/v1/sys/token":
			return jsonResponse(request, 200, "", map[string]string{
				"x-ware-csrf-token": "0,csrf-list,1,success,x",
			}), nil
		case "/content_api/v1/article/query_list":
			if request.Method != http.MethodPost {
				t.Fatalf("query list method = %s", request.Method)
			}
			if request.Header.Get("x-secsdk-csrf-token") != "csrf-list" {
				t.Fatalf("query list csrf = %q", request.Header.Get("x-secsdk-csrf-token"))
			}
			if err := json.NewDecoder(request.Body).Decode(&queryBody); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `{
				"err_no":0,
				"err_msg":"success",
				"data":[{
					"article_id":"7689415591787773978",
					"article_info":{
						"article_id":"7689415591787773978",
						"draft_id":"7688942552989876259",
						"title":"并发编程（五）：Atomic——语言层的原子性、可见性与有序性"
					}
				}],
				"cursor":"1",
				"has_more":false
			}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	account, posts, err := JuejinListPosts(context.Background(), client, juejinSession())
	if err != nil {
		t.Fatal(err)
	}
	if account != "4333630801325609" {
		t.Fatalf("account = %q", account)
	}
	if queryBody["user_id"] != "4333630801325609" || queryBody["sort_type"] != float64(2) || queryBody["cursor"] != "0" {
		t.Fatalf("query body = %#v", queryBody)
	}
	if len(posts) != 1 {
		t.Fatalf("posts = %#v", posts)
	}
	post := posts[0]
	if post.ID != "7689415591787773978" || post.DraftID != "7688942552989876259" || !post.Published {
		t.Fatalf("post = %#v", post)
	}
	if post.URL != "https://juejin.cn/post/7689415591787773978" {
		t.Fatalf("post URL = %q", post.URL)
	}
}

func TestJuejinImageUploadRewritesMarkdown(t *testing.T) {
	calls := []string{}
	var createBody map[string]any
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls = append(calls, request.Method+" "+request.URL.Host+request.URL.Path+"?"+request.URL.RawQuery)
		switch {
		case request.URL.Path == "/user_api/v1/sys/token":
			return jsonResponse(request, 200, "", map[string]string{"x-ware-csrf-token": "0,csrf,1,success,x"}), nil
		case request.URL.Path == "/imagex/v2/gen_token":
			return jsonResponse(request, 200, `{"err_no":0,"data":{"token":{"AccessKeyId":"ak","SecretAccessKey":"sk","SessionToken":"st","ExpiredTime":"2030-01-01T00:00:00Z"}}}`, nil), nil
		case request.URL.Host == "imagex.bytedanceapi.com" && request.URL.Query().Get("Action") == "ApplyImageUpload":
			if !strings.HasPrefix(request.Header.Get("authorization"), "AWS4-HMAC-SHA256 ") {
				t.Fatalf("apply authorization = %q", request.Header.Get("authorization"))
			}
			return jsonResponse(request, 200, `{"Result":{"UploadAddress":{"StoreInfos":[{"StoreUri":"store/image.png","Auth":"tos-auth","UploadID":"u"}],"UploadHosts":["upload.example.com"],"SessionKey":"session-key"}}}`, nil), nil
		case request.URL.Host == "upload.example.com" && request.Method == http.MethodPut:
			if request.Header.Get("authorization") != "tos-auth" {
				t.Fatalf("TOS auth = %q", request.Header.Get("authorization"))
			}
			if request.Header.Get("content-crc32") == "" {
				t.Fatal("Content-CRC32 is missing")
			}
			return jsonResponse(request, 200, "", nil), nil
		case request.URL.Host == "imagex.bytedanceapi.com" && request.URL.Query().Get("Action") == "CommitImageUpload":
			return jsonResponse(request, 200, `{"Result":{"RequestId":"ok"}}`, nil), nil
		case request.URL.Path == "/imagex/v2/get_img_url":
			return jsonResponse(request, 200, `{"err_no":0,"data":{"main_url":"https://p3-juejin.byteimg.com/native.png"}}`, nil), nil
		case request.URL.Path == "/content_api/v1/article_draft/create":
			if err := json.NewDecoder(request.Body).Decode(&createBody); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `{"err_no":0,"data":{"id":"with-image"}}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL.String())
			return nil, nil
		}
	})}
	adapter, err := NewJuejinAdapter(client, juejinSession())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.CreateDraft(context.Background(), DraftInput{
		Title: "Image", Description: "Image article",
		Markdown: "![x](data:image/png;base64,aGVsbG8=)",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "with-image" {
		t.Fatalf("result = %#v", result)
	}
	markdown, _ := createBody["mark_content"].(string)
	if markdown != "![x](https://p3-juejin.byteimg.com/native.png)" {
		t.Fatalf("markdown = %q", markdown)
	}
	if len(calls) < 6 {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestServiceRecreatesMissingRemoteDraftExactlyOnce(t *testing.T) {
	publicationPath := filepath.Join(t.TempDir(), "publications.json")
	if err := SavePublicationDraftResult(publicationPath, "example", "juejin", "old", DraftResult{
		ID: "missing", URL: "https://juejin.cn/editor/drafts/missing", Created: true,
	}, time.Date(2026, 9, 18, 4, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	detailCalls := 0
	updateCalls := 0
	createCalls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/user_api/v1/user/get":
			return jsonResponse(request, 200, `{"data":{"user_id":"u1"}}`, nil), nil
		case "/user_api/v1/sys/token":
			return jsonResponse(request, 200, "", map[string]string{
				"x-ware-csrf-token": "0,csrf,1,success,x",
			}), nil
		case "/content_api/v1/article_draft/detail":
			detailCalls++
			return jsonResponse(request, http.StatusNotFound, `{"err_no":404,"err_msg":"draft not found"}`, nil), nil
		case "/content_api/v1/article_draft/update":
			updateCalls++
			t.Fatal("update should not run after detail already proved the draft is missing")
			return nil, nil
		case "/content_api/v1/article_draft/create":
			createCalls++
			return jsonResponse(request, 200, `{"err_no":0,"data":{"id":"replacement"}}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s", request.URL.String())
			return nil, nil
		}
	})}

	service := Service{
		HTTPClient:      client,
		Now:             func() time.Time { return time.Date(2026, 9, 18, 5, 0, 0, 0, time.UTC) },
		PublicationPath: publicationPath,
	}
	result, err := service.CreateOrUpdateDraftInput(
		context.Background(),
		"juejin",
		juejinSession(),
		"",
		DraftInput{
			Slug: "example", Title: "Example", Description: "Desc",
			Markdown: "Changed body", Language: "zh-CN", ContentHash: "new",
		},
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if detailCalls != 1 || updateCalls != 0 || createCalls != 1 {
		t.Fatalf("detail/update/create calls = %d/%d/%d", detailCalls, updateCalls, createCalls)
	}
	if result.ID != "replacement" || !result.Created {
		t.Fatalf("result = %#v", result)
	}

	state, _, err := LoadPublicationState(publicationPath, "example", "juejin")
	if err != nil {
		t.Fatal(err)
	}
	if state.RemoteDraftID != "replacement" || state.DraftHash != "new" {
		t.Fatalf("durable state = %#v", state)
	}
}

func TestServiceSkipsUnchangedDraftWithoutNetwork(t *testing.T) {
	publicationPath := filepath.Join(t.TempDir(), "publications.json")
	if err := SavePublicationDraftResult(publicationPath, "example", "juejin", "same", DraftResult{
		ID: "draft-1", URL: "https://juejin.cn/editor/drafts/draft-1", Created: true,
	}, time.Date(2026, 9, 18, 4, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	service := Service{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			t.Fatalf("network should not run: %s", request.URL)
			return nil, nil
		})},
		PublicationPath: publicationPath,
	}
	result, err := service.CreateOrUpdateDraftInput(
		context.Background(),
		"juejin",
		juejinSession(),
		"",
		DraftInput{
			Slug: "example", Title: "Example", Description: "Desc",
			Markdown: "Body", Language: "zh-CN", ContentHash: "same",
		},
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped || result.ID != "draft-1" {
		t.Fatalf("result = %#v", result)
	}
}

func TestCRC32AndAWS4AreDeterministic(t *testing.T) {
	if got := CRC32Hex([]byte("123456789")); got != "cbf43926" {
		t.Fatalf("CRC32 = %s", got)
	}
	headers, err := SignAWS4("GET", "https://imagex.bytedanceapi.com/?Version=2018-08-01&Action=ApplyImageUpload", AWS4Credentials{
		AccessKeyID: "ak", SecretAccessKey: "sk", SecurityToken: "token",
		Region: "cn-north-1", Service: "imagex",
	}, time.Date(2026, 9, 18, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if headers["x-amz-date"] != "20260918T040000Z" || headers["x-amz-security-token"] != "token" {
		t.Fatalf("headers = %#v", headers)
	}
	const expectedAuthorization = "AWS4-HMAC-SHA256 Credential=ak/20260918/cn-north-1/imagex/aws4_request, " +
		"SignedHeaders=x-amz-date;x-amz-security-token, " +
		"Signature=0a3fddc7a02d7b815976528615d3b3b5892da8f950e6d714da2493ac934ae767"
	if headers["authorization"] != expectedAuthorization {
		t.Fatalf("authorization = %q", headers["authorization"])
	}
}

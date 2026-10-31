package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type csdnRoundTripFunc func(*http.Request) (*http.Response, error)

func (f csdnRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func csdnResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Request:    request,
	}
}

func csdnTestSession() Session {
	return Session{
		UserAgent: "BlogCTL-Test-UA",
		Cookies: []BrowserCookie{{
			Name: "UserToken", Value: "secret", Domain: ".csdn.net", Path: "/", Secure: true,
		}},
	}
}

func TestCSDNArticleIDAcceptsIDAndEditorLinks(t *testing.T) {
	cases := map[string]string{
		"147578947": "147578947",
		"https://blog.csdn.net/ThinkerQAQ/article/details/147578947": "147578947",
		"https://editor.csdn.net/md?articleId=147578947":             "147578947",
	}
	for input, want := range cases {
		if got := CSDNArticleID(input); got != want {
			t.Fatalf("CSDNArticleID(%q) = %q, want %q", input, got, want)
		}
	}
	if got := CSDNArticleID("https://editor.csdn.net/md"); got != "" {
		t.Fatalf("invalid reference = %q", got)
	}
}

func TestCSDNListPostsUsesSignedConsoleLists(t *testing.T) {
	client := &http.Client{Transport: csdnRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.URL.Host == "bizapi.csdn.net" && request.URL.Path == "/blog-console-api/v3/editor/getBaseInfo":
			return csdnResponse(request, 200, `{"code":200,"data":{"name":"ThinkerQAQ","nickname":"ThinkerQAQ"}}`), nil
		case request.URL.Host == "bizapi.csdn.net" && request.URL.Path == "/blog/phoenix/console/v1/article/list":
			if request.Header.Get("x-ca-key") != csdnKey || request.Header.Get("x-ca-signature") == "" {
				t.Fatalf("missing CSDN signed headers: %#v", request.Header)
			}
			switch request.URL.Query().Get("status") {
			case "draft":
				return csdnResponse(request, 200, `{
					"code":200,
					"data":{"list":[
						{"articleId":"147578946","title":"并发编程（四）：互斥锁的实现 · ThinkerQAQ"}
					],"page":1,"size":20,"total":1}
				}`), nil
			case "all_v3":
				return csdnResponse(request, 200, `{
					"code":200,
					"data":{"list":[
						{"articleId":"147578947","title":"并发编程（四）：互斥锁的实现"},
						{"articleId":"147578900","title":"并发编程（三）：语言层互斥锁"}
					],"page":1,"size":20,"total":2}
				}`), nil
			default:
				t.Fatalf("unexpected list status: %q", request.URL.Query().Get("status"))
				return nil, nil
			}
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	account, posts, err := CSDNListPosts(context.Background(), client, csdnTestSession())
	if err != nil {
		t.Fatal(err)
	}
	if account != "ThinkerQAQ" {
		t.Fatalf("account = %q", account)
	}
	want := []CSDNPost{
		{ID: "147578946", Title: "并发编程（四）：互斥锁的实现 · ThinkerQAQ", URL: "https://editor.csdn.net/md?articleId=147578946", Published: false},
		{ID: "147578947", Title: "并发编程（四）：互斥锁的实现", URL: "https://blog.csdn.net/ThinkerQAQ/article/details/147578947", Published: true},
		{ID: "147578900", Title: "并发编程（三）：语言层互斥锁", URL: "https://blog.csdn.net/ThinkerQAQ/article/details/147578900", Published: true},
	}
	if !reflect.DeepEqual(posts, want) {
		t.Fatalf("posts = %#v", posts)
	}
}

func TestCSDNSignaturePathCanonicalizesEmptyQueryValues(t *testing.T) {
	cases := map[string]string{
		"/blog-console-api/v3/editor/getArticle?id=147578947&model_type=":       "/blog-console-api/v3/editor/getArticle?id=147578947&model_type",
		"/blog/phoenix/console/v1/article/list?page=1&pageSize=20&status=draft": "/blog/phoenix/console/v1/article/list?page=1&pageSize=20&status=draft",
		"/blog-console-api/v3/editor/getBaseInfo":                               "/blog-console-api/v3/editor/getBaseInfo",
	}
	for input, want := range cases {
		if got := csdnSignaturePath(input); got != want {
			t.Fatalf("csdnSignaturePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCSDNLookupPostVerifiesDraftByID(t *testing.T) {
	client := &http.Client{Transport: csdnRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/blog-console-api/v3/editor/getBaseInfo":
			return csdnResponse(request, 200, `{"code":200,"data":{"name":"ThinkerQAQ","nickname":"ThinkerQAQ"}}`), nil
		case "/blog-console-api/v3/editor/getArticle":
			if request.URL.Query().Get("id") != "147578947" {
				t.Fatalf("id = %q", request.URL.Query().Get("id"))
			}
			if request.URL.RawQuery != "id=147578947&model_type=" {
				t.Fatalf("request query = %q", request.URL.RawQuery)
			}
			nonce := request.Header.Get("x-ca-nonce")
			if request.Header.Get("x-ca-key") != csdnKey || nonce == "" {
				t.Fatalf("missing CSDN signed headers: %#v", request.Header)
			}
			signString := "GET\n*/*\n\n\n\n" +
				"x-ca-key:" + csdnKey + "\n" +
				"x-ca-nonce:" + nonce + "\n" +
				"/blog-console-api/v3/editor/getArticle?id=147578947&model_type"
			if got, want := request.Header.Get("x-ca-signature"), hmacSHA256Base64(csdnSecret, signString); got != want {
				t.Fatalf("x-ca-signature = %q, want %q", got, want)
			}
			return csdnResponse(request, 200, `{
				"code":200,
				"data":{"article_id":"147578947","title":"并发编程（四）：互斥锁的实现","status":2}
			}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	account, post, err := CSDNLookupPost(context.Background(), client, csdnTestSession(), "147578947")
	if err != nil {
		t.Fatal(err)
	}
	if account != "ThinkerQAQ" || post.Published {
		t.Fatalf("account=%q post=%#v", account, post)
	}
	if post.URL != "https://editor.csdn.net/md?articleId=147578947" {
		t.Fatalf("draft URL = %q", post.URL)
	}
}

func TestCSDNTitleMatchesExactAndSuffix(t *testing.T) {
	if !CSDNTitleMatches("并发编程（四）：互斥锁的实现", "并发编程（四）：互斥锁的实现") {
		t.Fatal("exact title should match")
	}
	if !CSDNTitleMatches("并发编程（四）：互斥锁的实现", "并发编程（四）：互斥锁的实现 · ThinkerQAQ") {
		t.Fatal("platform suffix should match")
	}
	if CSDNTitleMatches("并发编程（四）", "并发编程（三）") {
		t.Fatal("unrelated titles must not match")
	}
}

func TestCSDNSaveUsesPlatformImageURLInMarkdownAndHTML(t *testing.T) {
	imageUploads := 0
	client := &http.Client{Transport: csdnRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.Method == http.MethodGet && request.URL.Host == "source.example" && request.URL.Path == "/diagram.png":
			response := csdnResponse(request, 200, "png-bytes")
			response.Header.Set("content-type", "image/png")
			return response, nil
		case request.Method == http.MethodPost && request.URL.Host == "bizapi.csdn.net" &&
			request.URL.Path == "/resource-api/v1/image/direct/upload/signature":
			return csdnResponse(request, 200, `{
				"code":200,
				"data":{
					"filePath":"blog/test.png",
					"host":"https://upload.example/",
					"accessId":"access",
					"policy":"policy",
					"signature":"signature",
					"callbackUrl":"",
					"callbackBody":"",
					"callbackBodyType":"",
					"customParam":{}
				}
			}`), nil
		case request.Method == http.MethodPost && request.URL.Host == "upload.example":
			imageUploads++
			if err := request.ParseMultipartForm(2 << 20); err != nil {
				t.Fatal(err)
			}
			file, _, err := request.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			_ = file.Close()
			return csdnResponse(request, 200, `{
				"code":200,
				"data":{"imageUrl":"https://img-blog.csdnimg.cn/blog/test.png"}
			}`), nil
		case request.Method == http.MethodPost && request.URL.Host == "bizapi.csdn.net" &&
			request.URL.Path == "/blog-console-api/v3/mdeditor/saveArticle":
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			markdown := valueString(payload["markdowncontent"])
			html := valueString(payload["content"])
			if !strings.Contains(markdown, "https://img-blog.csdnimg.cn/blog/test.png") ||
				!strings.Contains(html, "https://img-blog.csdnimg.cn/blog/test.png") {
				t.Fatalf("payload image URLs are inconsistent: markdown=%q html=%q", markdown, html)
			}
			if valueString(payload["tags"]) != "Go,Concurrency" {
				t.Fatalf("tags = %q", valueString(payload["tags"]))
			}
			return csdnResponse(request, 200, `{"code":200,"data":{"id":"147578947"}}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapterValue, err := NewCSDNAdapter(client, csdnTestSession())
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapterValue.CreateDraft(context.Background(), DraftInput{
		Title:    "并发编程（四）：互斥锁的实现",
		Markdown: "![diagram](https://source.example/diagram.png)",
		HTML:     `<p><img src="https://source.example/diagram.png"></p>`,
		Tags:     []string{"Go", "Concurrency"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if imageUploads != 1 || !result.Created || result.ID != "147578947" {
		t.Fatalf("uploads=%d result=%#v", imageUploads, result)
	}
}

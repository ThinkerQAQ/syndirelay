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
)

func TestTransformZhihuHTMLNormalizesTablesCodeAndImages(t *testing.T) {
	input := strings.Join([]string{
		`<table><thead><tr><th>Name</th><th>Value</th></tr></thead><tbody><tr><td>x</td><td>1</td></tr></tbody></table>`,
		`<pre><code class="language-go">count++</code></pre>`,
		`<p><img src="https://example.com/image.png" style="width: 10px" data-foo="bar"></p>`,
	}, "")
	got := transformZhihuHTML(input)

	if !strings.Contains(got, `<table data-draft-node="block" data-draft-type="table" data-size="normal" data-row-style="normal"><tbody>`) {
		t.Fatalf("table was not normalized: %s", got)
	}
	if strings.Contains(got, "<thead") {
		t.Fatalf("thead should be folded into tbody: %s", got)
	}
	if !strings.Contains(got, `<pre lang="go"><code>count++</code></pre>`) {
		t.Fatalf("code block was not normalized: %s", got)
	}
	if !strings.Contains(got, `<figure><img src="https://example.com/image.png"></figure>`) {
		t.Fatalf("image was not normalized: %s", got)
	}
	if strings.Contains(got, `style="width`) || strings.Contains(got, "data-foo=") {
		t.Fatalf("unsupported attributes were not removed: %s", got)
	}
	if !strings.Contains(got, "data-draft-node=") {
		t.Fatalf("Draft.js attributes were removed: %s", got)
	}
}

func TestTransformZhihuHTMLKeepsDraftTableAttributes(t *testing.T) {
	input := `<table data-draft-node="block" data-draft-type="table"><tbody><tr><td>a</td></tr></tbody></table>`
	got := transformZhihuHTML(input)
	for _, attribute := range []string{
		`data-draft-node="block"`,
		`data-draft-type="table"`,
		`data-size="normal"`,
		`data-row-style="normal"`,
	} {
		if !strings.Contains(got, attribute) {
			t.Fatalf("draft table attribute %q was lost: %s", attribute, got)
		}
	}
}

func TestZhihuCreateDraftMatchesCapturedPayload(t *testing.T) {
	var createPayload map[string]any
	var updatePayload map[string]any
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/api/articles/drafts":
			if err := json.NewDecoder(request.Body).Decode(&createPayload); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `{"id":"2086065787769562759"}`, nil), nil
		case "/api/articles/2086065787769562759/draft":
			if err := json.NewDecoder(request.Body).Decode(&updatePayload); err != nil {
				t.Fatal(err)
			}
			return jsonResponse(request, 200, `{}`, nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapter, err := NewZhihuAdapter(client, Session{UserAgent: "BlogCTL-Test-UA", Cookies: []BrowserCookie{{Name: "z_c0", Value: "secret", Domain: ".zhihu.com", Path: "/", Secure: true}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.CreateDraft(context.Background(), DraftInput{
		Title: "test", HTML: "<p>teset</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "2086065787769562759" || !result.Created {
		t.Fatalf("result = %#v", result)
	}
	if createPayload["title"] != "test" || valueString(createPayload["delta_time"]) != "0" || createPayload["can_reward"] != false {
		t.Fatalf("create payload = %#v", createPayload)
	}
	if _, exists := updatePayload["title"]; exists {
		t.Fatalf("update payload unexpectedly contains title: %#v", updatePayload)
	}
	if updatePayload["content"] != "<p>teset</p>" || updatePayload["table_of_contents"] != false || updatePayload["can_reward"] != false {
		t.Fatalf("update payload = %#v", updatePayload)
	}
}

func TestZhihuPrepareHTMLUploadsGeneratedAssetWithoutR2(t *testing.T) {
	root := t.TempDir()
	assetID := "generated-mermaid"
	assetSource := "blogctl-asset://mermaid/" + assetID
	assetDir := filepath.Join(root, "assets", "mermaid")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := []byte("png-bytes")
	if err := os.WriteFile(filepath.Join(assetDir, assetID+".png"), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	ossUploaded := false
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.Method == http.MethodPost && request.URL.Host == "api.zhihu.com" && request.URL.Path == "/images":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["image_hash"] != zhihuImageMD5(payload) || body["source"] != "article" {
				t.Fatalf("image token payload = %#v", body)
			}
			return jsonResponse(request, 200, "{\"upload_file\":{\"state\":0,\"image_id\":\"image-1\",\"object_key\":\"v2-test-key\"},\"upload_token\":{\"access_id\":\"access-id\",\"access_key\":\"access-secret\",\"access_token\":\"security-token\"}}", nil), nil
		case request.Method == http.MethodPut && request.URL.Host == "zhihu-pics-upload.zhimg.com" && request.URL.Path == "/v2-test-key":
			ossUploaded = true
			if got := request.Header.Get("content-type"); got != "image/png" {
				t.Fatalf("content-type = %q", got)
			}
			if got := request.Header.Get("x-oss-security-token"); got != "security-token" {
				t.Fatalf("security token = %q", got)
			}
			if got := request.Header.Get("authorization"); !strings.HasPrefix(got, "OSS access-id:") {
				t.Fatalf("authorization = %q", got)
			}
			if got := request.Header.Get("x-oss-date"); got == "" || !strings.HasSuffix(got, "GMT") {
				t.Fatalf("x-oss-date = %q", got)
			}
			raw, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != string(payload) {
				t.Fatalf("uploaded payload = %q", raw)
			}
			return jsonResponse(request, 200, "{}", nil), nil
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	})}

	adapterValue, err := NewZhihuAdapter(client, Session{
		UserAgent: "BlogCTL-Test-UA",
		Cookies:   []BrowserCookie{{Name: "z_c0", Value: "test", Domain: ".zhihu.com", Path: "/", Secure: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter := adapterValue.(*zhihuAdapter)
	html, err := adapter.prepareHTML(context.Background(), DraftInput{
		Markdown:         "![diagram](" + assetSource + ")",
		HTML:             "<p><img src=\"" + assetSource + "\"></p>",
		DistributionRoot: root,
		Assets: []PublishingAsset{{
			Kind: "mermaid", ID: assetID, Source: assetSource,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ossUploaded {
		t.Fatal("generated image was not uploaded to Zhihu OSS")
	}
	if !strings.Contains(html, "https://pic4.zhimg.com/v2-test-key") {
		t.Fatalf("html = %s", html)
	}
	if strings.Contains(html, assetSource) {
		t.Fatalf("internal asset reference leaked into Zhihu HTML: %s", html)
	}
}

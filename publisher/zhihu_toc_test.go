package publisher

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestNormalizeZhihuContentsUsesNativeTOC(t *testing.T) {
	html := `<p>开篇</p><h2 id="toc">目录</h2>` + "\n" +
		`<ul><li><a href="#0-intro">0. 引言</a></li>` +
		`<li><a href="#1-api">1. API</a><ul><li><a href="#1-1-foo">1.1 小结</a></li></ul></li></ul>` +
		`<h2 id="0-intro">0. 引言</h2><p>引言正文</p><h2>1. API</h2><p>正文</p>`
	result, err := normalizeZhihuContents(html)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, ">目录</h2>") || strings.Contains(result, `href="#1-api"`) {
		t.Fatalf("manual toc preserved: %s", result)
	}
	for _, expected := range []string{"<p>开篇</p>", "0. 引言</h2>", "1. API</h2>", "<p>引言正文</p>", "<p>正文</p>"} {
		if !strings.Contains(result, expected) {
			t.Fatalf("missing %q: %s", expected, result)
		}
	}
	if !zhihuContentHeading.MatchString(result) {
		t.Fatal("native headings were lost")
	}
	twice, err := normalizeZhihuContents(result)
	if err != nil || twice != result {
		t.Fatalf("not idempotent: %v", err)
	}
}

func TestNormalizeZhihuContentsPreservesNonTOCDocuments(t *testing.T) {
	for _, input := range []string{
		`<h2>第一节</h2><p>正文</p>`,
		`<h2>参考资料</h2><ul><li><a href="https://example.com">external</a></li></ul>`,
		`<h2>目录结构</h2><p>这是目录结构说明。</p>`,
	} {
		actual, err := normalizeZhihuContents(input)
		if err != nil || actual != input {
			t.Fatalf("modified arbitrary content: %q, %v", actual, err)
		}
	}
	for _, invalid := range []string{
		`<h2>目录</h2><ul><li><a href="https://example.com">其他内容</a></li></ul>`,
		`<h2>目录</h2><p>这里是正文，不能删除。</p>`,
		`<h2>目录</h2><ul><li>不能把普通列表当 TOC</li></ul>`,
	} {
		if _, err := normalizeZhihuContents(invalid); err == nil {
			t.Fatalf("silently destroyed potentially authored text: %s", invalid)
		}
	}
}

func TestZhihuDraftUsesNativeTOCAndDropsAuthoredTOC(t *testing.T) {
	var update map[string]any
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPatch || req.URL.Path != "/api/articles/123456/draft" {
			t.Fatalf("unexpected %s %s", req.Method, req.URL.String())
		}
		if err := json.NewDecoder(req.Body).Decode(&update); err != nil {
			t.Fatal(err)
		}
		return jsonResponse(req, 200, `{}`, nil), nil
	})}
	adapter, err := NewZhihuAdapter(client, Session{UserAgent: "Test-UA", Cookies: []BrowserCookie{{Name: "z_c0", Value: "test", Domain: ".zhihu.com", Path: "/", Secure: true}}})
	if err != nil {
		t.Fatal(err)
	}
	input := DraftInput{HTML: `<h2>目录</h2><ul><li><a href="#section-1">1. 章节</a></li></ul><h2>1. 章节</h2><p>正文</p>`}
	_, err = adapter.UpdateDraft(context.Background(), DraftRef{ID: "123456"}, input)
	if err != nil {
		t.Fatal(err)
	}
	if update["table_of_contents"] != true {
		t.Fatal("native table of contents not enabled")
	}
	content, _ := update["content"].(string)
	if strings.Contains(content, "<h2>目录</h2>") || !strings.Contains(content, "<h2>1. 章节</h2>") {
		t.Fatalf("transformed incorrectly: %s", content)
	}
}

package publisher

import (
	"strings"
	"testing"
)

func Test51CTOAuthenticatedDraftListHTMLFallback(t *testing.T) {
	html := `<!doctype html><html><body><h1>草稿箱</h1>
 <a href="/blogger/draft/2108123">SyndiRelay 端到端草稿测试（请勿发布）</a>
 <a href="https://blog.51cto.com/blogger/draft/2108123">编辑</a>
 <a href="/blogger/draft/2108110">杭州这一年</a>
 <a href="https://evil.example/blogger/draft/123">其他账户</a>
 <a href="https://blog.51cto.com/someone/13">已发布文章</a>
 <a href="/blogger/draft/12?delete=true">删除</a></body></html>`
	drafts, err := parse51CTODraftManagementPage([]byte(html))
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 2 || drafts[0].ID != "2108123" || drafts[0].Title != "SyndiRelay 端到端草稿测试（请勿发布）" || drafts[0].Published || drafts[1].ID != "2108110" {
		t.Fatalf("unexpected safe draft extraction %+v", drafts)
	}
}

func Test51CTOFallbackRejectsLoginPageAndMissingDrafts(t *testing.T) {
	for _, body := range []string{
		"<html><body>登录后可管理草稿</body></html>",
		"<html><body><h1>草稿箱</h1><a href='/blogger/publish'>去创建</a></body></html>",
	} {
		if _, err := parse51CTODraftManagementPage([]byte(body)); err == nil {
			t.Fatalf("accepted unsafe management page %q", body)
		}
	}
	if _, err := parse51CTODraftManagementPage([]byte(strings.Repeat("garbage", 200))); err == nil {
		t.Fatal("accepted garbage")
	}
}

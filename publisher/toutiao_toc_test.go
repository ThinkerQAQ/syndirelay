package publisher

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func toutiaoTOCListItems(t *testing.T, raw string) ([]string, int) {
	t.Helper()
	contextNode := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(raw), contextNode)
	if err != nil {
		t.Fatal(err)
	}
	var contents []string
	nestedLists := 0
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Data == "ul" || node.Data == "ol" {
			if node.Parent != nil && node.Parent.Data == "li" {
				nestedLists++
			}
		}
		if node.Type == html.ElementNode && node.Data == "li" {
			contents = append(contents, toutiaoText(node, false))
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	for _, node := range nodes {
		visit(node)
	}
	return contents, nestedLists
}

func TestToutiaoTOCFlattenNestedMarkdownHierarchy(t *testing.T) {
	raw := `<h2>目录</h2>
<ul>
<li><a href="#0">0. 这一篇继续回答什么？</a></li>
<li><a href="#1">1. HotSpot 如何实现 AtomicInteger？</a>
<ul>
<li><a href="#11">1.1 分层</a></li>
<li><a href="#13">1.3 Runtime / Language Implementation 层的三种保证</a>
<ul><li><a href="#131">1.3.1 Atomicity</a></li></ul>
</li>
</ul></li>
<li><a href="#2">2. Go Runtime 如何实现 sync/atomic？</a></li>
</ul>
<h2>文章正文</h2><p>这里有正常段落。</p>
<ul><li>正文列表 A</li><li><a href="https://example.com/guide">正文链接</a></li></ul>`
	actual := normalizeToutiaoTOC(raw)
	if strings.Contains(actual, `href="#`) {
		t.Fatalf("TOC contains broken in-page links: %s", actual)
	}
	if !strings.Contains(actual, `<a href="https://example.com/guide">正文链接</a>`) {
		t.Fatalf("unrelated article links changed: %s", actual)
	}
	items, nested := toutiaoTOCListItems(t, actual)
	expected := []string{
		"0. 这一篇继续回答什么？",
		"1. HotSpot 如何实现 AtomicInteger？",
		"　1.1 分层",
		"　1.3 Runtime / Language Implementation 层的三种保证",
		"　　1.3.1 Atomicity",
		"2. Go Runtime 如何实现 sync/atomic？",
		"正文列表 A", "正文链接",
	}
	if nested != 0 || len(items) != len(expected) {
		t.Fatalf("expected %d flat items, got %d with %d nested lists", len(expected), len(items), nested)
	}
	for index, want := range expected {
		if items[index] != want {
			t.Fatalf("item %d = %q, want %q", index, items[index], want)
		}
	}
}

func TestToutiaoTOCRepairsCollapsedCreatorEditorList(t *testing.T) {
	// Recorded in the real October 10 creator HAR: each parent LI contains
	// multiple consecutive anchor elements after nested lists are stripped.
	raw := `<h1 class="pgc-h-forward-slash">目录</h1>
<ul>
<li><a href="#0">0. 这一篇继续回答什么？</a></li>
<li><a href="#1">1. HotSpot 如何实现 AtomicInteger？</a> <a href="#11">1.1 分层</a> <a href="#131">1.3.1 Atomicity</a></li>
<li><a href="#2">2. Go Runtime 如何实现 sync/atomic？</a> <a href="#21">2.1 分层</a></li>
</ul><p>其他正文不变</p>`
	actual := normalizeToutiaoTOC(raw)
	items, nested := toutiaoTOCListItems(t, actual)
	if nested != 0 || len(items) != 6 {
		t.Fatalf("expected 6 separate list rows, found %d nested=%d: %s", len(items), nested, actual)
	}
	if items[2] != "　1.1 分层" || items[3] != "　　1.3.1 Atomicity" ||
		items[5] != "　2.1 分层" {
		t.Fatalf("incorrect collapsed hierarchy: %v", items)
	}
	if !strings.Contains(actual, "<p>其他正文不变</p>") {
		t.Fatal("article body lost after normalization")
	}
}

func TestToutiaoTOCLeavesOtherHTMLByteIdentical(t *testing.T) {
	raw := `<h2>Atomic 的实现</h2><ul><li><a href="#real">未标为目录的链接</a><ul><li>嵌套项</li></ul></li></ul><pre><code>1 &lt; 2</code></pre>`
	if got := normalizeToutiaoTOC(raw); got != raw {
		t.Fatalf("non-TOC content was modified: %s", got)
	}
}

func TestToutiaoTOCPrepareHTMLTransformsOnlyPlatformPayload(t *testing.T) {
	raw := `<h2>目录</h2><ul><li><a href="#1">1. 主章节</a><ul><li><a href="#11">1.1 子章节</a></li></ul></li></ul><p>正文</p>`
	adapter := &toutiaoAdapter{}
	actual, err := adapter.prepareHTML(context.Background(), DraftInput{HTML: raw})
	if err != nil {
		t.Fatal(err)
	}
	if actual == raw || strings.Contains(actual, `href="#11"`) || !strings.Contains(actual, "　1.1 子章节") {
		t.Fatalf("Toutiao HTML did not normalize TOC: %s", actual)
	}
}

func TestToutiaoSectionReferencesPreserveLabelsAndExternalFragments(t *testing.T) {
	raw := `<p>见<a href="#chapter"><strong>1.2</strong> <code>Thread</code></a>与<a href=" #other ">下一节</a>。</p><ul><li><a href="#nested">嵌套引用</a></li></ul><p><a href="https://go.dev/ref/mem#tmp_1" target="_blank">Go 官方文档</a> <a href="https://thinkerqaq.com/articles/a/#chapter">原文章节</a></p><pre><code>&lt;a href="#literal"&gt;代码示例&lt;/a&gt;</code></pre>`
	got := normalizeToutiaoSectionReferences(raw)
	if strings.Contains(got, `href="#chapter"`) || strings.Contains(got, `href=" #other "`) || strings.Contains(got, `href="#nested"`) {
		t.Fatalf("broken section links remain: %s", got)
	}
	for _, expected := range []string{`<strong>1.2</strong> <code>Thread</code>`, `与下一节。`, `<li>嵌套引用</li>`, `href="https://go.dev/ref/mem#tmp_1"`, `href="https://thinkerqaq.com/articles/a/#chapter"`, `&lt;a href=&#34;#literal&#34;&gt;代码示例&lt;/a&gt;`} {
		if !strings.Contains(got, expected) {
			t.Fatalf("lost %q: %s", expected, got)
		}
	}
	if again := normalizeToutiaoSectionReferences(got); again != got {
		t.Fatal("normalization is not idempotent")
	}
}
func TestToutiaoSectionReferencesLeaveUnrelatedHTMLByteIdentical(t *testing.T) {
	raw := `<p><a href="https://example.com/#part" class="source">官方资料</a></p><pre><code>1 &lt; 2</code></pre>`
	if got := normalizeToutiaoSectionReferences(raw); got != raw {
		t.Fatalf("unrelated HTML changed: %s", got)
	}
}
func TestToutiaoPrepareHTMLAlsoNormalizesBodySectionReferences(t *testing.T) {
	raw := `<h2>目录</h2><ul><li><a href="#one">1. 第一节</a></li></ul><h2 id="one">1. 第一节</h2><p>见<a href="#one"><em>第一节</em></a>。参考<a href="https://example.com/#api">外部 API</a>。</p>`
	adapter := &toutiaoAdapter{}
	got, err := adapter.prepareHTML(context.Background(), DraftInput{HTML: raw})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `href="#one"`) || !strings.Contains(got, `<em>第一节</em>`) || !strings.Contains(got, `href="https://example.com/#api"`) {
		t.Fatalf("invalid Toutiao payload: %s", got)
	}
}

func TestToutiaoTOCKeepsMatchedHeadingLinksAndFlattensHierarchy(t *testing.T) {
	raw := `<h2>目录</h2><ul><li><a href="#1">1. 主章节</a><ul><li><a href="#11-%E5%AD%90%E7%AB%A0%E8%8A%82">1.1 子章节</a></li><li><a href="#missing">1.2 无目标</a></li></ul></li></ul><h2 id="1">1. 主章节</h2><h3 id="11-子章节">1.1 子章节</h3><p>见<a href="#1">主章节</a>。</p>`
	got := normalizeToutiaoSectionReferences(normalizeToutiaoTOC(raw))
	if !strings.Contains(got, `href="#1"`) || !strings.Contains(got, `href="#11-%E5%AD%90%E7%AB%A0%E8%8A%82"`) || strings.Contains(got, `href="#missing"`) {
		t.Fatalf("invalid references: %s", got)
	}
	_, nested := toutiaoTOCListItems(t, got)
	if nested != 0 {
		t.Fatal("TOC remains nested")
	}
}

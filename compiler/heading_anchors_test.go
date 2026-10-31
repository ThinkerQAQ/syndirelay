package compiler

import (
	"strings"
	"testing"
)

func TestLinkedHeadingIDsUseAuthoredFragmentsAndRichLabels(t *testing.T) {
	raw := `<ul><li><a href="#12-%E5%88%86%E5%B1%82">1.2 <code>分层</code></a></li><li><a href="#custom-duplicate">重复</a></li><li><a href="#custom-duplicate-2">重复</a></li></ul><h2>1.2 <code>分层</code></h2><h3>重复</h3><h3>重复</h3><p>见<a href="#12-%E5%88%86%E5%B1%82">1.2 分层</a>与<a href="https://go.dev/ref/mem#part">官方资料</a>。</p>`
	got, err := addLinkedHeadingIDs(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<h2 id="12-分层">`, `<h3 id="custom-duplicate">`, `<h3 id="custom-duplicate-2">`, `href="#12-%E5%88%86%E5%B1%82"`, `href="https://go.dev/ref/mem#part"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s: %s", want, got)
		}
	}
	again, err := addLinkedHeadingIDs(got)
	if err != nil || again != got {
		t.Fatal("anchor generation is not idempotent")
	}
}
func TestLinkedHeadingIDsDoNotOverwriteExplicitTargets(t *testing.T) {
	raw := `<a href="#kept">标题</a><h2 id="kept">标题</h2><a href="#missing">不存在的章节</a><pre><code>## 代码不应成为标题</code></pre>`
	got, err := addLinkedHeadingIDs(raw)
	if err != nil || got != raw {
		t.Fatalf("unrelated HTML changed: %s %v", got, err)
	}
}

package publisher

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestToutiaoExternalizesOnlyValidSectionReferences(t *testing.T) {
	raw := `<h2 id="2-update">Update</h2><p><a href="#2-update">Jump to update</a> <a href="#missing">Missing</a> <a href="https://creativecommons.org/licenses/by-nc-sa/4.0/">License</a></p>`
	source := "https://thinkerqaq.com/en/articles/test/"
	result := toutiaoExternalizeSectionLinks(raw, source)
	if !strings.Contains(result, `href="https://thinkerqaq.com/en/articles/test/#2-update"`) {
		t.Fatalf("resolved section reference not externalized: %s", result)
	}
	if !strings.Contains(result, `href="https://thinkerqaq.com/en/articles/test/#missing"`) {
		t.Fatalf("authored source fragment reference not preserved: %s", result)
	}
	if !strings.Contains(result, `href="https://creativecommons.org/licenses/by-nc-sa/4.0/"`) {
		t.Fatalf("unrelated external link changed: %s", result)
	}
	if second := toutiaoExternalizeSectionLinks(result, source); second != result {
		t.Fatalf("section rewrite is not idempotent: %s", second)
	}
	if got := toutiaoExternalizeSectionLinks(raw, "javascript:alert(1)"); got != raw {
		t.Fatalf("unsafe canonical URL should not rewrite HTML: %s", got)
	}
}

func TestToutiaoDraftHTMLUsesSourceChapterLink(t *testing.T) {
	adapter := &toutiaoAdapter{}
	result, err := adapter.prepareHTML(context.Background(), DraftInput{
		HTML:         `<h2 id="2-update">Update</h2><p><a href="#2-update">Jump</a></p>`,
		CanonicalURL: "https://thinkerqaq.com/articles/sample/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `href="https://thinkerqaq.com/articles/sample/#2-update"`) {
		t.Fatalf("creator-bound HTML still contains a plain fragment: %s", result)
	}
}

func TestToutiaoTitleTruncationPreservesMeaningfulSuffix(t *testing.T) {
	original := "SyndiRelay 端到端草稿测试 2026-11-02（请勿发布）"
	truncated := truncateToutiaoTitle(original)
	if utf8.RuneCountInString(truncated) != 30 ||
		!strings.Contains(truncated, "…") || !strings.HasSuffix(truncated, "（请勿发布）") {
		t.Fatalf("invalid 30-rune title: %s", truncated)
	}
	if !ToutiaoTitleMatches(original, truncated) {
		t.Fatal("new truncated title not matched")
	}
	old := string([]rune(original)[:30])
	if !ToutiaoTitleMatches(original, old) {
		t.Fatalf("existing remote draft with old title no longer matched: %s", old)
	}
	if got := truncateToutiaoTitle("Short"); got != "Short" {
		t.Fatal("short title changed")
	}
	if !strings.HasSuffix(truncateToutiaoTitle(strings.Repeat("a", 70)), "…") {
		t.Fatal("ordinary long title did not get ellipsis")
	}
}

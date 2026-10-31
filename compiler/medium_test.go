package compiler

import (
	blogpublishing "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publishing"
	"reflect"
	"strings"
	"testing"
)

func TestMediumUTF16MarkupOffsets(t *testing.T) {
	warnings := []string{}
	inline := parseMediumInline("中😀 **bold**", &warnings)
	if inline.Text != "中😀 bold" {
		t.Fatalf("text = %q", inline.Text)
	}
	var bold *mediumMarkup
	for index := range inline.Markups {
		if inline.Markups[index].Type == mediumMarkupBold {
			bold = &inline.Markups[index]
			break
		}
	}
	if bold == nil {
		t.Fatal("missing bold markup")
	}
	if bold.Start != 4 || bold.End != 8 {
		t.Fatalf("bold offset = %d..%d, want 4..8", bold.Start, bold.End)
	}
}

func TestMediumBlockParity(t *testing.T) {
	blocks, _ := parseMediumBlocks("- parent\n  - child\n    - [x] done\n- [ ] todo")
	got := []string{}
	for _, block := range blocks {
		if block.Kind == "uli" {
			got = append(got, block.Text)
		}
	}
	want := []string{"parent", "↳ child", "↳ ↳ ☑ done", "☐ todo"}
	if len(got) != len(want) {
		t.Fatalf("items=%#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("items=%#v", got)
		}
	}

	quotes, _ := parseMediumBlocks("> [!NOTE]\n> Locks establish ordering.")
	if len(quotes) != 1 || quotes[0].Text != "Note.\nLocks establish ordering." {
		t.Fatalf("quote=%#v", quotes)
	}
	if len(quotes[0].Markups) == 0 || quotes[0].Markups[0].Start != 0 || quotes[0].Markups[0].End != 5 {
		t.Fatalf("markups=%#v", quotes[0].Markups)
	}
}

func TestMediumTableFlattening(t *testing.T) {
	got := flattenMediumTables("| Expression | Meaning |\n| --- | --- |\n| `a | b` | bitwise OR |\n")
	if !strings.Contains(got, "**`a | b`** — bitwise OR") {
		t.Fatalf("flattened=%q", got)
	}
}

func TestMediumCodeAndHeadingsMatchObservedEditorParagraphTypes(t *testing.T) {
	markdown := "## Parent heading\n\n#### Subsection\n\n```go\nfunc main() {\n  println(42)\n}\n```\n\n1. First item\n"
	blocks, warnings := parseMediumBlocks(markdown)
	if len(warnings) != 0 || len(blocks) != 4 {
		t.Fatalf("blocks=%+v warnings=%v", blocks, warnings)
	}
	if blocks[0].ParagraphType != mediumH2 ||
		blocks[1].ParagraphType != mediumParagraph ||
		len(blocks[1].Markups) == 0 ||
		blocks[1].Markups[len(blocks[1].Markups)-1].Type != mediumMarkupBold {
		t.Fatalf("H4 must use a distinct bold paragraph, never PRE: %+v", blocks[:2])
	}
	if blocks[2].Kind != "pre" || blocks[2].ParagraphType != 8 || !strings.Contains(blocks[2].Text, "println(42)") {
		t.Fatalf("code must use Medium PRE, not numbered list: %+v", blocks[2])
	}
	if blocks[3].Kind != "oli" || blocks[3].ParagraphType != 10 {
		t.Fatalf("numbered list must use Medium OLI: %+v", blocks[3])
	}
}

// A browser-driven image pass must not change any of the previously fixed
// Medium native text/heading/list/PRE paragraph mappings.
func TestMediumBrowserImagePlanKeepsNativeFormattingAndOrder(t *testing.T) {
	article := Article{Title: "Native format"}
	markdown := "## Section\n\n" + "§§§text\na := 1\n  b := 2\n§§§\n\n" +
		"![diagram](https://assets.example/a.png)\n\n- one\n\n" +
		"![diagram two](https://assets.example/b.png)\n\n1. two"
	markdown = strings.ReplaceAll(markdown, "§", string(rune(96)))
	payload, warnings := buildMediumPayload(article, "browser-image-test", PlatformConfig{}, markdown)
	if len(warnings) != 0 {
		t.Fatalf("compiler warnings: %v", warnings)
	}
	if len(payload.BrowserPlan.Images) != 2 || len(payload.BrowserPlan.Deltas) != len(payload.Deltas) {
		t.Fatalf("unexpected browser plan: %+v", payload.BrowserPlan)
	}
	wantedMarkers := []string{"[BLOGCTL_IMAGE_001]", "[BLOGCTL_IMAGE_002]"}
	found := []string{}
	for i, original := range payload.Deltas {
		browser := payload.BrowserPlan.Deltas[i]
		if browser.Image != nil {
			t.Fatalf("browser image at paragraph %d was not replaced", i)
		}
		if original.Image != nil {
			found = append(found, browser.Paragraph.Text)
			if browser.Paragraph.Type != mediumParagraph || browser.Paragraph.Text != wantedMarkers[len(found)-1] {
				t.Fatalf("image %d was not a standalone normal placeholder paragraph: %+v", i, browser)
			}
			if original.Image.URL != payload.BrowserPlan.Images[len(found)-1].URL {
				t.Fatal("image source order changed")
			}
			continue
		}
		if browser.Paragraph.Type != original.Paragraph.Type ||
			browser.Paragraph.Text != original.Paragraph.Text ||
			!reflect.DeepEqual(browser.Paragraph.Markups, original.Paragraph.Markups) {
			t.Fatalf("native formatting was changed at %d: original=%+v browser=%+v", i, original, browser)
		}
	}
	if !reflect.DeepEqual(found, wantedMarkers) {
		t.Fatalf("image markers wrong: %v", found)
	}
}

// Observed directly from an editor-created Medium draft: native paragraph
// type 6 renders BLOCKQUOTE, 9 renders UL LI, 10 renders OL LI, 13 renders H4.
// Keep the compiler from silently turning callouts into bullets or bullets
// into headings.
func TestMediumNativeParagraphStyleIDs(t *testing.T) {
	if mediumBlockquote != 6 || mediumULI != 9 || mediumOLI != 10 ||
		mediumPre != 8 || mediumH2 != 3 {
		t.Fatalf("Medium native paragraph types are incompatible with observed editor")
	}
	blocks, warnings := parseMediumBlocks(`> **Quoted sentence**

- First bullet
- Second bullet

1. Ordered`)
	if len(warnings) != 0 {
		t.Fatalf("unexpected compile warnings: %v", warnings)
	}
	got := make([]int, 0, len(blocks))
	for _, b := range blocks {
		got = append(got, b.ParagraphType)
	}
	want := []int{6, 9, 9, 10}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("native paragraph types=%v want %v", got, want)
	}
}

func TestMediumSectionReferencesUseOriginalArticleInsteadOfBrokenMediumFragments(t *testing.T) {
	profile := PlatformConfig{Language: "en"}
	payload, _ := buildMediumPayload(Article{Title: "Test"}, "example", profile, "See [**section**](#12-execution-layers) and [docs](https://example.org/docs#ok).")
	links := payload.Deltas[0].Paragraph.Markups
	found := false
	for _, link := range links {
		if link.Type != mediumMarkupLink {
			continue
		}
		if link.Href == CanonicalURL("example", "en")+"#12-execution-layers" {
			found = true
		}
		if strings.HasPrefix(link.Href, "#") {
			t.Fatalf("broken Medium fragment: %s", link.Href)
		}
	}
	if !found {
		t.Fatalf("missing original section link: %+v", links)
	}
	copyHTML := buildMediumCopyHTML(Article{Title: "Test"}, "example", profile, "See [section](#12-execution-layers).")
	if !strings.Contains(copyHTML, CanonicalURL("example", "en")+"#12-execution-layers") {
		t.Fatal("copy fallback has a broken fragment")
	}
}

func TestMediumCopyrightFooterUsesOneNativeBlockquote(t *testing.T) {
	profile := defaultPlatformConfig("medium")
	footer := mediumFooter(Article{Title: "Example"}, "https://thinkerqaq.com/en/articles/example/", profile)
	if footer == nil || !strings.Contains(footer.Text, "© ThinkerQAQ") || !strings.Contains(footer.Text, "Original article") {
		t.Fatalf("missing copyright notice in Medium native footer: %+v", footer)
	}
	if strings.Contains(footer.Text, "> ") {
		t.Fatalf("raw markdown leaked: %+v", footer)
	}
}

func TestMediumNativePayloadUsesContentCopyright(t *testing.T) {
	profile := defaultPlatformConfig("medium")
	source := blogpublishing.Copyright("en")
	source.Syndication = "CUSTOM MEDIUM RIGHTS"
	article := Article{Title: "Test"}
	payload, _ := buildMediumPayload(article, "example", profile, "Test content", source)
	if len(payload.Deltas) == 0 || !strings.Contains(payload.Deltas[len(payload.Deltas)-1].Paragraph.Text, "CUSTOM MEDIUM RIGHTS") {
		t.Fatalf("missing copyright in Medium delta: %+v", payload.Deltas)
	}
	html := buildMediumCopyHTML(article, "example", profile, "Test content", source)
	if !strings.Contains(html, "CUSTOM MEDIUM RIGHTS") {
		t.Fatalf("missing copyright in Medium fallback")
	}
}

package compiler

import (
	"os"
	"strings"
	"testing"
)

func TestMediumPreservesMarkdownTableAsImageWithoutFlattening(t *testing.T) {
	markdown := "Before\n\n| Question | Meaning |\n| --- | --- |\n| **Atomicity** | `count++` needs `read → write` |\n| **Visibility** | Existing **bold** text |\n\nAfter\n"
	portable, assets, err := CompileMediumPublishingMarkdown(markdown, DefaultAssetBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].Kind != "table" || !strings.Contains(assets[0].Content, "**Atomicity**") {
		t.Fatalf("incorrect table asset: %+v", assets)
	}
	if strings.Contains(portable, "| --- |") || strings.Contains(portable, "**Atomicity** —") || !strings.Contains(portable, "![Comparison table:") {
		t.Fatalf("table should be an image, got %q", portable)
	}
	blocks, warnings := parseMediumBlocks(portable)
	if len(warnings) > 0 || len(blocks) != 3 || blocks[0].Text != "Before" || blocks[1].Kind != "image" || blocks[2].Text != "After" {
		t.Fatalf("wrong paragraph structure: blocks=%+v warnings=%v", blocks, warnings)
	}
	// Non-Medium platforms must keep the original Markdown table.
	ordinary, other, err := CompilePublishingMarkdown(markdown, DefaultAssetBaseURL)
	if err != nil || len(other) != 0 || !strings.Contains(ordinary, "| **Atomicity** |") {
		t.Fatalf("non-Medium table damaged: %q %v", ordinary, err)
	}
}

func TestMediumTableInsideFenceRemainsCode(t *testing.T) {
	markdown := "```go\n| H | V |\n| --- | --- |\n| 1 | 2 |\n```\n"
	rendered, assets, err := CompileMediumPublishingMarkdown(markdown, DefaultAssetBaseURL)
	if err != nil || len(assets) != 0 || !strings.Contains(rendered, "| --- |") {
		t.Fatalf("fenced code must not turn into a table: %q %+v %v", rendered, assets, err)
	}
}

func TestMediumTableFallbackAvoidsDuplicateBoldMarkers(t *testing.T) {
	raw := "| Question | Meaning |\n| --- | --- |\n| **Atomicity** | `count++` must be atomic |\n"
	blocks, warnings := parseMediumBlocks(raw)
	if len(warnings) > 0 || len(blocks) != 1 {
		t.Fatalf("blocks=%+v warnings=%v", blocks, warnings)
	}
	if strings.Contains(blocks[0].Text, "**") || blocks[0].Text != "Atomicity — count++ must be atomic" {
		t.Fatalf("table fallback leaked Markdown: %q", blocks[0].Text)
	}
	if len(blocks[0].Markups) != 2 {
		t.Fatalf("expected bold and code markups, got %+v", blocks[0].Markups)
	}
}

func TestMediumSeriesZeroTableAndGoCodeStructure(t *testing.T) {
	source := "/mnt/c/Users/zsk/code/blog/blog-content/src/content/articles/en/concurrency-series-00.md"
	data, err := os.ReadFile(source)
	if err != nil {
		t.Skipf("integration source not present: %v", err)
	}
	article, err := ParseArticle(string(data), source)
	if err != nil {
		t.Fatal(err)
	}
	portable, assets, err := CompileMediumPublishingMarkdown(article.Body, DefaultAssetBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	var tableCount int
	for _, a := range assets {
		if a.Kind == "table" {
			tableCount++
		}
	}
	if tableCount != 1 {
		t.Fatalf("article 0 has %d table assets, expected one", tableCount)
	}
	payload, _ := buildMediumPayload(article, "concurrency-series-00", PlatformConfig{Language: "en"}, portable)
	if len(payload.BrowserPlan.Images) != 6 {
		t.Fatalf("expected original five pictures plus one table, got %d", len(payload.BrowserPlan.Images))
	}
	var goBlocks int
	for _, d := range payload.BrowserPlan.Deltas {
		if strings.Contains(d.Paragraph.Text, "Goroutine A") && strings.Contains(d.Paragraph.Text, "Goroutine B") && d.Paragraph.Type == mediumPre {
			goBlocks++
		}
		if strings.Contains(d.Paragraph.Text, "**Atomicity**") {
			t.Fatalf("leaked inline Markdown %q", d.Paragraph.Text)
		}
	}
	if goBlocks != 1 {
		t.Fatalf("Go source must remain one Medium PRE, got %d", goBlocks)
	}
}

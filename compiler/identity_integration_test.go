package compiler

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ThinkerQAQ/syndirelay/platform"
	"github.com/ThinkerQAQ/syndirelay/publishing"
)

func TestAllPlatformPublishingUsesOnlyConfiguredSiteAndCopyright(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"src/content/articles", "src/content/articles/en"} {
		filename := filepath.Join(root, dir, "fixture.md")
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		data := "---\ntitle: Site-agnostic test\ndescription: Example summary\ntags: [go, redis]\nstatus: published\ncoverImage: /cover.png\ncoverImageAlt: Cover\n---\n# Content\n\nThis is an **article**.\n"
		if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := publishing.DefaultConfig()
	cfg.Site = publishing.SiteConfig{Origin: "https://independent.example.org", NameZH: "独立作者站点", NameEN: "Independent Author Blog", ArticlesZH: "/notes/", ArticlesEN: "/en/notes/"}
	cfg.Copyright = publishing.CopyrightConfig{Mode: "config", ZH: publishing.CopyrightText{Syndication: "版权所有：独立作者。允许非商业转载。", Source: "原始文章"}, EN: publishing.CopyrightText{Syndication: "© Independent Author. License: CC BY-NC-SA 4.0.", Source: "Original article"}}
	cfg.Assets = publishing.AssetsConfig{Store: "none"}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source unavailable")
	}
	engine := filepath.Clean(filepath.Join(filepath.Dir(source), ".."))
	for _, id := range platform.IDs() {
		t.Run(id, func(t *testing.T) {
			result, err := CompilePlatform(context.Background(), CompileOptions{EngineRoot: engine, ContentRoot: root, Publishing: cfg, Platform: id, Articles: []string{"fixture"}, Node: "node", DryRun: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(result) != 1 {
				t.Fatalf("expected 1 article, got %d", len(result))
			}
			out := result[0]
			english := cfg.Platforms[id].Language == "en"
			expected := cfg.Site.Origin + cfg.Site.ArticlePath(out.Language) + "fixture/"
			if out.CanonicalURL != expected {
				t.Fatalf("canonical=%q expected=%q", out.CanonicalURL, expected)
			}
			payload := out.Markdown + out.HTML + out.FallbackHTML + string(out.Payload)
			if strings.Contains(strings.ToLower(payload), "thinkerqaq") || strings.Contains(payload, "pub-366a15") {
				t.Fatal("developer branding escaped into compiled output")
			}
			want := "版权所有：独立作者"
			if english {
				want = "© Independent Author"
			}
			if !strings.Contains(payload, want) {
				t.Fatalf("missing configured license for %s", id)
			}
			if !strings.Contains(payload, cfg.Site.Origin) {
				t.Fatal("missing configured site link")
			}
			if !strings.Contains(payload, "Independent Author Blog") && !strings.Contains(payload, "独立作者站点") {
				t.Fatal("configured site display name not found")
			}
		})
	}
}

func TestConfiguredSiteWithNoPolicyNeverInventsCopyright(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "src/content/articles/en")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture.md"), []byte("---\ntitle: My Test\ndescription: Brief overview\nstatus: published\ncoverImage: /cover.png\ncoverImageAlt: Cover\n---\nA simple article."), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := publishing.DefaultConfig()
	cfg.Site.Origin = "https://another.example.com"
	_, source, _, _ := runtime.Caller(0)
	out, err := CompilePlatform(context.Background(), CompileOptions{EngineRoot: filepath.Clean(filepath.Join(filepath.Dir(source), "..")), ContentRoot: root, Publishing: cfg, Platform: "devto", Articles: []string{"fixture"}, Node: "node", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out[0].Markdown, "©") || strings.Contains(out[0].Markdown, "ThinkerQAQ") {
		t.Fatal("unexpected implicit copyright attribution")
	}
	if !strings.Contains(out[0].Markdown, "another.example.com") {
		t.Fatal("missing configured origin")
	}
}

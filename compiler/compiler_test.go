package compiler

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	blogpublishing "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publishing"
)

const articleFixture = `---
title: "并发文章"
description: "这是一篇用于测试多平台分发的文章。"
tags:
  - Java
  - Go
status: published
coverImage: "/media/articles/test/cover.png"
coverImageAlt: "Cover"
---

## 正文

[站内链接](/articles/another/)
`

func TestParseArticleParity(t *testing.T) {
	article, err := ParseArticle(articleFixture, "fixture.md")
	if err != nil {
		t.Fatal(err)
	}
	if article.Title != "并发文章" || article.Status != "published" || len(article.Tags) != 2 {
		t.Fatalf("article = %#v", article)
	}
}

func TestTrackedURLKeepsLegacyParameterOrder(t *testing.T) {
	config := defaultPlatformConfig("juejin")
	got := trackedURL("https://thinkerqaq.com/articles/example/", config)
	want := "https://thinkerqaq.com/articles/example/?utm_source=juejin&utm_medium=referral&utm_campaign=article_syndication"
	if got != want {
		t.Fatalf("tracked URL = %q, want %q", got, want)
	}
}

func TestCompilePublishingMarkdownNormalizesLinksAndDiagrams(t *testing.T) {
	markdown := strings.Join([]string{
		"[outside](/articles/a/)",
		"<img src='/media/a.png'>",
		"```mermaid",
		"flowchart LR",
		"  accTitle: Lock path",
		"  A --> B",
		"```",
		"```text",
		"[inside](/articles/b/)",
		"```",
	}, "\n")
	output, assets, err := CompilePublishingMarkdown(markdown, DefaultAssetBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].Kind != "mermaid" || len(assets[0].ID) != 24 || assets[0].Alt != "Lock path" {
		t.Fatalf("assets = %#v", assets)
	}
	if !strings.Contains(output, "[outside](https://thinkerqaq.com/articles/a/)") {
		t.Fatalf("root link not normalized: %s", output)
	}
	if !strings.Contains(output, "<img src='https://thinkerqaq.com/media/a.png'>") {
		t.Fatalf("HTML link quote was not preserved: %s", output)
	}
	if !strings.Contains(output, "[inside](/articles/b/)") {
		t.Fatalf("code fence link changed: %s", output)
	}
	if strings.Contains(output, "flowchart LR") {
		t.Fatalf("diagram source leaked into output: %s", output)
	}
}

func TestHashnodeEnglishSourceTrackingAndCanonical(t *testing.T) {
	profile := defaultPlatformConfig("hashnode")
	if profile.Language != "en" || profile.Canonical.Mode != "native" {
		t.Fatalf("Hashnode should use English source and native canonical: %+v", profile)
	}
	canonical := "https://thinkerqaq.com/en/articles/concurrency-series-00/"
	footer := renderFooter(profile, canonical, "Concurrency Programming")
	if !strings.Contains(footer, "utm_source=hashnode&utm_medium=referral&utm_campaign=article_syndication") {
		t.Fatalf("Hashnode source tracking missing: %s", footer)
	}
	if !strings.Contains(footer, "ThinkerQAQ's personal blog") {
		t.Fatalf("Hashnode must generate English footer: %s", footer)
	}
	article, err := ParseArticle(strings.Replace(articleFixture, "status: published", "series: concurrency-programming\nstatus: published", 1), "fixture.md")
	if err != nil || article.Series != "concurrency-programming" {
		t.Fatalf("series metadata lost: %+v %v", article, err)
	}
}

func TestCopyrightFooterWithLegacyCustomTemplate(t *testing.T) {
	original := "https://thinkerqaq.com/articles/example/"
	for _, language := range []string{"zh-CN", "en"} {
		profile := defaultPlatformConfig("juejin")
		profile.Language = language
		profile.Footer.Template = "> {site} [{title}]({url})"
		footer := renderFooter(profile, original, "Example")
		if !strings.Contains(footer, "[Example]") || !strings.Contains(footer, "utm_source=juejin") {
			t.Fatalf("legacy tracking footer changed: %s", footer)
		}
		if !strings.Contains(footer, "© ThinkerQAQ") || !strings.Contains(footer, original+")") {
			t.Fatalf("missing copyright or untracked original link: %s", footer)
		}
		if strings.Count(footer, "© ThinkerQAQ") != 1 {
			t.Fatalf("duplicate copyright: %s", footer)
		}
		profile.Footer.Enabled = false
		if got := renderFooter(profile, original, "Example"); got != "" {
			t.Fatalf("disabled footer must remain disabled: %s", got)
		}
	}
}

func TestContentCopyrightChangesSyndicationFooterWithoutChangingTemplate(t *testing.T) {
	profile := defaultPlatformConfig("devto")
	canonical := "https://thinkerqaq.com/en/articles/example/"
	source := blogpublishing.Copyright("en")
	original := renderFooter(profile, canonical, "Article")
	source.Syndication = "CUSTOM CONTENT COPYRIGHT"
	footer := renderFooter(profile, canonical, "Article", source)
	if !strings.Contains(footer, "CUSTOM CONTENT COPYRIGHT") || strings.Contains(original, "CUSTOM CONTENT COPYRIGHT") {
		t.Fatalf("no override: %s", footer)
	}
	if !strings.Contains(footer, canonical+")") || !strings.Contains(footer, "utm_source=devto") {
		t.Fatalf("lost existing canonical attribution: %s", footer)
	}
}

func TestCompilePlatformReadsContentCopyrightForDevtoAndMedium(t *testing.T) {
	contentRoot := t.TempDir()
	policyRoot := filepath.Join(contentRoot, "src", "content", "site")
	if err := os.MkdirAll(policyRoot, 0755); err != nil {
		t.Fatal(err)
	}
	json := `{"zh":{"title":"版权","notice":"版权正文","photo":"摄影版权","syndication":"SYNDCUSTOM_ZH","source":"原文"},"en":{"title":"Copyright","notice":"Original text","photo":"Photography","syndication":"SYNDCUSTOM_EN","source":"Original"}}`
	if err := os.WriteFile(filepath.Join(policyRoot, "copyright.json"), []byte(json), 0644); err != nil {
		t.Fatal(err)
	}
	articleRoot := filepath.Join(contentRoot, "src", "content", "articles", "en")
	if err := os.MkdirAll(articleRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articleRoot, "example.md"), []byte(articleFixture), 0644); err != nil {
		t.Fatal(err)
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("missing caller path")
	}
	engineRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	for _, platform := range []string{"devto", "medium"} {
		options := CompileOptions{EngineRoot: engineRoot, ContentRoot: contentRoot, Publishing: blogpublishing.DefaultConfig(), Node: "node", Platform: platform, Articles: []string{"example"}, DryRun: true}
		compiled, err := CompilePlatform(context.Background(), options)
		if err != nil {
			t.Fatalf("%s compile: %v", platform, err)
		}
		if len(compiled) != 1 {
			t.Fatalf("%s results: %d", platform, len(compiled))
		}
		if platform == "devto" {
			if !strings.Contains(compiled[0].Markdown, "SYNDCUSTOM_EN") || !strings.Contains(compiled[0].HTML, "SYNDCUSTOM_EN") {
				t.Fatalf("%s did not load content policy", platform)
			}
		} else {
			if !strings.Contains(compiled[0].HTML, "SYNDCUSTOM_EN") || !strings.Contains(string(compiled[0].Payload), "SYNDCUSTOM_EN") {
				t.Fatalf("%s did not load content policy", platform)
			}
		}
	}
}

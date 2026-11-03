package compiler

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	blogpublishing "github.com/ThinkerQAQ/syndirelay/publishing"
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
	output, assets, err := CompilePublishingMarkdown(markdown, "https://assets.example.org/")
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].Kind != "mermaid" || len(assets[0].ID) != 24 || assets[0].Alt != "Lock path" {
		t.Fatalf("assets = %#v", assets)
	}
	if !strings.Contains(output, "[outside](/articles/a/)") {
		t.Fatalf("root link not normalized: %s", output)
	}
	if !strings.Contains(output, "<img src='/media/a.png'>") {
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
	if !strings.Contains(footer, "thinkerqaq.com") {
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
		footer := renderFooter(profile, original, "Example", blogpublishing.CopyrightText{Syndication: "© Fixture", Source: "Original article"})
		if !strings.Contains(footer, "[Example]") || !strings.Contains(footer, "utm_source=juejin") {
			t.Fatalf("legacy tracking footer changed: %s", footer)
		}
		if !strings.Contains(footer, "© Fixture") || !strings.Contains(footer, ">\n> © Fixture") {
			t.Fatalf("missing copyright or untracked original link: %s", footer)
		}
		if strings.Count(footer, "© Fixture") != 1 {
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
	source := blogpublishing.CopyrightText{Syndication: "© Fixture", Source: "Original article"}
	original := renderFooter(profile, canonical, "Article")
	source.Syndication = "CUSTOM CONTENT COPYRIGHT"
	footer := renderFooter(profile, canonical, "Article", source)
	if !strings.Contains(footer, "CUSTOM CONTENT COPYRIGHT") || strings.Contains(original, "CUSTOM CONTENT COPYRIGHT") {
		t.Fatalf("no override: %s", footer)
	}
	if strings.Contains(footer, "[Original article](") || !strings.Contains(footer, "utm_source=devto") {
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
		publishing := blogpublishing.DefaultConfig()
		publishing.Site.Origin = "https://example.org"
		options := CompileOptions{EngineRoot: engineRoot, ContentRoot: contentRoot, Publishing: publishing, Node: "node", Platform: platform, Articles: []string{"example"}, DryRun: true}
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

func TestFooterIsSingleBlockquoteWithoutRedundantOriginalLink(t *testing.T) {
	for _, platform := range blogpublishing.PlatformIDs() {
		for _, lang := range []string{"zh-CN", "en"} {
			profile := defaultPlatformConfig(platform)
			profile.Language = lang
			origin := "https://example.org/articles/example/"
			copyright := blogpublishing.CopyrightText{Syndication: "© Author. CC BY-NC-SA 4.0", Source: "Original article"}
			footer := renderFooter(profile, origin, "Example", copyright)
			if strings.Count(footer, "https://example.org") != 1 {
				t.Fatalf("%s/%s canonical must occur once: %q", platform, lang, footer)
			}
			if strings.Count(footer, "© Author") != 1 {
				t.Fatalf("%s/%s copyright duplicated", platform, lang)
			}
			if !strings.Contains(footer, "\n>\n> © Author") {
				t.Fatalf("%s/%s separated blockquotes: %q", platform, lang, footer)
			}
			if strings.Contains(footer, "[Original article]") || strings.Contains(footer, "[原文]") {
				t.Fatalf("%s/%s redundant source label", platform, lang)
			}
		}
	}
}

func TestCopyrightPlainParagraphForUnquotedTemplate(t *testing.T) {
	p := defaultPlatformConfig("devto")
	p.Footer.Template = "See [{site}]({url})."
	v := renderFooter(p, "https://example.org/articles/a/", "Article", blogpublishing.CopyrightText{Syndication: "© Licensed"})
	if !strings.Contains(v, ".\n\n© Licensed") || strings.Contains(v, "\n> ©") {
		t.Fatal(v)
	}
}

func TestCnblogsAndCSDNFooterUsesReadableParagraphsAndBoundedLicenseLink(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en"} {
		profile := defaultPlatformConfig("cnblogs")
		profile.Language = lang
		original := renderFooter(profile, "https://example.org/articles/test/", "Article",
			blogpublishing.CopyrightText{Syndication: "© Author. CC BY-NC-SA 4.0（https://creativecommons.org/licenses/by-nc-sa/4.0/deed.zh-hans）：允许非商业转载与改编；商业使用需另行授权。"})
		got := readableEditorFooter(original)
		if strings.Contains(got, "\n>") || strings.HasPrefix(got, ">") {
			t.Fatalf("unexpected blockquote: %s", got)
		}
		if !strings.Contains(got, "\n\n© Author.") {
			t.Fatalf("copyright not separated: %s", got)
		}
		if !strings.Contains(got, "[CC BY-NC-SA 4.0](https://creativecommons.org/licenses/by-nc-sa/4.0/deed.zh-hans)") {
			t.Fatalf("license not linked: %s", got)
		}
		if strings.Contains(got, "（https://creativecommons.org") {
			t.Fatalf("bare URL remains: %s", got)
		}
		if !strings.Contains(got, "商业使用需另行授权") {
			t.Fatalf("license text changed: %s", got)
		}
		if strings.Count(got, "https://example.org/articles/test/") != 1 {
			t.Fatalf("duplicate canonical URL: %s", got)
		}
	}
}

func TestPolicyLicenseLinksAreMarkdownOnEveryPlatform(t *testing.T) {
	for _, platform := range blogpublishing.PlatformIDs() {
		for _, language := range []string{"zh-CN", "en"} {
			profile := defaultPlatformConfig(platform)
			profile.Language = language
			policy := blogpublishing.CopyrightText{Syndication: "© Author. CC BY-NC-SA 4.0（https://creativecommons.org/licenses/by-nc-sa/4.0/deed.zh-hans）：允许非商业转载与改编；商业使用需另行授权。"}
			rendered := renderFooter(profile, "https://example.org/articles/example/", "Example", policy)
			if profile.Footer.Enabled && !strings.Contains(rendered, "[CC BY-NC-SA 4.0](https://creativecommons.org/licenses/by-nc-sa/4.0/deed.zh-hans)") {
				t.Fatalf("%s/%s: license not formal Markdown link: %s", platform, language, rendered)
			}
			if strings.Contains(rendered, "（https://creativecommons.org") {
				t.Fatalf("%s/%s: bare license URL: %s", platform, language, rendered)
			}
			if strings.Contains(rendered, "[原文](") || strings.Contains(rendered, "[Original article](") {
				t.Fatalf("%s/%s: duplicate source link", platform, language)
			}
		}
	}
}

func TestPolicyLicenseNormalizationIsIdempotentAndRespectsCustomContent(t *testing.T) {
	cases := []struct{ policy, expected string }{
		{"采用 CC BY-NC-SA 4.0（https://creativecommons.org/licenses/by-nc-sa/4.0/deed.zh-hans）：授权", "采用 [CC BY-NC-SA 4.0](https://creativecommons.org/licenses/by-nc-sa/4.0/deed.zh-hans)：授权"},
		{"See CC BY-NC-SA 4.0 (https://creativecommons.org/licenses/by-nc-sa/4.0/deed.en).", "See [CC BY-NC-SA 4.0](https://creativecommons.org/licenses/by-nc-sa/4.0/deed.en)."},
		{"自定义协议（https://example.org/license）：仅供查阅。", "自定义协议（[许可协议](https://example.org/license)）：仅供查阅。"},
		{"Already [MIT](https://opensource.org/licenses/MIT) licensed.", "Already [MIT](https://opensource.org/licenses/MIT) licensed."},
	}
	for _, tc := range cases {
		got := normalizeFooterPolicyLinks(tc.policy, "zh-CN")
		if got != tc.expected {
			t.Fatalf("want %q got %q", tc.expected, got)
		}
		if normalizeFooterPolicyLinks(got, "zh-CN") != got {
			t.Fatalf("not idempotent: %s", got)
		}
	}
}

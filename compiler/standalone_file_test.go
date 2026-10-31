package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blogplatform "github.com/ThinkerQAQ/syndirelay/platform"
)

func genericFixture(t *testing.T, markdown string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "hello.md")
	if err := os.WriteFile(file, []byte(markdown), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestCompilePlainMarkdownWithoutInkerioForEveryPlatform(t *testing.T) {
	file := genericFixture(t, "# Hello World\n\nA simple **independent** Markdown document.\n\n```go\nfmt.Println(1)\n```\n")
	for _, platform := range blogplatform.IDs() {
		t.Run(platform, func(t *testing.T) {
			article, err := CompileFile(context.Background(), FileCompileOptions{Path: file, Platform: platform})
			if err != nil {
				t.Fatal(err)
			}
			if article.Platform != platform || article.Title != "Hello World" || article.CanonicalURL != "" {
				t.Fatalf("invalid standalone article: %s %#v", platform, article)
			}
			if !strings.Contains(article.Markdown, "independent") {
				t.Fatalf("source content lost: %s", platform)
			}
			if strings.Contains(article.HTML+article.Markdown+string(article.Payload), "thinkerqaq.com") || strings.Contains(article.HTML+article.Markdown+string(article.Payload), "ThinkerQAQ") {
				t.Fatal("generic compilation leaked hardcoded blog identity")
			}
		})
	}
}

func TestStandaloneFrontmatterOptional(t *testing.T) {
	file := genericFixture(t, "---\ntitle: Custom Heading\ndescription: Custom description\ntags: [go, redis]\n---\nBody text.")
	got, err := CompileFile(context.Background(), FileCompileOptions{Path: file, Platform: "devto", CanonicalURL: "https://example.org/post/"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Custom Heading" || got.CanonicalURL != "https://example.org/post/" || len(got.Tags) != 2 {
		t.Fatalf("frontmatter not preserved: %#v", got)
	}
	if !strings.Contains(got.Markdown, "Body text.") {
		t.Fatal("body missing")
	}
}

func TestStandaloneRejectsInvalidCanonicalAndUnconfiguredDiagrams(t *testing.T) {
	file := genericFixture(t, "# Graph\n\n```mermaid\ngraph LR\nA --> B\n```")
	if _, err := CompileFile(context.Background(), FileCompileOptions{Path: file, Platform: "devto", CanonicalURL: "https://user:password@example.org/"}); err == nil {
		t.Fatal("credential-bearing URL accepted")
	}
	if _, err := CompileFile(context.Background(), FileCompileOptions{Path: file, Platform: "devto"}); err == nil || !strings.Contains(err.Error(), "asset-base-url") {
		t.Fatalf("expected missing media host error, got %v", err)
	}
	got, err := CompileFile(context.Background(), FileCompileOptions{Path: file, Platform: "devto", AssetBaseURL: "https://assets.example.org/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Assets) != 1 || !strings.HasPrefix(got.Assets[0].PublicURL, "https://assets.example.org/") {
		t.Fatalf("unexpected media: %#v", got.Assets)
	}
}

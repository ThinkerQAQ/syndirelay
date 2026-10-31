package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListArticlesIncludesSourceLanguageAndEnglishMirror(t *testing.T) {
	root := t.TempDir()
	articleRoot := filepath.Join(root, "src", "content", "articles")
	if err := os.MkdirAll(filepath.Join(articleRoot, "en", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(articleRoot, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	cn := "---\ntitle: 中文标题\nstatus: published\n---\n正文\n"
	en := "---\ntitle: English title\nstatus: published\n---\nBody\n"
	if err := os.WriteFile(filepath.Join(articleRoot, "nested", "example.md"), []byte(cn), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articleRoot, "en", "nested", "example.md"), []byte(en), 0o644); err != nil {
		t.Fatal(err)
	}

	articles, err := listArticles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 {
		t.Fatalf("articles = %#v", articles)
	}
	got := articles[0]
	if got.Slug != "nested/example" || got.Language != "zh-CN" || !got.EnglishMirror || got.SourcePath != "nested/example.md" {
		t.Fatalf("article = %#v", got)
	}
}

func TestListArticlesRequiresPublishedEnglishMirror(t *testing.T) {
	root := t.TempDir()
	articleRoot := filepath.Join(root, "src", "content", "articles")
	if err := os.MkdirAll(filepath.Join(articleRoot, "en"), 0o755); err != nil {
		t.Fatal(err)
	}
	cn := "---\ntitle: 中文标题\nstatus: published\n---\n正文\n"
	en := "---\ntitle: English title\nstatus: draft\n---\nBody\n"
	if err := os.WriteFile(filepath.Join(articleRoot, "example.md"), []byte(cn), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articleRoot, "en", "example.md"), []byte(en), 0o644); err != nil {
		t.Fatal(err)
	}
	articles, err := listArticles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 || articles[0].EnglishMirror {
		t.Fatalf("articles = %#v", articles)
	}
}

func TestListArticlesReportsMissingEnglishMirror(t *testing.T) {
	root := t.TempDir()
	articleRoot := filepath.Join(root, "src", "content", "articles")
	if err := os.MkdirAll(articleRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articleRoot, "only-cn.md"), []byte("---\ntitle: Only CN\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	articles, err := listArticles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 || articles[0].EnglishMirror {
		t.Fatalf("articles = %#v", articles)
	}
}

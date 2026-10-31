package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAISearchPrepareUsesExplicitContentCheckout(t *testing.T) {
	// The site checkout intentionally contains no src/content tree.
	legacy := t.TempDir()
	content := t.TempDir()
	root := filepath.Join(content, "src", "content")
	for _, collection := range []string{"articles", "notes", "note-translations"} {
		if err := os.MkdirAll(filepath.Join(root, collection), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	article := filepath.Join(root, "articles", "sample.md")
	if err := os.WriteFile(article, []byte("---\nstatus: published\n---\n# Sample\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONTENT_SHA", "")
	t.Setenv("CONTENT_BEFORE_SHA", "")
	t.Setenv("AI_SEARCH_FORCE_FULL_SYNC", "")
	out := filepath.Join(t.TempDir(), "ai-search-input")
	a := app{root: legacy, out: &bytes.Buffer{}}
	if err := a.runAISearchPrepare([]string{"--output", out, "--content-root", content}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "articles", "sample.md")); err != nil {
		t.Fatalf("expected public article copied from separate content checkout: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(out, "ai-search-changed-paths.txt")); err != nil || len(b) != 0 {
		t.Fatalf("empty change manifest expected for a non-versioned prepare, data=%q err=%v", b, err)
	}
}

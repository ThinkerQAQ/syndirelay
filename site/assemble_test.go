package site

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssembleCopiesPagesAndReplacesCopyrightPolicy(t *testing.T) {
	engine := t.TempDir()
	source := t.TempDir()
	for name, body := range map[string]string{
		"src/content/pages/about.md":      "about content",
		"src/content/pages/en/about.md":   "english about",
		"src/content/site/copyright.json": `{"zh":{"notice":"first"},"en":{"notice":"first"}}`,
	} {
		file := filepath.Join(source, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Assemble(engine, source); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pages/about.md", "pages/en/about.md", "site/copyright.json"} {
		if _, err := os.Stat(filepath.Join(engine, "src", "content", filepath.FromSlash(name))); err != nil {
			t.Fatal(err)
		}
	}
	otherSource := t.TempDir()
	if err := os.MkdirAll(filepath.Join(otherSource, "src", "content"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Assemble(engine, otherSource); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pages/about.md", "pages/en/about.md", "site/copyright.json"} {
		if _, err := os.Stat(filepath.Join(engine, "src", "content", filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Fatalf("stale content %s: %v", name, err)
		}
	}
}

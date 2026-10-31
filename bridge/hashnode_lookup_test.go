package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashnodeEnglishTitleIgnoresChineseSource(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "src", "content", "articles", "en")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: 'Concurrency Programming (0): Overview'\ndescription: Description\nstatus: published\n---\nBody\n"
	if err := os.WriteFile(filepath.Join(directory, "example.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := hashnodeEnglishTitle(root, "example")
	if err != nil || got != "Concurrency Programming (0): Overview" {
		t.Fatalf("Hashnode matching must use English title: %q %v", got, err)
	}
	if _, err := hashnodeEnglishTitle(root, "../example"); err == nil {
		t.Fatal("traversal slug must not be accepted")
	}
	if _, err := hashnodeEnglishTitle(root, "does-not-exist"); err == nil {
		t.Fatal("English article missing must fail closed")
	}
}

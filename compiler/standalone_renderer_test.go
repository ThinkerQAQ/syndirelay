package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStandaloneRendererRootFindsPortableBinParent(t *testing.T) {
	root := t.TempDir()
	renderer := filepath.Join(root, "renderers", "node")
	if err := os.MkdirAll(renderer, 0755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{filepath.Join(root, "package.json"), filepath.Join(renderer, "markdown-html.mjs")} {
		if err := os.WriteFile(file, []byte("fixture"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()
	exe := filepath.Join(bin, "syndirelay.exe")
	if got := standaloneRendererRoot(elsewhere, exe); got != root {
		t.Fatalf("portable install was not resolved from bin/: got %s, want %s", got, root)
	}
	if got := standaloneRendererRoot(root, exe); got != root {
		t.Fatalf("valid current checkout should be accepted: %s", got)
	}
	if got := standaloneRendererRoot(elsewhere, filepath.Join(elsewhere, "not-installed.exe")); got != elsewhere {
		t.Fatalf("unexpected search root: %s", got)
	}
}

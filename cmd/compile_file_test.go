package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCompileCLIWithoutEngineWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.md")
	if err := os.WriteFile(path, []byte("# Standalone\n\nNo Astro required.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	a := app{out: &out}
	if err := a.run([]string{"compile", "--file", path, "--platform", "devto"}); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["title"] != "Standalone" || result["canonicalUrl"] != "" {
		t.Fatalf("unexpected output: %v", result)
	}
}

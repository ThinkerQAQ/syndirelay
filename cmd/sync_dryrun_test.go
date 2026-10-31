package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncDryRunNeverUpdatesStoredWorkspaceConfig(t *testing.T) {
	base := t.TempDir()
	t.Setenv("SYNDIRELAY_DATA_DIR", base)
	cfgPath := filepath.Join(base, "blogctl.toml")
	original := "ui_locale = 'en'\ncontent_root = '/keep/me/original/content'\nengine_root = '/keep/me/original/runtime'\n"
	if err := os.WriteFile(cfgPath, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	contentRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(contentRoot, "src", "content", "articles"), 0755); err != nil {
		t.Fatal(err)
	}
	engineRoot := t.TempDir() // deliberately invalid: fail before any external process
	var output bytes.Buffer
	a := app{root: engineRoot, contentRoot: contentRoot, out: &output}
	err := a.runSync([]string{"--article", "example", "--platforms", "51cto,medium", "--dry-run"})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("expected invalid workspace; got err=%v output=%q", err, output.String())
	}
	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatal("dry-run modified user configuration")
	}
}

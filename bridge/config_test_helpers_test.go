package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func useIsolatedUserConfigDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("APPDATA", root)
	t.Setenv("BLOGCTL_DATA_DIR", "")

	expected := filepath.Join(root, "BlogCTL")
	t.Setenv("BLOGCTL_DATA_DIR", expected)
	dir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("ConfigDir is not absolute: %q", dir)
	}
	if dir != expected {
		t.Fatalf("ConfigDir = %q, want %q", dir, expected)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

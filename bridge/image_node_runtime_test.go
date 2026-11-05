package bridge

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestImageNodeExecutablePrefersBundledRuntime(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "node"
	if runtime.GOOS == "windows" {
		name = "node.exe"
	}
	want := filepath.Join(bin, name)
	if err := os.WriteFile(want, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := imageNodeExecutable(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got runtime %q; want bundled runtime %q", got, want)
	}
}

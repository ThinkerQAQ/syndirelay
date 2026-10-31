package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeEngineRoot(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "tools", "blogctl"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"package.json", "astro.config.mjs"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func makeContentRoot(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "src", "content", "articles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "data", "content-manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveSyncWorkspaceFromContentRepositoryFindsSiblingEngine(t *testing.T) {
	parent := t.TempDir()
	engineRoot := filepath.Join(parent, "ThinkerQAQ.github.io")
	contentRoot := filepath.Join(parent, "blog-content")
	makeEngineRoot(t, engineRoot)
	makeContentRoot(t, contentRoot)

	engine, content, err := resolveSyncWorkspaceFrom(
		filepath.Join(contentRoot, "src", "content", "articles"),
		func(string) string { return "" },
	)
	if err != nil {
		t.Fatal(err)
	}
	if engine != engineRoot || content != contentRoot {
		t.Fatalf("engine=%q content=%q", engine, content)
	}
}

func TestResolveSyncWorkspaceSupportsExplicitEngineRoot(t *testing.T) {
	parent := t.TempDir()
	engineRoot := filepath.Join(parent, "engine-anywhere")
	contentRoot := filepath.Join(parent, "content-anywhere")
	makeEngineRoot(t, engineRoot)
	makeContentRoot(t, contentRoot)

	getenv := func(name string) string {
		if name == engineRootEnvironment {
			return engineRoot
		}
		return ""
	}
	engine, content, err := resolveSyncWorkspaceFrom(contentRoot, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if engine != engineRoot || content != contentRoot {
		t.Fatalf("engine=%q content=%q", engine, content)
	}
}

func TestResolveSyncWorkspaceRejectsInvalidConfiguredContentRoot(t *testing.T) {
	parent := t.TempDir()
	engineRoot := filepath.Join(parent, "ThinkerQAQ.github.io")
	makeEngineRoot(t, engineRoot)
	invalid := filepath.Join(parent, "not-content")
	if err := os.MkdirAll(invalid, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := resolveSyncWorkspaceFrom(engineRoot, func(name string) string {
		if name == contentRootEnvironment {
			return invalid
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), contentRootEnvironment) {
		t.Fatalf("expected invalid %s error, got %v", contentRootEnvironment, err)
	}
}

func TestWithEnvironmentReplacesExistingValue(t *testing.T) {
	env := withEnvironment([]string{"PATH=/bin", "blog_content_root=old"}, "BLOG_CONTENT_ROOT", "new")
	if got := env[1]; got != "BLOG_CONTENT_ROOT=new" {
		t.Fatalf("got %q", got)
	}
}

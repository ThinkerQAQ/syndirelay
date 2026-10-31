package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParseSyncArgsRequiresExplicitScope(t *testing.T) {
	_, err := parseSyncArgs([]string{"--platforms", "medium"})
	if err == nil {
		t.Fatal("expected missing article scope to fail")
	}
	_, err = parseSyncArgs([]string{"--article", "a"})
	if err == nil {
		t.Fatal("expected missing platform scope to fail")
	}
}

func TestParseSyncArgsRejectsAllAndArticle(t *testing.T) {
	_, err := parseSyncArgs([]string{"--all", "--article", "a", "--platforms", "devto"})
	if err == nil {
		t.Fatal("expected conflicting article scopes to fail")
	}
}

func TestNPMInvocationUsesNodeForWindowsCommandShim(t *testing.T) {
	node := "C:/Program Files/nodejs/node.exe"
	npm := "C:/Program Files/nodejs/npm.cmd"
	program, args := npmInvocation("windows", node, npm, []string{"run", "build"})
	if program != node {
		t.Fatalf("program = %q, want %q", program, node)
	}
	wantCLI := "C:/Program Files/nodejs/node_modules/npm/bin/npm-cli.js"
	if filepath.ToSlash(args[0]) != wantCLI {
		t.Fatalf("npm CLI = %q, want %q", filepath.ToSlash(args[0]), wantCLI)
	}
	if !reflect.DeepEqual(args[1:], []string{"run", "build"}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestNPMInvocationRunsNPMDirectlyOnUnix(t *testing.T) {
	program, args := npmInvocation("linux", "/usr/bin/node", "/usr/bin/npm", []string{"test"})
	if program != "/usr/bin/npm" || !reflect.DeepEqual(args, []string{"test"}) {
		t.Fatalf("program = %q, args = %#v", program, args)
	}
}

func TestSearchOrigin(t *testing.T) {
	for _, property := range []string{"sc-domain:configured.example", "https://configured.example/"} {
		t.Run(property, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("BLOGCTL_DATA_DIR", dir)
			config := "[search]\nsite_origin = 'https://configured.example'\nurl_list_url = 'https://configured.example/sitemap-all.txt'\nsitemap_url = 'https://configured.example/sitemap-index.xml'\ngoogle_property = '" + property + "'\n"
			if err := os.WriteFile(filepath.Join(dir, "blogctl.toml"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			selected, err := searchSiteURL(nil)
			if err != nil || selected != property {
				t.Fatalf("configured selection = %q, err=%v", selected, err)
			}
			got, err := searchOrigin(selected)
			if err != nil || got != "https://configured.example" {
				t.Fatalf("searchOrigin(%q) = %q, err=%v", selected, got, err)
			}
			if _, err := searchOrigin("sc-domain:other.example"); err == nil {
				t.Fatal("expected unconfigured property to be rejected")
			}
			if _, err := searchSiteURL([]string{"--site-url", "sc-domain:other.example"}); err == nil {
				t.Fatal("expected unconfigured CLI selection to be rejected")
			}
		})
	}
}

func TestAISearchDurationAcceptsSecondsAndDuration(t *testing.T) {
	got, err := aiSearchDuration([]string{"--timeout", "15"}, "--timeout", time.Minute)
	if err != nil || got != 15*time.Second {
		t.Fatalf("seconds duration = %s, err=%v", got, err)
	}
	got, err = aiSearchDuration([]string{"--timeout", "2m"}, "--timeout", time.Minute)
	if err != nil || got != 2*time.Minute {
		t.Fatalf("parsed duration = %s, err=%v", got, err)
	}
}

func TestIsZeroSHA(t *testing.T) {
	if !isZeroSHA("000000") {
		t.Fatal("expected all-zero SHA to be detected")
	}
	if isZeroSHA("001000") {
		t.Fatal("non-zero SHA must not be treated as zero")
	}
}

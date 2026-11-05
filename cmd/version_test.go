package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	runtimeversion "github.com/ThinkerQAQ/syndirelay/internal/version"
)

func TestBlogCTLVersionMatchesExtension(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	root := filepath.Dir(filepath.Dir(filename))

	versionBytes, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimSpace(string(versionBytes))

	manifestBytes, err := os.ReadFile(filepath.Join(root, "extension", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name        string   `json:"name"`
		Version     string   `json:"version"`
		Permissions []string `json:"permissions"`
		SidePanel   struct {
			DefaultPath string `json:"default_path"`
		} `json:"side_panel"`
		Action struct {
			DefaultPopup string `json:"default_popup"`
		} `json:"action"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}

	if manifest.Name != "__MSG_extensionName__" {
		t.Fatalf("extension name = %q, want %q", manifest.Name, "__MSG_extensionName__")
	}
	if manifest.Version != version {
		t.Fatalf("extension version = %q, blogctl version = %q", manifest.Version, version)
	}
	if runtimeversion.Current != version {
		t.Fatalf("runtime version = %q, blogctl version = %q", runtimeversion.Current, version)
	}

	for _, filename := range []string{"package.json", "package-lock.json"} {
		content, err := os.ReadFile(filepath.Join(root, filename))
		if err != nil {
			t.Fatal(err)
		}
		var details struct {
			Version  string `json:"version"`
			Packages map[string]struct {
				Version string `json:"version"`
			} `json:"packages"`
		}
		if err := json.Unmarshal(content, &details); err != nil {
			t.Fatal(err)
		}
		if details.Version != version {
			t.Fatalf("%s package version=%q; want %q", filename, details.Version, version)
		}
		if filename == "package-lock.json" && details.Packages[""].Version != version {
			t.Fatalf("package-lock root version=%q; want %q", details.Packages[""].Version, version)
		}
	}
	if manifest.SidePanel.DefaultPath != "popup/popup.html" || manifest.Action.DefaultPopup != "" {
		t.Fatalf("extension sidebar = %q, popup = %q", manifest.SidePanel.DefaultPath, manifest.Action.DefaultPopup)
	}
	permissionFound := false
	for _, permission := range manifest.Permissions {
		if permission == "sidePanel" {
			permissionFound = true
		}
	}
	if !permissionFound {
		t.Fatal("sidePanel permission is missing")
	}
	if _, err := os.Stat(filepath.Join(root, "extension", filepath.FromSlash(manifest.SidePanel.DefaultPath))); err != nil {
		t.Fatalf("extension sidebar missing: %v", err)
	}
}

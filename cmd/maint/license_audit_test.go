package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testLicensePackage(t *testing.T) (root, goMod, lock string, manifest licenseManifest) {
	t.Helper()
	root = t.TempDir()
	goMod = filepath.Join(t.TempDir(), "go.mod")
	lock = filepath.Join(t.TempDir(), "package-lock.json")
	if err := os.WriteFile(goMod, []byte("module example.org/project\n\ngo 1.24\nrequire (\nexample.org/library v1.2.3\n)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, []byte(`{"lockfileVersion":3,"packages":{"":{"name":"test"},"node_modules/sharp":{"version":"0.35.4"},"node_modules/@img/sharp-libvips-darwin-arm64":{"version":"1.2.3","optional":true}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	manifest = licenseManifest{Schema: 1}
	for _, p := range []licenseDependency{
		{Ecosystem: "go", Name: "example.org/library", Version: "v1.2.3", License: "MIT", File: "go/example.org/library/LICENSE", Reviewed: true},
		{Ecosystem: "npm", Name: "node_modules/sharp", Version: "0.35.4", License: "Apache-2.0", File: "npm/sharp/LICENSE", Reviewed: true},
		{Ecosystem: "npm", Name: "node_modules/@img/sharp-libvips-darwin-arm64", Version: "1.2.3", License: "LGPL-3.0-or-later", File: "npm/sharp-libvips-darwin-arm64/LICENSE", Reviewed: true},
	} {
		text := []byte("The complete example fixture license for " + p.Name + " version " + p.Version)
		filename := filepath.Join(root, filepath.FromSlash(p.File))
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, text, 0600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(text)
		p.SHA256 = hex.EncodeToString(sum[:])
		manifest.Dependencies = append(manifest.Dependencies, p)
	}
	return root, goMod, lock, manifest
}
func writeTestManifest(t *testing.T, root string, manifest licenseManifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "MANIFEST.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestReleaseLicenseGateRequiresAllPinnedGoAndNpmPackages(t *testing.T) {
	root, mod, lock, manifest := testLicensePackage(t)
	writeTestManifest(t, root, manifest)
	if err := validateReleaseLicenses(root, mod, lock); err != nil {
		t.Fatal(err)
	}
	// Even an optional macOS binary package absent from Linux must be covered.
	manifest.Dependencies = manifest.Dependencies[:2]
	writeTestManifest(t, root, manifest)
	if err := validateReleaseLicenses(root, mod, lock); err == nil || !strings.Contains(err.Error(), "missing reviewed license") {
		t.Fatalf("expected missing optional package review: %v", err)
	}
}
func TestReleaseLicenseGateRejectsUnreviewedStaleOrTamperedText(t *testing.T) {
	for _, scenario := range []string{"unreviewed", "stale", "tampered", "duplicate", "traversal"} {
		t.Run(scenario, func(t *testing.T) {
			root, mod, lock, m := testLicensePackage(t)
			switch scenario {
			case "unreviewed":
				m.Dependencies[0].Reviewed = false
			case "stale":
				m.Dependencies[0].Version = "v9.9.9"
			case "tampered":
				m.Dependencies[0].SHA256 = strings.Repeat("0", 64)
			case "duplicate":
				m.Dependencies = append(m.Dependencies, m.Dependencies[0])
			case "traversal":
				m.Dependencies[0].File = "go/../../elsewhere/LICENSE"
			}
			writeTestManifest(t, root, m)
			if err := validateReleaseLicenses(root, mod, lock); err == nil {
				t.Fatal("unsafe release license bundle passed")
			}
		})
	}
}
func TestReleaseLicenseGateRejectsOnlyDummyFiles(t *testing.T) {
	root, mod, lock, _ := testLicensePackage(t)
	if err := validateReleaseLicenses(root, mod, lock); err == nil || !strings.Contains(err.Error(), "MANIFEST") {
		t.Fatalf("missing manifest erroneously accepted: %v", err)
	}
}

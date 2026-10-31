package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestExtensionZipContainsLicense(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "background.js"), []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "extension.zip")
	if err := zipDirectory(source, archive, []byte("MIT fixture license")); err != nil {
		t.Fatal(err)
	}
	checkZip(t, archive, map[string]string{
		"background.js": "fixture",
		"LICENSE":       "MIT fixture license",
	})
}

func TestExecutableZipContainsLicenseAndPermissions(t *testing.T) {
	source := filepath.Join(t.TempDir(), "blogctl-linux-amd64")
	if err := os.WriteFile(source, []byte("test binary"), 0755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "binary.zip")
	licenses := filepath.Join(t.TempDir(), "licenses")
	if err := os.MkdirAll(filepath.Join(licenses, "example.org", "library"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(licenses, "example.org", "library", "LICENSE"), []byte("BSD fixture license"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := zipExecutable(source, archive, []byte("MIT fixture license"), licenses); err != nil {
		t.Fatal(err)
	}
	checkZip(t, archive, map[string]string{
		"blogctl-linux-amd64": "test binary",
		"LICENSE":             "MIT fixture license",
		"THIRD_PARTY_LICENSES/example.org/library/LICENSE": "BSD fixture license",
	})
	z, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, entry := range z.File {
		if entry.Name == "blogctl-linux-amd64" && entry.Mode()&0111 == 0 {
			t.Fatal("zip must preserve Unix executable permission")
		}
	}
}

func TestMissingThirdPartyLicensesPreventBundling(t *testing.T) {
	root := t.TempDir()
	_, err := thirdPartyLicenseFiles(filepath.Join(root, "missing"))
	if err == nil {
		t.Fatal("missing third-party licenses must fail")
	}
	if _, err := thirdPartyLicenseFiles(root); err == nil {
		t.Fatal("empty third-party license directory must fail")
	}
}

func checkZip(t *testing.T, pathname string, expected map[string]string) {
	t.Helper()
	z, err := zip.OpenReader(pathname)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, entry := range z.File {
		value, found := expected[entry.Name]
		if !found {
			continue
		}
		r, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil || string(data) != value {
			t.Fatalf("invalid %q data: %v", entry.Name, err)
		}
		delete(expected, entry.Name)
	}
	if len(expected) != 0 {
		t.Fatalf("missing expected zip entries: %v", expected)
	}
}

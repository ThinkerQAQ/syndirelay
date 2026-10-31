//go:build linux || darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndUninstallNativeHostInIsolatedHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	executable := filepath.Join(home, "bin", "blogctl")
	paths := nativeHostManifestPaths(home)
	if len(paths) != 3 {
		t.Fatalf("expected Chrome, Chromium and Edge, got %d", len(paths))
	}
	count, err := registerNativeHost(executable)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("registered %d browsers", count)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !ownedNativeHostManifest(data) {
			t.Fatalf("bad manifest %s", path)
		}
	}
	count, err = unregisterNativeHost(executable)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("removed %d browsers", count)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("manifest still present: %s (%v)", path, err)
		}
	}
	count, err = unregisterNativeHost(executable)
	if err != nil || count != 0 {
		t.Fatalf("second uninstall: %d, %v", count, err)
	}
}

func TestUninstallDoesNotDeleteUnknownHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := nativeHostManifestPaths(home)[0]
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := unregisterNativeHost(filepath.Join(home, "blogctl")); err == nil {
		t.Fatal("uninstall removed an unknown manifest")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

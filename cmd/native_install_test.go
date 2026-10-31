package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestNativeHostManifest(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "blogctl")
	bytes, err := nativeHostManifestBytes(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !ownedNativeHostManifest(bytes) {
		t.Fatal("manifest not accepted by ownership check")
	}
	var decoded map[string]any
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded["path"]; got != executable {
		t.Fatalf("path: got %v, want %s", got, executable)
	}
	if got := decoded["name"]; got != nativeHostName {
		t.Fatalf("name: got %v", got)
	}
	if _, err := nativeHostManifestBytes("relative/blogctl"); err == nil {
		t.Fatal("relative path accepted")
	}
	decoded["name"] = "com.somebody.else"
	wrong, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if ownedNativeHostManifest(wrong) {
		t.Fatal("other host accepted")
	}
}

func TestNativeHostSetupRejectsExtraArguments(t *testing.T) {
	if err := runNativeHostSetup("install", []string{"--unexpected"}); err == nil {
		t.Fatal("unexpected arguments accepted")
	}
}

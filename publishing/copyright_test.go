package publishing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyrightPolicyBilingualAndComplete(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		copy := Copyright(language)
		if copy.Title == "" || copy.Notice == "" || copy.Photo == "" || copy.Syndication == "" || copy.Source == "" {
			t.Fatalf("incomplete policy for %s: %+v", language, copy)
		}
		if !strings.Contains(copy.Notice, "ThinkerQAQ") || !strings.Contains(copy.Syndication, "ThinkerQAQ") || !strings.Contains(copy.Photo, "ThinkerQAQ") {
			t.Fatalf("missing attribution for %s", language)
		}
	}
	if Copyright("en").Title == Copyright("zh-CN").Title {
		t.Fatal("policy should be localized")
	}
}

func TestLoadCopyrightPrefersContentAndRetainsLegacyFallback(t *testing.T) {
	root := t.TempDir()
	fallback, err := LoadCopyright(root, "en")
	if err != nil || fallback != Copyright("en") {
		t.Fatalf("legacy fallback: %v %+v", err, fallback)
	}
	filename := filepath.Join(root, "src", "content", "site", "copyright.json")
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	policy := `{"zh":{"title":"甲","notice":"乙","photo":"丙","syndication":"丁","source":"原文"},"en":{"title":"A","notice":"B","photo":"C","syndication":"CUSTOM COPYRIGHT","source":"Original"}}`
	if err := os.WriteFile(filename, []byte(policy), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := LoadCopyright(root, "en")
	if err != nil || result.Syndication != "CUSTOM COPYRIGHT" {
		t.Fatalf("content override: %v %+v", err, result)
	}
	zh, err := LoadCopyright(root, "zh-CN")
	if err != nil || zh.Title != "甲" {
		t.Fatalf("chinese locale: %v %+v", err, zh)
	}
}

func TestLoadCopyrightRejectsMalformedAndIncompletePolicies(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "src", "content", "site", "copyright.json")
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"{bad json}", `{"zh":{"title":"only title"}}`} {
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadCopyright(root, "en"); err == nil {
			t.Fatalf("accepted invalid content %s", content)
		}
	}
}

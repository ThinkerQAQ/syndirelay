package publishing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyrightIsUnbrandedWithoutOwnerConfiguration(t *testing.T) {
	for _, language := range []string{"zh-CN", "en"} {
		if v := Copyright(language); v.Syndication != "" || v.Source != "" {
			t.Fatalf("unexpected bundled author identity: %+v", v)
		}
	}
}

func TestLoadCopyrightPrefersContentAndMissingFileHasNoBrand(t *testing.T) {
	root := t.TempDir()
	fallback, err := LoadCopyright(root, "en")
	if err != nil || fallback.Syndication != "" {
		t.Fatalf("missing copyright must be empty: %+v %v", fallback, err)
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
}

func TestResolveCopyrightOverridePrecedenceAndModes(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "src", "content", "site", "copyright.json")
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		t.Fatal(err)
	}
	data := `{"zh":{"syndication":"CONTENT-ZH","source":"原文"},"en":{"syndication":"CONTENT-EN","source":"Original"}}`
	if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := CopyrightConfig{Mode: "auto", ZH: CopyrightText{Syndication: "MY-ZH", Source: "来源"}, EN: CopyrightText{Syndication: "MY-EN"}}
	result, err := ResolveCopyright(cfg, root, "zh-CN")
	if err != nil || result.Syndication != "MY-ZH" {
		t.Fatalf("auto override: %+v %v", result, err)
	}
	result, err = ResolveCopyright(cfg, root, "en")
	if err != nil || result.Syndication != "MY-EN" || result.Source != "Original article" {
		t.Fatalf("en override: %+v %v", result, err)
	}
	cfg.EN = CopyrightText{}
	result, err = ResolveCopyright(cfg, root, "en")
	if err != nil || result.Syndication != "CONTENT-EN" {
		t.Fatalf("content fallback: %+v %v", result, err)
	}
	cfg.Mode = "content"
	result, err = ResolveCopyright(cfg, root, "zh-CN")
	if err != nil || result.Syndication != "CONTENT-ZH" {
		t.Fatalf("content selection: %+v %v", result, err)
	}
	cfg.Mode = "none"
	result, err = ResolveCopyright(cfg, root, "zh-CN")
	if err != nil || result.Syndication != "" {
		t.Fatalf("none: %+v %v", result, err)
	}
	cfg.Mode = "config"
	if _, err := ResolveCopyright(cfg, root, "en"); err == nil {
		t.Fatal("missing config must fail")
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

// Auto mode must never conceal an incomplete local policy behind a content
// repository policy (or even silently emit no copyright when none exists).
func TestResolveCopyrightConfiguredInvalidDoesNotFallBack(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "content", "site", "copyright.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"zh":{"syndication":"CONTENT-ZH"},"en":{"syndication":"CONTENT-EN"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	invalid := CopyrightConfig{
		Mode: "auto",
		ZH:   CopyrightText{Syndication: "CONFIG-ZH"},
		EN:   CopyrightText{Source: "Original article"}, // user entered partial text
	}
	if _, err := ResolveCopyright(invalid, root, "en"); err == nil || !strings.Contains(err.Error(), "configured en copyright policy") {
		t.Fatalf("auto mode should reject partial local config rather than fall back: %v", err)
	}
	result, err := ResolveCopyright(invalid, root, "zh-CN")
	if err != nil || result.Syndication != "CONFIG-ZH" {
		t.Fatalf("configured zh should win: %+v, %v", result, err)
	}
	invalid.EN = CopyrightText{}
	result, err = ResolveCopyright(invalid, root, "en")
	if err != nil || result.Syndication != "CONTENT-EN" {
		t.Fatalf("absent local config should use content: %+v, %v", result, err)
	}

	invalid.EN = CopyrightText{Source: "Original article"}
	invalid.Mode = "content" // intentional content-only mode ignores overrides
	result, err = ResolveCopyright(invalid, root, "en")
	if err != nil || result.Syndication != "CONTENT-EN" {
		t.Fatalf("content mode should use content: %+v, %v", result, err)
	}
}

func TestResolveCopyrightMissingContentWithLocalPolicy(t *testing.T) {
	root := t.TempDir()
	cfg := CopyrightConfig{Mode: "auto", ZH: CopyrightText{Syndication: "CONFIG-ZH"}}
	result, err := ResolveCopyright(cfg, root, "zh-CN")
	if err != nil || result.Syndication != "CONFIG-ZH" {
		t.Fatalf("must not require content file when locally configured: %+v, %v", result, err)
	}
	result, err = ResolveCopyright(cfg, root, "en")
	if err != nil || result.Syndication != "" {
		t.Fatalf("absent local and content policy should be unbranded: %+v, %v", result, err)
	}
	cfg.Mode = "content"
	if _, err := ResolveCopyright(cfg, root, "zh-CN"); err == nil {
		t.Fatal("content mode should fail when required content file is missing")
	}
	cfg.Mode = "none"
	result, err = ResolveCopyright(cfg, root, "zh-CN")
	if err != nil || result.Syndication != "" {
		t.Fatalf("none mode should disable copyright: %+v, %v", result, err)
	}
}

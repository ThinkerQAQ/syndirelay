package publishing

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CopyrightText contains the owner's own policy, never SyndiRelay's branding.
type CopyrightText struct {
	Title       string `json:"title" toml:"title"`
	Notice      string `json:"notice" toml:"notice"`
	Photo       string `json:"photo" toml:"photo"`
	Syndication string `json:"syndication" toml:"syndication"`
	Source      string `json:"source" toml:"source"`
}

type CopyrightConfig struct {
	Mode string        `json:"mode" toml:"mode"` // auto, config, content, none
	ZH   CopyrightText `json:"zh" toml:"zh"`
	EN   CopyrightText `json:"en" toml:"en"`
}

// Empty copyright is intentional. In particular, there must be no personal
// embedded fallback in an independent Markdown publication service.
func Copyright(language string) CopyrightText { return CopyrightText{} }

func localePolicy(config CopyrightConfig, language string) CopyrightText {
	if language == "en" {
		return config.EN
	}
	return config.ZH
}

func defaultSourceLabel(language string) string {
	if language == "en" {
		return "Original article"
	}
	return "原文"
}

func validateCopyrightText(text CopyrightText, language string) (CopyrightText, error) {
	text.Syndication = strings.TrimSpace(text.Syndication)
	text.Source = strings.TrimSpace(text.Source)
	if text.Syndication == "" {
		return text, errors.New("copyright syndication must not be empty")
	}
	if text.Source == "" {
		text.Source = defaultSourceLabel(language)
	}
	return text, nil
}

// LoadCopyright reads the content-owned policy. A missing file returns an
// *empty* policy, never a hard-coded previous author's copyright claim.
func LoadCopyright(contentRoot, language string) (CopyrightText, error) {
	if strings.TrimSpace(contentRoot) == "" {
		return CopyrightText{}, nil
	}
	path := filepath.Join(contentRoot, "src", "content", "site", "copyright.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return CopyrightText{}, nil
	}
	if err != nil {
		return CopyrightText{}, fmt.Errorf("read content copyright policy: %w", err)
	}
	var policies map[string]CopyrightText
	if err := json.Unmarshal(data, &policies); err != nil {
		return CopyrightText{}, fmt.Errorf("parse content copyright policy %s: %w", path, err)
	}
	for _, locale := range []string{"zh", "en"} {
		policy, ok := policies[locale]
		if !ok {
			return CopyrightText{}, fmt.Errorf("content copyright policy %s missing %s locale", path, locale)
		}
		if _, err := validateCopyrightText(policy, locale); err != nil {
			return CopyrightText{}, fmt.Errorf("content copyright policy %s %s: %w", path, locale, err)
		}
	}
	if language == "en" {
		return validateCopyrightText(policies["en"], language)
	}
	return validateCopyrightText(policies["zh"], language)
}

// ResolveCopyright implements explicit SyndiRelay override > content-owned
// policy > empty. Explicit mode choices allow a user to enforce either source.
func ResolveCopyright(config CopyrightConfig, contentRoot, language string) (CopyrightText, error) {
	mode := strings.ToLower(strings.TrimSpace(config.Mode))
	if mode == "" {
		mode = "auto"
	}
	override := localePolicy(config, language)
	switch mode {
	case "none":
		return CopyrightText{}, nil
	case "auto", "config", "content":
	default:
		return CopyrightText{}, fmt.Errorf("unsupported publishing copyright mode %q", mode)
	}
	if mode != "content" && strings.TrimSpace(override.Syndication) != "" {
		return validateCopyrightText(override, language)
	}
	if mode == "config" {
		return CopyrightText{}, errors.New("copyright mode=config requires an explicit syndication text for the article language")
	}
	content, err := LoadCopyright(contentRoot, language)
	if err != nil {
		return CopyrightText{}, err
	}
	if mode == "content" && content.Syndication == "" {
		return CopyrightText{}, errors.New("copyright mode=content requires a content repository copyright.json")
	}
	return content, nil
}

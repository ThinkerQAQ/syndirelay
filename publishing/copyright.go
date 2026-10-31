package publishing

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed copyright.json
var copyrightFile embed.FS

type CopyrightText struct {
	Title       string `json:"title"`
	Notice      string `json:"notice"`
	Photo       string `json:"photo"`
	Syndication string `json:"syndication"`
	Source      string `json:"source"`
}

var copyrightPolicies = func() map[string]CopyrightText {
	data, err := copyrightFile.ReadFile("copyright.json")
	if err != nil {
		panic(err)
	}
	var result map[string]CopyrightText
	if err := json.Unmarshal(data, &result); err != nil {
		panic(err)
	}
	return result
}()

// Copyright returns the shared site and publishing attribution text.
func Copyright(language string) CopyrightText {
	if language == "en" {
		return copyrightPolicies["en"]
	}
	return copyrightPolicies["zh"]
}

// LoadCopyright reads the content-owned policy shared with the Astro site.
// Older content checkouts without the file use the embedded legacy defaults.
// Malformed or incomplete policies fail closed instead of publishing wrong terms.
func LoadCopyright(contentRoot, language string) (CopyrightText, error) {
	if strings.TrimSpace(contentRoot) == "" {
		return Copyright(language), nil
	}
	path := filepath.Join(contentRoot, "src", "content", "site", "copyright.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Copyright(language), nil
	}
	if err != nil {
		return CopyrightText{}, fmt.Errorf("read content copyright policy: %w", err)
	}
	var policies map[string]CopyrightText
	if err := json.Unmarshal(data, &policies); err != nil {
		return CopyrightText{}, fmt.Errorf("parse content copyright policy %s: %w", path, err)
	}
	for _, locale := range []string{"zh", "en"} {
		copy, exists := policies[locale]
		if !exists || strings.TrimSpace(copy.Title) == "" || strings.TrimSpace(copy.Notice) == "" || strings.TrimSpace(copy.Photo) == "" || strings.TrimSpace(copy.Syndication) == "" || strings.TrimSpace(copy.Source) == "" {
			return CopyrightText{}, fmt.Errorf("content copyright policy %s missing required fields for %s", path, locale)
		}
	}
	if language == "en" {
		return policies["en"], nil
	}
	return policies["zh"], nil
}

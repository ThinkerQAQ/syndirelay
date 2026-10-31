package social

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ResolveSource enforces the canonical BlogContent path and derives URLs without AI input.
func ResolveSource(contentRoot, origin string, source Source) (Source, error) {
	if !safeID.MatchString(source.Slug) {
		return Source{}, errors.New("invalid source slug")
	}
	if source.Language == "" {
		source.Language = "en"
	}
	if source.Language != "en" {
		return Source{}, errors.New("social posts require an English source")
	}
	var path, uri string
	switch source.Kind {
	case "article":
		path = filepath.Join(contentRoot, "src/content/articles/en", source.Slug+".md")
		uri = "/en/articles/" + url.PathEscape(source.Slug) + "/"
	case "series":
		path = filepath.Join(contentRoot, "src/content/series", source.Slug+".md")
		uri = "/en/series/" + url.PathEscape(source.Slug) + "/"
	case "project":
		path = filepath.Join(contentRoot, "src/content/projects", source.Slug+".md")
		uri = "/en/projects/" + url.PathEscape(source.Slug) + "/"
	default:
		return Source{}, errors.New("unsupported source kind")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Source{}, fmt.Errorf("source not found: %s/%s: %w", source.Kind, source.Slug, err)
	}
	// Series/project translations live in the frontmatter rather than English mirror files.
	if source.Kind != "article" && !strings.Contains(string(raw), "  en:") {
		return Source{}, errors.New("source has no English translation")
	}
	if origin == "" {
		origin = "https://thinkerqaq.com"
	}
	target, err := url.Parse(origin)
	if err != nil || target.Scheme != "https" || target.Host == "" || target.User != nil {
		return Source{}, errors.New("invalid configured site origin")
	}
	source.URL = strings.TrimSuffix(origin, "/") + uri
	return source, nil
}

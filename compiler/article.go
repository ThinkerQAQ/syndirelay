package compiler

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Article contains the frontmatter fields used by publishing. Other content
// metadata (series, translations, dates, etc.) belongs to the site schema.
type Article struct {
	Title         string   `yaml:"title"`
	Description   string   `yaml:"description"`
	Series        string   `yaml:"series"`
	Status        string   `yaml:"status"`
	Tags          []string `yaml:"tags"`
	CoverImage    string   `yaml:"coverImage"`
	CoverImageAlt string   `yaml:"coverImageAlt"`
	Body          string   `yaml:"-"`
}

func ParseArticle(markdown, source string) (article Article, parseErr error) {
	started := time.Now()
	defer func() {
		attributes := []any{"source", source, "operation", "parse-frontmatter", "durationMs", time.Since(started).Milliseconds()}
		if parseErr != nil {
			slog.Error("article frontmatter parsing failed", append(attributes, "status", "failed", "error", parseErr)...)
		} else {
			slog.Debug("article frontmatter parsed", append(attributes, "status", "completed", "tagCount", len(article.Tags))...)
		}
	}()
	normalized := strings.TrimPrefix(strings.ReplaceAll(markdown, "\r\n", "\n"), "\ufeff")
	if !strings.HasPrefix(normalized, "---\n") {
		return Article{}, fmt.Errorf("invalid article frontmatter: %s", source)
	}
	rest := normalized[4:]
	end := strings.Index(rest, "\n---\n")
	sep := 5
	if end < 0 && strings.HasSuffix(rest, "\n---") {
		end = len(rest) - 4
		sep = 4
	}
	if end < 0 {
		return Article{}, fmt.Errorf("invalid article frontmatter: %s", source)
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &article); err != nil {
		return Article{}, fmt.Errorf("invalid article frontmatter in %s: %w", source, err)
	}
	article.Body = strings.TrimSpace(rest[end+sep:])
	if article.Status == "" {
		article.Status = "draft"
	}
	if strings.TrimSpace(article.Title) == "" {
		return Article{}, fmt.Errorf("invalid title: %s", source)
	}
	if strings.TrimSpace(article.Description) == "" {
		return Article{}, fmt.Errorf("invalid description: %s", source)
	}
	if article.Body == "" {
		return Article{}, fmt.Errorf("article body is empty: %s", source)
	}
	if article.Status == "published" && (article.CoverImage == "" || article.CoverImageAlt == "") {
		return Article{}, fmt.Errorf("published article cover metadata is incomplete: %s", source)
	}
	return article, nil
}

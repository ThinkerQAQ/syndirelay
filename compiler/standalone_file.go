package compiler

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	blogplatform "github.com/ThinkerQAQ/syndirelay/platform"
	blogpublishing "github.com/ThinkerQAQ/syndirelay/publishing"
	"go.yaml.in/yaml/v3"
)

// FileCompileOptions configures standalone conversion without an Inkerio
// checkout, site metadata, platform credentials or public publishing.
type FileCompileOptions struct {
	Path         string
	Platform     string
	CanonicalURL string
	AssetBaseURL string
	Language     string
	RendererRoot string
	Node         string
}

// standaloneRendererRoot prefers the current checkout, then the portable
// installation adjacent to the executable. Installed binaries live in bin/,
// and the compiler must work from an arbitrary shell working directory.
func standaloneRendererRoot(current, executable string) string {
	candidates := []string{current}
	if executable != "" {
		bin := filepath.Dir(executable)
		candidates = append(candidates, bin, filepath.Dir(bin))
	}
	for _, root := range candidates {
		if root == "" {
			continue
		}
		if info, err := os.Stat(filepath.Join(root, "renderers", "node", "markdown-html.mjs")); err == nil && info.Mode().IsRegular() {
			if manifest, err := os.Stat(filepath.Join(root, "package.json")); err == nil && manifest.Mode().IsRegular() {
				return root
			}
		}
	}
	return current
}

var headingPattern = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)

func parseGenericMarkdown(text, source string) (Article, error) {
	var article Article
	normalized := strings.TrimPrefix(strings.ReplaceAll(text, "\r\n", "\n"), "\ufeff")
	body := normalized
	if strings.HasPrefix(normalized, "---\n") {
		rest := normalized[4:]
		i := strings.Index(rest, "\n---\n")
		if i < 0 {
			return Article{}, fmt.Errorf("unterminated YAML frontmatter in %s", source)
		}
		if err := yaml.Unmarshal([]byte(rest[:i]), &article); err != nil {
			return Article{}, fmt.Errorf("invalid frontmatter in %s: %w", source, err)
		}
		body = rest[i+5:]
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return Article{}, errors.New("Markdown body is empty")
	}
	if strings.TrimSpace(article.Title) == "" {
		if match := headingPattern.FindStringSubmatch(body); match != nil {
			article.Title = strings.TrimSpace(match[1])
		}
	}
	if article.Title == "" {
		article.Title = strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	}
	if strings.TrimSpace(article.Description) == "" {
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "```") && !strings.HasPrefix(line, "|") && !strings.HasPrefix(line, "!") {
				article.Description = truncateRunes(line, 220)
				break
			}
		}
		if article.Description == "" {
			article.Description = article.Title
		}
	}
	article.Body = body
	article.Status = "published" // Only a local compile preview; not a publishing side effect.
	return article, nil
}

func validateOptionalHTTPS(raw, field string) error {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%s requires an HTTPS URL without credentials or fragment", field)
	}
	return nil
}

// CompileFile uses the original 11 platform compilers with a separate, optional
// content reader. It never inherits the ThinkerQAQ copyright/footer/canonical.
func CompileFile(ctx context.Context, options FileCompileOptions) (CompiledArticle, error) {
	if !blogplatform.Supported(options.Platform) {
		return CompiledArticle{}, fmt.Errorf("unsupported platform: %s", options.Platform)
	}
	if err := validateOptionalHTTPS(options.CanonicalURL, "canonical-url"); err != nil {
		return CompiledArticle{}, err
	}
	if err := validateOptionalHTTPS(options.AssetBaseURL, "asset-base-url"); err != nil {
		return CompiledArticle{}, err
	}
	full, err := filepath.Abs(options.Path)
	if err != nil {
		return CompiledArticle{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return CompiledArticle{}, err
	}
	if !info.Mode().IsRegular() {
		return CompiledArticle{}, errors.New("Markdown input must be a regular file")
	}
	if info.Size() > 5*1024*1024 {
		return CompiledArticle{}, errors.New("Markdown input exceeds 5 MiB")
	}
	text, err := os.ReadFile(full)
	if err != nil {
		return CompiledArticle{}, err
	}
	article, err := parseGenericMarkdown(string(text), full)
	if err != nil {
		return CompiledArticle{}, err
	}
	if err := validateOptionalHTTPS(article.CoverImage, "coverImage"); err != nil {
		return CompiledArticle{}, err
	}
	profile := blogpublishing.DefaultPlatformConfig(options.Platform)
	profile.Footer.Enabled = false
	profile.Tracking.Enabled = false
	profile.Canonical.Mode = "none"
	if options.CanonicalURL != "" && blogplatform.For(options.Platform).NativeCanonical {
		profile.Canonical.Mode = "native"
	}
	if options.Language != "" {
		if options.Language != "en" && options.Language != "zh-CN" {
			return CompiledArticle{}, errors.New("language must be en or zh-CN")
		}
		profile.Language = options.Language
	}
	assetBase := options.AssetBaseURL
	if assetBase == "" {
		assetBase = "https://invalid.example/"
	}
	root := strings.TrimSpace(options.RendererRoot)
	if root == "" {
		root = strings.TrimSpace(os.Getenv("SYNDIRELAY_HOME"))
	}
	if root == "" {
		current, cwdErr := os.Getwd()
		if cwdErr != nil {
			return CompiledArticle{}, cwdErr
		}
		executable, _ := os.Executable()
		root = standaloneRendererRoot(current, executable)
	}
	slug := strings.TrimSuffix(filepath.Base(full), filepath.Ext(full))
	settings := CompileOptions{EngineRoot: root, Generic: true, CanonicalOverride: options.CanonicalURL,
		Node: options.Node, Platform: options.Platform, DryRun: true}
	policy := blogpublishing.CopyrightText{} // no author-specific fallback
	var out CompiledArticle
	if options.Platform == "medium" {
		out, err = compileMedium(compileContext{options: settings, profile: profile, slug: slug,
			article: article, sourceDir: filepath.Dir(full), assetBase: assetBase, policy: policy})
	} else {
		out, err = compileOne(ctx, settings, profile, slug, article, full, assetBase, policy)
	}
	if err != nil {
		return CompiledArticle{}, err
	}
	if assetBase == "https://invalid.example/" && len(out.Assets) > 0 {
		return CompiledArticle{}, errors.New("diagram/table media require an explicit --asset-base-url to avoid unintended third-party URLs")
	}
	return out, nil
}

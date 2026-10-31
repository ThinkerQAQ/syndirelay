package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	blogplatform "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/platform"
	blogpublishing "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publishing"
)

type CompileOptions struct {
	EngineRoot  string
	ContentRoot string
	Publishing  PublishingConfig
	Node        string
	Env         []string
	Platform    string
	Articles    []string
	All         bool
	DryRun      bool
}

func CanonicalURL(slug, language string) string {
	parts := strings.Split(slug, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	prefix := "/articles/"
	if language == "en" {
		prefix = "/en/articles/"
	}
	return SiteOrigin + prefix + strings.Join(parts, "/") + "/"
}
func resolveArticleAssetURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	base, _ := url.Parse(SiteOrigin)
	ref, err := url.Parse(value)
	if err != nil {
		return value
	}
	return base.ResolveReference(ref).String()
}
func truncateRunes(value string, limit int) string {
	if limit < 1 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	r := []rune(value)
	return string(r[:limit-1]) + "…"
}
func trackedURL(canonical string, config PlatformConfig) string {
	u, err := url.Parse(canonical)
	if err != nil {
		return canonical
	}
	if !config.Tracking.Enabled {
		return u.String()
	}
	pairs := make([]string, 0, 3)
	for _, item := range [][2]string{
		{"utm_source", strings.TrimSpace(config.Tracking.Source)},
		{"utm_medium", strings.TrimSpace(config.Tracking.Medium)},
		{"utm_campaign", strings.TrimSpace(config.Tracking.Campaign)},
	} {
		if item[1] != "" {
			pairs = append(pairs, url.QueryEscape(item[0])+"="+url.QueryEscape(item[1]))
		}
	}
	u.RawQuery = strings.Join(pairs, "&")
	return u.String()
}
func renderFooter(config PlatformConfig, canonical, title string, policy ...blogpublishing.CopyrightText) string {
	if !config.Footer.Enabled {
		return ""
	}
	site := "ThinkerQAQ 的个人博客"
	if config.Language == "en" {
		site = "ThinkerQAQ's personal blog"
	}
	v := config.Footer.Template
	v = strings.ReplaceAll(v, "{url}", trackedURL(canonical, config))
	v = strings.ReplaceAll(v, "{title}", title)
	v = strings.ReplaceAll(v, "{site}", site)
	// Preserve custom/legacy attribution templates while appending the same
	// copyright policy displayed by the site. Keep the canonical URL untracked.
	copy := blogpublishing.Copyright(config.Language)
	if len(policy) > 0 {
		copy = policy[0]
	}
	return strings.TrimSpace(v) + "\n\n> " + copy.Syndication + " [" + copy.Source + "](" + canonical + ")"
}
func normalizeDevtoTags(tags []string) []string {
	var result []string
	seen := map[string]struct{}{}
	for _, tag := range tags {
		c := strings.ToLower(strings.Join(strings.Fields(tag), ""))
		if c == "" || len(c) > 30 {
			continue
		}
		valid := true
		for i, r := range c {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r == '-' && i > 0) {
				continue
			}
			valid = false
			break
		}
		if !valid {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		result = append(result, c)
		if len(result) == 4 {
			break
		}
	}
	return result
}
func renderHTML(ctx context.Context, o CompileOptions, markdown string) (string, error) {
	node := strings.TrimSpace(o.Node)
	if node == "" {
		var err error
		node, err = exec.LookPath("node")
		if err != nil {
			return "", errors.New("node was not found for Markdown HTML renderer")
		}
	}
	renderer := filepath.Join(o.EngineRoot, "tools", "blogctl", "renderers", "node", "markdown-html.mjs")
	cmd := exec.CommandContext(ctx, node, renderer)
	cmd.Dir = o.EngineRoot
	if len(o.Env) > 0 {
		cmd.Env = o.Env
	} else {
		cmd.Env = os.Environ()
	}
	cmd.Stdin = strings.NewReader(markdown)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("render publishing HTML: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if o.Platform == "toutiao" {
		return addLinkedHeadingIDs(stdout.String())
	}
	return stdout.String(), nil
}
func articleRoot(root, language string) string {
	p := filepath.Join(root, "src", "content", "articles")
	if language == "en" {
		return filepath.Join(p, "en")
	}
	return p
}
func sourceFile(root, slug, language string) string {
	return filepath.Join(articleRoot(root, language), filepath.FromSlash(slug)) + ".md"
}
func publishedSlugs(root, language string) ([]string, error) {
	base := articleRoot(root, language)
	var result []string
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if language != "en" && path != base && entry.Name() == "en" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		article, err := ParseArticle(string(data), path)
		if err != nil {
			return err
		}
		if article.Status != "published" {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		result = append(result, strings.TrimSuffix(filepath.ToSlash(rel), ".md"))
		return nil
	})
	sort.Strings(result)
	return result, err
}
func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func compileOne(ctx context.Context, o CompileOptions, profile PlatformConfig, slug string, article Article, source, assetBase string, policy blogpublishing.CopyrightText) (CompiledArticle, error) {
	canonical := CanonicalURL(slug, profile.Language)
	body, assets, err := CompilePublishingMarkdown(article.Body, assetBase)
	if err != nil {
		return CompiledArticle{}, err
	}
	body = strings.TrimSpace(body)
	footer := renderFooter(profile, canonical, article.Title, policy)
	var compiled CompiledArticle
	var hashSource string
	if o.Platform == "devto" || o.Platform == "hashnode" {
		markdown := body
		if footer != "" {
			markdown += "\n\n---\n\n" + footer
		}
		markdown += "\n"
		html, err := renderHTML(ctx, o, markdown)
		if err != nil {
			return CompiledArticle{}, err
		}
		native := ""
		if profile.Canonical.Mode == "native" {
			native = canonical
		}
		tags := normalizeDevtoTags(article.Tags)
		if o.Platform == "hashnode" {
			tags = append([]string{}, article.Tags...)
		}
		cover := resolveArticleAssetURL(article.CoverImage)
		hashPayload := struct {
			Title              string   `json:"title"`
			Description        string   `json:"description"`
			Markdown           string   `json:"markdown"`
			NativeCanonicalURL string   `json:"nativeCanonicalUrl"`
			Tags               []string `json:"tags"`
			CoverImageURL      string   `json:"coverImageUrl"`
			Series             string   `json:"series"`
			Published          bool     `json:"published"`
		}{article.Title, truncateRunes(article.Description, 256), markdown, native, tags, cover, article.Series, false}
		payload, _ := json.Marshal(hashPayload)
		hashSource = string(payload)
		compiled = CompiledArticle{Version: ProtocolVersion, Slug: slug, Platform: o.Platform, Title: article.Title, Description: truncateRunes(article.Description, 256), Series: article.Series, Markdown: markdown, HTML: html, Language: profile.Language, CanonicalURL: canonical, NativeCanonicalURL: native, Tags: tags, CoverImageURL: cover, Published: false, SourceDir: filepath.Dir(source), Assets: assets}
	} else {
		limit := 256
		if o.Platform == "juejin" {
			limit = 100
		}
		markdown := body
		if footer != "" {
			markdown += "\n\n---\n\n" + footer
		}
		markdown = strings.TrimSpace(markdown)
		html, err := renderHTML(ctx, o, markdown)
		if err != nil {
			return CompiledArticle{}, err
		}
		hashSource = article.Title + "\n" + markdown + "\n<!-- blogctl-html -->\n" + html
		compiled = CompiledArticle{Version: ProtocolVersion, Slug: slug, Platform: o.Platform, Title: article.Title, Description: truncateRunes(article.Description, limit), Series: article.Series, Markdown: markdown, HTML: html, Language: profile.Language, CanonicalURL: canonical, Tags: append([]string{}, article.Tags...), CoverImageURL: resolveArticleAssetURL(article.CoverImage), Published: false, SourceDir: filepath.Dir(source), Assets: assets}
	}
	compiled.ContentHash = hashText(hashSource)
	if !o.DryRun {
		compiled.Markdown = replaceAssetURLs(compiled.Markdown, assets)
		compiled.HTML = replaceAssetURLs(compiled.HTML, assets)
		for i := range compiled.Assets {
			compiled.Assets[i].Source = "blogctl-asset://" + compiled.Assets[i].Kind + "/" + compiled.Assets[i].ID
		}
	} else {
		for i := range compiled.Assets {
			compiled.Assets[i].Source = compiled.Assets[i].PublicURL
		}
	}
	return compiled, nil
}
func CompilePlatform(ctx context.Context, o CompileOptions) ([]CompiledArticle, error) {
	if !blogplatform.Supported(o.Platform) {
		return nil, fmt.Errorf("unsupported platform: %s", o.Platform)
	}
	config, err := normalizePublishingConfig(o.Publishing)
	if err != nil {
		return nil, err
	}
	profile, ok := config.Platforms[o.Platform]
	if !ok {
		profile = defaultPlatformConfig(o.Platform)
	}
	if profile.Language == "" {
		profile.Language = blogplatform.DefaultLanguage(o.Platform)
	}
	policy, err := blogpublishing.LoadCopyright(o.ContentRoot, profile.Language)
	if err != nil {
		return nil, err
	}
	slugs := append([]string{}, o.Articles...)
	if o.All {
		slugs, err = publishedSlugs(o.ContentRoot, profile.Language)
		if err != nil {
			return nil, err
		}
	}
	result := make([]CompiledArticle, 0, len(slugs))
	for _, slug := range slugs {
		source := sourceFile(o.ContentRoot, slug, profile.Language)
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, err
		}
		article, err := ParseArticle(string(data), source)
		if err != nil {
			return nil, err
		}
		if article.Status != "published" {
			return nil, fmt.Errorf("article is not published: %s", slug)
		}
		var compiled CompiledArticle
		if o.Platform == "medium" {
			compiled, err = compileMedium(compileContext{
				options: o, profile: profile, slug: slug, article: article,
				sourceDir: filepath.Dir(source), assetBase: config.Assets.R2.PublicBaseURL, policy: policy,
			})
		} else {
			compiled, err = compileOne(ctx, o, profile, slug, article, source, config.Assets.R2.PublicBaseURL, policy)
		}
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", o.Platform, slug, err)
		}
		result = append(result, compiled)
	}
	return result, nil
}

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
	"regexp"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	blogplatform "github.com/ThinkerQAQ/syndirelay/platform"
	blogpublishing "github.com/ThinkerQAQ/syndirelay/publishing"
)

type CompileOptions struct {
	EngineRoot        string
	Generic           bool   // A standalone Markdown input, not a blog-content source
	CanonicalOverride string // Optional explicit canonical, never inferred from ThinkerQAQ
	ContentRoot       string
	Publishing        PublishingConfig
	Node              string
	Env               []string
	Platform          string
	Articles          []string
	All               bool
	DryRun            bool
}

// CanonicalURL is retained for legacy test helpers; without a configured
// site origin it returns only a relative path, never an owner's domain.
func CanonicalURL(slug, language string) string {
	return canonicalWithSite(blogpublishing.DefaultConfig().Site, slug, language)
}
func canonicalWithSite(site blogpublishing.SiteConfig, slug, language string) string {
	parts := strings.Split(slug, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.TrimRight(site.Origin, "/") + site.ArticlePath(language) + strings.Join(parts, "/") + "/"
}
func canonicalFor(o CompileOptions, slug, language string) string {
	if o.Generic {
		return o.CanonicalOverride
	}
	return canonicalWithSite(o.Publishing.Site, slug, language)
}
func originFor(o CompileOptions) string {
	if !o.Generic {
		return o.Publishing.Site.Origin
	}
	if o.CanonicalOverride == "" {
		return ""
	}
	u, err := url.Parse(o.CanonicalOverride)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
func assetURLFor(o CompileOptions, value string) string {
	value = strings.TrimSpace(value)
	if o.Generic || value == "" {
		return value
	}
	ref, err := url.Parse(value)
	if err != nil {
		return value
	}
	if ref.IsAbs() {
		return ref.String()
	}
	origin := originFor(o)
	if origin == "" {
		return value
	}
	base, _ := url.Parse(origin)
	return base.ResolveReference(ref).String()
}

// resolveArticleAssetURL is a legacy helper. It must not invent a blog domain.
func resolveArticleAssetURL(value string) string { return strings.TrimSpace(value) }
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
	site := strings.TrimSpace(config.Footer.SiteName)
	if site == "" {
		u, err := url.Parse(canonical)
		if err == nil {
			site = u.Hostname()
		}
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
	if strings.TrimSpace(copy.Syndication) == "" {
		return strings.TrimSpace(v)
	}
	// The attribution template already links to the canonical article. A
	// second "Original article" link after the copyright is redundant.
	// Keep the two paragraphs in one Markdown blockquote so DEV.to and
	// other Markdown renderers do not split the footer into separate callouts.
	licensed := normalizeFooterPolicyLinks(strings.TrimSpace(copy.Syndication), config.Language)
	first := strings.TrimSpace(v)
	if strings.HasPrefix(first, "> ") {
		return first + "\n>\n> " + strings.ReplaceAll(licensed, "\n", "\n> ")
	}
	return first + "\n\n" + licensed
}

// Normalize bare policy URLs before platform adaptation. Most platform
// Markdown parsers will otherwise autolink the URL and sometimes swallow the
// following Chinese text. This changes markup, never the policy itself.
// A named CC license becomes the link label, not a redundant second link.
var namedCCLicenseURL = regexp.MustCompile(`(?i)(CC[[:space:]]+BY[-A-Z0-9.]+[[:space:]]+[0-9]+(?:\.[0-9]+)?)[[:space:]]*[（(](https://[^\s）)]+)[）)]`)
var parenthesizedPolicyURL = regexp.MustCompile(`([（(])(https://[^\s）)]+)([）)])`)

func normalizeFooterPolicyLinks(policy, language string) string {
	policy = namedCCLicenseURL.ReplaceAllString(policy, `[$1]($2)`)
	label := "许可协议"
	if language == "en" {
		label = "License"
	}
	// Work against the entire string, not an isolated fragment, so the URL
	// inside an existing [label](url) is never converted a second time.
	matches := parenthesizedPolicyURL.FindAllStringSubmatchIndex(policy, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		if m[0] > 0 && policy[m[0]-1] == ']' {
			continue
		}
		url := policy[m[4]:m[5]]
		replacement := policy[m[2]:m[3]] + "[" + label + "](" + url + ")" + policy[m[6]:m[7]]
		policy = policy[:m[0]] + replacement + policy[m[1]:]
	}
	return policy
}

// CNBlogs and CSDN use compact callout styling for blockquotes. They still
// receive the exact same Markdown attribution and license links; only the
// outer blockquote layout is replaced with two ordinary paragraphs.
func readableEditorFooter(footer string) string {
	if strings.TrimSpace(footer) == "" {
		return ""
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(footer), "\n") {
		line = strings.TrimSpace(line)
		if line == ">" {
			continue
		}
		lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "> ")))
	}
	return strings.Join(lines, "\n\n")
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

// Locate the unchanged Markdown compiler in either a legacy workspace or a
// standalone SyndiRelay installation. Never fall back to an Inkerio installation.
func findPublishingRenderer(workspace string) (string, error) {
	candidates := []string{}
	if workspace != "" {
		candidates = append(candidates, filepath.Join(workspace, "renderers", "node", "markdown-html.mjs"))
	}
	if root := os.Getenv("SYNDIRELAY_HOME"); root != "" {
		candidates = append(candidates, filepath.Join(root, "renderers", "node", "markdown-html.mjs"))
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "renderers", "node", "markdown-html.mjs"))
	}
	if _, filename, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(filename), "..", "renderers", "node", "markdown-html.mjs"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", errors.New("SyndiRelay Markdown renderer is missing; install the package with renderers/node and npm dependencies")
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
	renderer, err := findPublishingRenderer(o.EngineRoot)
	if err != nil {
		return "", err
	}
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
	canonical := canonicalFor(o, slug, profile.Language)
	body, assets, err := compilePublishingMarkdown(article.Body, assetBase, false, originFor(o))
	if err != nil {
		return CompiledArticle{}, err
	}
	body = strings.TrimSpace(body)
	footer := renderFooter(profile, canonical, article.Title, policy)
	if o.Platform == "cnblogs" || o.Platform == "csdn" {
		footer = readableEditorFooter(footer)
	}
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
		cover := assetURLFor(o, article.CoverImage)
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
		compiled = CompiledArticle{Version: ProtocolVersion, Slug: slug, Platform: o.Platform, Title: article.Title, Description: truncateRunes(article.Description, limit), Series: article.Series, Markdown: markdown, HTML: html, Language: profile.Language, CanonicalURL: canonical, Tags: append([]string{}, article.Tags...), CoverImageURL: assetURLFor(o, article.CoverImage), Published: false, SourceDir: filepath.Dir(source), Assets: assets}
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
	o.Publishing = config
	if config.Site.Origin == "" {
		return nil, errors.New("publishing.site.origin is required to compile content repository articles; configure your own site origin")
	}
	profile, ok := config.Platforms[o.Platform]
	if !ok {
		profile = defaultPlatformConfig(o.Platform)
	}
	if profile.Language == "" {
		profile.Language = blogplatform.DefaultLanguage(o.Platform)
	}
	profile.Footer.SiteName = config.Site.DisplayName(profile.Language)
	policy, err := blogpublishing.ResolveCopyright(config.Copyright, o.ContentRoot, profile.Language)
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

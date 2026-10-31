package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	blogpublishing "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publishing"
)

type compileContext struct {
	options   CompileOptions
	profile   PlatformConfig
	slug      string
	article   Article
	sourceDir string
	assetBase string
	policy    blogpublishing.CopyrightText
}

type mediumImage struct {
	URL string `json:"url"`
	Alt string `json:"alt"`
}

type mediumParagraphPayload struct {
	Type     int            `json:"type"`
	Text     string         `json:"text"`
	Markups  []mediumMarkup `json:"markups"`
	Layout   *int           `json:"layout,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type mediumDeltaPayload struct {
	Type      int                    `json:"type"`
	Index     int                    `json:"index"`
	Paragraph mediumParagraphPayload `json:"paragraph"`
	Image     *mediumImage           `json:"image,omitempty"`
}

// mediumBrowserImage records where the browser editor must insert an image.
// The compiler does not upload images or guess Medium's private image Delta
// protocol. Markers are regular native paragraphs until the editor replaces
// them with browser-inserted images.
type mediumBrowserImage struct {
	Marker string `json:"marker"`
	URL    string `json:"url"`
	Alt    string `json:"alt"`
}

// mediumBrowserPlan preserves the existing native text/code/list Delta mapping
// while deferring only actual image insertion to a real Medium editor.
type mediumBrowserPlan struct {
	Deltas []mediumDeltaPayload `json:"deltas"`
	Images []mediumBrowserImage `json:"images"`
}

type mediumPayload struct {
	Title        string               `json:"title"`
	Deltas       []mediumDeltaPayload `json:"deltas"`
	BrowserPlan  mediumBrowserPlan    `json:"browserPlan"`
	CanonicalURL string               `json:"canonicalUrl"`
	Tags         []string             `json:"tags"`
	CoverImage   *mediumImage         `json:"coverImage"`
}

func mediumFooter(article Article, canonical string, profile PlatformConfig, policy ...blogpublishing.CopyrightText) *mediumInline {
	markdown := strings.TrimSpace(renderFooter(profile, canonical, article.Title, policy...))
	if markdown == "" {
		return nil
	}
	if strings.HasPrefix(markdown, ">") {
		markdown = strings.TrimPrefix(markdown, ">")
		if strings.HasPrefix(markdown, " ") {
			markdown = markdown[1:]
		}
	}
	markdown = strings.ReplaceAll(markdown, "\n\n> ", " ")
	inline := parseMediumInline(strings.TrimSpace(markdown), nil)
	return &inline
}

func firstMediumTags(tags []string) []string {
	limit := len(tags)
	if limit > 5 {
		limit = 5
	}
	return append([]string{}, tags[:limit]...)
}

// Medium assigns its own opaque paragraph IDs. Source heading fragments are
// valid only on the original article, including lower-level headings that
// Medium represents as bold paragraphs. Keep these links useful on both
// initial creation and full replacements without guessing Medium IDs.
func resolveMediumFragmentLinks(blocks []mediumBlock, canonical string) {
	for i := range blocks {
		for j := range blocks[i].Markups {
			markup := &blocks[i].Markups[j]
			if markup.Type == mediumMarkupLink && strings.HasPrefix(markup.Href, "#") {
				fragment := markup.Href
				markup.Href = canonical + fragment
				blocks[i].HTML = strings.ReplaceAll(blocks[i].HTML, `href="`+mediumEscapeHTML(fragment)+`"`, `href="`+mediumEscapeHTML(markup.Href)+`"`)
			}
		}
	}
}

func buildMediumPayload(article Article, slug string, profile PlatformConfig, compiledMarkdown string, policy ...blogpublishing.CopyrightText) (mediumPayload, []string) {
	canonical := CanonicalURL(slug, profile.Language)
	blocks, warnings := parseMediumBlocks(compiledMarkdown)
	resolveMediumFragmentLinks(blocks, canonical)
	coverURL := resolveArticleAssetURL(article.CoverImage)
	content := blocks
	if coverURL != "" {
		content = append([]mediumBlock{{Kind: "image", URL: coverURL, Alt: article.CoverImageAlt}}, blocks...)
	}
	deltas := make([]mediumDeltaPayload, 0, len(content)+1)
	for index, block := range content {
		if block.Kind == "image" {
			layout := 1
			deltas = append(deltas, mediumDeltaPayload{
				Type: 1, Index: index,
				Paragraph: mediumParagraphPayload{
					Type: mediumImageParagraph, Text: "", Markups: []mediumMarkup{},
					Layout: &layout, Metadata: map[string]any{},
				},
				Image: &mediumImage{URL: block.URL, Alt: block.Alt},
			})
			continue
		}
		markups := block.Markups
		if markups == nil {
			markups = []mediumMarkup{}
		}
		deltas = append(deltas, mediumDeltaPayload{
			Type: 1, Index: index,
			Paragraph: mediumParagraphPayload{Type: block.ParagraphType, Text: block.Text, Markups: markups},
		})
	}
	if footer := mediumFooter(article, canonical, profile, policy...); footer != nil {
		deltas = append(deltas, mediumDeltaPayload{
			Type: 1, Index: len(deltas),
			Paragraph: mediumParagraphPayload{Type: mediumBlockquote, Text: footer.Text, Markups: footer.Markups},
		})
	}
	nativeCanonical := ""
	if profile.Canonical.Mode == "native" {
		nativeCanonical = canonical
	}
	var cover *mediumImage
	if coverURL != "" {
		cover = &mediumImage{URL: coverURL, Alt: article.CoverImageAlt}
	}
	// Browser mode must never flatten or reinterpret non-image paragraphs.
	// PRE, headings, lists and inline markups use the same verified mappings
	// as the existing native Medium payload.
	browser := mediumBrowserPlan{
		Deltas: make([]mediumDeltaPayload, 0, len(deltas)),
		Images: []mediumBrowserImage{},
	}
	for _, delta := range deltas {
		if delta.Image == nil {
			browser.Deltas = append(browser.Deltas, delta)
			continue
		}
		// Stable position and image ordering let the browser insert at the
		// corresponding block without reconstructing nearby content.
		marker := fmt.Sprintf("[BLOGCTL_IMAGE_%03d]", len(browser.Images)+1)
		browser.Images = append(browser.Images, mediumBrowserImage{
			Marker: marker, URL: delta.Image.URL, Alt: delta.Image.Alt,
		})
		browser.Deltas = append(browser.Deltas, mediumDeltaPayload{
			Type: delta.Type, Index: delta.Index,
			Paragraph: mediumParagraphPayload{
				Type: mediumParagraph, Text: marker, Markups: []mediumMarkup{},
			},
		})
	}
	return mediumPayload{
		Title: article.Title, Deltas: deltas, BrowserPlan: browser,
		CanonicalURL: nativeCanonical, Tags: firstMediumTags(article.Tags),
		CoverImage: cover,
	}, warnings
}

func renderMediumBlocks(blocks []mediumBlock) string {
	out := []string{}
	listKind := ""
	closeList := func() {
		if listKind != "" {
			if listKind == "uli" {
				out = append(out, "</ul>")
			} else {
				out = append(out, "</ol>")
			}
			listKind = ""
		}
	}
	for _, block := range blocks {
		if block.Kind == "uli" || block.Kind == "oli" {
			if listKind != block.Kind {
				closeList()
				if block.Kind == "uli" {
					out = append(out, "<ul>")
				} else {
					out = append(out, "<ol>")
				}
				listKind = block.Kind
			}
			out = append(out, "<li>"+block.HTML+"</li>")
			continue
		}
		closeList()
		switch block.Kind {
		case "image":
			caption := ""
			if block.Alt != "" {
				caption = "<figcaption>" + mediumEscapeHTML(block.Alt) + "</figcaption>"
			}
			out = append(out, "<figure class=\"body-image\"><img src=\""+mediumEscapeHTML(block.URL)+"\" alt=\""+mediumEscapeHTML(block.Alt)+"\">"+caption+"</figure>")
		case "pre":
			language := ""
			if block.Language != "" {
				language = " class=\"language-" + mediumEscapeHTML(block.Language) + "\""
			}
			out = append(out, "<pre><code"+language+">"+mediumEscapeHTML(block.Text)+"</code></pre>")
		case "blockquote":
			out = append(out, "<blockquote><p>"+strings.ReplaceAll(block.HTML, "\n", "<br>")+"</p></blockquote>")
		case "heading":
			level := 3
			if block.Level <= 2 {
				level = 2
			}
			out = append(out, fmt.Sprintf("<h%d>%s</h%d>", level, block.HTML, level))
		default:
			out = append(out, "<p>"+block.HTML+"</p>")
		}
	}
	closeList()
	return strings.Join(out, "\n")
}

func buildMediumCopyHTML(article Article, slug string, profile PlatformConfig, compiledMarkdown string, policy ...blogpublishing.CopyrightText) string {
	canonical := CanonicalURL(slug, profile.Language)
	blocks, _ := parseMediumBlocks(compiledMarkdown)
	resolveMediumFragmentLinks(blocks, canonical)
	body := renderMediumBlocks(blocks)
	coverURL := resolveArticleAssetURL(article.CoverImage)
	coverHTML := ""
	if coverURL != "" {
		coverHTML = "<figure class=\"cover\"><img src=\"" + mediumEscapeHTML(coverURL) + "\" alt=\"" + mediumEscapeHTML(article.CoverImageAlt) + "\"></figure>\n"
	}
	footerHTML := ""
	if footer := mediumFooter(article, canonical, profile, policy...); footer != nil {
		footerHTML = "\n<hr>\n<blockquote><p>" + footer.HTML + "</p></blockquote>"
	}
	language := profile.Language
	if language != "zh-CN" {
		language = "en"
	}
	template := `<!doctype html>
<html lang="{{LANG}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{TITLE}} — Medium Copy</title>
<style>
body{font-family:Georgia,"Times New Roman",serif;max-width:760px;margin:40px auto;padding:0 24px 80px;line-height:1.65;color:#242424}
#toolbar{position:sticky;top:0;background:#fff;padding:12px 0;border-bottom:1px solid #e5e5e5;margin-bottom:32px;z-index:10}
button{font:inherit;padding:9px 14px;cursor:pointer}.cover{margin:0 0 32px}.cover img{display:block;width:100%;height:auto}h1{font-size:2.35rem;line-height:1.15}h2{font-size:1.7rem;margin-top:2.1em}h3{font-size:1.3rem;margin-top:1.7em}
code{font-family:Consolas,"SFMono-Regular",Menlo,monospace}p code,li code{background:#f2f2f2;padding:.08em .28em;border-radius:3px}
pre{font-family:Consolas,"SFMono-Regular",Menlo,monospace;white-space:pre;overflow-x:auto;background:#f7f7f7;padding:16px;border-radius:4px;line-height:1.5}pre code{background:transparent;padding:0}
blockquote{border-left:3px solid #242424;margin-left:0;padding-left:18px}.body-image{margin:2em 0}.body-image img{display:block;max-width:100%;height:auto;margin:0 auto}.body-image figcaption{text-align:center;color:#757575;font-size:.9rem;margin-top:.6em}ul,ol{padding-left:1.4em}li{margin:.35em 0}hr{border:0;border-top:1px solid #ddd;margin:2em 0}
</style>
</head>
<body>
<div id="toolbar"><button id="copyBtn">Copy for Medium</button><span id="status" style="margin-left:10px;color:#666"></span></div>
<article id="article">
<h1>{{TITLE}}</h1>
{{COVER}}{{BODY}}{{FOOTER}}
</article>
<script>
document.getElementById('copyBtn').addEventListener('click', async () => {
  const article = document.getElementById('article');
  const status = document.getElementById('status');
  try {
    await navigator.clipboard.write([new ClipboardItem({
      'text/html': new Blob([article.innerHTML], {type:'text/html'}),
      'text/plain': new Blob([article.innerText], {type:'text/plain'})
    })]);
    status.textContent = 'Copied. Paste into Medium.';
  } catch (error) {
    const range = document.createRange(); range.selectNodeContents(article);
    const selection = window.getSelection(); selection.removeAllRanges(); selection.addRange(range);
    document.execCommand('copy'); selection.removeAllRanges();
    status.textContent = 'Copied with fallback. Paste into Medium.';
  }
});
</script>
</body>
</html>`
	return strings.NewReplacer(
		"{{LANG}}", language,
		"{{TITLE}}", mediumEscapeHTML(article.Title),
		"{{COVER}}", coverHTML,
		"{{BODY}}", body,
		"{{FOOTER}}", footerHTML,
	).Replace(template)
}

func jsonStringify(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buffer.String(), "\n"), nil
}

func compileMedium(ctx compileContext) (CompiledArticle, error) {
	article := ctx.article
	portable, assets, err := CompileMediumPublishingMarkdown(article.Body, ctx.assetBase)
	if err != nil {
		return CompiledArticle{}, err
	}
	portable = strings.TrimSpace(portable)
	payload, warnings := buildMediumPayload(article, ctx.slug, ctx.profile, portable, ctx.policy)
	fallback := buildMediumCopyHTML(article, ctx.slug, ctx.profile, portable, ctx.policy)
	hashValue, err := jsonStringify(struct {
		Payload          mediumPayload `json:"payload"`
		FallbackHTML     string        `json:"fallbackHTML"`
		RequiresFallback bool          `json:"requiresFallback"`
	}{payload, fallback, false})
	if err != nil {
		return CompiledArticle{}, err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return CompiledArticle{}, err
	}
	html := fallback
	markdown := portable
	if !ctx.options.DryRun {
		markdown = replaceAssetURLs(markdown, assets)
		html = replaceAssetURLs(html, assets)
		payloadText := replaceAssetURLs(string(payloadJSON), assets)
		payloadJSON = []byte(payloadText)
		for index := range assets {
			assets[index].Source = "blogctl-asset://" + assets[index].Kind + "/" + assets[index].ID
		}
	} else {
		for index := range assets {
			assets[index].Source = assets[index].PublicURL
		}
	}
	return CompiledArticle{
		Version: ProtocolVersion, Slug: ctx.slug, Platform: "medium", Title: article.Title, Description: article.Description,
		Markdown: markdown, HTML: html, Language: ctx.profile.Language, CanonicalURL: CanonicalURL(ctx.slug, ctx.profile.Language),
		NativeCanonicalURL: payload.CanonicalURL, Tags: payload.Tags, CoverImageURL: resolveArticleAssetURL(article.CoverImage),
		Published: false, Payload: payloadJSON, FallbackHTML: fallback, RequiresFallback: false, Warnings: warnings,
		ContentHash: hashText(hashValue), SourceDir: ctx.sourceDir, Assets: assets,
	}, nil
}

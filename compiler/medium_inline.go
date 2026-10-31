package compiler

import (
	"net/url"
	"regexp"
	"strings"
)

const (
	mediumParagraph      = 1
	mediumH2             = 3
	mediumImageParagraph = 4
	mediumBlockquote     = 6  // Verified in Medium native /p/{id}?format=json after editor quote action.
	mediumPre            = 8  // Confirmed from Medium editor rendering.
	mediumULI            = 9  // Verified from real Medium native unordered-list paragraphs.
	mediumOLI            = 10 // Medium ordered-list paragraph; do not treat code as OLI.

	mediumMarkupBold   = 1
	mediumMarkupItalic = 2
	mediumMarkupLink   = 3
	mediumMarkupCode   = 10
	mediumMarkupStrike = 11
)

type mediumMarkup struct {
	Type       int    `json:"type"`
	Start      int    `json:"start"`
	End        int    `json:"end"`
	Href       string `json:"href,omitempty"`
	AnchorType *int   `json:"anchorType,omitempty"`
}

type mediumInline struct {
	Text    string
	Markups []mediumMarkup
	HTML    string
}

type inlineTokenPattern struct {
	kind string
	re   *regexp.Regexp
}

var mediumInlinePatterns = []inlineTokenPattern{
	{"image", regexp.MustCompile(`!\[([^]]*)\]\(([^)]+)\)`)},
	{"link", regexp.MustCompile(`\[([^]]+)\]\(([^)]+)\)`)},
	{"bold", regexp.MustCompile(`\*\*([^*]+?)\*\*`)},
	{"strike", regexp.MustCompile(`~~([^~]+?)~~`)},
	{"code", regexp.MustCompile("`([^`\\n]+)`")},
	{"italic", regexp.MustCompile(`\*([^*\n]+?)\*`)},
}

type mediumInlineToken struct {
	kind   string
	start  int
	end    int
	first  string
	second string
}

func utf16Length(value string) int {
	count := 0
	for _, r := range value {
		if r > 0xffff {
			count += 2
		} else {
			count++
		}
	}
	return count
}

func mediumEscapeHTML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
	)
	return replacer.Replace(value)
}

func mediumAbsoluteHref(href string) string {
	if href == "" || strings.HasPrefix(href, "#") {
		return href
	}
	base, _ := url.Parse(SiteOrigin)
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

func mediumFindInlineToken(source string) (mediumInlineToken, bool) {
	var winner mediumInlineToken
	found := false
	for _, pattern := range mediumInlinePatterns {
		indexes := pattern.re.FindStringSubmatchIndex(source)
		if indexes == nil {
			continue
		}
		if found && indexes[0] >= winner.start {
			continue
		}
		token := mediumInlineToken{kind: pattern.kind, start: indexes[0], end: indexes[1]}
		if len(indexes) >= 4 && indexes[2] >= 0 {
			token.first = source[indexes[2]:indexes[3]]
		}
		if len(indexes) >= 6 && indexes[4] >= 0 {
			token.second = source[indexes[4]:indexes[5]]
		}
		winner = token
		found = true
	}
	return winner, found
}

func shiftMediumMarkups(markups []mediumMarkup, offset int) []mediumMarkup {
	result := make([]mediumMarkup, len(markups))
	for index, markup := range markups {
		markup.Start += offset
		markup.End += offset
		result[index] = markup
	}
	return result
}

func parseMediumInline(source string, warnings *[]string) mediumInline {
	rest := source
	var text strings.Builder
	var html strings.Builder
	markups := []mediumMarkup{}

	for rest != "" {
		token, ok := mediumFindInlineToken(rest)
		if !ok {
			text.WriteString(rest)
			html.WriteString(mediumEscapeHTML(rest))
			break
		}
		prefix := rest[:token.start]
		text.WriteString(prefix)
		html.WriteString(mediumEscapeHTML(prefix))
		start := utf16Length(text.String())

		switch token.kind {
		case "code":
			text.WriteString(token.first)
			html.WriteString("<code>" + mediumEscapeHTML(token.first) + "</code>")
			markups = append(markups, mediumMarkup{Type: mediumMarkupCode, Start: start, End: utf16Length(text.String())})
		case "image":
			href := mediumAbsoluteHref(token.second)
			label := "[Image]"
			if token.first != "" {
				label = "[Image: " + token.first + "]"
			}
			text.WriteString(label)
			html.WriteString("<a href=\"" + mediumEscapeHTML(href) + "\">" + mediumEscapeHTML(label) + "</a>")
			anchor := 0
			markups = append(markups, mediumMarkup{Type: mediumMarkupLink, Start: start, End: utf16Length(text.String()), Href: href, AnchorType: &anchor})
			if warnings != nil {
				*warnings = append(*warnings, "Image "+token.second+" is represented as a link in Medium draft M0")
			}
		default:
			inner := parseMediumInline(token.first, warnings)
			text.WriteString(inner.Text)
			markups = append(markups, shiftMediumMarkups(inner.Markups, start)...)
			end := utf16Length(text.String())
			switch token.kind {
			case "link":
				href := mediumAbsoluteHref(token.second)
				html.WriteString("<a href=\"" + mediumEscapeHTML(href) + "\">" + inner.HTML + "</a>")
				anchor := 0
				markups = append(markups, mediumMarkup{Type: mediumMarkupLink, Start: start, End: end, Href: href, AnchorType: &anchor})
			case "bold":
				html.WriteString("<strong>" + inner.HTML + "</strong>")
				markups = append(markups, mediumMarkup{Type: mediumMarkupBold, Start: start, End: end})
			case "strike":
				html.WriteString("<del>" + inner.HTML + "</del>")
				markups = append(markups, mediumMarkup{Type: mediumMarkupStrike, Start: start, End: end})
			case "italic":
				html.WriteString("<em>" + inner.HTML + "</em>")
				markups = append(markups, mediumMarkup{Type: mediumMarkupItalic, Start: start, End: end})
			}
		}
		rest = rest[token.end:]
	}
	return mediumInline{Text: text.String(), Markups: markups, HTML: html.String()}
}

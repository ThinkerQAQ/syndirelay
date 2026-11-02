package publisher

import (
	"errors"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Zhihu builds its own heading navigation. Authored GitHub-style fragment
// links are not portable to Zhihu's editor, so remove only an unmistakable
// heading + anchor-only list and let Zhihu generate its native directory.
// Anything ambiguous is rejected instead of discarding article content.
var zhihuAuthoredTOCHeading = regexp.MustCompile(`(?is)<h[1-6]\b[^>]*>\s*(?:目录|table\s+of\s+contents)\s*</h[1-6]>`)
var zhihuContentHeading = regexp.MustCompile(`(?i)<h[2-6]\b`)

func normalizeZhihuContents(body string) (string, error) {
	heading := zhihuAuthoredTOCHeading.FindStringIndex(body)
	if heading == nil {
		return body, nil
	}
	tail := body[heading[1]:]
	spaces := len(tail) - len(strings.TrimLeft(tail, " \n\r\t"))
	list := tail[spaces:]
	tokenizer := html.NewTokenizer(strings.NewReader(list))
	depth, consumed, links, inLink := 0, 0, 0, false
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			return "", errors.New("Zhihu table of contents is not an anchor-only list; manual review required")
		}
		consumed += len(tokenizer.Raw())
		token := tokenizer.Token()
		name := strings.ToLower(token.Data)
		switch tokenType {
		case html.StartTagToken:
			if name == "ul" || name == "ol" {
				depth++
			} else if depth > 0 && name == "a" {
				valid := false
				for _, attr := range token.Attr {
					if attr.Key == "href" && strings.HasPrefix(attr.Val, "#") && len(attr.Val) > 1 {
						valid = true
					}
				}
				if !valid {
					return "", errors.New("Zhihu TOC contains a non-fragment link; manual review required")
				}
				links++
				inLink = true
			}
		case html.EndTagToken:
			if name == "a" {
				inLink = false
			}
			if name == "ul" || name == "ol" {
				depth--
				if depth == 0 {
					if links == 0 {
						return "", errors.New("Zhihu TOC has no fragment links")
					}
					return body[:heading[0]] + body[heading[1]+spaces+consumed:], nil
				}
				if depth < 0 {
					return "", errors.New("Zhihu TOC has invalid list nesting")
				}
			}
		case html.TextToken:
			if strings.TrimSpace(token.Data) != "" && (depth == 0 || !inLink) {
				return "", errors.New("Zhihu TOC contains non-link text; manual review required")
			}
		}
		if depth == 0 && tokenType != html.TextToken && name != "ul" && name != "ol" {
			return "", errors.New("Zhihu TOC heading is not followed by a list")
		}
	}
}

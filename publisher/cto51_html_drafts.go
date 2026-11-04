package publisher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var cto51DraftEditorLink = regexp.MustCompile(`^/blogger/draft/([0-9]{1,20})$`)

// 51CTO occasionally returns a non-JSON response from its legacy AJAX draft
// listing. The authenticated draft-management page is a safe read-only
// fallback. Extract only exact draft-editor links, never public article links.
func parse51CTODraftManagementPage(content []byte) ([]Cto51Post, error) {
	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return nil, err
	}
	var text func(*html.Node) string
	text = func(n *html.Node) string {
		if n == nil {
			return ""
		}
		if n.Type == html.TextNode {
			return n.Data
		}
		var b strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			b.WriteString(text(c))
		}
		return b.String()
	}
	if !strings.Contains(text(doc), "草稿箱") && !strings.Contains(text(doc), "草稿管理") {
		return nil, errors.New("51CTO draft management page is not authenticated or has changed")
	}
	seen := map[string]bool{}
	drafts := []Cto51Post{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := ""
			for _, a := range n.Attr {
				if a.Key == "href" {
					href = a.Val
					break
				}
			}
			u, err := url.Parse(strings.TrimSpace(href))
			if err == nil && u.RawQuery == "" && u.Fragment == "" && (u.Host == "" || strings.EqualFold(u.Host, "blog.51cto.com")) {
				match := cto51DraftEditorLink.FindStringSubmatch(u.Path)
				if len(match) == 2 {
					title := strings.TrimSpace(strings.Join(strings.Fields(text(n)), " "))
					if title != "" && title != "编辑" && !seen[match[1]] {
						seen[match[1]] = true
						drafts = append(drafts, Cto51Post{
							ID: match[1], Title: title, URL: "https://blog.51cto.com" + u.Path,
						})
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if len(drafts) == 0 {
		return nil, errors.New("51CTO draft management returned no exact draft editor links; manual ID selection is still supported")
	}
	return drafts, nil
}

func (c *cto51Adapter) listDraftsFromManagement(ctx context.Context) ([]Cto51Post, error) {
	req, err := c.request(ctx, http.MethodGet, cto51Origin+"/creative-center/draft", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("51CTO draft management returned HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return parse51CTODraftManagementPage(raw)
}

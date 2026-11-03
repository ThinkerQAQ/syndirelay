package publisher

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// toutiaoExternalizeSectionLinks changes only valid in-page fragment links
// to their original canonical article anchors. The Toutiao editor removes
// same-page fragment links, but retains external HTTPS links. When the source
// has no canonical URL, leave the compiled HTML unchanged.
func toutiaoExternalizeSectionLinks(raw, canonical string) string {
	base, err := url.Parse(strings.TrimSpace(canonical))
	if err != nil || base.Scheme != "https" || base.Hostname() == "" ||
		base.User != nil || base.Opaque != "" {
		return raw
	}
	base.Fragment = ""
	base.RawFragment = ""
	baseURL := base.String()

	root := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(raw), root)
	if err != nil {
		return raw
	}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	changed := false
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.A {
			for idx := range n.Attr {
				attr := &n.Attr[idx]
				href := strings.TrimSpace(attr.Val)
				if attr.Key == "href" && strings.HasPrefix(href, "#") {
					fragment, decodeErr := url.PathUnescape(strings.TrimPrefix(href, "#"))
					if decodeErr != nil || fragment == "" || strings.ContainsAny(fragment, "\x00\r\n") {
						continue
					}
					attr.Val = baseURL + href
					changed = true
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	if !changed {
		return raw
	}
	var result strings.Builder
	for node := root.FirstChild; node != nil; node = node.NextSibling {
		if err := html.Render(&result, node); err != nil {
			return raw
		}
	}
	return result.String()
}

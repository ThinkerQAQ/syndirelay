package compiler

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// addLinkedHeadingIDs resolves authored TOC/reference labels to headings.
// Reuse the IDs in the source links rather than inventing a second slug rule:
// manually authored fragments can differ from a renderer's default slugs.
func addLinkedHeadingIDs(raw string) (string, error) {
	root := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(raw), root)
	if err != nil {
		return "", err
	}
	for _, node := range nodes {
		root.AppendChild(node)
	}
	headings := map[string][]*html.Node{}
	targets := map[string]bool{}
	var links []*html.Node
	var text func(*html.Node) string
	text = func(n *html.Node) string {
		if n.Type == html.TextNode {
			return n.Data
		}
		var b strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			b.WriteString(text(c))
		}
		return b.String()
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "id" || (n.DataAtom == atom.A && a.Key == "name") {
					targets[a.Val] = true
				}
			}
			if len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
				label := strings.Join(strings.Fields(text(n)), " ")
				headings[label] = append(headings[label], n)
			}
			if n.DataAtom == atom.A {
				links = append(links, n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	changed := false
	for _, link := range links {
		for _, a := range link.Attr {
			value := strings.TrimSpace(a.Val)
			if a.Key != "href" || !strings.HasPrefix(value, "#") {
				continue
			}
			id, err := url.PathUnescape(strings.TrimPrefix(value, "#"))
			if err != nil || id == "" || targets[id] {
				continue
			}
			label := strings.Join(strings.Fields(text(link)), " ")
			for _, heading := range headings[label] {
				hasID := false
				for _, attr := range heading.Attr {
					if attr.Key == "id" {
						hasID = true
						break
					}
				}
				if hasID {
					continue
				}
				heading.Attr = append(heading.Attr, html.Attribute{Key: "id", Val: id})
				targets[id] = true
				changed = true
				break
			}
		}
	}
	if !changed {
		return raw, nil
	}
	var out strings.Builder
	for n := root.FirstChild; n != nil; n = n.NextSibling {
		if err := html.Render(&out, n); err != nil {
			return "", err
		}
	}
	return out.String(), nil
}

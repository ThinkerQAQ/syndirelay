package publisher

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// normalizeToutiaoTOC converts the nested Markdown TOC into one list item per
// heading. Toutiao's creator editor merges nested LI links into the parent
// item, making the original TOC unreadable. The compiler supplies heading IDs
// so references can be retained when their targets exist. Only a list immediately following a TOC heading
// is rewritten; article body lists are preserved. Matched heading links remain
// clickable; unresolved fragments degrade to plain labels.
func normalizeToutiaoTOC(raw string) string {
	root := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	fragment, err := html.ParseFragment(strings.NewReader(raw), root)
	if err != nil {
		return raw
	}
	for _, node := range fragment {
		root.AppendChild(node)
	}
	targets := toutiaoAnchorTargets(root)
	changed := false
	for heading := root.FirstChild; heading != nil; heading = heading.NextSibling {
		if !toutiaoTOCHeading(heading) {
			continue
		}
		list := heading.NextSibling
		for list != nil && (list.Type == html.CommentNode ||
			(list.Type == html.TextNode && strings.TrimSpace(list.Data) == "")) {
			list = list.NextSibling
		}
		if !toutiaoListNode(list) {
			continue
		}
		replacement := &html.Node{Type: html.ElementNode, Data: "ul", DataAtom: atom.Ul}
		toutiaoAppendTOCItems(list, 0, replacement, targets)
		if replacement.FirstChild == nil {
			continue
		}
		list.Parent.InsertBefore(replacement, list)
		list.Parent.RemoveChild(list)
		changed = true
	}
	if !changed {
		return raw
	}
	var result strings.Builder
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&result, child); err != nil {
			return raw
		}
	}
	return result.String()
}

func toutiaoTOCHeading(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}
	switch node.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
	default:
		return false
	}
	text := strings.ToLower(strings.TrimSpace(toutiaoText(node, false)))
	switch text {
	case "目录", "table of contents", "contents":
		return true
	}
	return false
}

func toutiaoListNode(node *html.Node) bool {
	return node != nil && node.Type == html.ElementNode && (node.Data == "ul" || node.Data == "ol")
}

func toutiaoText(node *html.Node, ignoreLists bool) string {
	if node == nil || (ignoreLists && toutiaoListNode(node)) {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var b strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		b.WriteString(toutiaoText(child, ignoreLists))
	}
	return b.String()
}

type toutiaoTOCLink struct{ label, href string }

func toutiaoOwnLinks(node *html.Node, links *[]toutiaoTOCLink) {
	if toutiaoListNode(node) {
		return
	}
	if node.Type == html.ElementNode && node.Data == "a" {
		if text := strings.Join(strings.Fields(toutiaoText(node, false)), " "); text != "" {
			href := ""
			for _, attr := range node.Attr {
				if attr.Key == "href" {
					href = attr.Val
					break
				}
			}
			*links = append(*links, toutiaoTOCLink{label: text, href: href})
		}
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		toutiaoOwnLinks(child, links)
	}
}

func toutiaoTOCDepth(label string, nestedDepth int) int {
	fields := strings.Fields(label)
	if len(fields) == 0 {
		return nestedDepth
	}
	// 1. Heading -> depth 0, 1.1 Heading -> depth 1,
	// 1.3.1 Heading -> depth 2. Non-numbered labels use DOM nesting.
	numeric := strings.TrimSuffix(fields[0], ".")
	sections := strings.Split(numeric, ".")
	if numeric == "" {
		return nestedDepth
	}
	for _, section := range sections {
		if section == "" {
			return nestedDepth
		}
		for _, digit := range section {
			if digit < '0' || digit > '9' {
				return nestedDepth
			}
		}
	}
	return min(len(sections)-1, 3)
}

func toutiaoAddTOCItem(dest *html.Node, label string, depth int, href string) {
	if label == "" {
		return
	}
	item := &html.Node{Type: html.ElementNode, Data: "li", DataAtom: atom.Li}
	// U+3000 is not collapsed by HTML whitespace normalization.
	if href == "" {
		item.AppendChild(&html.Node{Type: html.TextNode, Data: strings.Repeat("　", toutiaoTOCDepth(label, depth)) + label})
	} else {
		indent := strings.Repeat("　", toutiaoTOCDepth(label, depth))
		if indent != "" {
			item.AppendChild(&html.Node{Type: html.TextNode, Data: indent})
		}
		link := &html.Node{Type: html.ElementNode, Data: "a", DataAtom: atom.A, Attr: []html.Attribute{{Key: "href", Val: href}}}
		link.AppendChild(&html.Node{Type: html.TextNode, Data: label})
		item.AppendChild(link)
	}
	dest.AppendChild(item)
}

func toutiaoAppendTOCItems(list *html.Node, depth int, output *html.Node, targets map[string]bool) {
	for item := list.FirstChild; item != nil; item = item.NextSibling {
		if item.Type != html.ElementNode || item.Data != "li" {
			continue
		}
		var links []toutiaoTOCLink
		toutiaoOwnLinks(item, &links)
		if len(links) > 0 {
			for _, link := range links {
				href := link.href
				if strings.HasPrefix(strings.TrimSpace(href), "#") && !toutiaoFragmentResolves(href, targets) {
					href = ""
				}
				toutiaoAddTOCItem(output, link.label, depth, href)
			}
		} else {
			toutiaoAddTOCItem(output, strings.Join(strings.Fields(toutiaoText(item, true)), " "), depth, "")
		}
		for child := item.FirstChild; child != nil; child = child.NextSibling {
			if toutiaoListNode(child) {
				toutiaoAppendTOCItems(child, depth+1, output, targets)
			}
		}
	}
}

// Keep fragment references when their target exists in the compiled payload.
// Only unresolved references degrade to rich text labels; external links stay intact.
func normalizeToutiaoSectionReferences(raw string) string {
	root := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(raw), root)
	if err != nil {
		return raw
	}
	for _, node := range nodes {
		root.AppendChild(node)
	}
	targets := toutiaoAnchorTargets(root)
	changed := false
	var visit func(*html.Node)
	visit = func(parent *html.Node) {
		for node := parent.FirstChild; node != nil; {
			next := node.NextSibling
			visit(node)
			if node.Type == html.ElementNode && node.DataAtom == atom.A {
				for _, attr := range node.Attr {
					if attr.Key == "href" && strings.HasPrefix(strings.TrimSpace(attr.Val), "#") && !toutiaoFragmentResolves(attr.Val, targets) {
						for child := node.FirstChild; child != nil; child = node.FirstChild {
							node.RemoveChild(child)
							parent.InsertBefore(child, node)
						}
						parent.RemoveChild(node)
						changed = true
						break
					}
				}
			}
			node = next
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

func toutiaoAnchorTargets(root *html.Node) map[string]bool {
	targets := map[string]bool{}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "id" || (n.DataAtom == atom.A && attr.Key == "name") {
					targets[attr.Val] = true
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	return targets
}
func toutiaoFragmentResolves(href string, targets map[string]bool) bool {
	id, err := url.PathUnescape(strings.TrimPrefix(strings.TrimSpace(href), "#"))
	return err == nil && id != "" && targets[id]
}

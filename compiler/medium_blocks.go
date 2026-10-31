package compiler

import (
	"regexp"
	"strconv"
	"strings"
)

type mediumBlock struct {
	Kind          string
	ParagraphType int
	Text          string
	Markups       []mediumMarkup
	HTML          string
	Level         int
	Language      string
	URL           string
	Alt           string
	Depth         int
}

var (
	mediumTOCHeading      = regexp.MustCompile(`(?i)^#{1,3}\s+(table\s+of\s+contents|contents|目录)\s*$`)
	mediumAnyHeading      = regexp.MustCompile(`^#{1,6}\s+`)
	mediumHeading         = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	mediumFence           = regexp.MustCompile("^```([^\\s`]*)\\s*$")
	mediumFenceEnd        = regexp.MustCompile("^\\s*```")
	mediumUnordered       = regexp.MustCompile(`^\s*[-*+]\s+(.*)$`)
	mediumOrdered         = regexp.MustCompile(`^\s*\d+[.)]\s+(.*)$`)
	mediumListLine        = regexp.MustCompile(`^([-*+]\s+|\d+[.)]\s+)`)
	mediumTableSeparator  = regexp.MustCompile(`^:?-{3,}:?$`)
	mediumAdmonition      = regexp.MustCompile(`(?i)^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*(.*)$`)
	mediumStandaloneImage = regexp.MustCompile(`^!\[([^]]*)\]\((\S+)(\s+["']([^"']*)["'])?\)$`)
)

var mediumAdmonitionLabels = map[string]string{
	"NOTE": "Note", "TIP": "Tip", "IMPORTANT": "Important", "WARNING": "Warning", "CAUTION": "Caution",
}

func stripMediumTOC(markdown string) string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	output := []string{}
	for index := 0; index < len(lines); {
		if !mediumTOCHeading.MatchString(strings.TrimSpace(lines[index])) {
			output = append(output, lines[index])
			index++
			continue
		}
		index++
		for index < len(lines) && strings.TrimSpace(lines[index]) == "" {
			index++
		}
		for index < len(lines) {
			value := strings.TrimSpace(lines[index])
			if value == "" {
				index++
				continue
			}
			if strings.Trim(value, "-") == "" && len(value) >= 3 {
				index++
				break
			}
			if mediumAnyHeading.MatchString(value) {
				break
			}
			if mediumListLine.MatchString(value) {
				index++
				continue
			}
			break
		}
	}
	return strings.TrimSpace(strings.Join(output, "\n"))
}

func splitMediumTableRow(line string) []string {
	text := strings.TrimSpace(line)
	if strings.HasPrefix(text, "|") {
		text = text[1:]
	}
	if strings.HasSuffix(text, "|") {
		text = text[:len(text)-1]
	}
	cells := []string{}
	var current strings.Builder
	escaped := false
	codeTicks := 0
	for index := 0; index < len(text); index++ {
		char := text[index]
		if escaped {
			current.WriteByte(char)
			escaped = false
			continue
		}
		if char == '\\' {
			current.WriteByte(char)
			escaped = true
			continue
		}
		if char == '`' {
			run := 1
			for index+run < len(text) && text[index+run] == '`' {
				run++
			}
			current.WriteString(strings.Repeat("`", run))
			index += run - 1
			if codeTicks == run {
				codeTicks = 0
			} else if codeTicks == 0 {
				codeTicks = run
			}
			continue
		}
		if char == '|' && codeTicks == 0 {
			cells = append(cells, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteByte(char)
	}
	cells = append(cells, strings.TrimSpace(current.String()))
	return cells
}
func isMediumTableSeparator(line string) bool {
	cells := splitMediumTableRow(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !mediumTableSeparator.MatchString(strings.TrimSpace(cell)) {
			return false
		}
	}
	return true
}
func flattenMediumTables(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := []string{}
	inFence := false
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if mediumFenceEnd.MatchString(line) {
			inFence = !inFence
			out = append(out, line)
			continue
		}
		if !inFence && strings.Contains(line, "|") && index+1 < len(lines) && isMediumTableSeparator(lines[index+1]) {
			headers := splitMediumTableRow(line)
			index += 2
			for index < len(lines) && strings.Contains(lines[index], "|") && strings.TrimSpace(lines[index]) != "" {
				cells := splitMediumTableRow(lines[index])
				rowTitle := "Row"
				if len(cells) > 0 && cells[0] != "" {
					rowTitle = cells[0]
				} else if len(headers) > 0 && headers[0] != "" {
					rowTitle = headers[0]
				}
				if len(cells) == 2 {
					if !strings.HasPrefix(rowTitle, "**") || !strings.HasSuffix(rowTitle, "**") {
						rowTitle = "**" + rowTitle + "**"
					}
					out = append(out, rowTitle+" — "+cells[1], "")
				} else {
					out = append(out, "**"+rowTitle+"**")
					details := []string{}
					for ci := 1; ci < len(cells); ci++ {
						header := "Column "
						if ci < len(headers) && headers[ci] != "" {
							header = headers[ci]
						} else {
							header += itoaMedium(ci + 1)
						}
						details = append(details, "**"+header+":** "+cells[ci])
					}
					if len(details) > 0 {
						out = append(out, strings.Join(details, " · "))
					}
					out = append(out, "")
				}
				index++
			}
			index--
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
func itoaMedium(value int) string { return strconv.Itoa(value) }
func mediumIsBlockStart(line string) bool {
	value := strings.TrimSpace(line)
	if value == "" || mediumFenceEnd.MatchString(value) || mediumAnyHeading.MatchString(value) || strings.HasPrefix(value, ">") || mediumUnordered.MatchString(value) || mediumOrdered.MatchString(value) || mediumStandaloneImage.MatchString(value) {
		return true
	}
	return strings.Trim(value, "-") == "" && len(value) >= 3
}
func mediumListDepth(line string) int {
	indent := 0
	for _, r := range line {
		if r == ' ' {
			indent++
		} else if r == '\t' {
			indent += 4
		} else {
			break
		}
	}
	return indent / 2
}
func normalizeMediumTask(value string) string {
	trim := value
	if strings.HasPrefix(trim, "[ ] ") {
		return "☐ " + trim[4:]
	}
	if strings.HasPrefix(trim, "[x] ") || strings.HasPrefix(trim, "[X] ") {
		return "☑ " + trim[4:]
	}
	return value
}
func normalizeMediumAdmonition(values []string) []string {
	if len(values) == 0 {
		return values
	}
	m := mediumAdmonition.FindStringSubmatch(strings.TrimSpace(values[0]))
	if m == nil {
		return values
	}
	label := mediumAdmonitionLabels[strings.ToUpper(m[1])]
	first := "**" + label + ".**"
	if strings.TrimSpace(m[2]) != "" {
		first += " " + m[2]
	}
	return append([]string{first}, values[1:]...)
}
func decorateMediumList(inline mediumInline, depth int) mediumInline {
	if depth <= 0 {
		return inline
	}
	prefix := strings.Repeat("↳ ", depth)
	inline.Text = prefix + inline.Text
	inline.HTML = mediumEscapeHTML(prefix) + inline.HTML
	inline.Markups = shiftMediumMarkups(inline.Markups, utf16Length(prefix))
	return inline
}
func mediumStandaloneImageBlock(line string) (mediumBlock, bool) {
	m := mediumStandaloneImage.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return mediumBlock{}, false
	}
	alt := m[1]
	target := mediumAbsoluteHref(m[2])
	label := "[Image]"
	if alt != "" {
		label = "[Image: " + alt + "]"
	}
	anchor := 0
	return mediumBlock{Kind: "image", ParagraphType: mediumParagraph, Text: label, Markups: []mediumMarkup{{Type: mediumMarkupLink, Start: 0, End: utf16Length(label), Href: target, AnchorType: &anchor}}, HTML: "<img src=\"" + mediumEscapeHTML(target) + "\" alt=\"" + mediumEscapeHTML(alt) + "\">", URL: target, Alt: alt}, true
}
func parseMediumBlocks(markdown string) ([]mediumBlock, []string) {
	warnings := []string{}
	normalized := flattenMediumTables(stripMediumTOC(markdown))
	lines := strings.Split(normalized, "\n")
	blocks := []mediumBlock{}
	for index := 0; index < len(lines); {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			index++
			continue
		}
		if strings.Trim(trimmed, "-") == "" && len(trimmed) >= 3 {
			index++
			continue
		}
		if image, ok := mediumStandaloneImageBlock(trimmed); ok {
			blocks = append(blocks, image)
			index++
			continue
		}
		if f := mediumFence.FindStringSubmatch(trimmed); f != nil {
			language := f[1]
			index++
			code := []string{}
			for index < len(lines) && !mediumFenceEnd.MatchString(lines[index]) {
				code = append(code, lines[index])
				index++
			}
			if index < len(lines) {
				index++
			}
			blocks = append(blocks, mediumBlock{Kind: "pre", ParagraphType: mediumPre, Text: strings.Join(code, "\n"), Language: language, Markups: []mediumMarkup{}})
			continue
		}
		if h := mediumHeading.FindStringSubmatch(line); h != nil {
			level := len(h[1])
			inline := parseMediumInline(h[2], &warnings)
			// The HAR's H4=8 is from the already malformed draft: Medium
			// rendered it as a code block. Medium's native H4 style is not
			// verified, so use a distinct bold paragraph instead of
			// emitting PRE or collapsing H4 into the same H2/H3 style.
			ptype := mediumH2
			if level >= 4 {
				ptype = mediumParagraph
				if inline.Text != "" {
					inline.Markups = append(inline.Markups, mediumMarkup{
						Type: mediumMarkupBold, Start: 0, End: utf16Length(inline.Text),
					})
				}
			}
			blocks = append(blocks, mediumBlock{Kind: "heading", Level: level, ParagraphType: ptype, Text: inline.Text, Markups: inline.Markups, HTML: inline.HTML})
			index++
			continue
		}
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), ">") {
			values := []string{}
			for index < len(lines) && strings.HasPrefix(strings.TrimLeft(lines[index], " \t"), ">") {
				v := strings.TrimLeft(lines[index], " \t")
				v = strings.TrimPrefix(v, ">")
				v = strings.TrimPrefix(v, " ")
				values = append(values, v)
				index++
			}
			values = normalizeMediumAdmonition(values)
			groups := [][]string{}
			current := []string{}
			for _, v := range values {
				if strings.TrimSpace(v) == "" {
					if len(current) > 0 {
						groups = append(groups, current)
						current = []string{}
					}
					continue
				}
				current = append(current, v)
			}
			if len(current) > 0 {
				groups = append(groups, current)
			}
			for _, g := range groups {
				inline := parseMediumInline(strings.Join(g, "\n"), &warnings)
				blocks = append(blocks, mediumBlock{Kind: "blockquote", ParagraphType: mediumBlockquote, Text: inline.Text, Markups: inline.Markups, HTML: inline.HTML})
			}
			continue
		}
		if u := mediumUnordered.FindStringSubmatch(line); u != nil {
			depth := mediumListDepth(line)
			inline := decorateMediumList(parseMediumInline(normalizeMediumTask(u[1]), &warnings), depth)
			blocks = append(blocks, mediumBlock{Kind: "uli", ParagraphType: mediumULI, Depth: depth, Text: inline.Text, Markups: inline.Markups, HTML: inline.HTML})
			index++
			continue
		}
		if o := mediumOrdered.FindStringSubmatch(line); o != nil {
			depth := mediumListDepth(line)
			inline := decorateMediumList(parseMediumInline(o[1], &warnings), depth)
			blocks = append(blocks, mediumBlock{Kind: "oli", ParagraphType: mediumOLI, Depth: depth, Text: inline.Text, Markups: inline.Markups, HTML: inline.HTML})
			index++
			continue
		}
		paragraph := []string{trimmed}
		index++
		for index < len(lines) && !mediumIsBlockStart(lines[index]) {
			paragraph = append(paragraph, strings.TrimSpace(lines[index]))
			index++
		}
		inline := parseMediumInline(strings.Join(paragraph, " "), &warnings)
		blocks = append(blocks, mediumBlock{Kind: "paragraph", ParagraphType: mediumParagraph, Text: inline.Text, Markups: inline.Markups, HTML: inline.HTML})
	}
	unique := []string{}
	seen := map[string]struct{}{}
	for _, w := range warnings {
		if _, ok := seen[w]; !ok {
			seen[w] = struct{}{}
			unique = append(unique, w)
		}
	}
	return blocks, unique
}

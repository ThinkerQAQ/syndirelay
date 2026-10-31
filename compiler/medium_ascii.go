package compiler

import (
	"strings"
	"unicode"
)

// mediumASCIIDiagram is deliberately conservative; normal snippets
// remain native PRE blocks (in particular count++, ready=true etc.).
func mediumASCIIDiagram(source string) bool {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(source, "\r\n", "\n")), "\n")
	if len(lines) < 3 {
		return false
	}
	maxWidth := 0
	glyphs := 0
	multicolumn := 0
	for _, line := range lines {
		width := 0
		for _, ch := range line {
			if strings.ContainsRune("│┌┐┘└┬┴┼├┤─▼▲↓↑→←", ch) {
				glyphs++
			}
			if unicode.Is(unicode.Han, ch) {
				width += 2
			} else {
				width++
			}
		}
		if width > maxWidth {
			maxWidth = width
		}
		if strings.Contains(line, "          ") && len(strings.TrimSpace(line)) > 16 {
			multicolumn++
		}
	}
	return glyphs >= 2 || (maxWidth >= 60 && multicolumn >= 2)
}

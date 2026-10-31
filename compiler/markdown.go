package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const SiteOrigin = "https://thinkerqaq.com"
const MermaidRenderer = "@mermaid-js/mermaid-cli@11.17.0"
const plantUMLVersion = "1.2026.7"

var fenceStartPattern = regexp.MustCompile("^( {0,3})(`{3,}|~{3,})(.*)$")
var fenceEndPattern = regexp.MustCompile("^( {0,3})(`{3,}|~{3,})\\s*$")
var plantStartPattern = regexp.MustCompile("(?im)^\\s*@start([A-Za-z0-9_]+)\\b")
var plantEndPattern = regexp.MustCompile("(?im)^\\s*@end([A-Za-z0-9_]+)\\b")
var plantUnsafePattern = regexp.MustCompile("(?im)^\\s*!\\s*(include[A-Za-z]*|import|theme)\\b|%(getenv|load[A-Za-z]*|filename|dirpath)\\s*\\(")
var accTitlePattern = regexp.MustCompile("(?m)^\\s*accTitle:\\s*(.+?)\\s*$")
var accDescrPattern = regexp.MustCompile("(?m)^\\s*accDescr:\\s*(.+?)\\s*$")
var mdRootLinkPattern = regexp.MustCompile("(\\]\\()/([^/])")
var htmlRootLinkPattern = regexp.MustCompile("(?i)((href|src)=[\"'])/([^/])")

type fenceOpening struct {
	marker byte
	length int
	info   string
}

func fenceStart(line string) (fenceOpening, bool) {
	m := fenceStartPattern.FindStringSubmatch(line)
	if m == nil {
		return fenceOpening{}, false
	}
	return fenceOpening{marker: m[2][0], length: len(m[2]), info: strings.TrimSpace(m[3])}, true
}
func fenceEnd(line string, o fenceOpening) bool {
	m := fenceEndPattern.FindStringSubmatch(line)
	return m != nil && m[2][0] == o.marker && len(m[2]) >= o.length
}
func fenceLanguage(info string) string {
	f := strings.Fields(info)
	if len(f) == 0 {
		return ""
	}
	return strings.ToLower(f[0])
}
func normalizeNewlines(v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(v, "\r\n", "\n"), "\r", "\n")
}
func looksLikeMermaid(source string) bool {
	for _, line := range strings.Split(normalizeNewlines(source), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		for _, p := range []string{"flowchart", "graph", "sequenceDiagram", "classDiagram", "stateDiagram", "erDiagram", "gantt", "pie", "journey", "gitGraph", "mindmap", "timeline", "quadrantChart", "sankey-beta", "xychart-beta", "block-beta", "packet-beta", "architecture-beta", "kanban"} {
			if line == p || strings.HasPrefix(line, p+" ") || strings.HasPrefix(line, p+"-") {
				return true
			}
		}
		return false
	}
	return false
}
func diagramKind(language, source string) string {
	inferred := language == "" || language == "text" || language == "plaintext"
	if language == "mermaid" || ((language == "diagram" || language == "uml" || inferred) && looksLikeMermaid(source)) {
		return "mermaid"
	}
	if language == "puml" || language == "plantuml" || ((language == "diagram" || language == "uml" || inferred) && plantStartPattern.MatchString(source)) {
		return "plantuml"
	}
	return ""
}
func normalizePlantUML(source string) (string, error) {
	v := strings.TrimSpace(normalizeNewlines(source))
	if v == "" {
		return "", errors.New("empty PlantUML diagram")
	}
	if plantUnsafePattern.MatchString(v) {
		return "", errors.New("external includes, themes and environment/file access are disabled for diagrams")
	}
	starts := plantStartPattern.FindAllStringSubmatch(v, -1)
	ends := plantEndPattern.FindAllStringSubmatch(v, -1)
	if len(starts) == 0 && len(ends) == 0 {
		v = "@startuml\n" + v + "\n@enduml"
	} else if len(starts) != 1 || len(ends) != 1 || !strings.EqualFold(starts[0][1], ends[0][1]) {
		return "", errors.New("each PlantUML block must contain exactly one matching @start… / @end… pair")
	}
	return v + "\n", nil
}
func assetURL(base, key string) (string, error) {
	if strings.TrimSpace(base) == "" {
		base = DefaultAssetBaseURL
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errors.New("asset public base URL must use HTTPS")
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	r, err := url.Parse(key)
	if err != nil {
		return "", err
	}
	return u.ResolveReference(r).String(), nil
}
func assetForDiagram(kind, source, base string) (Asset, error) {
	if kind == "mermaid" {
		n := strings.TrimSpace(normalizeNewlines(source))
		if n == "" {
			return Asset{}, errors.New("empty Mermaid diagram")
		}
		sum := sha256.Sum256([]byte(MermaidRenderer + "\n" + n + "\n"))
		id := hex.EncodeToString(sum[:])[:24]
		key := "generated/mermaid/" + id + ".png"
		pub, err := assetURL(base, key)
		if err != nil {
			return Asset{}, err
		}
		alt := "Mermaid diagram"
		if m := accDescrPattern.FindStringSubmatch(n); m != nil && strings.TrimSpace(m[1]) != "" {
			alt = strings.TrimSpace(m[1])
		} else if m := accTitlePattern.FindStringSubmatch(n); m != nil && strings.TrimSpace(m[1]) != "" {
			alt = strings.TrimSpace(m[1])
		}
		return Asset{Kind: kind, ID: id, Renderer: MermaidRenderer, ObjectKey: key, PublicURL: pub, Content: n, Alt: alt}, nil
	}
	if kind == "table" {
		n := strings.TrimRight(normalizeNewlines(source), "\n")
		sum := sha256.Sum256([]byte("medium-table:v1:svg-text\n" + n))
		id := hex.EncodeToString(sum[:])[:24]
		key := "generated/table/" + id + ".png"
		pub, err := assetURL(base, key)
		if err != nil {
			return Asset{}, err
		}
		headers := splitMediumTableRow(strings.Split(n, "\n")[0])
		return Asset{Kind: "table", ID: id, Renderer: "medium-table", ObjectKey: key, PublicURL: pub, Content: n, Alt: "Comparison table: " + strings.Join(headers, "; ")}, nil
	}
	if kind == "ascii" {
		n := strings.TrimRight(normalizeNewlines(source), "\n")
		sum := sha256.Sum256([]byte("medium-ascii:v1:preserve-spaces\n" + n))
		id := hex.EncodeToString(sum[:])[:24]
		key := "generated/ascii/" + id + ".png"
		pub, err := assetURL(base, key)
		if err != nil {
			return Asset{}, err
		}
		return Asset{Kind: kind, ID: id, Renderer: "text-art", ObjectKey: key, PublicURL: pub, Content: n, Alt: "Monospaced execution diagram (original text layout preserved)"}, nil
	}
	if kind == "plantuml" {
		n, err := normalizePlantUML(source)
		if err != nil {
			return Asset{}, err
		}
		sum := sha256.Sum256([]byte("plantuml:" + plantUMLVersion + ":sandbox:utf8:svg:v1\n" + n))
		id := hex.EncodeToString(sum[:])
		key := "generated/plantuml/" + id + ".png"
		pub, err := assetURL(base, key)
		if err != nil {
			return Asset{}, err
		}
		return Asset{Kind: kind, ID: id, Renderer: "plantuml", ObjectKey: key, PublicURL: pub, Content: n, Alt: "PlantUML diagram"}, nil
	}
	return Asset{}, fmt.Errorf("unsupported diagram kind: %s", kind)
}
func absoluteRootLinks(line string) string {
	line = mdRootLinkPattern.ReplaceAllString(line, "${1}"+SiteOrigin+"/${2}")
	return htmlRootLinkPattern.ReplaceAllString(line, "${1}"+SiteOrigin+"/${3}")
}
func escapeAlt(v string) string {
	v = strings.ReplaceAll(v, "\\", "\\\\")
	v = strings.ReplaceAll(v, "]", "\\]")
	return strings.Join(strings.Fields(v), " ")
}

// Medium cannot preserve the horizontal layout of long PRE text diagrams.
func CompileMediumPublishingMarkdown(markdown, base string) (string, []Asset, error) {
	return compilePublishingMarkdown(markdown, base, true)
}
func CompilePublishingMarkdown(markdown, base string) (string, []Asset, error) {
	return compilePublishingMarkdown(markdown, base, false)
}
func compilePublishingMarkdown(markdown, base string, mediumASCII bool) (string, []Asset, error) {
	lines := strings.Split(normalizeNewlines(markdown), "\n")
	out := make([]string, 0, len(lines))
	assets := map[string]Asset{}
	var order []string
	for i := 0; i < len(lines); {
		if mediumASCII && i+1 < len(lines) && strings.Contains(lines[i], "|") && isMediumTableSeparator(lines[i+1]) {
			header := splitMediumTableRow(lines[i])
			if len(header) >= 2 && len(splitMediumTableRow(lines[i+1])) == len(header) {
				end := i + 2
				for end < len(lines) && strings.TrimSpace(lines[end]) != "" && strings.Contains(lines[end], "|") && len(splitMediumTableRow(lines[end])) == len(header) {
					end++
				}
				if end > i+2 {
					a, err := assetForDiagram("table", strings.Join(lines[i:end], "\n"), base)
					if err != nil {
						return "", nil, err
					}
					k := a.Kind + ":" + a.ID
					if _, found := assets[k]; !found {
						assets[k] = a
						order = append(order, k)
					}
					out = append(out, "!["+escapeAlt(a.Alt)+"]("+a.PublicURL+")")
					i = end
					continue
				}
			}
		}
		o, ok := fenceStart(lines[i])
		if !ok {
			out = append(out, absoluteRootLinks(lines[i]))
			i++
			continue
		}
		start := i
		i++
		for i < len(lines) && !fenceEnd(lines[i], o) {
			i++
		}
		closed := i < len(lines)
		end := i
		source := strings.Join(lines[start+1:end], "\n")
		language := fenceLanguage(o.info)
		kind := diagramKind(language, source)
		if kind == "" && mediumASCII && (language == "text" || language == "plaintext" || language == "") && mediumASCIIDiagram(source) {
			kind = "ascii"
		}
		if kind != "" && !closed {
			return "", nil, errors.New("unclosed diagram fenced block")
		}
		if kind == "" {
			if closed {
				end++
			}
			out = append(out, lines[start:end]...)
			i = end
			continue
		}
		a, err := assetForDiagram(kind, source, base)
		if err != nil {
			return "", nil, err
		}
		k := a.Kind + ":" + a.ID
		if _, ok := assets[k]; !ok {
			assets[k] = a
			order = append(order, k)
		}
		out = append(out, "!["+escapeAlt(a.Alt)+"]("+a.PublicURL+")")
		if closed {
			i++
		}
	}
	result := make([]Asset, 0, len(order))
	for _, k := range order {
		result = append(result, assets[k])
	}
	return strings.Join(out, "\n"), result, nil
}
func replaceAssetURLs(value string, assets []Asset) string {
	for _, a := range assets {
		value = strings.ReplaceAll(value, a.PublicURL, "blogctl-asset://"+a.Kind+"/"+a.ID)
	}
	return value
}

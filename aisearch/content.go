package aisearch

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	IndexSchemaVersion = 3
	MaxItemKeyLength   = 128
	DefaultBlogOrigin  = "https://thinkerqaq.com"
)

type Frontmatter struct {
	Data map[string]string
	Body string
}

type PrepareResult struct {
	Articles         int
	Notes            int
	NoteTranslations int
}

type Document struct {
	Key           string
	Collection    string
	ID            string
	Language      string
	Title         string
	URL           string
	Priority      int
	SchemaVersion int
	SourcePath    string
	Content       string
}

func ParseFrontmatter(markdown string) Frontmatter {
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return Frontmatter{Data: map[string]string{}, Body: markdown}
	}
	end := strings.Index(normalized[4:], "\n---\n")
	if end < 0 {
		return Frontmatter{Data: map[string]string{}, Body: markdown}
	}
	end += 4
	block := normalized[4:end]
	body := normalized[end+5:]
	data := map[string]string{}
	for _, line := range strings.Split(block, "\n") {
		key, raw, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || !validFrontmatterKey(key) {
			continue
		}
		value := strings.TrimSpace(raw)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		data[key] = value
	}
	return Frontmatter{Data: data, Body: body}
}

func validFrontmatterKey(value string) bool {
	for index, r := range value {
		if index == 0 {
			if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
				return false
			}
			continue
		}
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return value != ""
}

func walkMarkdown(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func copyFile(source, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func PrepareInput(sourceRoot, outputRoot string) (PrepareResult, error) {
	if strings.TrimSpace(sourceRoot) == "" || strings.TrimSpace(outputRoot) == "" {
		return PrepareResult{}, errors.New("AI Search source and output roots are required")
	}
	if err := os.RemoveAll(outputRoot); err != nil {
		return PrepareResult{}, err
	}
	if err := os.MkdirAll(outputRoot, 0o755); err != nil {
		return PrepareResult{}, err
	}

	result := PrepareResult{}
	publicNoteIDs := map[string]struct{}{}

	articlesBase := filepath.Join(sourceRoot, "articles")
	articleFiles, err := walkMarkdown(articlesBase)
	if err != nil {
		return result, err
	}
	for _, absolute := range articleFiles {
		data, err := os.ReadFile(absolute)
		if err != nil {
			return result, err
		}
		frontmatter := ParseFrontmatter(string(data))
		if frontmatter.Data["status"] != "published" {
			continue
		}
		relative, _ := filepath.Rel(articlesBase, absolute)
		if err := copyFile(absolute, filepath.Join(outputRoot, "articles", relative)); err != nil {
			return result, err
		}
		result.Articles++
	}

	notesBase := filepath.Join(sourceRoot, "notes")
	noteFiles, err := walkMarkdown(notesBase)
	if err != nil {
		return result, err
	}
	for _, absolute := range noteFiles {
		data, err := os.ReadFile(absolute)
		if err != nil {
			return result, err
		}
		frontmatter := ParseFrontmatter(string(data))
		if strings.EqualFold(strings.TrimSpace(frontmatter.Data["indexable"]), "false") {
			continue
		}
		relative, _ := filepath.Rel(notesBase, absolute)
		id := strings.TrimSuffix(filepath.ToSlash(relative), filepath.Ext(relative))
		publicNoteIDs[id] = struct{}{}
		if err := copyFile(absolute, filepath.Join(outputRoot, "notes", relative)); err != nil {
			return result, err
		}
		result.Notes++
	}

	translationsBase := filepath.Join(sourceRoot, "note-translations")
	translationFiles, err := walkMarkdown(translationsBase)
	if err != nil {
		return result, err
	}
	for _, absolute := range translationFiles {
		data, err := os.ReadFile(absolute)
		if err != nil {
			return result, err
		}
		frontmatter := ParseFrontmatter(string(data))
		id := strings.TrimSpace(frontmatter.Data["translationOf"])
		language := strings.ToLower(strings.TrimSpace(frontmatter.Data["language"]))
		if id == "" || language == "" || language == "zh" {
			continue
		}
		if _, ok := publicNoteIDs[id]; !ok {
			continue
		}
		relative, _ := filepath.Rel(translationsBase, absolute)
		if err := copyFile(absolute, filepath.Join(outputRoot, "note-translations", relative)); err != nil {
			return result, err
		}
		result.NoteTranslations++
	}
	return result, nil
}

func ContentLanguage(collection, id string, data map[string]string) (string, error) {
	explicit := strings.ToLower(strings.TrimSpace(data["language"]))
	if explicit != "" {
		if explicit != "zh" && explicit != "en" {
			return "", fmt.Errorf("unsupported content language %s for %s/%s", explicit, collection, id)
		}
		return explicit, nil
	}
	if collection == "articles" && strings.HasPrefix(id, "en/") {
		return "en", nil
	}
	return "zh", nil
}

func escapeURIComponent(value string) string {
	const hexChars = "0123456789ABCDEF"
	var builder strings.Builder
	for _, b := range []byte(value) {
		if (b >= 'A' && b <= 'Z') ||
			(b >= 'a' && b <= 'z') ||
			(b >= '0' && b <= '9') ||
			b == '-' || b == '_' || b == '.' || b == '!' ||
			b == '~' || b == '*' || b == '\'' || b == '(' || b == ')' {
			builder.WriteByte(b)
			continue
		}
		builder.WriteByte('%')
		builder.WriteByte(hexChars[b>>4])
		builder.WriteByte(hexChars[b&0x0f])
	}
	return builder.String()
}

func ContentSourceURL(blogOrigin, collection, id, language string) string {
	origin := strings.TrimRight(strings.TrimSpace(blogOrigin), "/")
	routeID := id
	if collection == "articles" && language != "zh" && strings.HasPrefix(routeID, language+"/") {
		routeID = strings.TrimPrefix(routeID, language+"/")
	}
	parts := strings.Split(routeID, "/")
	for index := range parts {
		parts[index] = escapeURIComponent(parts[index])
	}
	prefix := ""
	if language != "zh" {
		prefix = "/" + language
	}
	return origin + prefix + "/" + collection + "/" + strings.Join(parts, "/") + "/"
}

func ItemKey(collection, id string) string {
	reversible := "blog--" + collection + "--" + escapeURIComponent(id) + ".md"
	if len(reversible) <= MaxItemKeyLength {
		return reversible
	}
	digest := sha256.Sum256([]byte(collection + "\x00" + id))
	return "blog--" + collection + "--h-" + hex.EncodeToString(digest[:])[:32] + ".md"
}

func loadCollectionDocuments(contentRoot, blogOrigin, collection string, priority int, publicNoteIDs map[string]struct{}) (map[string]Document, error) {
	base := filepath.Join(contentRoot, collection)
	files, err := walkMarkdown(base)
	if err != nil {
		return nil, err
	}
	documents := map[string]Document{}
	for _, absolute := range files {
		relative, _ := filepath.Rel(base, absolute)
		relativeSlash := filepath.ToSlash(relative)
		id := strings.TrimSuffix(relativeSlash, filepath.Ext(relativeSlash))
		data, err := os.ReadFile(absolute)
		if err != nil {
			return nil, err
		}
		frontmatter := ParseFrontmatter(string(data))
		if collection == "articles" && frontmatter.Data["status"] != "published" {
			continue
		}
		if collection == "notes" && strings.EqualFold(strings.TrimSpace(frontmatter.Data["indexable"]), "false") {
			continue
		}
		if collection == "notes" {
			publicNoteIDs[id] = struct{}{}
		}
		language, err := ContentLanguage(collection, id, frontmatter.Data)
		if err != nil {
			return nil, err
		}
		title := strings.TrimSpace(frontmatter.Data["title"])
		if title == "" {
			title = filepath.Base(id)
		}
		sourceURL := ContentSourceURL(blogOrigin, collection, id, language)
		description := strings.TrimSpace(frontmatter.Data["description"])
		sections := []string{
			"# " + title,
			"Source URL: " + sourceURL,
			"Collection: " + collection,
			"Language: " + language,
		}
		if description != "" {
			sections = append(sections, "\n"+description+"\n")
		}
		sections = append(sections, strings.TrimSpace(frontmatter.Body))
		content := strings.TrimSpace(strings.Join(sections, "\n\n")) + "\n"
		key := ItemKey(collection, id)
		documents[key] = Document{
			Key: key, Collection: collection, ID: id, Language: language,
			Title: title, URL: sourceURL, Priority: priority,
			SchemaVersion: IndexSchemaVersion,
			SourcePath:    "src/content/" + collection + "/" + relativeSlash,
			Content:       content,
		}
	}
	return documents, nil
}

func LoadDocuments(contentRoot, blogOrigin string) (map[string]Document, error) {
	if strings.TrimSpace(blogOrigin) == "" {
		blogOrigin = DefaultBlogOrigin
	}
	documents := map[string]Document{}
	publicNoteIDs := map[string]struct{}{}

	for _, spec := range []struct {
		Collection string
		Priority   int
	}{
		{"articles", 2},
		{"notes", 1},
	} {
		loaded, err := loadCollectionDocuments(contentRoot, blogOrigin, spec.Collection, spec.Priority, publicNoteIDs)
		if err != nil {
			return nil, err
		}
		for key, document := range loaded {
			documents[key] = document
		}
	}

	translationsBase := filepath.Join(contentRoot, "note-translations")
	files, err := walkMarkdown(translationsBase)
	if err != nil {
		return nil, err
	}
	for _, absolute := range files {
		relative, _ := filepath.Rel(translationsBase, absolute)
		relativeSlash := filepath.ToSlash(relative)
		data, err := os.ReadFile(absolute)
		if err != nil {
			return nil, err
		}
		frontmatter := ParseFrontmatter(string(data))
		id := strings.TrimSpace(frontmatter.Data["translationOf"])
		language := strings.ToLower(strings.TrimSpace(frontmatter.Data["language"]))
		if id == "" || language == "" || language == "zh" {
			continue
		}
		if language != "en" {
			return nil, fmt.Errorf("unsupported content language %s for notes/%s", language, id)
		}
		if _, ok := publicNoteIDs[id]; !ok {
			continue
		}
		title := strings.TrimSpace(frontmatter.Data["title"])
		if title == "" {
			title = filepath.Base(id)
		}
		sourceURL := ContentSourceURL(blogOrigin, "notes", id, language)
		sections := []string{
			"# " + title,
			"Source URL: " + sourceURL,
			"Collection: notes",
			"Language: " + language,
		}
		if description := strings.TrimSpace(frontmatter.Data["description"]); description != "" {
			sections = append(sections, "\n"+description+"\n")
		}
		sections = append(sections, strings.TrimSpace(frontmatter.Body))
		key := ItemKey("notes", language+"/"+id)
		documents[key] = Document{
			Key: key, Collection: "notes", ID: id, Language: language,
			Title: title, URL: sourceURL, Priority: 1,
			SchemaVersion: IndexSchemaVersion,
			SourcePath:    "src/content/note-translations/" + relativeSlash,
			Content:       strings.TrimSpace(strings.Join(sections, "\n\n")) + "\n",
		}
	}
	return documents, nil
}

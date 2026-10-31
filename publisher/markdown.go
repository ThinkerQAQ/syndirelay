package publisher

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	markdownImagePattern = regexp.MustCompile(`!\[[^\]]*\]\(([^\s\)]+)`)
	htmlImagePattern     = regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["']`)
)

func parseGeneratedMarkdown(raw []byte) (title, description, body string, err error) {
	text := strings.ReplaceAll(strings.TrimPrefix(string(raw), "\ufeff"), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", "", "", errors.New("generated platform document is missing frontmatter")
	}
	rest := text[len("---\n"):]
	index := strings.Index(rest, "\n---")
	if index < 0 {
		return "", "", "", errors.New("generated platform document has invalid frontmatter")
	}
	frontmatter := rest[:index]
	body = strings.TrimSpace(rest[index+len("\n---"):])
	for _, line := range strings.Split(frontmatter, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "title" && key != "description" {
			continue
		}
		decoded := value
		if strings.HasPrefix(value, "\"") {
			if unquoted, unquoteErr := strconv.Unquote(value); unquoteErr == nil {
				decoded = unquoted
			}
		}
		if key == "title" {
			title = decoded
		} else {
			description = decoded
		}
	}
	if title == "" || body == "" {
		return "", "", "", errors.New("generated platform document is missing title or body")
	}
	return title, description, body, nil
}

func imageSources(markdown string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, pattern := range []*regexp.Regexp{markdownImagePattern, htmlImagePattern} {
		for _, match := range pattern.FindAllStringSubmatch(markdown, -1) {
			if len(match) < 2 {
				continue
			}
			source := strings.Trim(strings.TrimSpace(match[1]), "<>")
			if source == "" {
				continue
			}
			if _, exists := seen[source]; exists {
				continue
			}
			seen[source] = struct{}{}
			result = append(result, source)
		}
	}
	return result
}

func decodeDataURI(source string) ([]byte, string, error) {
	if !strings.HasPrefix(source, "data:") {
		return nil, "", errors.New("not a data URI")
	}
	header, encoded, ok := strings.Cut(strings.TrimPrefix(source, "data:"), ",")
	if !ok {
		return nil, "", errors.New("invalid data URI")
	}
	parts := strings.Split(header, ";")
	contentType := parts[0]
	isBase64 := false
	for _, part := range parts[1:] {
		if strings.EqualFold(part, "base64") {
			isBase64 = true
		}
	}
	if isBase64 {
		payload, err := base64.StdEncoding.DecodeString(encoded)
		return payload, contentType, err
	}
	decoded, err := url.PathUnescape(encoded)
	if err != nil {
		return nil, "", err
	}
	return []byte(decoded), contentType, nil
}

func loadImage(client *http.Client, source, sourceDir string) ([]byte, string, error) {
	if strings.HasPrefix(source, "data:") {
		return decodeDataURI(source)
	}
	parsed, err := url.Parse(source)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		response, requestErr := client.Get(source)
		if requestErr != nil {
			return nil, "", requestErr
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, "", fmt.Errorf("image download returned HTTP %d", response.StatusCode)
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, 25*1024*1024+1))
		if readErr != nil {
			return nil, "", readErr
		}
		if len(payload) > 25*1024*1024 {
			return nil, "", errors.New("image exceeds 25 MiB")
		}
		contentType := response.Header.Get("content-type")
		if contentType == "" {
			contentType = http.DetectContentType(payload)
		}
		return payload, contentType, nil
	}
	if sourceDir == "" {
		return nil, "", fmt.Errorf("cannot resolve relative image %q", source)
	}
	file := filepath.Clean(filepath.Join(sourceDir, filepath.FromSlash(source)))
	payload, err := os.ReadFile(file)
	if err != nil {
		return nil, "", err
	}
	contentType := mime.TypeByExtension(filepath.Ext(file))
	if contentType == "" {
		contentType = http.DetectContentType(payload)
	}
	return payload, contentType, nil
}

func replaceImages(markdown string, replacements map[string]string) string {
	for source, target := range replacements {
		markdown = strings.ReplaceAll(markdown, source, target)
	}
	return markdown
}

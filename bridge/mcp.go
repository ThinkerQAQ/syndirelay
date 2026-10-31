package bridge

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	blogplatform "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/platform"
)

// MCP does not receive cookies. The extension reads the pending platforms and
// calls the existing authenticated /v1/sessions/{platform} endpoint.
func (s *Server) handleMCPSessionRefresh(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var request struct {
			Platforms []string `json:"platforms"`
		}
		if err := readJSON(r, 4096, &request); err != nil {
			writeError(w, err)
			return
		}
		pending := make([]string, 0)
		s.mu.Lock()
		for _, platform := range request.Platforms {
			if platform == "devto" {
				continue
			} // API key authentication.
			if _, ok := browserSessionPlatforms[platform]; !ok {
				s.mu.Unlock()
				writeAPIError(w, 400, "unsupported_platform", "unknown browser platform", nil)
				return
			}
			if current, ok := s.sessions[platform]; ok && current.ExpiresAt.After(s.now().Add(90*time.Second)) {
				continue
			}
			s.sessionRefresh[platform] = s.now().Add(90 * time.Second)
			pending = append(pending, platform)
		}
		s.mu.Unlock()
		writeJSON(w, 200, map[string]any{"pending": pending})
	case http.MethodGet:
		pending := make([]string, 0)
		s.mu.Lock()
		for platform, deadline := range s.sessionRefresh {
			if !deadline.After(s.now()) {
				delete(s.sessionRefresh, platform)
				continue
			}
			pending = append(pending, platform)
		}
		s.mu.Unlock()
		writeJSON(w, 200, map[string]any{"pending": pending})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMCPPlatforms(w http.ResponseWriter) {
	result := make([]map[string]any, 0)
	for _, id := range blogplatform.IDs() {
		result = append(result, map[string]any{"id": id, "label": blogplatform.Label(id), "capabilities": blogplatform.For(id)})
	}
	writeJSON(w, 200, map[string]any{"platforms": result})
}

var mcpRequestID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{7,127}$`)

var mcpArticleSlug = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9/_-]{0,180}$`)

func (s *Server) handleMCPArticle(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/v1/mcp/article/")
	if !mcpArticleSlug.MatchString(slug) || strings.Contains(slug, "//") || strings.Contains(slug, "..") {
		writeAPIError(w, 400, "invalid_article", "invalid article slug", nil)
		return
	}
	lang := r.URL.Query().Get("language")
	if lang != "zh-CN" && lang != "en" {
		writeAPIError(w, 400, "invalid_language", "language must be zh-CN or en", nil)
		return
	}
	s.mu.Lock()
	root := s.config.ContentRoot
	s.mu.Unlock()
	if root == "" {
		writeAPIError(w, 400, "invalid_workspace", "content root not configured", nil)
		return
	}
	base := filepath.Join(root, "src", "content", "articles")
	if lang == "en" {
		base = filepath.Join(base, "en")
	}
	path := filepath.Join(base, filepath.FromSlash(slug)+".md")
	file, err := os.Open(path)
	if err != nil {
		writeAPIError(w, 404, "not_found", "article not found", nil)
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil || len(raw) > 1<<20 {
		writeAPIError(w, 413, "article_too_large", "article exceeds 1MiB", nil)
		return
	}
	writeJSON(w, 200, map[string]any{"slug": slug, "language": lang, "markdown": string(raw)})
}

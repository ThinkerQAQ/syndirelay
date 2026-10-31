package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	blogcompiler "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/compiler"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

type devtoArticleCandidate struct {
	ID                 int64  `json:"id"`
	Title              string `json:"title"`
	URL                string `json:"url"`
	Canonical          string `json:"canonical_url"`
	Published          bool   `json:"published"`
	PublishedAt        string `json:"published_at"`
	PublishedTimestamp string `json:"published_timestamp"`
	Bound              bool   `json:"bound"`
	BindingState       string `json:"bindingState,omitempty"`
}

func devtoCandidatePublished(candidate devtoArticleCandidate) bool {
	return candidate.Published || strings.TrimSpace(candidate.PublishedAt) != "" || strings.TrimSpace(candidate.PublishedTimestamp) != ""
}

func devtoArticleMatches(candidate devtoArticleCandidate, slug, title string) bool {
	if strings.EqualFold(strings.TrimSpace(candidate.Title), strings.TrimSpace(title)) {
		return true
	}
	parsed, err := url.Parse(candidate.Canonical)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := parsed.Hostname()
	// Previously distributed articles still point to the old host during the migration.
	if !strings.EqualFold(host, strings.TrimPrefix(blogcompiler.SiteOrigin, "https://")) &&
		!strings.EqualFold(host, "thinkerqaq.github.io") {
		return false
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	return len(segments) > 0 && segments[len(segments)-1] == slug
}

func (s *Server) handleDevtoArticleSearch(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	article, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	s.mu.Lock()
	key := devtoAPIKey(s.config)
	client := s.httpClient
	s.mu.Unlock()
	if key == "" {
		writeAPIError(response, http.StatusBadRequest, "api_key_required", "DEV.to API Key is not configured", nil)
		return
	}
	binding, _, bindingErr := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "devto")
	if bindingErr != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_load_failed", bindingErr.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	started := time.Now()
	candidates := []devtoArticleCandidate{}
	truncated := false
	for page := 1; page <= 5; page++ {
		endpoint := fmt.Sprintf("https://dev.to/api/articles/me/all?page=%d&per_page=100", page)
		upstream, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			writeAPIError(response, http.StatusInternalServerError, "request_failed", err.Error(), nil)
			return
		}
		upstream.Header.Set("api-key", key)
		upstream.Header.Set("accept", "application/vnd.forem.api-v1+json")
		result, err := client.Do(upstream)
		if err != nil {
			slog.Warn("devto article search failed", "operation", "article-search", "platform", "devto", "durationMs", time.Since(started).Milliseconds(), "errorType", fmt.Sprintf("%T", err))
			writeAPIError(response, http.StatusBadGateway, "lookup_failed", "DEV.to article lookup failed", nil)
			return
		}
		var batch []devtoArticleCandidate
		decodeErr := json.NewDecoder(http.MaxBytesReader(response, result.Body, 2*1024*1024)).Decode(&batch)
		_ = result.Body.Close()
		if result.StatusCode != http.StatusOK || decodeErr != nil {
			slog.Warn("devto article search rejected", "operation", "article-search", "platform", "devto", "status", result.StatusCode, "durationMs", time.Since(started).Milliseconds())
			writeAPIError(response, http.StatusBadGateway, "lookup_failed", "DEV.to article lookup returned an invalid response", nil)
			return
		}
		for _, candidate := range batch {
			candidate.Published = devtoCandidatePublished(candidate)
			candidateID := fmt.Sprint(candidate.ID)
			bound := (candidate.Published && binding.PublishedRemoteID == candidateID) ||
				(!candidate.Published && binding.RemoteDraftID == candidateID)
			if bound || devtoArticleMatches(candidate, slug, article.Title) {
				candidates = append(candidates, candidate)
			}
		}
		if len(batch) < 100 {
			break
		}
		if page == 5 {
			truncated = true
		}
	}
	for index := range candidates {
		candidates[index].Published = devtoCandidatePublished(candidates[index])
		if candidates[index].Published && binding.PublishedRemoteID == fmt.Sprint(candidates[index].ID) {
			candidates[index].Bound = true
			candidates[index].BindingState = "published"
		}
		if !candidates[index].Published && binding.RemoteDraftID == fmt.Sprint(candidates[index].ID) {
			candidates[index].Bound = true
			candidates[index].BindingState = "draft"
		}
	}
	bindings := []map[string]any{}
	if binding.RemoteDraftID != "" {
		bindings = append(bindings, map[string]any{"postId": binding.RemoteDraftID, "state": "draft", "url": binding.DraftURL})
	}
	if binding.PublishedRemoteID != "" {
		bindings = append(bindings, map[string]any{"postId": binding.PublishedRemoteID, "state": "published", "url": binding.PublishedURL})
	}
	slog.Info("devto article search completed", "operation", "article-search", "platform", "devto", "slug", slug, "candidateCount", len(candidates), "truncated", truncated, "durationMs", time.Since(started).Milliseconds())
	writeJSON(response, http.StatusOK, map[string]any{"candidates": candidates, "bindings": bindings, "truncated": truncated})
}

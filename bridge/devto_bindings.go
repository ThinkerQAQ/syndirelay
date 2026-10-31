package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

func (s *Server) devtoCandidateByID(ctx context.Context, slug, postID string) (devtoArticleCandidate, string, error) {
	_, root, err := s.cnBlogsArticle(slug)
	if err != nil {
		return devtoArticleCandidate{}, "", err
	}
	id, err := strconv.ParseInt(strings.TrimSpace(postID), 10, 64)
	if err != nil || id <= 0 {
		return devtoArticleCandidate{}, "", errors.New("invalid DEV.to article id")
	}
	s.mu.Lock()
	key := devtoAPIKey(s.config)
	client := s.httpClient
	s.mu.Unlock()
	if key == "" {
		return devtoArticleCandidate{}, "", errors.New("DEV.to API Key is not configured")
	}
	for page := 1; page <= 5; page++ {
		endpoint := fmt.Sprintf("https://dev.to/api/articles/me/all?page=%d&per_page=100", page)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return devtoArticleCandidate{}, "", err
		}
		request.Header.Set("api-key", key)
		request.Header.Set("accept", "application/vnd.forem.api-v1+json")
		response, err := client.Do(request)
		if err != nil {
			return devtoArticleCandidate{}, "", err
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		_ = response.Body.Close()
		if readErr != nil {
			return devtoArticleCandidate{}, "", readErr
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return devtoArticleCandidate{}, "", fmt.Errorf("DEV.to article lookup returned HTTP %d", response.StatusCode)
		}
		var batch []devtoArticleCandidate
		if err := json.Unmarshal(raw, &batch); err != nil {
			return devtoArticleCandidate{}, "", err
		}
		for _, candidate := range batch {
			if candidate.ID == id {
				candidate.Published = devtoCandidatePublished(candidate)
				return candidate, root, nil
			}
		}
		if len(batch) < 100 {
			break
		}
	}
	return devtoArticleCandidate{}, "", errors.New("selected DEV.to article was not found")
}

func devtoPostID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
	}
	return ""
}

func devtoBindingViews(binding publisher.PublicationBinding) []map[string]any {
	result := []map[string]any{}
	if binding.RemoteDraftID != "" {
		result = append(result, map[string]any{"postId": binding.RemoteDraftID, "state": "draft", "url": binding.DraftURL})
	}
	if binding.PublishedRemoteID != "" {
		result = append(result, map[string]any{"postId": binding.PublishedRemoteID, "state": "published", "url": binding.PublishedURL})
	}
	return result
}

func (s *Server) handleDevtoBindingPut(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	s.distributionMu.Lock()
	defer s.distributionMu.Unlock()
	var body struct {
		PostID  any    `json:"postId"`
		State   string `json:"state"`
		Replace bool   `json:"replace"`
	}
	if err := readJSON(request, 4096, &body); err != nil {
		writeError(response, err)
		return
	}
	postID := devtoPostID(body.PostID)
	state := strings.TrimSpace(body.State)
	if postID == "" || (state != "draft" && state != "published") {
		writeAPIError(response, http.StatusBadRequest, "invalid_request", "postId and draft/published state are required", nil)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	candidate, _, err := s.devtoCandidateByID(ctx, slug, postID)
	if err != nil {
		writeAPIError(response, http.StatusConflict, "candidate_missing", err.Error(), nil)
		return
	}
	actualState := "draft"
	if candidate.Published {
		actualState = "published"
	}
	if state != actualState {
		writeAPIError(response, http.StatusConflict, "candidate_state_changed", "DEV.to article publication state changed", nil)
		return
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "devto")
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_load_failed", err.Error(), nil)
		return
	}
	existing := binding.RemoteDraftID
	if state == "published" {
		existing = binding.PublishedRemoteID
	}
	if existing != "" && existing != postID && !body.Replace {
		writeAPIError(response, http.StatusConflict, "binding_exists", "replace the existing DEV.to binding explicitly", nil)
		return
	}
	binding.Slug = slug
	binding.Platform = "devto"
	binding.Source = "manual"
	binding.VerifiedAt = time.Now().UTC().Format(time.RFC3339)
	if state == "published" {
		binding.PublishedRemoteID = postID
		binding.PublishedURL = candidate.URL
	} else {
		binding.RemoteDraftID = postID
		binding.DraftURL = candidate.URL
	}
	if err := publisher.SavePublicationBinding(s.publicationBindingsPath(), binding); err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_save_failed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"binding": devtoBindingViews(binding)})
}

func (s *Server) handleDevtoBindingDelete(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	s.distributionMu.Lock()
	defer s.distributionMu.Unlock()
	var body struct {
		State  string `json:"state"`
		PostID any    `json:"postId"`
	}
	if err := readJSON(request, 4096, &body); err != nil {
		writeError(response, err)
		return
	}
	_, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	if err := publisher.DeletePublicationBindingState(s.publicationBindingsPath(), slug, "devto", strings.TrimSpace(body.State), devtoPostID(body.PostID)); err != nil {
		writeAPIError(response, http.StatusConflict, "binding_changed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"removed": true})
}

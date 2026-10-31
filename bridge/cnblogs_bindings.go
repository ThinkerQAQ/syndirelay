package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

type cnBlogsBindingView struct {
	PostID          string `json:"postId"`
	State           string `json:"state"`
	EditURL         string `json:"editUrl,omitempty"`
	PublicURL       string `json:"publicUrl,omitempty"`
	Account         string `json:"account,omitempty"`
	Source          string `json:"source,omitempty"`
	RemoteUpdatedAt string `json:"remoteUpdatedAt,omitempty"`
	VerifiedAt      string `json:"verifiedAt,omitempty"`
}

func cnBlogsBindingViews(binding publisher.PublicationBinding) []cnBlogsBindingView {
	result := make([]cnBlogsBindingView, 0, 2)
	if binding.RemoteDraftID != "" {
		result = append(result, cnBlogsBindingView{
			PostID: binding.RemoteDraftID, State: "draft", EditURL: publisher.CNBlogsDraftEditorURL(binding.RemoteDraftID),
			Account: binding.Account, Source: binding.Source, VerifiedAt: binding.VerifiedAt,
		})
	}
	if binding.PublishedRemoteID != "" {
		result = append(result, cnBlogsBindingView{
			PostID: binding.PublishedRemoteID, State: "published", PublicURL: binding.PublishedURL,
			Account: binding.Account, Source: binding.Source, RemoteUpdatedAt: binding.RemoteUpdatedAt, VerifiedAt: binding.VerifiedAt,
		})
	}
	return result
}

func cnBlogsBindingViewForState(binding publisher.PublicationBinding, state string) (cnBlogsBindingView, bool) {
	for _, item := range cnBlogsBindingViews(binding) {
		if item.State == state {
			return item, true
		}
	}
	return cnBlogsBindingView{}, false
}

func (s *Server) cnBlogsArticle(slug string) (articleSummary, string, error) {
	s.mu.Lock()
	contentRoot := s.config.ContentRoot
	s.mu.Unlock()
	articles, err := listArticles(contentRoot)
	if err != nil {
		return articleSummary{}, "", err
	}
	for _, article := range articles {
		if article.Slug == slug {
			return article, contentRoot, nil
		}
	}
	return articleSummary{}, "", errors.New("local article not found")
}

func (s *Server) handleCNBlogsBindingGet(response http.ResponseWriter, request *http.Request, slug string) {
	if !allowReadOnlyBridgeStatus(response, request) {
		return
	}
	article, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	binding, found, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "cnblogs")
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_read_failed", err.Error(), nil)
		return
	}
	views := []cnBlogsBindingView{}
	var current cnBlogsBindingView
	if found {
		views = cnBlogsBindingViews(binding)
		if len(views) > 0 {
			current = views[0]
			for _, candidate := range views {
				if candidate.State == "published" {
					current = candidate
					break
				}
			}
		}
	}
	writeJSON(response, http.StatusOK, map[string]any{"article": article, "binding": current, "found": len(views) > 0, "bindings": views})
}

func (s *Server) handleArticleLinks(response http.ResponseWriter, request *http.Request, slug string) {
	if !allowReadOnlyBridgeStatus(response, request) {
		return
	}
	_, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	links, err := publisher.LoadArticleLinks(s.publicationBindingsPath(), slug)
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "article_links_read_failed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"links": links})
}

func (s *Server) handleCNBlogsBindingVerify(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	_, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	binding, found, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "cnblogs")
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_read_failed", err.Error(), nil)
		return
	}
	if !found {
		writeJSON(response, http.StatusOK, map[string]any{"found": false})
		return
	}
	state := request.URL.Query().Get("state")
	if state != "draft" && state != "published" {
		if binding.PublishedRemoteID != "" {
			state = "published"
		} else {
			state = "draft"
		}
	}
	view, found := cnBlogsBindingViewForState(binding, state)
	if !found {
		writeJSON(response, http.StatusOK, map[string]any{"found": false})
		return
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("cnblogs")
	if err != nil {
		writeAPIError(response, http.StatusBadRequest, "session_required", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	started := time.Now()
	account, post, err := publisher.CNBlogsGetPost(ctx, client, session, view.PostID)
	if err != nil {
		slog.Warn("cnblogs publication verify failed", "operation", "publication-verify", "slug", slug, "durationMs", time.Since(started).Milliseconds(), "errorType", fmt.Sprintf("%T", err))
		writeAPIError(response, http.StatusBadGateway, "verification_failed", err.Error(), nil)
		return
	}
	if binding.Account != "" && !strings.EqualFold(binding.Account, account) {
		writeAPIError(response, http.StatusConflict, "account_mismatch", "publication belongs to a different CNBlogs account", nil)
		return
	}
	slog.Info("cnblogs publication verified", "operation", "publication-verify", "slug", slug, "postId", post.ID, "durationMs", time.Since(started).Milliseconds())
	writeJSON(response, http.StatusOK, map[string]any{"found": true, "binding": view, "post": post, "account": account})
}

func (s *Server) handleCNBlogsBindingSearch(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	article, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("cnblogs")
	if err != nil {
		writeAPIError(response, http.StatusBadRequest, "session_required", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	started := time.Now()
	account, candidates, err := publisher.CNBlogsSearchPosts(ctx, client, session, article.Title)
	if err != nil {
		slog.Warn("cnblogs binding search failed", "operation", "binding-search", "slug", slug, "durationMs", time.Since(started).Milliseconds(), "errorType", fmt.Sprintf("%T", err))
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	slog.Info("cnblogs binding search completed", "operation", "binding-search", "slug", slug, "candidateCount", len(candidates), "durationMs", time.Since(started).Milliseconds())
	writeJSON(response, http.StatusOK, map[string]any{"account": account, "candidates": candidates})
}

func (s *Server) handleCNBlogsBindingPut(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	if _, _, err := s.cnBlogsArticle(slug); err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	var body struct {
		Reference string `json:"reference"`
		State     string `json:"state"`
		Replace   bool   `json:"replace"`
	}
	if err := readJSON(request, 4096, &body); err != nil {
		writeError(response, err)
		return
	}
	id, err := publisher.ParseCNBlogsPostReference(body.Reference)
	if err != nil {
		writeAPIError(response, http.StatusBadRequest, "invalid_reference", err.Error(), nil)
		return
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("cnblogs")
	if err != nil {
		writeAPIError(response, http.StatusBadRequest, "session_required", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	account, post, err := publisher.CNBlogsGetPost(ctx, client, session, id)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "verification_failed", err.Error(), nil)
		return
	}
	s.distributionMu.Lock()
	defer s.distributionMu.Unlock()
	binding, found, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "cnblogs")
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_read_failed", err.Error(), nil)
		return
	}
	if !found {
		binding = publisher.PublicationBinding{Slug: slug, Platform: "cnblogs"}
	}
	if binding.Account != "" && !strings.EqualFold(binding.Account, account) {
		writeAPIError(response, http.StatusConflict, "account_mismatch", "publication belongs to a different CNBlogs account", nil)
		return
	}
	state := "draft"
	existingID := binding.RemoteDraftID
	if post.Published {
		state = "published"
		existingID = binding.PublishedRemoteID
	}
	// Match the selected draft/published state against the authenticated
	// creator result; never silently bind into the other state.
	requestedState := strings.TrimSpace(body.State)
	if requestedState != "" && requestedState != state {
		writeAPIError(response, http.StatusConflict, "candidate_state_changed", "CNBlogs article publication state does not match the selected state", nil)
		return
	}
	if existingID != "" && existingID != post.ID && !body.Replace {
		writeAPIError(response, http.StatusConflict, "binding_exists", "remove or replace the existing publication reference explicitly before linking another post", nil)
		return
	}

	binding.Account = account
	binding.Source = "manual"
	binding.VerifiedAt = time.Now().UTC().Format(time.RFC3339)
	if state == "published" {
		binding.PublishedRemoteID = post.ID
		binding.PublishedURL = post.URL
		binding.RemoteUpdatedAt = post.UpdatedAt
	} else {
		binding.RemoteDraftID = post.ID
		binding.DraftURL = publisher.CNBlogsDraftEditorURL(post.ID)
	}
	if err := publisher.SavePublicationBinding(s.publicationBindingsPath(), binding); err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_save_failed", err.Error(), nil)
		return
	}
	view, _ := cnBlogsBindingViewForState(binding, state)
	slog.Info("cnblogs publication reference saved", "operation", "publication-save", "slug", slug, "postId", post.ID, "state", state)
	writeJSON(response, http.StatusOK, map[string]any{"binding": view})
}

func (s *Server) handleCNBlogsBindingDelete(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	_, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	var body struct {
		State  string `json:"state"`
		PostID string `json:"postId"`
	}
	if err := readJSON(request, 4096, &body); err != nil {
		writeError(response, err)
		return
	}
	if body.State != "draft" && body.State != "published" {
		writeAPIError(response, http.StatusBadRequest, "invalid_state", "draft or published state is required", nil)
		return
	}
	s.distributionMu.Lock()
	defer s.distributionMu.Unlock()
	if err := publisher.DeletePublicationBindingState(s.publicationBindingsPath(), slug, "cnblogs", body.State, body.PostID); err != nil {
		writeAPIError(response, http.StatusConflict, "binding_changed", err.Error(), nil)
		return
	}
	slog.Info("cnblogs publication reference removed", "operation", "publication-delete", "slug", slug, "postId", body.PostID, "state", body.State)
	writeJSON(response, http.StatusOK, map[string]any{"removed": true})
}

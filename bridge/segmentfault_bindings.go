package bridge

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

type segmentFaultBindingView struct {
	PostID string `json:"postId"`
	State  string `json:"state"`
	URL    string `json:"url,omitempty"`
}

type segmentFaultCandidateView struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Published    bool   `json:"published"`
	Bound        bool   `json:"bound"`
	BindingState string `json:"bindingState,omitempty"`
}

func segmentFaultBindingViews(binding publisher.PublicationBinding) []segmentFaultBindingView {
	result := []segmentFaultBindingView{}
	if binding.RemoteDraftID != "" {
		result = append(result, segmentFaultBindingView{
			PostID: binding.RemoteDraftID, State: "draft", URL: binding.DraftURL,
		})
	}
	if binding.PublishedRemoteID != "" {
		result = append(result, segmentFaultBindingView{
			PostID: binding.PublishedRemoteID, State: "published", URL: binding.PublishedURL,
		})
	}
	return result
}

func segmentFaultBindingState(binding publisher.PublicationBinding, post publisher.SegmentFaultPost) (bool, string) {
	if post.Published && binding.PublishedRemoteID == post.ID {
		return true, "published"
	}
	if !post.Published && binding.RemoteDraftID == post.ID {
		return true, "draft"
	}
	return false, ""
}

func (s *Server) segmentFaultCandidates(ctx context.Context, slug string) (
	articleSummary, string, string, []publisher.SegmentFaultPost, publisher.PublicationBinding, error,
) {
	article, root, err := s.cnBlogsArticle(slug)
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("segmentfault")
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	account, posts, err := publisher.SegmentFaultListPosts(ctx, client, session)
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "segmentfault")
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	matches := make([]publisher.SegmentFaultPost, 0, len(posts))
	for _, post := range posts {
		bound, _ := segmentFaultBindingState(binding, post)
		if bound || publisher.SegmentFaultTitleMatches(article.Title, post.Title) {
			matches = append(matches, post)
		}
	}
	return article, root, account, matches, binding, nil
}

func (s *Server) handleSegmentFaultArticleList(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	_, _, account, posts, binding, err := s.segmentFaultCandidates(ctx, slug)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	candidates := make([]segmentFaultCandidateView, 0, len(posts))
	for _, post := range posts {
		bound, state := segmentFaultBindingState(binding, post)
		candidates = append(candidates, segmentFaultCandidateView{
			ID: post.ID, Title: post.Title, URL: post.URL, Published: post.Published,
			Bound: bound, BindingState: state,
		})
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"account": account, "candidates": candidates, "bindings": segmentFaultBindingViews(binding),
	})
}

func (s *Server) handleSegmentFaultBindingPut(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	s.distributionMu.Lock()
	defer s.distributionMu.Unlock()
	var body struct {
		PostID  string `json:"postId"`
		State   string `json:"state"`
		Replace bool   `json:"replace"`
	}
	if err := readJSON(request, 4096, &body); err != nil {
		writeError(response, err)
		return
	}
	body.PostID = strings.TrimSpace(body.PostID)
	body.State = strings.TrimSpace(body.State)
	if body.PostID == "" || (body.State != "draft" && body.State != "published") {
		writeAPIError(response, http.StatusBadRequest, "invalid_request", "postId and draft/published state are required", nil)
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	_, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("segmentfault")
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "session_unavailable", err.Error(), nil)
		return
	}
	account, posts, err := publisher.SegmentFaultListPosts(ctx, client, session)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "segmentfault")
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_load_failed", err.Error(), nil)
		return
	}
	var selected *publisher.SegmentFaultPost
	for index := range posts {
		state := "draft"
		if posts[index].Published {
			state = "published"
		}
		if posts[index].ID == body.PostID && state == body.State {
			selected = &posts[index]
			break
		}
	}
	if selected == nil {
		writeAPIError(response, http.StatusConflict, "candidate_missing", "SegmentFault list no longer contains the selected matching article", nil)
		return
	}

	existingID := binding.RemoteDraftID
	if body.State == "published" {
		existingID = binding.PublishedRemoteID
	}
	if existingID != "" && existingID != body.PostID && !body.Replace {
		writeAPIError(response, http.StatusConflict, "binding_exists", "replace the existing SegmentFault binding explicitly", nil)
		return
	}

	binding.Slug = slug
	binding.Platform = "segmentfault"
	binding.Account = account
	binding.Source = "manual"
	binding.VerifiedAt = time.Now().UTC().Format(time.RFC3339)
	if body.State == "published" {
		binding.PublishedRemoteID = selected.ID
		binding.PublishedURL = selected.URL
	} else {
		binding.RemoteDraftID = selected.ID
		binding.DraftURL = selected.URL
	}
	if err := publisher.SavePublicationBinding(s.publicationBindingsPath(), binding); err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_save_failed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"binding": segmentFaultBindingViews(binding),
	})
}

func (s *Server) handleSegmentFaultBindingDelete(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	s.distributionMu.Lock()
	defer s.distributionMu.Unlock()
	var body struct {
		State  string `json:"state"`
		PostID string `json:"postId"`
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
	if err := publisher.DeletePublicationBindingState(s.publicationBindingsPath(), slug, "segmentfault", body.State, body.PostID); err != nil {
		writeAPIError(response, http.StatusConflict, "binding_changed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"removed": true})
}

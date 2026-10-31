package bridge

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

type csdnBindingView struct {
	PostID string `json:"postId"`
	State  string `json:"state"`
	URL    string `json:"url,omitempty"`
}

type csdnCandidateView struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Published    bool   `json:"published"`
	Bound        bool   `json:"bound"`
	BindingState string `json:"bindingState,omitempty"`
}

func csdnBindingViews(binding publisher.PublicationBinding) []csdnBindingView {
	result := []csdnBindingView{}
	if binding.RemoteDraftID != "" {
		result = append(result, csdnBindingView{
			PostID: binding.RemoteDraftID, State: "draft", URL: binding.DraftURL,
		})
	}
	if binding.PublishedRemoteID != "" {
		result = append(result, csdnBindingView{
			PostID: binding.PublishedRemoteID, State: "published", URL: binding.PublishedURL,
		})
	}
	return result
}

func csdnBindingState(binding publisher.PublicationBinding, post publisher.CSDNPost) (bool, string) {
	if post.Published && binding.PublishedRemoteID == post.ID {
		return true, "published"
	}
	if !post.Published && binding.RemoteDraftID == post.ID {
		return true, "draft"
	}
	return false, ""
}

func appendCSDNCandidate(posts []publisher.CSDNPost, candidate publisher.CSDNPost) []publisher.CSDNPost {
	for index := range posts {
		if posts[index].ID == candidate.ID {
			posts[index] = candidate
			return posts
		}
	}
	return append(posts, candidate)
}

func (s *Server) csdnCandidates(ctx context.Context, slug string) (
	articleSummary, string, string, []publisher.CSDNPost, publisher.PublicationBinding, error,
) {
	article, root, err := s.cnBlogsArticle(slug)
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("csdn")
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	account, posts, err := publisher.CSDNListPosts(ctx, client, session)
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	matches := make([]publisher.CSDNPost, 0, len(posts))
	for _, post := range posts {
		if publisher.CSDNTitleMatches(article.Title, post.Title) {
			matches = append(matches, post)
		}
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "csdn")
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}

	// Re-verify bound IDs individually so a stored binding remains visible even
	// if CSDN temporarily omits it from a paginated list response.
	for _, postID := range []string{binding.RemoteDraftID, binding.PublishedRemoteID} {
		postID = strings.TrimSpace(postID)
		if postID == "" {
			continue
		}
		_, post, lookupErr := publisher.CSDNLookupPost(ctx, client, session, postID)
		if lookupErr == nil {
			matches = appendCSDNCandidate(matches, post)
		}
	}
	return article, root, account, matches, binding, nil
}

func (s *Server) handleCSDNLookupContext(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()

	article, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("csdn")
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "session_unavailable", err.Error(), nil)
		return
	}
	account, err := publisher.CSDNAccount(ctx, client, session)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "account_lookup_failed", err.Error(), nil)
		return
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "csdn")
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_load_failed", err.Error(), nil)
		return
	}
	boundPosts := []csdnCandidateView{}
	for _, postID := range []string{binding.RemoteDraftID, binding.PublishedRemoteID} {
		postID = strings.TrimSpace(postID)
		if postID == "" {
			continue
		}
		_, post, lookupErr := publisher.CSDNLookupPost(ctx, client, session, postID)
		if lookupErr != nil {
			continue
		}
		bound, state := csdnBindingState(binding, post)
		boundPosts = append(boundPosts, csdnCandidateView{
			ID: post.ID, Title: post.Title, URL: post.URL, Published: post.Published,
			Bound: bound, BindingState: state,
		})
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"title": article.Title, "account": account,
		"bindings": csdnBindingViews(binding), "boundPosts": boundPosts,
	})
}

func (s *Server) handleCSDNArticleList(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()

	_, _, account, posts, binding, err := s.csdnCandidates(ctx, slug)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	candidates := make([]csdnCandidateView, 0, len(posts))
	for _, post := range posts {
		bound, state := csdnBindingState(binding, post)
		candidates = append(candidates, csdnCandidateView{
			ID: post.ID, Title: post.Title, URL: post.URL, Published: post.Published,
			Bound: bound, BindingState: state,
		})
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"account": account, "candidates": candidates, "bindings": csdnBindingViews(binding),
	})
}

func (s *Server) handleCSDNBindingPut(response http.ResponseWriter, request *http.Request, slug string) {
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
	body.PostID = publisher.CSDNArticleID(body.PostID)
	body.State = strings.TrimSpace(body.State)
	if body.PostID == "" {
		writeAPIError(response, http.StatusBadRequest, "invalid_request", "a valid CSDN article id or URL is required", nil)
		return
	}

	article, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("csdn")
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "session_unavailable", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	account, selected, err := publisher.CSDNLookupPost(ctx, client, session, body.PostID)
	if err != nil {
		writeAPIError(response, http.StatusConflict, "candidate_missing", err.Error(), nil)
		return
	}
	if !publisher.CSDNTitleMatches(article.Title, selected.Title) {
		writeAPIError(response, http.StatusConflict, "candidate_mismatch", "selected CSDN article does not match the local article title", nil)
		return
	}
	actualState := "draft"
	if selected.Published {
		actualState = "published"
	}
	if body.State != "" && body.State != actualState {
		writeAPIError(response, http.StatusConflict, "candidate_state_changed", "CSDN article publication state changed", nil)
		return
	}
	body.State = actualState

	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "csdn")
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_load_failed", err.Error(), nil)
		return
	}
	existingID := binding.RemoteDraftID
	if body.State == "published" {
		existingID = binding.PublishedRemoteID
	}
	if existingID != "" && existingID != body.PostID && !body.Replace {
		writeAPIError(response, http.StatusConflict, "binding_exists", "replace the existing CSDN binding explicitly", nil)
		return
	}

	binding.Slug = slug
	binding.Platform = "csdn"
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
	writeJSON(response, http.StatusOK, map[string]any{"binding": csdnBindingViews(binding)})
}

func (s *Server) handleCSDNBindingDelete(response http.ResponseWriter, request *http.Request, slug string) {
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
	if err := publisher.DeletePublicationBindingState(s.publicationBindingsPath(), slug, "csdn", body.State, body.PostID); err != nil {
		writeAPIError(response, http.StatusConflict, "binding_changed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"removed": true})
}

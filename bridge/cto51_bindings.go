package bridge

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

type cto51BindingView struct {
	PostID string `json:"postId"`
	State  string `json:"state"`
	URL    string `json:"url,omitempty"`
}

type cto51CandidateView struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Published    bool   `json:"published"`
	Bound        bool   `json:"bound"`
	BindingState string `json:"bindingState,omitempty"`
}

func cto51BindingViews(binding publisher.PublicationBinding) []cto51BindingView {
	result := []cto51BindingView{}
	if binding.RemoteDraftID != "" {
		result = append(result, cto51BindingView{
			PostID: binding.RemoteDraftID, State: "draft", URL: binding.DraftURL,
		})
	}
	if binding.PublishedRemoteID != "" {
		result = append(result, cto51BindingView{
			PostID: binding.PublishedRemoteID, State: "published", URL: binding.PublishedURL,
		})
	}
	return result
}

func cto51BindingState(binding publisher.PublicationBinding, post publisher.Cto51Post) (bool, string) {
	if post.Published && binding.PublishedRemoteID == post.ID {
		return true, "published"
	}
	if !post.Published && binding.RemoteDraftID == post.ID {
		return true, "draft"
	}
	return false, ""
}

func (s *Server) cto51Candidates(ctx context.Context, slug string) (
	articleSummary, string, string, []publisher.Cto51Post, publisher.PublicationBinding, error,
) {
	article, root, err := s.cnBlogsArticle(slug)
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("51cto")
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	account, posts, err := publisher.Cto51ListPosts(ctx, client, session)
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "51cto")
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	matches := make([]publisher.Cto51Post, 0, len(posts))
	for _, post := range posts {
		bound, _ := cto51BindingState(binding, post)
		if bound || publisher.Cto51TitleMatches(article.Title, post.Title) {
			matches = append(matches, post)
		}
	}
	return article, root, account, matches, binding, nil
}

func (s *Server) handleCto51ArticleList(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	_, _, account, posts, binding, err := s.cto51Candidates(ctx, slug)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	candidates := make([]cto51CandidateView, 0, len(posts))
	for _, post := range posts {
		bound, state := cto51BindingState(binding, post)
		candidates = append(candidates, cto51CandidateView{
			ID: post.ID, Title: post.Title, URL: post.URL, Published: post.Published,
			Bound: bound, BindingState: state,
		})
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"account": account, "candidates": candidates, "bindings": cto51BindingViews(binding),
	})
}

func (s *Server) handleCto51BindingPut(response http.ResponseWriter, request *http.Request, slug string) {
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
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("51cto")
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "session_unavailable", err.Error(), nil)
		return
	}
	account, posts, err := publisher.Cto51ListPosts(ctx, client, session)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "51cto")
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_load_failed", err.Error(), nil)
		return
	}
	var selected *publisher.Cto51Post
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
		writeAPIError(response, http.StatusConflict, "candidate_missing", "51CTO list no longer contains the selected matching article", nil)
		return
	}

	existingID := binding.RemoteDraftID
	if body.State == "published" {
		existingID = binding.PublishedRemoteID
	}
	if existingID != "" && existingID != body.PostID && !body.Replace {
		writeAPIError(response, http.StatusConflict, "binding_exists", "replace the existing 51CTO binding explicitly", nil)
		return
	}

	binding.Slug = slug
	binding.Platform = "51cto"
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
	writeJSON(response, http.StatusOK, map[string]any{"binding": cto51BindingViews(binding)})
}

func (s *Server) handleCto51BindingDelete(response http.ResponseWriter, request *http.Request, slug string) {
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
	if err := publisher.DeletePublicationBindingState(s.publicationBindingsPath(), slug, "51cto", body.State, body.PostID); err != nil {
		writeAPIError(response, http.StatusConflict, "binding_changed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"removed": true})
}

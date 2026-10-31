package bridge

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

type toutiaoBindingView struct {
	PostID string `json:"postId"`
	State  string `json:"state"`
	URL    string `json:"url,omitempty"`
}

type toutiaoCandidateView struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Published    bool   `json:"published"`
	Bound        bool   `json:"bound"`
	BindingState string `json:"bindingState,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}

func toutiaoBindingViews(binding publisher.PublicationBinding) []toutiaoBindingView {
	views := []toutiaoBindingView{}
	if binding.RemoteDraftID != "" {
		views = append(views, toutiaoBindingView{PostID: binding.RemoteDraftID, State: "draft", URL: binding.DraftURL})
	}
	if binding.PublishedRemoteID != "" {
		views = append(views, toutiaoBindingView{PostID: binding.PublishedRemoteID, State: "published", URL: binding.PublishedURL})
	}
	return views
}

func toutiaoBindingState(binding publisher.PublicationBinding, post publisher.ToutiaoPost) (bool, string) {
	if post.Published && binding.PublishedRemoteID == post.ID {
		return true, "published"
	}
	if !post.Published && binding.RemoteDraftID == post.ID {
		return true, "draft"
	}
	return false, ""
}

func (s *Server) toutiaoCandidates(ctx context.Context, slug string) (string, []publisher.ToutiaoPost, publisher.PublicationBinding, error) {
	article, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		return "", nil, publisher.PublicationBinding{}, err
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("toutiao")
	if err != nil {
		return "", nil, publisher.PublicationBinding{}, err
	}
	account, posts, err := publisher.ToutiaoListPosts(ctx, client, session)
	if err != nil {
		return "", nil, publisher.PublicationBinding{}, err
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "toutiao")
	if err != nil {
		return "", nil, publisher.PublicationBinding{}, err
	}
	matches := make([]publisher.ToutiaoPost, 0, len(posts))
	for _, post := range posts {
		bound, _ := toutiaoBindingState(binding, post)
		if bound || publisher.ToutiaoTitleMatches(article.Title, post.Title) {
			matches = append(matches, post)
		}
	}
	return account, matches, binding, nil
}

func (s *Server) handleToutiaoArticleList(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	account, posts, binding, err := s.toutiaoCandidates(ctx, slug)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	candidates := make([]toutiaoCandidateView, 0, len(posts))
	for _, post := range posts {
		bound, state := toutiaoBindingState(binding, post)
		candidates = append(candidates, toutiaoCandidateView{
			ID: post.ID, Title: post.Title, URL: post.URL, Published: post.Published,
			Bound: bound, BindingState: state, UpdatedAt: post.ModifiedAt,
		})
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"account": account, "candidates": candidates, "bindings": toutiaoBindingViews(binding),
	})
}

func (s *Server) handleToutiaoBindingPut(response http.ResponseWriter, request *http.Request, slug string) {
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
	account, posts, binding, err := s.toutiaoCandidates(ctx, slug)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	var selected *publisher.ToutiaoPost
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
		writeAPIError(response, http.StatusConflict, "candidate_missing", "Toutiao account no longer contains the selected article", nil)
		return
	}
	existingID := binding.RemoteDraftID
	if body.State == "published" {
		existingID = binding.PublishedRemoteID
	}
	if existingID != "" && existingID != body.PostID && !body.Replace {
		writeAPIError(response, http.StatusConflict, "binding_exists", "replace the existing Toutiao binding explicitly", nil)
		return
	}
	binding.Slug = slug
	binding.Platform = "toutiao"
	binding.Account = account
	binding.Source = "manual"
	binding.VerifiedAt = time.Now().UTC().Format(time.RFC3339)
	if body.State == "published" {
		if binding.PublishedRemoteID != selected.ID ||
			(binding.RemoteUpdatedAt != "" && binding.RemoteUpdatedAt != selected.ModifiedAt) {
			// A previous submission may be pending review. Retain the local
			// submitted hash when only its revision baseline was invalidated.
			binding.PublishedHash = ""
		}
		binding.PublishedRemoteID = selected.ID
		binding.PublishedURL = selected.URL
		binding.RemoteUpdatedAt = selected.ModifiedAt
	} else {
		binding.RemoteDraftID = selected.ID
		binding.DraftURL = selected.URL
	}
	if err := publisher.SavePublicationBinding(s.publicationBindingsPath(), binding); err != nil {
		writeAPIError(response, http.StatusConflict, "binding_save_failed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"bindings": toutiaoBindingViews(binding)})
}

func (s *Server) handleToutiaoBindingDelete(response http.ResponseWriter, request *http.Request, slug string) {
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
	if _, _, err := s.cnBlogsArticle(slug); err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	if err := publisher.DeletePublicationBindingState(s.publicationBindingsPath(), slug, "toutiao", body.State, body.PostID); err != nil {
		writeAPIError(response, http.StatusConflict, "binding_changed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"removed": true})
}

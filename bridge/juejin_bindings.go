package bridge

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

func juejinDraftID(binding publisher.PublicationBinding) string {
	if id := strings.TrimSpace(binding.RemoteDraftID); id != "" {
		return id
	}
	// Fall back to the editor URL's trailing id when only a draft URL is stored.
	if raw := strings.TrimSpace(binding.DraftURL); raw != "" {
		if parsed, err := url.Parse(raw); err == nil {
			segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			if len(segments) > 0 {
				return strings.TrimSpace(segments[len(segments)-1])
			}
		}
	}
	return ""
}

type juejinBindingView struct {
	PostID string `json:"postId"`
	State  string `json:"state"`
	URL    string `json:"url,omitempty"`
}

type juejinCandidateView struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Published    bool   `json:"published"`
	Bound        bool   `json:"bound"`
	BindingState string `json:"bindingState,omitempty"`
}

func juejinBindingViews(binding publisher.PublicationBinding) []juejinBindingView {
	result := []juejinBindingView{}
	if binding.RemoteDraftID != "" {
		result = append(result, juejinBindingView{
			PostID: binding.RemoteDraftID, State: "draft", URL: binding.DraftURL,
		})
	}
	if binding.PublishedRemoteID != "" {
		result = append(result, juejinBindingView{
			PostID: binding.PublishedRemoteID, State: "published", URL: binding.PublishedURL,
		})
	}
	return result
}

func juejinBindingState(binding publisher.PublicationBinding, post publisher.JuejinPost) (bool, string) {
	if post.Published && binding.PublishedRemoteID == post.ID {
		return true, "published"
	}
	// Juejin exposes different draft_id and article_id values after publish.
	// Match the stored draft binding against the published record's draft_id so
	// the UI can surface the draft -> published transition.
	if post.Published && binding.RemoteDraftID != "" && binding.RemoteDraftID == post.DraftID {
		return true, "draft"
	}
	if !post.Published && binding.RemoteDraftID == post.ID {
		return true, "draft"
	}
	return false, ""
}

func (s *Server) juejinCandidates(ctx context.Context, slug string) (
	articleSummary, string, string, []publisher.JuejinPost, publisher.PublicationBinding, error,
) {
	article, root, err := s.cnBlogsArticle(slug)
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("juejin")
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	account, published, _, err := publisher.JuejinListAllPosts(ctx, client, session)
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "juejin")
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}

	// Discover the account's draft and published lists, then optionally verify
	// a legacy locally recorded draft by ID. Discovery no longer needs any binding.
	posts := make([]publisher.JuejinPost, 0, len(published)+1)
	posts = append(posts, published...)
	if localDraftID := juejinDraftID(binding); localDraftID != "" {
		if draft, lookupErr := publisher.JuejinLookupDraft(ctx, client, session, localDraftID); lookupErr == nil {
			posts = append(posts, draft)
		}
	}

	matches := make([]publisher.JuejinPost, 0, len(posts))
	seen := map[string]struct{}{}
	for _, post := range posts {
		bound, _ := juejinBindingState(binding, post)
		if !bound && !publisher.JuejinTitleMatches(article.Title, post.Title) {
			continue
		}
		if _, exists := seen[post.ID]; exists {
			continue
		}
		seen[post.ID] = struct{}{}
		matches = append(matches, post)
	}
	return article, root, account, matches, binding, nil
}

func (s *Server) handleJuejinArticleList(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	_, _, account, posts, binding, err := s.juejinCandidates(ctx, slug)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	candidates := make([]juejinCandidateView, 0, len(posts))
	for _, post := range posts {
		bound, state := juejinBindingState(binding, post)
		candidates = append(candidates, juejinCandidateView{
			ID: post.ID, Title: post.Title, URL: post.URL, Published: post.Published,
			Bound: bound, BindingState: state,
		})
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"account": account, "candidates": candidates, "bindings": juejinBindingViews(binding),
	})
}

func (s *Server) handleJuejinBindingPut(response http.ResponseWriter, request *http.Request, slug string) {
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
	_, _, account, posts, binding, err := s.juejinCandidates(ctx, slug)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	var selected *publisher.JuejinPost
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
	if selected == nil && body.State == "draft" {
		session, client, sessionErr := (bridgeNativePublisher{server: s}).publisherSession("juejin")
		if sessionErr == nil {
			if direct, lookupErr := publisher.JuejinLookupDraft(ctx, client, session, body.PostID); lookupErr == nil && !direct.Published {
				selected = &direct
			}
		}
	}
	if selected == nil {
		writeAPIError(response, http.StatusConflict, "candidate_missing", "Juejin could not verify the selected article id in the signed-in account", nil)
		return
	}

	existingID := binding.RemoteDraftID
	if body.State == "published" {
		existingID = binding.PublishedRemoteID
	}
	if existingID != "" && existingID != body.PostID && !body.Replace {
		writeAPIError(response, http.StatusConflict, "binding_exists", "replace the existing Juejin binding explicitly", nil)
		return
	}

	binding.Slug = slug
	binding.Platform = "juejin"
	binding.Account = account
	binding.Source = "manual"
	binding.VerifiedAt = time.Now().UTC().Format(time.RFC3339)
	if body.State == "published" {
		binding.PublishedRemoteID = selected.ID
		binding.PublishedURL = selected.URL
		if binding.RemoteDraftID == selected.DraftID || binding.RemoteDraftID == selected.ID {
			binding.RemoteDraftID = ""
			binding.DraftURL = ""
		}
	} else {
		binding.RemoteDraftID = selected.ID
		binding.DraftURL = selected.URL
	}
	if err := publisher.SavePublicationBinding(s.publicationBindingsPath(), binding); err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_save_failed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"binding": juejinBindingViews(binding)})
}

func (s *Server) handleJuejinBindingDelete(response http.ResponseWriter, request *http.Request, slug string) {
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
	if err := publisher.DeletePublicationBindingState(s.publicationBindingsPath(), slug, "juejin", body.State, body.PostID); err != nil {
		writeAPIError(response, http.StatusConflict, "binding_changed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"removed": true})
}

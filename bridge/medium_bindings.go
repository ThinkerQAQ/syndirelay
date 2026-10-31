package bridge

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	blogcompiler "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/compiler"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

type mediumBindingView struct {
	PostID string `json:"postId"`
	State  string `json:"state"`
	URL    string `json:"url,omitempty"`
}

type mediumCandidateView struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Published    bool   `json:"published"`
	Bound        bool   `json:"bound"`
	BindingState string `json:"bindingState,omitempty"`
}

func mediumBindingViews(binding publisher.PublicationBinding) []mediumBindingView {
	result := []mediumBindingView{}
	if binding.RemoteDraftID != "" {
		result = append(result, mediumBindingView{
			PostID: binding.RemoteDraftID, State: "draft", URL: binding.DraftURL,
		})
	}
	if binding.PublishedRemoteID != "" {
		result = append(result, mediumBindingView{
			PostID: binding.PublishedRemoteID, State: "published", URL: binding.PublishedURL,
		})
	}
	return result
}

func mediumBindingState(binding publisher.PublicationBinding, post mediumPost) (bool, string) {
	if post.Published && binding.PublishedRemoteID == post.ID {
		return true, "published"
	}
	if !post.Published && binding.RemoteDraftID == post.ID {
		return true, "draft"
	}
	return false, ""
}

func mediumLocalCanonicalURL(article articleSummary) string {
	return blogcompiler.CanonicalURL(strings.TrimSpace(article.Slug), article.Language)
}

func (s *Server) handleMediumLookupContext(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	article, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "medium")
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_load_failed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"title": article.Title, "canonicalUrl": mediumLocalCanonicalURL(article),
		"bindings": mediumBindingViews(binding),
	})
}

func (s *Server) mediumCandidates(ctx context.Context, slug string) (
	articleSummary, string, string, []mediumPost, publisher.PublicationBinding, error,
) {
	article, root, err := s.cnBlogsArticle(slug)
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("medium")
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	account, posts, err := (mediumClient{httpClient: client}).listPosts(ctx, mediumPlatformSession(session))
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	matches := make([]mediumPost, 0, len(posts))
	for _, post := range posts {
		if mediumTitleMatches(article.Title, post.Title) {
			matches = append(matches, post)
		}
	}
	// Medium headlines are often rewritten for the platform. If title matching
	// finds nothing, use the article's canonical footer link as the durable identity.
	if len(matches) == 0 {
		canonicalURL := mediumLocalCanonicalURL(article)
		mediumAPI := mediumClient{httpClient: client}
		mediumSession := mediumPlatformSession(session)
		for _, post := range posts {
			referencesCanonical, lookupErr := mediumAPI.postReferencesCanonical(ctx, mediumSession, post.ID, canonicalURL)
			if lookupErr == nil && referencesCanonical {
				matches = append(matches, post)
			}
		}
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "medium")
	if err != nil {
		return articleSummary{}, "", "", nil, publisher.PublicationBinding{}, err
	}
	return article, root, account, matches, binding, nil
}

func (s *Server) handleMediumArticleList(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()

	_, _, account, posts, binding, err := s.mediumCandidates(ctx, slug)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	candidates := make([]mediumCandidateView, 0, len(posts))
	for _, post := range posts {
		bound, state := mediumBindingState(binding, post)
		candidates = append(candidates, mediumCandidateView{
			ID: post.ID, Title: post.Title, URL: post.URL, Published: post.Published,
			Bound: bound, BindingState: state,
		})
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"account": account, "candidates": candidates, "bindings": mediumBindingViews(binding),
	})
}

func (s *Server) handleMediumBindingPut(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	s.distributionMu.Lock()
	defer s.distributionMu.Unlock()
	var body struct {
		PostID    string `json:"postId"`
		State     string `json:"state"`
		Replace   bool   `json:"replace"`
		Manual    bool   `json:"manual"`
		Candidate *struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			URL       string `json:"url"`
			Published bool   `json:"published"`
		} `json:"candidate"`
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

	article, _, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", "local article not found", nil)
		return
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "medium")
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_load_failed", err.Error(), nil)
		return
	}
	account := ""
	var selected *mediumPost
	if body.Candidate != nil && body.Candidate.ID == body.PostID &&
		body.Candidate.Published == (body.State == "published") &&
		(body.Manual || mediumTitleMatches(article.Title, body.Candidate.Title)) {
		if parsed, parseErr := url.Parse(body.Candidate.URL); parseErr == nil &&
			strings.HasSuffix(strings.ToLower(parsed.Host), "medium.com") && strings.Contains(parsed.Path, body.PostID) {
			selected = &mediumPost{ID: body.PostID, Title: body.Candidate.Title, URL: body.Candidate.URL, Published: body.Candidate.Published}
		}
	}
	if selected == nil {
		session, client, sessionErr := (bridgeNativePublisher{server: s}).publisherSession("medium")
		if sessionErr != nil {
			writeAPIError(response, http.StatusUnauthorized, "session_required", sessionErr.Error(), nil)
			return
		}
		lookupAccount, posts, lookupErr := (mediumClient{httpClient: client}).listPosts(request.Context(), mediumPlatformSession(session))
		if lookupErr != nil {
			writeAPIError(response, http.StatusBadGateway, "lookup_failed", lookupErr.Error(), nil)
			return
		}
		account = lookupAccount
		for _, post := range posts {
			if post.ID == body.PostID && post.Published == (body.State == "published") {
				candidate := post
				selected = &candidate
				break
			}
		}
		if selected == nil {
			writeAPIError(response, http.StatusConflict, "candidate_missing", "Medium candidate was not found in the current account", nil)
			return
		}
	}

	existingID := binding.RemoteDraftID
	if body.State == "published" {
		existingID = binding.PublishedRemoteID
	}
	if existingID != "" && existingID != body.PostID && !body.Replace {
		writeAPIError(response, http.StatusConflict, "binding_exists", "replace the existing Medium binding explicitly", nil)
		return
	}

	binding.Slug = slug
	binding.Platform = "medium"
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
	writeJSON(response, http.StatusOK, map[string]any{"binding": mediumBindingViews(binding)})
}

func (s *Server) handleMediumBindingDelete(response http.ResponseWriter, request *http.Request, slug string) {
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
	if err := publisher.DeletePublicationBindingState(s.publicationBindingsPath(), slug, "medium", body.State, body.PostID); err != nil {
		writeAPIError(response, http.StatusConflict, "binding_changed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"removed": true})
}

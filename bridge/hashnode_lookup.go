package bridge

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

type hashnodeCandidateView struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Published    bool   `json:"published"`
	Bound        bool   `json:"bound"`
	BindingState string `json:"bindingState,omitempty"`
}

func hashnodeEnglishTitle(contentRoot, slug string) (string, error) {
	if slug == "" || strings.Contains(slug, "\\") || filepath.IsAbs(slug) || strings.Contains(slug, "..") {
		return "", errors.New("invalid article slug")
	}
	path := filepath.Join(contentRoot, "src", "content", "articles", "en", filepath.FromSlash(slug)+".md")
	title := strings.TrimSpace(readFrontmatterScalar(path, "title"))
	if title == "" {
		return "", errors.New("English article title is missing")
	}
	return title, nil
}

func (s *Server) handleHashnodeArticleList(response http.ResponseWriter, request *http.Request, slug string) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	_, contentRoot, err := s.cnBlogsArticle(slug)
	if err != nil {
		writeAPIError(response, http.StatusNotFound, "article_not_found", err.Error(), nil)
		return
	}
	title, err := hashnodeEnglishTitle(contentRoot, slug)
	if err != nil {
		writeAPIError(response, http.StatusBadRequest, "english_article_missing", err.Error(), nil)
		return
	}
	session, client, err := (bridgeNativePublisher{server: s}).publisherSession("hashnode")
	if err != nil {
		writeAPIError(response, http.StatusBadRequest, "session_required", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 40*time.Second)
	defer cancel()
	account, posts, incomplete, err := publisher.HashnodeListPosts(ctx, client, session)
	if err != nil {
		writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
		return
	}
	// Fail closed: an incomplete inventory cannot establish that no remote
	// article exists. Do not enable the Create button on partial discovery.
	if incomplete {
		writeAPIError(response, http.StatusConflict, "lookup_incomplete", "Hashnode remote list is incomplete; refusing to infer there are no duplicates", nil)
		return
	}
	binding, _, err := publisher.LoadPublicationBinding(s.publicationBindingsPath(), slug, "hashnode")
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "binding_failed", err.Error(), nil)
		return
	}
	candidates := []hashnodeCandidateView{}
	for _, post := range posts {
		bound := (!post.Published && post.ID == binding.RemoteDraftID) ||
			(post.Published && post.ID == binding.PublishedRemoteID)
		if !bound && strings.Join(strings.Fields(title), " ") != strings.Join(strings.Fields(post.Title), " ") {
			continue
		}
		bindingState := ""
		if bound {
			if post.Published {
				bindingState = "published"
			} else {
				bindingState = "draft"
			}
		}
		candidates = append(candidates, hashnodeCandidateView{ID: post.ID, Title: post.Title, URL: post.URL,
			Published: post.Published, Bound: bound, BindingState: bindingState})
	}
	writeJSON(response, http.StatusOK, map[string]any{"account": account, "candidates": candidates})
}

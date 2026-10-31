package bridge

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/social"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/social/buffer"
)

type socialWrite struct {
	Revision string                 `json:"revision"`
	Platform string                 `json:"platform"`
	Posts    map[string]social.Post `json:"posts"`
}

func (s *Server) socialService() (social.Service, error) {
	root, err := ConfigDir()
	if err != nil {
		return social.Service{}, err
	}
	s.mu.Lock()
	key := strings.TrimSpace(s.config.Social.Buffer.APIKey)
	httpClient := s.httpClient
	s.mu.Unlock()
	if key == "" {
		key = os.Getenv("BUFFER_API_KEY")
	}
	return social.Service{DataRoot: root, Buffer: buffer.Client{APIKey: key, HTTPClient: httpClient}}, nil
}
func (s *Server) handleSocial(w http.ResponseWriter, r *http.Request, parts []string) {
	// Never expose unpublished drafts to unauthenticated HTTP clients.
	if r.Header.Get("x-thinkerqaq-token") != s.token {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "invalid bridge token", nil)
		return
	}
	svc, err := s.socialService()
	if err != nil {
		writeError(w, err)
		return
	}
	root := svc.DataRoot
	if len(parts) == 3 && parts[2] == "channels" && r.Method == http.MethodGet {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		channels, err := svc.Buffer.Channels(ctx)
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, "buffer", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "channels": channels})
		return
	}
	if len(parts) == 3 && parts[2] == "drafts" && r.Method == http.MethodGet {
		drafts, err := social.List(root)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "drafts": drafts})
		return
	}
	if len(parts) < 4 || parts[2] != "drafts" {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown social endpoint", nil)
		return
	}
	id := parts[3]
	draft, err := social.Load(root, id)
	if errors.Is(err, os.ErrNotExist) {
		writeAPIError(w, http.StatusNotFound, "not_found", "draft not found", nil)
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_draft", err.Error(), nil)
		return
	}
	if len(parts) == 4 && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "draft": draft})
		return
	}
	if len(parts) == 4 && r.Method == http.MethodPut {
		var body socialWrite
		if err := readJSON(r, 128*1024, &body); err != nil {
			writeError(w, err)
			return
		}
		if body.Revision != draft.Revision {
			writeAPIError(w, http.StatusConflict, "revision_conflict", "draft changed; reload before editing", nil)
			return
		}
		if len(draft.Remote) != 0 {
			writeAPIError(w, http.StatusConflict, "remote_exists", "remote draft already exists; editing refused", nil)
			return
		}
		draft.Posts = body.Posts
		draft.ApprovedRevision = ""
		draft.Revision = draft.Hash()
		if err := social.Save(root, draft); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_draft", err.Error(), nil)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "draft": draft})
		return
	}
	if len(parts) != 5 || r.Method != http.MethodPost {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown social action", nil)
		return
	}
	var body socialWrite
	if err := readJSON(r, 128*1024, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.Revision != draft.Revision {
		writeAPIError(w, http.StatusConflict, "revision_conflict", "draft changed; reload before continuing", nil)
		return
	}
	switch parts[4] {
	case "stage", "reconcile", "publish":
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if parts[4] == "stage" {
			draft, err = svc.Stage(ctx, id, body.Platform, body.Revision)
		}
		if parts[4] == "publish" {
			draft, err = svc.Publish(ctx, id, body.Platform, body.Revision)
		}
		if parts[4] == "reconcile" {
			draft, err = svc.Reconcile(ctx, id, body.Platform, body.Revision)
		}
	default:
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown social action", nil)
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusConflict, "social_action_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "draft": draft})
}

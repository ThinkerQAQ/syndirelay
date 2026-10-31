package bridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Browser transport only: the existing publishing job still owns execution.
// Claimed writes are never redelivered after an ambiguous browser disconnect.
type toutiaoBrowserRequest struct {
	ID      string `json:"id"`
	Body    string `json:"body"`
	claimed bool
	reply   chan toutiaoBrowserReply
}
type toutiaoBrowserReply struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
	Error  string `json:"error"`
	ID     string `json:"id"`
}
type toutiaoBrowserHub struct {
	mu      sync.Mutex
	pending map[string]*toutiaoBrowserRequest
}

func newToutiaoBrowserHub() *toutiaoBrowserHub {
	return &toutiaoBrowserHub{pending: map[string]*toutiaoBrowserRequest{}}
}
func (h *toutiaoBrowserHub) send(ctx context.Context, body string) (toutiaoBrowserReply, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return toutiaoBrowserReply{}, err
	}
	item := &toutiaoBrowserRequest{ID: hex.EncodeToString(nonce), Body: body, reply: make(chan toutiaoBrowserReply, 1)}
	h.mu.Lock()
	h.pending[item.ID] = item
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.pending, item.ID); h.mu.Unlock() }()
	wait, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	select {
	case r := <-item.reply:
		if r.Error != "" {
			return r, errors.New("Toutiao browser request failed; inspect the remote draft before retrying")
		}
		return r, nil
	case <-wait.Done():
		return toutiaoBrowserReply{}, errors.New("Toutiao browser request timed out; inspect remote state before retrying")
	}
}
func (h *toutiaoBrowserHub) next() *toutiaoBrowserRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, item := range h.pending {
		if !item.claimed {
			item.claimed = true
			return &toutiaoBrowserRequest{ID: item.ID, Body: item.Body}
		}
	}
	return nil
}
func (h *toutiaoBrowserHub) complete(r toutiaoBrowserReply) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	item := h.pending[r.ID]
	if item == nil || !item.claimed {
		return false
	}
	delete(h.pending, r.ID)
	item.reply <- r
	return true
}

type toutiaoBrowserTransport struct {
	base http.RoundTripper
	hub  *toutiaoBrowserHub
}

func (t toutiaoBrowserTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodPost || req.URL.Scheme != "https" || req.URL.Host != "mp.toutiao.com" || req.URL.Path != "/mp/agw/article/publish" {
		return t.base.RoundTrip(req)
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, (8<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 8<<20 {
		return nil, errors.New("Toutiao article request is too large")
	}
	r, err := t.hub.send(req.Context(), string(body))
	if err != nil {
		return nil, err
	}
	if r.Status < 100 || r.Status > 599 {
		return nil, errors.New("invalid Toutiao browser HTTP status")
	}
	return &http.Response{StatusCode: r.Status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(r.Body)), Request: req}, nil
}
func (s *Server) handleToutiaoBrowser(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("origin")
	if r.Header.Get("x-thinkerqaq-token") != s.token || (!validBrowserExtensionOrigin(origin) && !(r.Method == http.MethodGet && origin == "")) {
		writeAPIError(w, 403, "forbidden", "authorized Extension required", nil)
		return
	}
	if origin != "" {
		w.Header().Set("access-control-allow-origin", origin)
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v1/toutiao/browser/next" {
		writeJSON(w, 200, map[string]any{"request": s.toutiaoRequests.next()})
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/v1/toutiao/browser/result" {
		var body toutiaoBrowserReply
		if err := readJSON(r, 1<<20, &body); err != nil {
			writeError(w, err)
			return
		}
		if !s.toutiaoRequests.complete(body) {
			writeAPIError(w, 409, "not_pending", "request is no longer pending", nil)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	writeAPIError(w, 404, "not_found", "unknown browser transport operation", nil)
}

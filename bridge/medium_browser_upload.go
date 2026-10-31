package bridge

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

// In-memory, single-consumer Bridge-to-Extension handoff.
// Only the logged-in Medium tab performs the browser-native upload.
type mediumBrowserUploadHub struct {
	mu      sync.Mutex
	pending map[string]*mediumPendingUpload
	order   []string
}
type mediumPendingUpload struct {
	id, draftID, filename, mime string
	mode, marker                string
	canonicalURL                string
	expectedDocument            []map[string]any
	expectedImages              int
	payload                     []byte
	claimed                     bool
	claimedAt                   time.Time
	reply                       chan mediumBrowserUploadReply
}
type mediumBrowserUploadReply struct {
	fileID        string
	width, height int
	inserted      bool
	imageCount    int
	err           error
}

var mediumEditorPostID = regexp.MustCompile(`^[0-9a-f]{8,24}$`)

func newMediumBrowserUploadHub() *mediumBrowserUploadHub {
	return &mediumBrowserUploadHub{pending: make(map[string]*mediumPendingUpload)}
}
func (h *mediumBrowserUploadHub) upload(ctx context.Context, draftID string, image publisher.RehostImage) (mediumUploadResult, error) {
	if !mediumEditorPostID.MatchString(draftID) {
		return mediumUploadResult{}, errors.New("Medium browser upload requires a valid draft ID")
	}
	if len(image.Payload) == 0 || len(image.Payload) > 20<<20 {
		return mediumUploadResult{}, errors.New("Medium browser image size is invalid")
	}
	mime := mediumImageContentType(image.Payload, image.ContentType)
	if !strings.HasPrefix(mime, "image/") {
		return mediumUploadResult{}, errors.New("Medium browser upload requires image data")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return mediumUploadResult{}, err
	}
	item := &mediumPendingUpload{id: hex.EncodeToString(nonce), draftID: draftID, filename: mediumImageFilename(image.Source, mime), mime: mime, payload: image.Payload, reply: make(chan mediumBrowserUploadReply, 1)}
	h.mu.Lock()
	h.pending[item.id] = item
	h.order = append(h.order, item.id)
	h.mu.Unlock()
	slog.Info("medium browser upload queued", "operation", "medium-browser-upload", "draftId", draftID, "byteSize", len(image.Payload))
	defer func() { h.mu.Lock(); delete(h.pending, item.id); h.mu.Unlock() }()
	waitCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	select {
	case result := <-item.reply:
		if result.err != nil {
			return mediumUploadResult{}, result.err
		}
		return mediumUploadResult{FileID: result.fileID, Width: result.width, Height: result.height}, nil
	case <-waitCtx.Done():
		return mediumUploadResult{}, fmt.Errorf("Medium browser upload timed out or was cancelled: %w", waitCtx.Err())
	}
}
func (h *mediumBrowserUploadHub) next() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.order) > 4096 {
		alive := h.order[:0]
		for _, id := range h.order {
			if _, ok := h.pending[id]; ok {
				alive = append(alive, id)
			}
		}
		h.order = alive
	}
	for _, id := range h.order {
		item, ok := h.pending[id]
		if !ok || (item.claimed && time.Since(item.claimedAt) < 45*time.Second) {
			continue
		}
		item.claimed = true
		item.claimedAt = time.Now()
		slog.Info("medium browser upload claimed", "operation", "medium-browser-upload", "draftId", item.draftID)
		return map[string]any{"id": item.id, "draftId": item.draftID, "filename": item.filename, "mimeType": item.mime, "base64": base64.StdEncoding.EncodeToString(item.payload), "mode": item.mode, "marker": item.marker, "expectedImages": item.expectedImages, "canonicalUrl": item.canonicalURL, "expectedDocument": item.expectedDocument}
	}
	return nil
}
func (h *mediumBrowserUploadHub) complete(id, fileID string, width, height int, reportedError string) error {
	return h.completeTask(id, fileID, width, height, reportedError, false, 0)
}

func (h *mediumBrowserUploadHub) completeTask(id, fileID string, width, height int, reportedError string, inserted bool, imageCount int) error {
	h.mu.Lock()
	item := h.pending[id]
	if item == nil || !item.claimed {
		h.mu.Unlock()
		return errors.New("Medium browser upload is not pending")
	}
	delete(h.pending, id)
	h.mu.Unlock()
	slog.Info("medium browser upload result received", "operation", "medium-browser-upload", "draftId", item.draftID, "ok", reportedError == "")
	if reportedError != "" {
		item.reply <- mediumBrowserUploadReply{err: fmt.Errorf("Medium browser upload failed: %s", truncate(reportedError, 250))}
		return nil
	}
	if item.mode == "insert" || item.mode == "verify" || item.mode == "canonical" || item.mode == "prepare" || item.mode == "document" {
		if !inserted || (item.mode == "verify" && imageCount != item.expectedImages) {
			item.reply <- mediumBrowserUploadReply{err: errors.New("Medium browser did not confirm the saved image content")}
			return nil
		}
		item.reply <- mediumBrowserUploadReply{inserted: true, imageCount: imageCount}
		return nil
	}
	if !strings.HasPrefix(fileID, "1*") || len(fileID) > 256 || width < 1 || height < 1 || width > 30000 || height > 30000 {
		item.reply <- mediumBrowserUploadReply{err: errors.New("Medium browser returned an invalid native image reference")}
		return nil
	}
	item.reply <- mediumBrowserUploadReply{fileID: fileID, width: width, height: height}
	return nil
}
func (s *Server) handleMediumBrowserUpload(response http.ResponseWriter, request *http.Request) {
	origin := request.Header.Get("origin")
	// Browser extension GET fetches can omit Origin entirely, even when
	// the corresponding POST includes it. The Bridge Token is mandatory for
	// both. Only the read-only queue claim allows an absent Origin; never
	// allow a supplied untrusted Origin or weaken writes.
	isQueueRead := request.Method == http.MethodGet && request.URL.Path == "/v1/medium/browser-upload/next"
	originAllowed := validBrowserExtensionOrigin(origin) || (isQueueRead && origin == "")
	if request.Header.Get("x-thinkerqaq-token") != s.token || !originAllowed {
		writeAPIError(response, http.StatusForbidden, "forbidden", "Medium browser upload requires the authorized Extension", nil)
		return
	}
	if s.mediumUploads == nil {
		writeAPIError(response, http.StatusServiceUnavailable, "unavailable", "Medium upload hub unavailable", nil)
		return
	}
	switch {
	case request.URL.Path == "/v1/medium/browser-upload/next" && request.Method == http.MethodGet:
		writeJSON(response, http.StatusOK, map[string]any{"ok": true, "upload": s.mediumUploads.next()})
	case request.URL.Path == "/v1/medium/browser-upload/result" && request.Method == http.MethodPost:
		var body struct {
			ID         string `json:"id"`
			FileID     string `json:"fileId"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			Error      string `json:"error"`
			Inserted   bool   `json:"inserted"`
			ImageCount int    `json:"imageCount"`
		}
		if err := readJSON(request, 4096, &body); err != nil {
			writeError(response, err)
			return
		}
		if err := s.mediumUploads.completeTask(body.ID, body.FileID, body.Width, body.Height, body.Error, body.Inserted, body.ImageCount); err != nil {
			writeAPIError(response, http.StatusConflict, "not_pending", err.Error(), nil)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"ok": true})
	default:
		writeAPIError(response, http.StatusNotFound, "not_found", "Unknown browser upload endpoint", nil)
	}
}

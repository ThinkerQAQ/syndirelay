package bridge

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// These providers expose the local submission contract while their remote
// adapters are deferred. Never create a successful job or advance a snapshot.
func (s *Server) handleDeferredSearchSubmission(response http.ResponseWriter, request *http.Request, provider string) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	started := s.now().UTC()
	var input struct {
		Mode string `json:"mode"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeAPIError(response, http.StatusBadRequest, "invalid_search_submit", err.Error(), nil)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeAPIError(response, http.StatusBadRequest, "invalid_search_submit", "request must contain exactly one JSON object", nil)
		return
	}
	if input.Mode != "incremental" && input.Mode != "full" {
		writeAPIError(response, http.StatusBadRequest, "invalid_search_submit", "mode must be incremental or full", nil)
		return
	}
	s.searchMu.Lock()
	defer s.searchMu.Unlock()
	state := s.loadSearchState()
	operation := &state.So360
	if provider == "sogou" {
		operation = &state.Sogou
	}
	message := "远端提交尚未接入；本次未向搜索引擎发送 URL。"
	*operation = searchOperationState{
		State: "unsupported", Mode: input.Mode, StartedAt: started.Format(time.RFC3339),
		FinishedAt: s.now().UTC().Format(time.RFC3339), Error: message,
	}
	if err := saveSearchIndexState(state); err != nil {
		slog.Error("search submission state save failed", "operation", "search-submit", "provider", provider, "mode", input.Mode, "error", err)
		writeAPIError(response, http.StatusInternalServerError, "search_state_save_failed", err.Error(), nil)
		return
	}
	slog.Info("search submission unavailable", "operation", "search-submit", "provider", provider, "mode", input.Mode,
		"origin", state.Site.Origin, "result", "unsupported", "submitted", 0, "durationMs", s.now().Sub(started).Milliseconds())
	writeJSON(response, http.StatusOK, map[string]any{
		"ok": true, "index": state,
		"result": map[string]any{"provider": provider, "mode": input.Mode, "supported": false,
			"submitted": false, "urlCount": 0, "code": "submission_not_implemented", "message": message},
	})
}

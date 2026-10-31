package bridge

import (
	"net/http"
	"strings"

	blogplatform "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/platform"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

func (s *Server) handlePublications(response http.ResponseWriter) {
	s.distributionMu.Lock()
	records, listErr := publisher.ListPublicationRecords(s.publicationBindingsPath())
	s.distributionMu.Unlock()
	if listErr != nil {
		writeAPIError(response, http.StatusBadRequest, "invalid_request", listErr.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"records": records})
}

func (s *Server) handlePublicationPendingResolve(response http.ResponseWriter, request *http.Request) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	slug := strings.TrimSpace(request.URL.Query().Get("article"))
	platformID := strings.TrimSpace(strings.ToLower(request.URL.Query().Get("platform")))
	if slug == "" || !blogplatform.Supported(platformID) {
		writeAPIError(response, http.StatusBadRequest, "invalid_request", "article and supported platform are required", nil)
		return
	}
	var body struct {
		Fields []string `json:"fields"`
	}
	if err := readJSON(request, 8*1024, &body); err != nil {
		writeError(response, err)
		return
	}

	s.distributionMu.Lock()
	defer s.distributionMu.Unlock()
	remaining, err := publisher.ResolvePublicationPendingFields(s.publicationBindingsPath(), slug, platformID, body.Fields)
	if err != nil {
		writeAPIError(response, http.StatusConflict, "pending_fields_resolve_failed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"pendingFields": remaining})
}

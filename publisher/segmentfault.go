package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const segmentFaultOrigin = "https://segmentfault.com"

var (
	segmentFaultUserPattern  = regexp.MustCompile(`href=["']/u/([^"'/]+)`)
	segmentFaultTokenPattern = regexp.MustCompile(`(?i)(?:"Token"\s*:\s*"([^"]+)"|"key"\s*:\s*"([^"]+)")`)
)

type segmentFaultAdapter struct {
	client    *http.Client
	session   Session
	userAgent string
	token     string
	now       func() time.Time
}

func NewSegmentFaultAdapter(base *http.Client, session Session) (Adapter, error) {
	client, err := HTTPClientForSession(base, session)
	if err != nil {
		return nil, err
	}
	return &segmentFaultAdapter{client: client, session: session, userAgent: session.UserAgent, now: time.Now}, nil
}

func (s *segmentFaultAdapter) ID() string { return "segmentfault" }

func (s *segmentFaultAdapter) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *segmentFaultAdapter) request(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	return browserRequest(ctx, method, rawURL, segmentFaultOrigin, segmentFaultOrigin+"/", s.userAgent, body)
}

func (s *segmentFaultAdapter) CheckAuth(ctx context.Context) (AuthResult, error) {
	req, err := s.request(ctx, http.MethodGet, segmentFaultOrigin+"/user/settings", nil)
	if err != nil {
		return AuthResult{}, err
	}
	response, err := s.client.Do(req)
	if err != nil {
		return AuthResult{}, platformError(ErrUpstream, s.ID(), "auth", 0, err.Error(), true)
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return AuthResult{}, err
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return AuthResult{Authenticated: false}, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return AuthResult{}, classifyHTTP(s.ID(), "auth", response.StatusCode, string(raw))
	}
	match := segmentFaultUserPattern.FindStringSubmatch(string(raw))
	if len(match) != 2 {
		return AuthResult{Authenticated: false}, nil
	}
	return AuthResult{Authenticated: true, UserID: match[1], Username: match[1]}, nil
}

func (s *segmentFaultAdapter) sessionToken(ctx context.Context) (string, error) {
	if s.token != "" {
		return s.token, nil
	}
	req, err := s.request(ctx, http.MethodGet, segmentFaultOrigin+"/write", nil)
	if err != nil {
		return "", err
	}
	response, err := s.client.Do(req)
	if err != nil {
		return "", platformError(ErrUpstream, s.ID(), "session-token", 0, err.Error(), true)
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 4<<20)
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", classifyHTTP(s.ID(), "session-token", response.StatusCode, string(raw))
	}
	match := segmentFaultTokenPattern.FindStringSubmatch(string(raw))
	if len(match) < 2 {
		return "", platformError(ErrCSRF, s.ID(), "session-token", response.StatusCode, "editor session token was not found", false)
	}
	for _, candidate := range match[1:] {
		if strings.TrimSpace(candidate) != "" {
			s.token = strings.TrimSpace(candidate)
			return s.token, nil
		}
	}
	return "", platformError(ErrCSRF, s.ID(), "session-token", response.StatusCode, "editor session token was empty", false)
}

func parseSegmentFaultID(raw []byte) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"id", "draft_id"} {
			if id := valueString(typed[key]); id != "" {
				return id, nil
			}
		}
		if data, ok := typed["data"].(map[string]any); ok {
			if id := valueString(data["id"]); id != "" {
				return id, nil
			}
		}
		if message := responseMessage(typed["message"], typed["msg"], typed["error"]); message != "upstream rejected the request" {
			return "", fmt.Errorf("%s", message)
		}
	case []any:
		if len(typed) >= 2 && valueString(typed[0]) == "0" {
			if data, ok := typed[1].(map[string]any); ok {
				if id := valueString(data["id"]); id != "" {
					return id, nil
				}
			}
			if id := valueString(typed[1]); id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("response did not contain a draft id")
}

func (s *segmentFaultAdapter) uploadImage(ctx context.Context, image RehostImage, token string) (string, error) {
	body, bodyType, err := multipartBody(nil, "image", inferImageFilename(image.Source, image.ContentType), image.ContentType, image.Payload)
	if err != nil {
		return "", err
	}
	req, err := s.request(ctx, http.MethodPost, segmentFaultOrigin+"/gateway/image", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", bodyType)
	req.Header.Set("token", token)
	response, err := s.client.Do(req)
	if err != nil {
		return "", platformError(ErrUpload, s.ID(), "image-upload", 0, err.Error(), true)
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", classifyHTTP(s.ID(), "image-upload", response.StatusCode, string(raw))
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", platformError(ErrUpload, s.ID(), "image-upload", response.StatusCode, "invalid JSON response", false)
	}
	if object, ok := value.(map[string]any); ok {
		if target := valueString(object["result"]); target != "" {
			return target, nil
		}
		if target := valueString(object["url"]); target != "" {
			return target, nil
		}
	}
	if array, ok := value.([]any); ok && len(array) > 1 && valueString(array[0]) == "0" {
		if target := valueString(array[1]); target != "" {
			return target, nil
		}
	}
	return "", platformError(ErrUpload, s.ID(), "image-upload", response.StatusCode, "image URL missing", false)
}

func (s *segmentFaultAdapter) prepareMarkdown(ctx context.Context, input DraftInput, token string) (string, error) {
	return rehostMarkdownImages(ctx, s.client, input, ImageRehostOptions{
		Platform:       s.ID(),
		FailOpenRemote: true,
		AlreadyHosted: func(source string) bool {
			return strings.Contains(strings.ToLower(source), "segmentfault.com")
		},
	}, func(ctx context.Context, image RehostImage) (string, error) {
		return s.uploadImage(ctx, image, token)
	})
}

type segmentFaultTag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func normalizeSegmentFaultTagName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *segmentFaultAdapter) resolveTagIDs(ctx context.Context, token string, names []string, referer string) ([]int64, error) {
	started := time.Now()
	requested := make([]string, 0, len(names))
	seenNames := map[string]struct{}{}
	for _, name := range names {
		normalized := normalizeSegmentFaultTagName(name)
		if normalized == "" {
			continue
		}
		if _, exists := seenNames[normalized]; exists {
			continue
		}
		seenNames[normalized] = struct{}{}
		requested = append(requested, normalized)
	}
	if len(requested) == 0 {
		return nil, platformError(ErrValidation, s.ID(), "resolve-tags", 0, "SegmentFault requires at least one article tag", false)
	}

	signedURL, err := segmentFaultSignedURL(segmentFaultOrigin+"/gateway/tags", s.currentTime())
	if err != nil {
		return nil, platformError(ErrUpstream, s.ID(), "resolve-tags", 0, err.Error(), false)
	}
	req, err := s.request(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Del("origin")
	if strings.TrimSpace(referer) != "" {
		req.Header.Set("referer", referer)
	}
	req.Header.Set("accept", "application/json, text/plain, */*")
	req.Header.Set("token", token)
	req.Header.Set("authorization", "Bearer "+token)
	response, err := s.client.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "SegmentFault tag resolution failed",
			"node", "segmentfault-adapter", "operation", "resolve-tags", "result", "upstream-error",
			"requestedCount", len(requested), "durationMs", time.Since(started).Milliseconds(), "error", err)
		return nil, platformError(ErrUpstream, s.ID(), "resolve-tags", 0, err.Error(), true)
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, classifyHTTP(s.ID(), "resolve-tags", response.StatusCode, string(raw))
	}
	var payload struct {
		Rows map[string][]segmentFaultTag `json:"rows"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, platformError(ErrUpstream, s.ID(), "resolve-tags", response.StatusCode, "invalid JSON response", false)
	}
	available := map[string]int64{}
	for _, tags := range payload.Rows {
		for _, tag := range tags {
			name := normalizeSegmentFaultTagName(tag.Name)
			if name != "" && tag.ID != 0 {
				available[name] = tag.ID
			}
		}
	}
	resolved := make([]int64, 0, len(requested))
	unresolved := make([]string, 0)
	seenIDs := map[int64]struct{}{}
	for _, name := range requested {
		id, exists := available[name]
		if !exists {
			unresolved = append(unresolved, name)
			continue
		}
		if _, exists := seenIDs[id]; exists {
			continue
		}
		seenIDs[id] = struct{}{}
		resolved = append(resolved, id)
	}
	if len(resolved) == 0 {
		// SegmentFault requires catalog tag IDs. Prefer a real backend tag
		// returned by its own taxonomy, not a fabricated numeric ID.
		for _, fallback := range []string{"后端", "backend", "后端开发"} {
			if id, ok := available[fallback]; ok {
				resolved = append(resolved, id)
				slog.InfoContext(ctx, "SegmentFault using backend tag fallback",
					"operation", "resolve-tags", "fallback", fallback, "requestedCount", len(requested))
				break
			}
		}
		if len(resolved) == 0 {
			return nil, platformError(ErrValidation, s.ID(), "resolve-tags", response.StatusCode,
				"none of the article tags are supported and backend fallback is unavailable: "+strings.Join(unresolved, ", "), false)
		}
	}
	result := "success"
	if len(unresolved) > 0 {
		result = "partial"
	}
	slog.InfoContext(ctx, "SegmentFault tags resolved",
		"node", "segmentfault-adapter", "operation", "resolve-tags", "result", result,
		"requestedCount", len(requested), "resolvedCount", len(resolved), "unresolvedTags", unresolved,
		"durationMs", time.Since(started).Milliseconds())
	return resolved, nil
}

func (s *segmentFaultAdapter) saveDraft(ctx context.Context, refID string, input DraftInput) (DraftResult, error) {
	token, err := s.sessionToken(ctx)
	if err != nil {
		return DraftResult{}, err
	}
	referer := segmentFaultEditorURL(refID)
	tagIDs, err := s.resolveTagIDs(ctx, token, input.Tags, referer)
	if err != nil {
		return DraftResult{}, err
	}
	content, err := s.prepareMarkdown(ctx, input, token)
	if err != nil {
		return DraftResult{}, err
	}

	method := http.MethodPost
	rawURL := segmentFaultOrigin + "/gateway/draft"
	operation := "create-draft"
	payload := map[string]any{
		"title":     input.Title,
		"tags":      tagIDs,
		"text":      content,
		"object_id": "",
		"type":      "article",
		"language":  "",
		"cover":     "",
	}
	if refID != "" {
		method = http.MethodPut
		rawURL += "/" + url.PathEscape(refID)
		operation = "update-draft"
		if numericID, parseErr := strconv.ParseInt(refID, 10, 64); parseErr == nil {
			payload["id"] = numericID
		} else {
			payload["id"] = refID
		}
	}

	body, _ := json.Marshal(payload)
	req, err := s.request(ctx, method, rawURL, strings.NewReader(string(body)))
	if err != nil {
		return DraftResult{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json, text/plain, */*")
	req.Header.Set("token", token)
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("referer", referer)

	response, err := s.client.Do(req)
	if err != nil {
		return DraftResult{}, platformError(ErrUpstream, s.ID(), operation, 0, err.Error(), true)
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return DraftResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if refID != "" && (response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone) {
			return DraftResult{}, platformError(ErrRemoteDraftMissing, s.ID(), operation, response.StatusCode, string(raw), false)
		}
		return DraftResult{}, classifyHTTP(s.ID(), operation, response.StatusCode, string(raw))
	}

	id := refID
	if len(strings.TrimSpace(string(raw))) > 0 {
		if parsedID, parseErr := parseSegmentFaultID(raw); parseErr == nil && parsedID != "" {
			id = parsedID
		} else if refID == "" {
			return DraftResult{}, platformError(ErrUpstream, s.ID(), operation, response.StatusCode, parseErr.Error(), false)
		}
	}
	if id == "" {
		return DraftResult{}, platformError(ErrUpstream, s.ID(), operation, response.StatusCode, "response did not contain a draft id", false)
	}
	return DraftResult{
		ID:      id,
		URL:     segmentFaultOrigin + "/write?draftId=" + url.QueryEscape(id),
		Created: refID == "",
		Updated: refID != "",
	}, nil
}

func (s *segmentFaultAdapter) CreateDraft(ctx context.Context, input DraftInput) (DraftResult, error) {
	return s.saveDraft(ctx, "", input)
}

func (s *segmentFaultAdapter) UpdateDraft(ctx context.Context, ref DraftRef, input DraftInput) (DraftResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return DraftResult{}, platformError(ErrValidation, s.ID(), "update-draft", 0, "draft id is required", false)
	}
	return s.saveDraft(ctx, ref.ID, input)
}

func (s *segmentFaultAdapter) PublishDraft(ctx context.Context, ref DraftRef, input DraftInput) (PublishResult, error) {
	draftID := strings.TrimSpace(ref.ID)
	if draftID == "" {
		return PublishResult{}, platformError(ErrValidation, s.ID(), "publish-draft", 0, "draft id is required", false)
	}
	numericDraftID, err := strconv.ParseInt(draftID, 10, 64)
	if err != nil {
		return PublishResult{}, platformError(ErrValidation, s.ID(), "publish-draft", 0, "draft id must be numeric", false)
	}
	editor, err := s.loadEditorContext(ctx, draftID)
	if err != nil {
		return PublishResult{}, err
	}
	if strings.TrimSpace(editor.Draft.Title) == "" || strings.TrimSpace(editor.Draft.Text) == "" {
		return PublishResult{}, platformError(ErrUpstream, s.ID(), "publish-draft", 0, "remote draft content is missing", false)
	}

	tagIDs := make([]int64, 0, len(editor.Draft.Tags))
	seen := map[int64]struct{}{}
	for _, tag := range editor.Draft.Tags {
		if tag.ID == 0 {
			continue
		}
		if _, exists := seen[tag.ID]; exists {
			continue
		}
		seen[tag.ID] = struct{}{}
		tagIDs = append(tagIDs, tag.ID)
	}
	if len(tagIDs) == 0 {
		tagIDs, err = s.resolveTagIDs(ctx, editor.Token, input.Tags, segmentFaultEditorURL(draftID))
		if err != nil {
			return PublishResult{}, err
		}
	}

	payload := map[string]any{
		"tags":     tagIDs,
		"title":    editor.Draft.Title,
		"text":     editor.Draft.Text,
		"draft_id": numericDraftID,
		"blog_id":  selectSegmentFaultBlogID(editor.Blogs),
		"type":     1,
		"url":      "",
		"cover":    editor.Draft.Cover,
		"license":  nil,
		"log":      "",
	}
	body, _ := json.Marshal(payload)
	req, err := s.request(ctx, http.MethodPost, segmentFaultOrigin+"/gateway/article", strings.NewReader(string(body)))
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json, text/plain, */*")
	req.Header.Set("token", editor.Token)
	req.Header.Set("authorization", "Bearer "+editor.Token)
	req.Header.Set("referer", segmentFaultEditorURL(draftID))

	response, err := s.client.Do(req)
	if err != nil {
		return PublishResult{}, platformError(ErrUpstream, s.ID(), "publish-draft", 0, err.Error(), true)
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 4<<20)
	if err != nil {
		return PublishResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return PublishResult{}, classifyHTTP(s.ID(), "publish-draft", response.StatusCode, string(raw))
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return PublishResult{}, platformError(ErrUpstream, s.ID(), "publish-draft", response.StatusCode, "invalid JSON response", false)
	}
	data, _ := decoded["data"].(map[string]any)
	articleID := valueString(data["id"])
	if articleID == "" {
		return PublishResult{}, platformError(ErrUpstream, s.ID(), "publish-draft", response.StatusCode, "published article id is missing", false)
	}
	return PublishResult{
		ID:  articleID,
		URL: segmentFaultOrigin + "/a/" + url.PathEscape(articleID),
	}, nil
}

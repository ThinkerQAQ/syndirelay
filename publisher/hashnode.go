package publisher

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const hashnodeOrigin = "https://hashnode.com"

// hashnodeAdapter mirrors user-initiated browser editor draft operations.
// Unlike the official Pro API, these routes are undocumented and may change.
// No HAR session data is used: credentials are supplied by the live Extension.
type hashnodeAdapter struct {
	client        *http.Client
	uploadClient  *http.Client
	userAgent     string
	origin        string
	publicationID string
	username      string
}

func NewHashnodeAdapter(base *http.Client, session Session) (Adapter, error) {
	return newHashnodeAdapter(base, session, hashnodeOrigin)
}

func newHashnodeAdapter(base *http.Client, session Session, origin string) (*hashnodeAdapter, error) {
	client, err := HTTPClientForSession(base, session)
	if err != nil {
		return nil, err
	}
	if base == nil {
		base = http.DefaultClient
	}
	uploadClient := *base
	uploadClient.Jar = nil
	uploadClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &hashnodeAdapter{client: client, uploadClient: &uploadClient, userAgent: session.UserAgent, origin: strings.TrimRight(origin, "/")}, nil
}

func (h *hashnodeAdapter) ID() string { return "hashnode" }

func (h *hashnodeAdapter) request(ctx context.Context, method, path string, data any, output any, operation string) error {
	var body io.Reader
	if data != nil {
		var err error
		body, err = encodeJSONBody(data)
		if err != nil {
			return err
		}
	}
	req, err := browserRequest(ctx, method, h.origin+path, h.origin, h.origin+"/", h.userAgent, body)
	if err != nil {
		return err
	}
	req.Header.Set("accept", "application/json")
	if data != nil {
		req.Header.Set("content-type", "application/json")
	}
	if err := doJSON(h.client, req, h.ID(), operation, output); err != nil {
		return err
	}
	return nil
}

func (h *hashnodeAdapter) CheckAuth(ctx context.Context) (AuthResult, error) {
	var response struct {
		Success      bool `json:"success"`
		Publications []struct {
			ID       string `json:"_id"`
			Username string `json:"username"`
		} `json:"publications"`
	}
	if err := h.request(ctx, http.MethodGet, "/api/publications", nil, &response, "auth"); err != nil {
		if IsKind(err, ErrAuthExpired) {
			return AuthResult{Authenticated: false}, nil
		}
		return AuthResult{}, err
	}
	if !response.Success || len(response.Publications) == 0 {
		return AuthResult{Authenticated: false}, nil
	}
	selected := strings.TrimSpace(os.Getenv("HASHNODE_PUBLICATION_ID"))
	if selected == "" && len(response.Publications) == 1 {
		selected = response.Publications[0].ID
	}
	if selected == "" {
		return AuthResult{}, errors.New("multiple Hashnode publications: set HASHNODE_PUBLICATION_ID to select one")
	}
	for _, publication := range response.Publications {
		if publication.ID == selected {
			h.publicationID = selected
			h.username = publication.Username
			return AuthResult{Authenticated: true, UserID: selected}, nil
		}
	}
	return AuthResult{}, errors.New("configured HASHNODE_PUBLICATION_ID is not in the signed-in account")
}

func (h *hashnodeAdapter) ensurePublication() error {
	if h.publicationID == "" {
		return errors.New("Hashnode publication is not authenticated")
	}
	return nil
}

func (h *hashnodeAdapter) CreateDraft(ctx context.Context, _ DraftInput) (DraftResult, error) {
	if err := h.ensurePublication(); err != nil {
		return DraftResult{}, err
	}
	var response struct {
		Success bool   `json:"success"`
		DraftID string `json:"draftId"`
	}
	// Creating the empty draft is the ONLY side effect at this stage.
	// The service persists this ID before uploading assets or editing content.
	if err := h.request(ctx, http.MethodPost, "/api/drafts", map[string]any{
		"publicationId": h.publicationID,
	}, &response, "create-draft"); err != nil {
		return DraftResult{}, err
	}
	if !response.Success || response.DraftID == "" {
		return DraftResult{}, errors.New("Hashnode did not return a draft ID; inspect the remote drafts before retrying")
	}
	return DraftResult{ID: response.DraftID, URL: h.origin + "/draft/" + url.PathEscape(response.DraftID), Created: true}, nil
}

func newHashnodeEditID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	s := hex.EncodeToString(raw)
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}

func (h *hashnodeAdapter) UpdateDraft(ctx context.Context, ref DraftRef, input DraftInput) (DraftResult, error) {
	if ref.ID == "" {
		return DraftResult{}, errors.New("Hashnode draft ID is missing")
	}
	path := "/api/drafts/" + url.PathEscape(ref.ID)
	var read struct {
		Success bool `json:"success"`
		Draft   struct {
			ID          string `json:"_id"`
			Publication string `json:"publication"`
			DateUpdated int64  `json:"dateUpdated"`
		} `json:"draft"`
	}
	if err := h.request(ctx, http.MethodGet, path, nil, &read, "read-draft"); err != nil {
		return DraftResult{}, err
	}
	if !read.Success || read.Draft.ID != ref.ID || read.Draft.Publication != h.publicationID {
		return DraftResult{}, errors.New("Hashnode draft not found in the selected publication")
	}
	var lock struct {
		Success bool `json:"success"`
		Locked  bool `json:"locked"`
	}
	if err := h.request(ctx, http.MethodPost, path+"/lock", nil, &lock, "lock-draft"); err != nil {
		return DraftResult{}, err
	}
	if !lock.Success || !lock.Locked {
		return DraftResult{}, errors.New("Hashnode draft is locked by another editor")
	}
	defer func() {
		// Best effort, but bounded; never retry ambiguous writes on unlock failures.
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = h.request(unlockCtx, http.MethodDelete, path+"/lock", nil, nil, "unlock-draft")
	}()
	sessionID, err := newHashnodeEditID()
	if err != nil {
		return DraftResult{}, err
	}
	save := func(fields map[string]any) error {
		fields["baseDateUpdated"] = read.Draft.DateUpdated
		fields["editSessionId"] = sessionID
		var updated struct {
			Success     bool   `json:"success"`
			DraftID     string `json:"draftId"`
			DateUpdated int64  `json:"dateUpdated"`
		}
		if err := h.request(ctx, http.MethodPut, path, fields, &updated, "update-draft"); err != nil {
			return err
		}
		if !updated.Success || updated.DraftID != ref.ID || updated.DateUpdated == 0 {
			return errors.New("Hashnode draft update did not confirm the new revision")
		}
		read.Draft.DateUpdated = updated.DateUpdated
		return nil
	}
	if err := save(map[string]any{"title": input.Title}); err != nil {
		return DraftResult{}, err
	}
	markdown, err := rehostMarkdownImages(ctx, h.client, input, ImageRehostOptions{
		Platform: "hashnode", SourceDir: input.SourceDir, FailOpenRemote: true,
		AlreadyHosted: func(source string) bool {
			u, err := url.Parse(source)
			return err == nil && u.Scheme == "https" && strings.EqualFold(u.Hostname(), "cdn.hashnode.com")
		},
	}, h.uploadImage)
	if err != nil {
		return DraftResult{}, err
	}
	if err := save(map[string]any{"contentMarkdown": markdown}); err != nil {
		return DraftResult{}, err
	}
	if canonical := strings.TrimSpace(input.NativeCanonicalURL); canonical != "" {
		if err := save(map[string]any{"originalArticleURL": canonical, "isRepublished": true}); err != nil {
			return DraftResult{}, err
		}
	}
	// Cover image uses the same first-party host before any R2 fallback.
	if cover := strings.TrimSpace(input.CoverImageURL); cover != "" {
		data, mimeType, err := loadRehostImage(h.client, input, cover, input.SourceDir)
		if err != nil {
			return DraftResult{}, err
		}
		img := RehostImage{Source: cover, Payload: data, ContentType: mimeType}
		uploaded, err := h.uploadImage(ctx, img)
		if err != nil {
			uploaded, err = tryR2Fallback(ctx, h.client, input, img, err)
			if err != nil {
				return DraftResult{}, err
			}
		}
		if err := save(map[string]any{"coverImage": uploaded}); err != nil {
			return DraftResult{}, err
		}
	}
	return DraftResult{ID: ref.ID, URL: h.origin + "/draft/" + url.PathEscape(ref.ID), Updated: true}, nil
}

func (h *hashnodeAdapter) uploadImage(ctx context.Context, image RehostImage) (string, error) {
	if len(image.Payload) == 0 {
		return "", errors.New("empty Hashnode image")
	}
	contentType := strings.TrimSpace(image.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(image.Payload)
	}
	var authorization struct {
		Success      bool   `json:"success"`
		PresignedURL string `json:"presignedUrl"`
		CDNURL       string `json:"cdnUrl"`
	}
	if err := h.request(ctx, http.MethodPost, "/api/upload/image", map[string]string{
		"contentType": contentType, "type": "cover", "mode": "presigned",
	}, &authorization, "image-presign"); err != nil {
		return "", err
	}
	if !authorization.Success {
		return "", errors.New("Hashnode image upload authorization failed")
	}
	target, err := url.Parse(authorization.PresignedURL)
	if err != nil {
		return "", errors.New("invalid Hashnode presigned upload address")
	}
	if target.Scheme != "https" || !strings.HasSuffix(strings.ToLower(target.Hostname()), ".r2.cloudflarestorage.com") {
		return "", errors.New("unexpected Hashnode upload host")
	}
	cdn, err := url.Parse(authorization.CDNURL)
	if err != nil || cdn.Scheme != "https" || !strings.EqualFold(cdn.Hostname(), "cdn.hashnode.com") {
		return "", errors.New("unexpected Hashnode CDN host")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target.String(), bytes.NewReader(image.Payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", contentType)
	// Use the base HTTP client: never forward Hashnode cookies to R2.
	res, err := h.uploadClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Hashnode image upload failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("Hashnode image upload returned HTTP %d", res.StatusCode)
	}
	return cdn.String(), nil
}

func (h *hashnodeAdapter) PublishDraft(context.Context, DraftRef, DraftInput) (PublishResult, error) {
	return PublishResult{}, platformError(ErrNotImplemented, h.ID(), "publish-draft", 0,
		"Hashnode publishing is not enabled; please publish the draft in the browser", false)
}

var _ Adapter = (*hashnodeAdapter)(nil)

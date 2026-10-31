package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	htmlstd "html"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

const mediumOrigin = "https://medium.com"

var mediumCookieNames = map[string]struct{}{
	"sid": {}, "uid": {}, "xsrf": {}, "cf_clearance": {},
}

var (
	mediumAnchorPattern    = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	mediumHTMLTagPattern   = regexp.MustCompile(`(?s)<[^>]+>`)
	mediumDraftPathPattern = regexp.MustCompile(`^/p/([0-9a-f]{8,})/edit$`)
	mediumPostIDPattern    = regexp.MustCompile(`-([0-9a-f]{8,})$`)
	mediumProfilePattern   = regexp.MustCompile(`(?i)(?:https://medium\.com)?/@([A-Za-z0-9_-]+)`)
)

type mediumClient struct {
	httpClient            *http.Client
	browserUpload         func(context.Context, string, publisher.RehostImage) (mediumUploadResult, error)
	browserInsert         func(context.Context, string, string, publisher.RehostImage) error
	browserVerify         func(context.Context, string, int) error
	browserParagraphCount func(context.Context, string) (int, error)
	browserDocument       func(context.Context, string, mediumDraft) error
	// Report the story ID immediately; image preparation can take several minutes.
	onStoryCreated    func(string)
	verifyAfterCreate bool
}

type mediumCoverImage struct {
	URL string `json:"url"`
	Alt string `json:"alt"`
}

type mediumBrowserDraftImage struct {
	Marker string `json:"marker"`
	URL    string `json:"url"`
	Alt    string `json:"alt"`
}

type mediumBrowserDraftPlan struct {
	Deltas []map[string]any          `json:"deltas"`
	Images []mediumBrowserDraftImage `json:"images"`
}

type mediumDraft struct {
	Title        string                  `json:"title"`
	Deltas       []map[string]any        `json:"deltas"`
	BrowserPlan  *mediumBrowserDraftPlan `json:"browserPlan,omitempty"`
	CanonicalURL string                  `json:"canonicalUrl"`
	Tags         []string                `json:"tags"`
	CoverImage   *mediumCoverImage       `json:"coverImage,omitempty"`
}

type mediumPost struct {
	ID        string
	Title     string
	URL       string
	Published bool
}

type mediumPostMeta struct {
	ID               string
	LatestRev        int
	FirstPublishedAt int64
	UniqueSlug       string
	MediumURL        string
	Username         string
}

type mediumPostPresentation struct {
	Title    string
	Subtitle string
}

type mediumUploadResult struct {
	FileID string
	Width  int
	Height int
}

func filterMediumCookies(cookies []browserCookie) map[string]string {
	out := make(map[string]string)
	for _, cookie := range cookies {
		if _, ok := mediumCookieNames[cookie.Name]; ok && cookie.Value != "" {
			out[cookie.Name] = cookie.Value
		}
	}
	return out
}

func stripMediumXSSI(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "])}") && !strings.HasPrefix(trimmed, ")]}") {
		return trimmed
	}
	if i := strings.IndexByte(trimmed, '\n'); i >= 0 {
		return strings.TrimSpace(trimmed[i+1:])
	}
	for _, opener := range []byte{'{', '['} {
		if i := strings.IndexByte(trimmed, opener); i >= 0 {
			return strings.TrimSpace(trimmed[i:])
		}
	}
	return ""
}

func cookieHeader(cookies map[string]string) string {
	order := []string{"sid", "uid", "rid", "xsrf", "cf_clearance", "_cfuvid"}
	parts := make([]string, 0, len(cookies))
	for _, name := range order {
		if value := cookies[name]; value != "" {
			parts = append(parts, name+"="+value)
		}
	}
	return strings.Join(parts, "; ")
}

func mediumRequestCookieValue(header, name string) string {
	for _, pair := range strings.Split(header, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && key == name {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mediumSessionCookieHeader(session platformSession) string {
	values := make(map[string]string)
	order := make([]string, 0)
	for _, pair := range strings.Split(session.RequestCookieHeader, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		if _, exists := values[name]; !exists {
			order = append(order, name)
		}
		values[name] = strings.TrimSpace(value)
	}

	missingNames := make([]string, 0, len(session.Cookies))
	for name := range session.Cookies {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if _, exists := values[name]; !exists {
			missingNames = append(missingNames, name)
		}
	}
	sort.Strings(missingNames)
	for _, name := range missingNames {
		values[name] = session.Cookies[name]
		order = append(order, name)
	}

	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, name+"="+values[name])
	}
	return strings.Join(parts, "; ")
}

func mediumSessionXSRF(session platformSession) string {
	if value := strings.TrimSpace(session.Cookies["xsrf"]); value != "" {
		return value
	}
	return mediumRequestCookieValue(session.RequestCookieHeader, "xsrf")
}

func (c mediumClient) primeXSRF(ctx context.Context, session platformSession) platformSession {
	if xsrf := mediumSessionXSRF(session); xsrf != "" {
		if session.Cookies == nil {
			session.Cookies = map[string]string{}
		}
		session.Cookies["xsrf"] = xsrf
		return session
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediumOrigin+"/", nil)
	if err != nil {
		return session
	}
	setMediumHeaders(req, session, mediumOrigin+"/")
	req.Header.Del("content-type")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return session
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 256<<10))
	for _, cookie := range response.Cookies() {
		if cookie.Name == "xsrf" && cookie.Value != "" {
			if session.Cookies == nil {
				session.Cookies = map[string]string{}
			}
			session.Cookies["xsrf"] = cookie.Value
			break
		}
	}
	return session
}

func decodeMediumResponse(response *http.Response, output any) error {
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Upstream HTML errors (often a 504 proxy/CDN page) are not an API
		// response. Do not leak the large HTML document into task events.
		trimmed := strings.TrimSpace(string(raw))
		if strings.Contains(strings.ToLower(response.Header.Get("content-type")), "text/html") ||
			strings.HasPrefix(strings.ToLower(trimmed), "<!doctype html") ||
			strings.HasPrefix(strings.ToLower(trimmed), "<html") {
			return fmt.Errorf("Medium request failed (%d): upstream returned an HTML error page", response.StatusCode)
		}
		return fmt.Errorf("Medium request failed (%d): %s", response.StatusCode, truncate(stripMediumXSSI(string(raw)), 500))
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal([]byte(stripMediumXSSI(string(raw))), output); err != nil {
		return fmt.Errorf("Medium returned invalid JSON: %w", err)
	}
	return nil
}

func (c mediumClient) createStory(ctx context.Context, session platformSession) (string, string, error) {
	session = c.primeXSRF(ctx, session)
	body, err := json.Marshal(map[string]any{
		"deltas":     []any{},
		"baseRev":    -1,
		"coverless":  true,
		"visibility": 0,
	})
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediumOrigin+"/new-story", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	setMediumHeaders(req, session, mediumOrigin+"/new-story")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	var decoded struct {
		Success bool `json:"success"`
		Payload struct {
			Value struct {
				ID        string `json:"id"`
				MediumURL string `json:"mediumUrl"`
			} `json:"value"`
		} `json:"payload"`
		Error string `json:"error"`
	}
	if err := decodeMediumResponse(response, &decoded); err != nil {
		return "", "", err
	}
	if !decoded.Success || strings.TrimSpace(decoded.Payload.Value.ID) == "" {
		return "", "", fmt.Errorf("Medium new-story did not return a post id: %s", truncate(decoded.Error, 300))
	}
	return decoded.Payload.Value.ID, decoded.Payload.Value.MediumURL, nil
}

// Image format conversion is only useful when Medium rejects a file format.
// A gateway failure indicates an unavailable endpoint, not a PNG incompatibility.
type mediumUploadHTTPError struct {
	status int
	err    error
}

func (e *mediumUploadHTTPError) Error() string { return e.err.Error() }
func (e *mediumUploadHTTPError) Unwrap() error { return e.err }

func mediumUploadGatewayFailure(err error) bool {
	var status *mediumUploadHTTPError
	return errors.As(err, &status) && (status.status >= 500 || status.status == http.StatusTooManyRequests)
}

func (c mediumClient) uploadImage(ctx context.Context, session platformSession, draftID string, image publisher.RehostImage) (mediumUploadResult, error) {
	if c.browserUpload != nil {
		return c.browserUpload(ctx, draftID, image)
	}
	result, err := c.uploadImageOnce(ctx, session, draftID, image)
	if err == nil || mediumImageContentType(image.Payload, image.ContentType) != "image/png" {
		return result, err
	}
	var status *mediumUploadHTTPError
	if errors.As(err, &status) && status.status != http.StatusBadRequest && status.status != http.StatusUnsupportedMediaType {
		// Never retry gateway/auth/rate-limit failures as JPEG.
		return mediumUploadResult{}, err
	}
	retryImage, conversionErr := mediumPNGAsJPEG(image)
	if conversionErr != nil {
		return mediumUploadResult{}, err
	}
	slog.Warn("medium PNG upload failed; retrying as JPEG", "operation", "image-upload-retry", "originalByteSize", len(image.Payload), "retryByteSize", len(retryImage.Payload), "errorType", fmt.Sprintf("%T", err))
	result, retryErr := c.uploadImageOnce(ctx, session, draftID, retryImage)
	if retryErr != nil {
		return mediumUploadResult{}, fmt.Errorf("Medium PNG upload failed: %v; JPEG retry failed: %w", err, retryErr)
	}
	return result, nil
}

func (c mediumClient) uploadImageOnce(ctx context.Context, session platformSession, draftID string, image publisher.RehostImage) (mediumUploadResult, error) {
	started := time.Now()
	session = c.primeXSRF(ctx, session)
	contentType := mediumImageContentType(image.Payload, image.ContentType)
	filename := mediumImageFilename(image.Source, contentType)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name": "uploadedFile", "filename": filename,
	}))
	partHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return mediumUploadResult{}, err
	}
	if _, err := part.Write(image.Payload); err != nil {
		return mediumUploadResult{}, err
	}
	if err := writer.Close(); err != nil {
		return mediumUploadResult{}, err
	}

	// Medium's editor sends ?source=6 for image uploads (verified from a
	// successful browser HAR). The file remains multipart uploadedFile.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediumOrigin+"/_/upload?source=6", &body)
	if err != nil {
		return mediumUploadResult{}, err
	}
	referer := mediumOrigin + "/new-story"
	if draftID != "" {
		referer = mediumOrigin + "/p/" + url.PathEscape(draftID) + "/edit"
	}
	setMediumHeaders(req, session, referer)
	// These are upload-specific browser headers, not the JSON API headers.
	req.Header.Set("accept", "*/*")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("pragma", "no-cache")
	req.Header.Set("dnt", "1")
	req.Header.Del("x-requested-with")
	req.Header.Del("x-client-date")
	// Upload requests are browser XHRs, not Medium JSON API calls.
	req.Header.Set("content-type", writer.FormDataContentType())
	requestCookieHeader := req.Header.Get("cookie")
	slog.Info("medium image upload started", "operation", "image-upload", "contentType", contentType, "byteSize", len(image.Payload), "requestCookieNames", cookieHeaderNames(requestCookieHeader), "hasClearance", cookieHeaderHasName(requestCookieHeader, "cf_clearance"))
	response, err := c.httpClient.Do(req)
	if err != nil {
		slog.Warn("medium image upload request failed", "operation", "image-upload", "contentType", contentType, "byteSize", len(image.Payload), "durationMs", time.Since(started).Milliseconds(), "errorType", fmt.Sprintf("%T", err))
		return mediumUploadResult{}, err
	}
	slog.Info("medium image upload response", "operation", "image-upload", "status", response.StatusCode, "contentType", contentType, "byteSize", len(image.Payload), "durationMs", time.Since(started).Milliseconds())
	var decoded struct {
		Success bool `json:"success"`
		Payload struct {
			Value struct {
				FileID    string `json:"fileId"`
				ImgWidth  int    `json:"imgWidth"`
				ImgHeight int    `json:"imgHeight"`
			} `json:"value"`
		} `json:"payload"`
	}
	if err := decodeMediumResponse(response, &decoded); err != nil {
		slog.Warn("medium image upload rejected", "operation", "image-upload", "status", response.StatusCode, "contentType", contentType, "byteSize", len(image.Payload), "durationMs", time.Since(started).Milliseconds(), "errorType", fmt.Sprintf("%T", err))
		return mediumUploadResult{}, &mediumUploadHTTPError{status: response.StatusCode, err: err}
	}
	if !decoded.Success || strings.TrimSpace(decoded.Payload.Value.FileID) == "" {
		return mediumUploadResult{}, errors.New("Medium image upload did not return a file id")
	}
	return mediumUploadResult{
		FileID: decoded.Payload.Value.FileID,
		Width:  decoded.Payload.Value.ImgWidth,
		Height: decoded.Payload.Value.ImgHeight,
	}, nil
}

func mediumPNGAsJPEG(imageValue publisher.RehostImage) (publisher.RehostImage, error) {
	decoded, err := png.Decode(bytes.NewReader(imageValue.Payload))
	if err != nil {
		return publisher.RehostImage{}, err
	}
	bounds := decoded.Bounds()
	flattened := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(flattened, flattened.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(flattened, flattened.Bounds(), decoded, bounds.Min, draw.Over)
	var payload bytes.Buffer
	if err := jpeg.Encode(&payload, flattened, &jpeg.Options{Quality: 90}); err != nil {
		return publisher.RehostImage{}, err
	}
	return publisher.RehostImage{
		Source: imageValue.Source, Payload: payload.Bytes(), ContentType: "image/jpeg",
	}, nil
}

func mediumImageContentType(payload []byte, declared string) string {
	detected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(payload), ";")[0]))
	if strings.HasPrefix(detected, "image/") {
		return detected
	}
	declared = strings.ToLower(strings.TrimSpace(strings.Split(declared, ";")[0]))
	if strings.HasPrefix(declared, "image/") {
		return declared
	}
	return "application/octet-stream"
}

func mediumImageFilename(source, contentType string) string {
	extension := ".png"
	switch strings.ToLower(strings.Split(contentType, ";")[0]) {
	case "image/jpeg":
		extension = ".jpg"
	case "image/gif":
		extension = ".gif"
	case "image/webp":
		extension = ".webp"
	case "image/svg+xml":
		extension = ".svg"
	}
	if parsed, err := url.Parse(source); err == nil {
		if name := filepath.Base(parsed.Path); name != "" && name != "." && name != "/" {
			if strings.EqualFold(filepath.Ext(name), extension) ||
				(extension == ".jpg" && strings.EqualFold(filepath.Ext(name), ".jpeg")) {
				return name
			}
		}
	}
	return "image" + extension
}

func mediumPublishingAsset(input publisher.DraftInput, source string) (publisher.PublishingAsset, bool) {
	for _, asset := range input.Assets {
		if asset.Source == source || asset.PublicURL == source {
			return asset, true
		}
	}
	return publisher.PublishingAsset{}, false
}

func (c mediumClient) loadImage(ctx context.Context, input publisher.DraftInput, source string) (publisher.RehostImage, error) {
	if asset, ok := mediumPublishingAsset(input, source); ok && strings.HasPrefix(source, "blogctl-asset://") {
		if strings.TrimSpace(input.DistributionRoot) == "" {
			return publisher.RehostImage{}, fmt.Errorf("distribution root is missing for generated asset %s", asset.ID)
		}
		path := filepath.Join(input.DistributionRoot, "assets", asset.Kind, asset.ID+".png")
		payload, err := os.ReadFile(path)
		if err != nil {
			return publisher.RehostImage{}, err
		}
		return publisher.RehostImage{Source: source, Payload: payload, ContentType: "image/png"}, nil
	}
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return publisher.RehostImage{}, fmt.Errorf("unsupported Medium image source: %s", source)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return publisher.RehostImage{}, err
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		return publisher.RehostImage{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return publisher.RehostImage{}, fmt.Errorf("Medium image source returned HTTP %d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, 20<<20))
	if err != nil {
		return publisher.RehostImage{}, err
	}
	return publisher.RehostImage{
		Source: source, Payload: payload,
		ContentType: response.Header.Get("content-type"),
	}, nil
}

func cloneMediumDelta(delta map[string]any) map[string]any {
	raw, _ := json.Marshal(delta)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	return result
}

func (c mediumClient) prepareDraftDeltas(
	ctx context.Context,
	session platformSession,
	draft mediumDraft,
	input publisher.DraftInput,
	draftID string,
) ([]map[string]any, int, error) {
	deltas := make([]map[string]any, 0, len(draft.Deltas)+1)
	deltas = append(deltas, map[string]any{
		"type":      1,
		"index":     0,
		"paragraph": map[string]any{"type": 3, "text": draft.Title, "markups": []any{}},
	})
	fallbacks := 0
	for _, original := range draft.Deltas {
		delta := cloneMediumDelta(original)
		delta["index"] = len(deltas)
		imageSpec, isImage := delta["image"].(map[string]any)
		if !isImage {
			deltas = append(deltas, delta)
			continue
		}
		source := strings.TrimSpace(mediumValueString(imageSpec["url"]))
		image, err := c.loadImage(ctx, input, source)
		if err != nil {
			return nil, fallbacks, fmt.Errorf("Medium image load failed: %w", err)
		}
		uploaded, uploadErr := c.uploadImage(ctx, session, draftID, image)
		if uploadErr != nil {
			// A text hyperlink is not a substitute for a Medium image block.
			// Refuse to write a visually corrupted draft. The user can still
			// paste the saved HTML fallback manually while native upload is down.
			return nil, fallbacks, fmt.Errorf("Medium image upload unavailable: refusing to replace image %q with a text link: %w", source, uploadErr)
		}
		paragraph, _ := delta["paragraph"].(map[string]any)
		if paragraph == nil {
			paragraph = map[string]any{}
		}
		paragraph["type"] = 4
		paragraph["text"] = ""
		paragraph["markups"] = []any{}
		paragraph["layout"] = 1
		// medium.com3.har confirms these three native image metadata fields.
		// Do not add an unobserved "alt" property to the saved Delta.
		paragraph["metadata"] = map[string]any{
			"id": uploaded.FileID, "originalWidth": uploaded.Width,
			"originalHeight": uploaded.Height,
		}
		delta["paragraph"] = paragraph
		delete(delta, "image")
		deltas = append(deltas, delta)
	}
	return deltas, fallbacks, nil
}

func (c mediumClient) writeDeltas(
	ctx context.Context,
	session platformSession,
	postID string,
	baseRev int,
	deltas []map[string]any,
) (int, error) {
	body, err := json.Marshal(map[string]any{"id": postID, "baseRev": baseRev, "deltas": deltas})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediumOrigin+"/p/"+postID+"/deltas", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	setMediumHeaders(req, session, mediumOrigin+"/p/"+postID+"/edit")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	var decoded struct {
		Success bool `json:"success"`
		Payload struct {
			Value struct {
				LatestRev int `json:"latestRev"`
			} `json:"value"`
		} `json:"payload"`
	}
	if err := decodeMediumResponse(response, &decoded); err != nil {
		return 0, err
	}
	if !decoded.Success {
		return 0, errors.New("Medium delta write was rejected")
	}
	return decoded.Payload.Value.LatestRev, nil
}

func (c mediumClient) createDraft(
	ctx context.Context,
	session platformSession,
	draft mediumDraft,
	input publisher.DraftInput,
) (map[string]any, error) {
	if draft.Title == "" || draft.Deltas == nil {
		return nil, fmt.Errorf("title and deltas are required")
	}
	session = c.primeXSRF(ctx, session)
	// The editor uploads with the current story's /p/{id}/edit referer.
	// Therefore we need a story ID before uploading. Report that ID even
	// when image preparation fails so the user can recover the empty draft.
	postID, mediumURL, err := c.createStory(ctx, session)
	if err != nil {
		return nil, err
	}
	if c.onStoryCreated != nil {
		c.onStoryCreated(postID)
	}
	if draft.BrowserPlan != nil && c.browserInsert != nil {
		return c.createDraftUsingBrowserImages(ctx, session, draft, input, postID, mediumURL)
	}
	deltas, fallbacks, err := c.prepareDraftDeltas(ctx, session, draft, input, postID)
	if err != nil {
		return mediumDraftResult(postID, mediumURL, draft, fallbacks, true),
			fmt.Errorf("Medium draft %s was created but images were not saved: %w", postID, err)
	}
	if _, err := c.writeDeltas(ctx, session, postID, -1, deltas); err != nil {
		return mediumDraftResult(postID, mediumURL, draft, fallbacks, true), fmt.Errorf("Medium draft %s was created but content was not saved: %w", postID, err)
	}
	if c.verifyAfterCreate {
		if err := c.verifyDraft(ctx, session, postID, deltas); err != nil {
			return mediumDraftResult(postID, mediumURL, draft, fallbacks, true), fmt.Errorf("Medium draft %s was created but remote content verification failed: %w", postID, err)
		}
	}
	return mediumDraftResult(postID, mediumURL, draft, fallbacks, true), nil
}

func (c mediumClient) postMeta(ctx context.Context, session platformSession, postID string) (mediumPostMeta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediumOrigin+"/p/"+postID+"/notes", nil)
	if err != nil {
		return mediumPostMeta{}, err
	}
	setMediumHeaders(req, session, mediumOrigin+"/p/"+postID+"/edit")
	req.Header.Del("content-type")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return mediumPostMeta{}, err
	}
	var decoded struct {
		Success bool `json:"success"`
		Payload struct {
			Post struct {
				ID               string `json:"id"`
				LatestRev        int    `json:"latestRev"`
				FirstPublishedAt int64  `json:"firstPublishedAt"`
				UniqueSlug       string `json:"uniqueSlug"`
				MediumURL        string `json:"mediumUrl"`
				Creator          struct {
					Username string `json:"username"`
				} `json:"creator"`
			} `json:"post"`
		} `json:"payload"`
	}
	if err := decodeMediumResponse(response, &decoded); err != nil {
		return mediumPostMeta{}, err
	}
	if !decoded.Success || decoded.Payload.Post.ID == "" {
		return mediumPostMeta{}, errors.New("Medium post metadata is unavailable")
	}
	post := decoded.Payload.Post
	return mediumPostMeta{
		ID: post.ID, LatestRev: post.LatestRev, FirstPublishedAt: post.FirstPublishedAt,
		UniqueSlug: post.UniqueSlug, MediumURL: post.MediumURL, Username: post.Creator.Username,
	}, nil
}

func (c mediumClient) paragraphCount(ctx context.Context, session platformSession, postID string) (int, error) {
	query := `query BlogCTLMediumPostBodyQuery($postId: ID!) {
  postResult(id: $postId) {
    __typename
    ... on Post {
      id
      content {
        bodyModel {
          paragraphs { name __typename }
          __typename
        }
        __typename
      }
      __typename
    }
  }
}`
	payload, _ := json.Marshal([]any{map[string]any{
		"operationName": "BlogCTLMediumPostBodyQuery",
		"variables":     map[string]any{"postId": postID},
		"query":         query,
	}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediumOrigin+"/_/graphql", bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	setMediumGraphQLHeaders(req, session, mediumOrigin+"/p/"+postID+"/edit", "BlogCTLMediumPostBodyQuery")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return 0, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("Medium post body query failed (%d)", response.StatusCode)
	}
	var decoded []struct {
		Data struct {
			PostResult struct {
				Content struct {
					BodyModel struct {
						Paragraphs []struct {
							Name string `json:"name"`
						} `json:"paragraphs"`
					} `json:"bodyModel"`
				} `json:"content"`
			} `json:"postResult"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded) == 0 {
		return 0, errors.New("Medium post body query returned invalid JSON")
	}
	return len(decoded[0].Data.PostResult.Content.BodyModel.Paragraphs), nil
}

func (c mediumClient) postPresentation(ctx context.Context, session platformSession, postID string) (mediumPostPresentation, error) {
	query := `query BlogCTLMediumPostPresentationQuery($postId: ID!) {
  postResult(id: $postId) {
    __typename
    ... on Post {
      id
      title
      previewContent { subtitle __typename }
      __typename
    }
  }
}`
	payload, _ := json.Marshal([]any{map[string]any{
		"operationName": "BlogCTLMediumPostPresentationQuery",
		"variables":     map[string]any{"postId": postID},
		"query":         query,
	}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediumOrigin+"/_/graphql", bytes.NewReader(payload))
	if err != nil {
		return mediumPostPresentation{}, err
	}
	setMediumGraphQLHeaders(req, session, mediumOrigin+"/p/"+postID+"/edit", "BlogCTLMediumPostPresentationQuery")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return mediumPostPresentation{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return mediumPostPresentation{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return mediumPostPresentation{}, fmt.Errorf("Medium post presentation query failed (%d)", response.StatusCode)
	}
	var decoded []struct {
		Data struct {
			PostResult struct {
				ID             string `json:"id"`
				Title          string `json:"title"`
				PreviewContent struct {
					Subtitle string `json:"subtitle"`
				} `json:"previewContent"`
			} `json:"postResult"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stripMediumXSSI(string(raw))), &decoded); err != nil || len(decoded) == 0 {
		return mediumPostPresentation{}, errors.New("Medium post presentation query returned invalid JSON")
	}
	value := decoded[0].Data.PostResult
	if strings.TrimSpace(value.ID) == "" {
		return mediumPostPresentation{}, errors.New("Medium post presentation is unavailable")
	}
	return mediumPostPresentation{
		Title: strings.TrimSpace(value.Title), Subtitle: strings.TrimSpace(value.PreviewContent.Subtitle),
	}, nil
}

func mediumCanonicalComparable(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return ""
	}
	if strings.EqualFold(parsed.Host, "medium.com") && parsed.Path == "/r/" {
		if target := strings.TrimSpace(parsed.Query().Get("url")); target != "" {
			parsed, err = url.Parse(target)
			if err != nil || parsed.Host == "" {
				return ""
			}
		}
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(strings.ToLower(parsed.Scheme+"://"+parsed.Host), "/") + strings.TrimRight(parsed.EscapedPath(), "/")
}

func mediumCanonicalMatches(candidate, canonicalURL string) bool {
	return mediumCanonicalComparable(candidate) != "" &&
		mediumCanonicalComparable(candidate) == mediumCanonicalComparable(canonicalURL)
}

func (c mediumClient) postLinks(ctx context.Context, session platformSession, postID string) ([]string, error) {
	query := `query BlogCTLMediumPostLinksQuery($postId: ID!) {
  postResult(id: $postId) {
    __typename
    ... on Post {
      id
      content {
        bodyModel {
          paragraphs {
            markups { href __typename }
            __typename
          }
          __typename
        }
        __typename
      }
      __typename
    }
  }
}`
	payload, _ := json.Marshal([]any{map[string]any{
		"operationName": "BlogCTLMediumPostLinksQuery",
		"variables":     map[string]any{"postId": postID},
		"query":         query,
	}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediumOrigin+"/_/graphql", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setMediumGraphQLHeaders(req, session, mediumOrigin+"/p/"+postID+"/edit", "BlogCTLMediumPostLinksQuery")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Medium post links query failed (%d)", response.StatusCode)
	}
	var decoded []struct {
		Data struct {
			PostResult struct {
				ID      string `json:"id"`
				Content struct {
					BodyModel struct {
						Paragraphs []struct {
							Markups []struct {
								Href string `json:"href"`
							} `json:"markups"`
						} `json:"paragraphs"`
					} `json:"bodyModel"`
				} `json:"content"`
			} `json:"postResult"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stripMediumXSSI(string(raw))), &decoded); err != nil || len(decoded) == 0 {
		return nil, errors.New("Medium post links query returned invalid JSON")
	}
	if strings.TrimSpace(decoded[0].Data.PostResult.ID) == "" {
		return nil, errors.New("Medium post links are unavailable")
	}
	links := []string{}
	for _, paragraph := range decoded[0].Data.PostResult.Content.BodyModel.Paragraphs {
		for _, markup := range paragraph.Markups {
			if href := strings.TrimSpace(markup.Href); href != "" {
				links = append(links, href)
			}
		}
	}
	return links, nil
}

func (c mediumClient) postReferencesCanonical(ctx context.Context, session platformSession, postID, canonicalURL string) (bool, error) {
	links, err := c.postLinks(ctx, session, postID)
	if err != nil {
		return false, err
	}
	for _, link := range links {
		if mediumCanonicalMatches(link, canonicalURL) {
			return true, nil
		}
	}
	return false, nil
}

func (c mediumClient) updateDraft(
	ctx context.Context,
	session platformSession,
	postID string,
	draft mediumDraft,
	input publisher.DraftInput,
) (map[string]any, error) {
	session = c.primeXSRF(ctx, session)
	meta, err := c.postMeta(ctx, session, postID)
	if err != nil {
		return nil, err
	}
	if meta.FirstPublishedAt > 0 {
		return nil, errors.New("Medium story is already published; safe published-article updates are not supported yet")
	}
	count, err := c.paragraphCount(ctx, session, postID)
	if err != nil {
		return nil, err
	}
	replacement, fallbacks, err := c.prepareDraftDeltas(ctx, session, draft, input, postID)
	if err != nil {
		return nil, err
	}
	if count < 1 || len(replacement) < 1 {
		return nil, errors.New("Medium draft is missing its title paragraph")
	}
	// Match the create document semantics: replace title AND body, instead
	// of relying on the existing paragraph 0 and its formatting.
	deltas, err := mediumFullReplacementDeltas(count, draft.Title, replacement[1:])
	if err != nil {
		return nil, err
	}
	if _, err := c.writeDeltas(ctx, session, postID, meta.LatestRev, deltas); err != nil {
		return nil, err
	}
	if err := c.verifyDraft(ctx, session, postID, deltas[count:]); err != nil {
		return nil, fmt.Errorf("Medium complete draft replacement verification failed: %w", err)
	}
	return mediumDraftResult(postID, meta.MediumURL, draft, fallbacks, false), nil
}

func mediumDraftResult(postID, mediumURL string, draft mediumDraft, fallbacks int, created bool) map[string]any {
	result := map[string]any{
		"postId":            postID,
		"draftUrl":          mediumOrigin + "/p/" + postID + "/edit",
		"mediumUrl":         nullString(mediumURL),
		"canonicalUrl":      nullString(draft.CanonicalURL),
		"canonicalPending":  strings.TrimSpace(draft.CanonicalURL) != "",
		"tagsPending":       len(draft.Tags) > 0,
		"coverImagePending": draft.CoverImage != nil && strings.TrimSpace(draft.CoverImage.URL) != "",
		"imageFallbacks":    fallbacks,
		"created":           created,
		"updated":           !created,
	}
	if draft.CoverImage != nil {
		result["coverImageUrl"] = nullString(strings.TrimSpace(draft.CoverImage.URL))
		result["coverImageAlt"] = nullString(strings.TrimSpace(draft.CoverImage.Alt))
	}
	return result
}

func (c mediumClient) publishDraft(
	ctx context.Context,
	session platformSession,
	postID, title, subtitle string,
) (map[string]any, error) {
	session = c.primeXSRF(ctx, session)
	meta, err := c.postMeta(ctx, session, postID)
	if err != nil {
		return nil, err
	}
	presentation, err := c.postPresentation(ctx, session, postID)
	if err != nil {
		return nil, err
	}
	if presentation.Title != "" {
		title = presentation.Title
	}
	if presentation.Subtitle != "" {
		subtitle = presentation.Subtitle
	}
	body, err := json.Marshal(map[string]any{
		"title": title, "subtitle": subtitle, "metaDescription": "", "latestRev": meta.LatestRev,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediumOrigin+"/p/"+postID+"/publish", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	setMediumHeaders(req, session, mediumOrigin+"/p/"+postID+"/edit")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	var decoded struct {
		Success bool `json:"success"`
		Payload struct {
			Value struct {
				ID         string `json:"id"`
				UniqueSlug string `json:"uniqueSlug"`
				MediumURL  string `json:"mediumUrl"`
				Creator    struct {
					Username string `json:"username"`
				} `json:"creator"`
			} `json:"value"`
		} `json:"payload"`
	}
	if err := decodeMediumResponse(response, &decoded); err != nil {
		return nil, err
	}
	if !decoded.Success || strings.TrimSpace(decoded.Payload.Value.ID) == "" {
		return nil, errors.New("Medium publish did not return a post")
	}
	value := decoded.Payload.Value
	target := strings.TrimSpace(value.MediumURL)
	username := strings.TrimSpace(value.Creator.Username)
	if username == "" {
		username = meta.Username
	}
	if target == "" && username != "" && value.UniqueSlug != "" {
		target = mediumOrigin + "/@" + url.PathEscape(username) + "/" + value.UniqueSlug
	}
	if target == "" && username != "" && meta.UniqueSlug != "" {
		target = mediumOrigin + "/@" + url.PathEscape(username) + "/" + meta.UniqueSlug
	}
	if target == "" {
		target = mediumOrigin + "/p/" + postID
	}
	return map[string]any{"postId": value.ID, "url": target}, nil
}

func normalizeMediumTitle(value string) string {
	value = htmlstd.UnescapeString(mediumHTMLTagPattern.ReplaceAllString(value, " "))
	return strings.Join(strings.Fields(value), " ")
}

func mediumTitleMatches(local, remote string) bool {
	local = normalizeMediumTitle(local)
	remote = normalizeMediumTitle(remote)
	if local == "" || remote == "" {
		return false
	}
	if local == remote {
		return true
	}
	for _, separator := range []string{" · ", " - ", " — "} {
		if strings.HasPrefix(remote, local+separator) {
			return true
		}
	}
	if strings.HasSuffix(remote, "…") {
		return strings.HasPrefix(local, strings.TrimSuffix(remote, "…"))
	}
	return false
}

func parseMediumStoryLinks(raw string, published bool) []mediumPost {
	posts := []mediumPost{}
	seen := map[string]struct{}{}
	for _, match := range mediumAnchorPattern.FindAllStringSubmatch(raw, -1) {
		if len(match) != 3 {
			continue
		}
		href := htmlstd.UnescapeString(strings.TrimSpace(match[1]))
		title := normalizeMediumTitle(match[2])
		if title == "" {
			continue
		}
		parsed, err := url.Parse(href)
		if err != nil {
			continue
		}
		if parsed.Host != "" && !strings.HasSuffix(strings.ToLower(parsed.Host), "medium.com") {
			continue
		}
		if !published {
			candidatePath := strings.TrimSuffix(parsed.Path, "/")
			pathMatch := mediumDraftPathPattern.FindStringSubmatch(candidatePath)
			if len(pathMatch) != 2 {
				continue
			}
			id := pathMatch[1]
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			posts = append(posts, mediumPost{
				ID: id, Title: title, URL: mediumOrigin + "/p/" + id + "/edit",
			})
			continue
		}
		if !strings.HasPrefix(parsed.Path, "/@") {
			continue
		}
		slug := strings.Trim(strings.TrimPrefix(parsed.Path, "/"), "/")
		idMatch := mediumPostIDPattern.FindStringSubmatch(slug)
		if len(idMatch) != 2 {
			continue
		}
		id := idMatch[1]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		posts = append(posts, mediumPost{
			ID: id, Title: title,
			URL: mediumOrigin + parsed.Path, Published: true,
		})
	}
	return posts
}

func mediumAccountFromStoriesHTML(raw string) string {
	for _, match := range mediumProfilePattern.FindAllStringSubmatch(raw, -1) {
		if len(match) == 2 && strings.TrimSpace(match[1]) != "" {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
}

func (c mediumClient) storyListPagePage(ctx context.Context, session platformSession, published bool, cursor string) ([]mediumPost, string, string, error) {
	postType := "POST_TYPE_DRAFT"
	operation := "BlogCTLMediumDraftsQuery"
	if published {
		postType = "POST_TYPE_PUBLISHED"
		operation = "BlogCTLMediumPublishedQuery"
	}
	query := fmt.Sprintf(`query %s($pagingOptions: PagingOptions) {
  viewer {
    id
    username
    latestPostsConnection(type: %s, includeResponses: false, includeSuspended: true, includeDeleted: false, paging: $pagingOptions) {
      pagingInfo { next { to } }
      postPreviews { postId post { id title mediumUrl uniqueSlug isPublished } }
    }
  }
}`, operation, postType)
	payload, err := json.Marshal([]any{map[string]any{
		"operationName": operation,
		"variables": map[string]any{"pagingOptions": map[string]any{
			"to": cursor, "limit": 25, "order": "DESC",
		}},
		"query": query,
	}})
	if err != nil {
		return nil, "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediumOrigin+"/_/graphql", bytes.NewReader(payload))
	if err != nil {
		return nil, "", "", err
	}
	setMediumGraphQLHeaders(req, session, mediumOrigin+"/me/stories", operation)
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, "", "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", "", fmt.Errorf("Medium stories query failed (%d)", response.StatusCode)
	}
	var decoded []struct {
		Data struct {
			Viewer struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Latest   struct {
					PagingInfo struct {
						Next *struct {
							To string `json:"to"`
						} `json:"next"`
					} `json:"pagingInfo"`
					PostPreviews []struct {
						PostID string `json:"postId"`
						Post   struct {
							ID         string `json:"id"`
							Title      string `json:"title"`
							MediumURL  string `json:"mediumUrl"`
							UniqueSlug string `json:"uniqueSlug"`
						} `json:"post"`
					} `json:"postPreviews"`
				} `json:"latestPostsConnection"`
			} `json:"viewer"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(stripMediumXSSI(string(raw))), &decoded); err != nil || len(decoded) == 0 {
		return nil, "", "", errors.New("Medium stories query returned invalid JSON")
	}
	if len(decoded[0].Errors) > 0 {
		return nil, "", "", fmt.Errorf("Medium stories query failed: %s", truncate(decoded[0].Errors[0].Message, 300))
	}
	viewer := decoded[0].Data.Viewer
	if strings.TrimSpace(viewer.ID) == "" {
		return nil, "", "", errors.New("Medium browser session is not authenticated")
	}
	posts := make([]mediumPost, 0, len(viewer.Latest.PostPreviews))
	for _, preview := range viewer.Latest.PostPreviews {
		id := strings.TrimSpace(preview.Post.ID)
		if id == "" {
			id = strings.TrimSpace(preview.PostID)
		}
		title := normalizeMediumTitle(preview.Post.Title)
		if id == "" || title == "" {
			continue
		}
		target := strings.TrimSpace(preview.Post.MediumURL)
		if published {
			if target == "" && viewer.Username != "" && preview.Post.UniqueSlug != "" {
				target = mediumOrigin + "/@" + url.PathEscape(viewer.Username) + "/" + preview.Post.UniqueSlug
			}
			if target == "" {
				target = mediumOrigin + "/p/" + id
			}
		} else {
			target = mediumOrigin + "/p/" + id + "/edit"
		}
		posts = append(posts, mediumPost{ID: id, Title: title, URL: target, Published: published})
	}
	nextCursor := ""
	if viewer.Latest.PagingInfo.Next != nil {
		nextCursor = strings.TrimSpace(viewer.Latest.PagingInfo.Next.To)
	}
	return posts, strings.TrimSpace(viewer.Username), nextCursor, nil
}

func (c mediumClient) storyListPage(ctx context.Context, session platformSession, published bool) ([]mediumPost, string, error) {
	posts := []mediumPost{}
	account := ""
	cursor := ""
	for page := 1; page <= 100; page++ {
		pagePosts, pageAccount, nextCursor, err := c.storyListPagePage(ctx, session, published, cursor)
		if err != nil {
			return nil, "", err
		}
		posts = append(posts, pagePosts...)
		if account == "" {
			account = pageAccount
		}
		if nextCursor == "" || nextCursor == cursor {
			return posts, account, nil
		}
		cursor = nextCursor
	}
	return nil, "", errors.New("Medium stories query exceeded pagination limit")
}

func (c mediumClient) account(ctx context.Context, session platformSession) (string, error) {
	query := `query BlogCTLMediumViewerQuery { viewer { id username name __typename } }`
	payload, _ := json.Marshal([]any{map[string]any{
		"operationName": "BlogCTLMediumViewerQuery",
		"variables":     map[string]any{},
		"query":         query,
	}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediumOrigin+"/_/graphql", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	setMediumGraphQLHeaders(req, session, mediumOrigin+"/", "BlogCTLMediumViewerQuery")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Medium viewer query failed (%d)", response.StatusCode)
	}
	var decoded []struct {
		Data struct {
			Viewer struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"viewer"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded) == 0 ||
		strings.TrimSpace(decoded[0].Data.Viewer.ID) == "" {
		return "", errors.New("Medium browser session is not authenticated")
	}
	username := strings.TrimSpace(decoded[0].Data.Viewer.Username)
	if username == "" {
		username = strings.TrimSpace(decoded[0].Data.Viewer.ID)
	}
	return username, nil
}

func (c mediumClient) listPosts(ctx context.Context, session platformSession) (string, []mediumPost, error) {
	drafts, account, err := c.storyListPage(ctx, session, false)
	if err != nil {
		return "", nil, err
	}
	published, publishedAccount, err := c.storyListPage(ctx, session, true)
	if err != nil {
		return "", nil, err
	}
	if account == "" {
		account = publishedAccount
	}
	return account, append(drafts, published...), nil
}

func setMediumHeaders(req *http.Request, session platformSession, referer string) {
	req.Header.Set("accept", "application/json")
	req.Header.Set("accept-language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("origin", mediumOrigin)
	req.Header.Set("referer", referer)
	req.Header.Set("cookie", mediumSessionCookieHeader(session))
	req.Header.Set("user-agent", session.UserAgent)
	if major := mediumChromeMajor(session.UserAgent); major > 0 {
		// Match the browser family advertised by the actual User-Agent.
		// Edge sends Microsoft Edge, not Google Chrome, in Client Hints.
		if strings.Contains(session.UserAgent, "Edg/") {
			req.Header.Set("sec-ch-ua", fmt.Sprintf(`"Microsoft Edge";v="%d", "Chromium";v="%d", "Not(A:Brand";v="24"`, major, major))
		} else {
			req.Header.Set("sec-ch-ua", fmt.Sprintf(`"Chromium";v="%d", "Google Chrome";v="%d", "Not_A Brand";v="99"`, major, major))
		}
	}
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("x-requested-with", "XMLHttpRequest")
	req.Header.Set("x-obvious-cid", "web")
	req.Header.Set("x-client-date", fmt.Sprintf("%d", time.Now().UnixMilli()))
	if xsrf := mediumSessionXSRF(session); xsrf != "" {
		req.Header.Set("x-xsrf-token", xsrf)
	}
}

func setMediumGraphQLHeaders(req *http.Request, session platformSession, referer, operation string) {
	setMediumHeaders(req, session, referer)
	if operation != "" {
		req.Header.Set("graphql-operation", operation)
	}
}

func mediumValueString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

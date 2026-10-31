package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	htmlstd "html"
	"io"
	"net/http"
	"regexp"
	"strings"
)

var (
	devtoAuthenticityTokenPattern = regexp.MustCompile(`(?i)name=["']authenticity_token["'][^>]*value=["']([^"']+)["']`)
	devtoAuthenticityValuePattern = regexp.MustCompile(`(?i)value=["']([^"']+)["'][^>]*name=["']authenticity_token["']`)
)

func (a *devtoAdapter) hasBrowserSession() bool {
	return len(a.browserCookies) > 0
}

func devtoBrowserCookieHeader(cookies []BrowserCookie) string {
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		name := strings.TrimSpace(cookie.Name)
		if name == "" || cookie.Value == "" {
			continue
		}
		parts = append(parts, name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}

func (a *devtoAdapter) browserAuthenticityToken(ctx context.Context) (string, error) {
	if a.browserCSRFToken != "" {
		return a.browserCSRFToken, nil
	}
	if !a.hasBrowserSession() {
		return "", errors.New("DEV.to browser session is unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.origin+"/dashboard", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("accept", "text/html,application/xhtml+xml")
	request.Header.Set("cookie", devtoBrowserCookieHeader(a.browserCookies))
	if a.browserUserAgent != "" {
		request.Header.Set("user-agent", a.browserUserAgent)
	}
	response, err := a.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("DEV.to browser session check returned HTTP %d", response.StatusCode)
	}
	text := string(raw)
	match := devtoAuthenticityTokenPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		match = devtoAuthenticityValuePattern.FindStringSubmatch(text)
	}
	if len(match) != 2 || strings.TrimSpace(match[1]) == "" {
		return "", errors.New("DEV.to authenticity token was not found")
	}
	a.browserCSRFToken = htmlstd.UnescapeString(strings.TrimSpace(match[1]))
	return a.browserCSRFToken, nil
}

func (a *devtoAdapter) uploadBrowserImage(ctx context.Context, image RehostImage) (string, error) {
	token, err := a.browserAuthenticityToken(ctx)
	if err != nil {
		return "", err
	}
	body, contentType, err := multipartBody(
		map[string]string{"authenticity_token": token},
		"image[]",
		inferImageFilename(image.Source, image.ContentType),
		image.ContentType,
		image.Payload,
	)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.origin+"/image_uploads", body)
	if err != nil {
		return "", err
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set("content-type", contentType)
	request.Header.Set("cookie", devtoBrowserCookieHeader(a.browserCookies))
	request.Header.Set("origin", a.origin)
	request.Header.Set("referer", a.origin+"/new")
	request.Header.Set("x-csrf-token", token)
	request.Header.Set("x-requested-with", "XMLHttpRequest")
	if a.browserUserAgent != "" {
		request.Header.Set("user-agent", a.browserUserAgent)
	}
	response, err := a.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("DEV.to image upload returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	var decoded struct {
		Links []string `json:"links"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("DEV.to image upload returned invalid JSON: %w", err)
	}
	if len(decoded.Links) == 0 || strings.TrimSpace(decoded.Links[0]) == "" {
		return "", errors.New("DEV.to image upload did not return a link")
	}
	return strings.TrimSpace(decoded.Links[0]), nil
}

func devtoHostedImage(source string) bool {
	value := strings.ToLower(strings.TrimSpace(source))
	return strings.HasPrefix(value, "https://dev-to-uploads.s3.") ||
		strings.HasPrefix(value, "https://media2.dev.to/")
}

func (a *devtoAdapter) prepareCoverImage(ctx context.Context, input DraftInput) (string, error) {
	source := strings.TrimSpace(input.CoverImageURL)
	if source == "" || devtoHostedImage(source) || !a.hasBrowserSession() {
		return source, nil
	}
	payload, contentType, err := loadRehostImage(a.client, input, source, input.SourceDir)
	if err != nil {
		if isRemoteHTTPImage(source) {
			return source, nil
		}
		return "", platformError(ErrUpload, "devto", "download-cover-image", 0, err.Error(), true)
	}
	image := RehostImage{Source: source, Payload: payload, ContentType: contentType}
	target, uploadErr := a.uploadBrowserImage(ctx, image)
	if uploadErr == nil && strings.TrimSpace(target) != "" {
		return strings.TrimSpace(target), nil
	}
	fallback, fallbackErr := tryR2Fallback(ctx, a.client, input, image, uploadErr)
	if fallbackErr == nil && strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback), nil
	}
	if isRemoteHTTPImage(source) {
		return source, nil
	}
	return "", fallbackErr
}

func (a *devtoAdapter) prepareImages(ctx context.Context, input DraftInput) (DraftInput, error) {
	markdown, err := rehostMarkdownImages(ctx, a.client, input, ImageRehostOptions{
		Platform:       "devto",
		FailOpenRemote: true,
		AlreadyHosted:  devtoHostedImage,
	}, a.uploadBrowserImage)
	if err != nil {
		return input, err
	}
	input.Markdown = markdown
	cover, err := a.prepareCoverImage(ctx, input)
	if err != nil {
		return input, err
	}
	input.CoverImageURL = cover
	return input, nil
}

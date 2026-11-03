package r2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxUploadAttempts = 3

func retryableUploadStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500 && status <= 599
}

func waitUploadRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt+1) * 150 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Upload creates a content-addressed R2 object once. A conditional 412 means
// the exact object key already exists; retries cannot overwrite that key.
// A retry after an ambiguous network failure is safe for this immutable key.
func Upload(ctx context.Context, client *http.Client, config Config, objectKey string, payload []byte, contentType string) (string, error) {
	if err := Validate(config); err != nil {
		return "", err
	}
	objectKey = strings.TrimSpace(strings.TrimPrefix(objectKey, "/"))
	if objectKey == "" {
		return "", errors.New("R2 object key is empty")
	}
	if client == nil {
		client = http.DefaultClient
	}
	rawURL := Endpoint(config) + "/" + url.PathEscape(strings.TrimSpace(config.Bucket)) + "/" + encodePath(objectKey)

	for attempt := 0; attempt < maxUploadAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		// Re-sign each attempt so the AWS request timestamp stays current.
		headers, err := SignPut(rawURL, payload, contentType, config, time.Now())
		if err != nil {
			return "", err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, bytes.NewReader(payload))
		if err != nil {
			return "", err
		}
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 ||
				response.StatusCode == http.StatusPreconditionFailed {
				// If-None-Match: * is atomic on R2. PreconditionFailed means
				// another request already committed this content-addressed key.
				return PublicURL(config, objectKey)
			}
			if !retryableUploadStatus(response.StatusCode) || attempt == maxUploadAttempts-1 {
				return "", fmt.Errorf("R2 upload returned HTTP %d", response.StatusCode)
			}
		} else {
			if response != nil && response.Body != nil {
				response.Body.Close()
			}
			if attempt == maxUploadAttempts-1 {
				return "", fmt.Errorf("R2 upload request failed: %w", err)
			}
		}
		if err := waitUploadRetry(ctx, attempt); err != nil {
			return "", err
		}
	}
	return "", errors.New("R2 upload retry budget exhausted")
}

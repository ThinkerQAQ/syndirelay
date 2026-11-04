package r2

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
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

// Upload preserves legacy overwrite behavior for caller-provided, mutable keys.
// Retrying an ambiguous write could overwrite changed content, so it remains
// a single attempt. Content-addressed callers must use UploadImmutable.
func Upload(ctx context.Context, client *http.Client, config Config, objectKey string, payload []byte, contentType string) (string, error) {
	return upload(ctx, client, config, objectKey, payload, contentType, false)
}

// UploadImmutable creates a digest-named object at most once. A lost response
// can be retried safely, and 412 means another writer already created the key.
func UploadImmutable(ctx context.Context, client *http.Client, config Config, objectKey string, payload []byte, contentType string) (string, error) {
	objectKey = strings.TrimSpace(strings.TrimPrefix(objectKey, "/"))
	hash := sha256.Sum256(payload)
	digest := hex.EncodeToString(hash[:16])
	if !strings.HasPrefix(path.Base(objectKey), digest+".") {
		return "", errors.New("immutable R2 object key does not match payload checksum")
	}
	return upload(ctx, client, config, objectKey, payload, contentType, true)
}

func upload(ctx context.Context, client *http.Client, config Config, objectKey string, payload []byte, contentType string, immutable bool) (string, error) {
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

	attempts := 1
	if immutable {
		attempts = maxUploadAttempts
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		var headers map[string]string
		var err error
		if immutable {
			headers, err = SignPutIfAbsent(rawURL, payload, contentType, config, time.Now())
		} else {
			headers, err = SignPut(rawURL, payload, contentType, config, time.Now())
		}
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
				immutable && response.StatusCode == http.StatusPreconditionFailed {
				return PublicURL(config, objectKey)
			}
			if !immutable || !retryableUploadStatus(response.StatusCode) || attempt == attempts-1 {
				return "", fmt.Errorf("R2 upload returned HTTP %d", response.StatusCode)
			}
		} else {
			if response != nil && response.Body != nil {
				response.Body.Close()
			}
			if attempt == attempts-1 {
				return "", fmt.Errorf("R2 upload request failed: %w", err)
			}
		}
		if err := waitUploadRetry(ctx, attempt); err != nil {
			return "", err
		}
	}
	return "", errors.New("R2 upload retry budget exhausted")
}

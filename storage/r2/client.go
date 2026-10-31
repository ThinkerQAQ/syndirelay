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
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("R2 upload returned HTTP %d", response.StatusCode)
	}
	return PublicURL(config, objectKey)
}

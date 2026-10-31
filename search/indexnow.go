package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	DefaultIndexNowEndpoint = "https://www.bing.com/indexnow"
	DefaultIndexNowKey      = "fb26fca3ba9449c6816b6d79b0a41cec"
	MaxIndexNowURLs         = 10_000
)

var indexNowKeyPattern = regexp.MustCompile(`^[A-Za-z0-9-]{8,128}$`)

type IndexNowConfig struct {
	Origin      string
	Endpoint    string
	Key         string
	KeyLocation string
}

type IndexNowBatchResult struct {
	HTTPStatus int `json:"httpStatus"`
	Attempts   int `json:"attempts"`
}

type IndexNowResult struct {
	URLCount   int                   `json:"urlCount"`
	BatchCount int                   `json:"batchCount"`
	Results    []IndexNowBatchResult `json:"results"`
	Skipped    bool                  `json:"skipped,omitempty"`
}

func ResolveIndexNowConfig(origin, publicRoot, endpoint, key, keyLocation string, verifyKeyFile bool) (IndexNowConfig, error) {
	normalizedOrigin, err := NormalizeOrigin(origin)
	if err != nil {
		return IndexNowConfig{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = DefaultIndexNowKey
	}
	if !indexNowKeyPattern.MatchString(key) {
		return IndexNowConfig{}, errors.New("IndexNow key must be 8-128 URL-safe characters")
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = DefaultIndexNowEndpoint
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || parsedEndpoint.Scheme != "https" || parsedEndpoint.Host == "" {
		return IndexNowConfig{}, errors.New("IndexNow endpoint must be an HTTPS URL")
	}
	keyLocation = strings.TrimSpace(keyLocation)
	if keyLocation == "" {
		keyLocation = normalizedOrigin + "/" + key + ".txt"
	}
	normalizedLocation, err := NormalizeURL(keyLocation, normalizedOrigin, "IndexNow key location")
	if err != nil {
		return IndexNowConfig{}, err
	}
	if verifyKeyFile {
		parsed, _ := url.Parse(normalizedLocation)
		relative, err := url.PathUnescape(strings.TrimLeft(parsed.Path, "/"))
		if err != nil {
			return IndexNowConfig{}, err
		}
		file, err := safePath(publicRoot, filepath.FromSlash(relative), "IndexNow key file")
		if err != nil {
			return IndexNowConfig{}, err
		}
		hosted, err := os.ReadFile(file)
		if err != nil {
			return IndexNowConfig{}, err
		}
		if strings.TrimSpace(string(hosted)) != key {
			return IndexNowConfig{}, fmt.Errorf("IndexNow key file does not match configured key: %s", file)
		}
	}
	return IndexNowConfig{
		Origin:      normalizedOrigin,
		Endpoint:    parsedEndpoint.String(),
		Key:         key,
		KeyLocation: normalizedLocation,
	}, nil
}

func SubmitIndexNow(ctx context.Context, client *http.Client, urls []string, config IndexNowConfig) (IndexNowResult, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if len(urls) == 0 {
		return IndexNowResult{Skipped: true}, nil
	}
	unique := map[string]struct{}{}
	for _, rawURL := range urls {
		normalized, err := NormalizeURL(rawURL, config.Origin, "submitted page URL")
		if err != nil {
			return IndexNowResult{}, err
		}
		unique[normalized] = struct{}{}
	}
	ordered := make([]string, 0, len(unique))
	for rawURL := range unique {
		ordered = append(ordered, rawURL)
	}
	// URL order does not matter to IndexNow, but deterministic payloads help logs/tests.
	sort.Strings(ordered)

	result := IndexNowResult{URLCount: len(ordered)}
	for start := 0; start < len(ordered); start += MaxIndexNowURLs {
		end := minInt(start+MaxIndexNowURLs, len(ordered))
		payload := map[string]any{
			"host":        mustURL(config.Origin).Host,
			"key":         config.Key,
			"keyLocation": config.KeyLocation,
			"urlList":     ordered[start:end],
		}
		data, _ := json.Marshal(payload)
		batch, err := submitIndexNowBatch(ctx, client, config.Endpoint, data)
		if err != nil {
			return result, err
		}
		result.Results = append(result.Results, batch)
	}
	result.BatchCount = len(result.Results)
	return result, nil
}

func submitIndexNowBatch(ctx context.Context, client *http.Client, endpoint string, payload []byte) (IndexNowBatchResult, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return IndexNowBatchResult{}, err
		}
		request.Header.Set("content-type", "application/json; charset=utf-8")
		response, err := client.Do(request)
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			response.Body.Close()
			if response.StatusCode == 200 || response.StatusCode == 202 {
				return IndexNowBatchResult{HTTPStatus: response.StatusCode, Attempts: attempt}, nil
			}
			if response.StatusCode != 429 && response.StatusCode < 500 {
				return IndexNowBatchResult{}, fmt.Errorf("IndexNow rejected payload with HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
			}
			lastErr = fmt.Errorf("IndexNow temporary HTTP %d", response.StatusCode)
		} else {
			lastErr = err
		}
		if attempt < 3 {
			timer := time.NewTimer(time.Duration(1<<attempt) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return IndexNowBatchResult{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return IndexNowBatchResult{}, lastErr
}

func CheckIndexNowKey(ctx context.Context, client *http.Client, config IndexNowConfig) error {
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, config.KeyLocation, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if readErr != nil {
		return readErr
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("IndexNow key location check failed with HTTP %d", response.StatusCode)
	}
	if strings.TrimSpace(string(body)) != config.Key {
		return errors.New("IndexNow key location content does not match configured key")
	}
	return nil
}

func mustURL(value string) *url.URL {
	parsed, _ := url.Parse(value)
	return parsed
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

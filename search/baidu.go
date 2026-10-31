package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	BaiduSubmitEndpoint = "http://data.zz.baidu.com/urls"
	MaxBaiduURLs        = 2_000
)

type BaiduConfig struct {
	Endpoint string
	Site     string
	Token    string
}

type BaiduBatchResult struct {
	HTTPStatus  int      `json:"httpStatus"`
	Submitted   int      `json:"submitted"`
	Success     int      `json:"success"`
	Remain      int      `json:"remain"`
	NotSameSite []string `json:"notSameSite,omitempty"`
	NotValid    []string `json:"notValid,omitempty"`
	Complete    bool     `json:"complete"`
}

type BaiduResult struct {
	URLCount     int                `json:"urlCount"`
	SuccessCount int                `json:"successCount"`
	Remain       int                `json:"remain"`
	BatchCount   int                `json:"batchCount"`
	Complete     bool               `json:"complete"`
	Results      []BaiduBatchResult `json:"results"`
	Skipped      bool               `json:"skipped,omitempty"`
}

func ResolveBaiduConfig(origin, site, token string) (BaiduConfig, error) {
	origin, err := NormalizeOrigin(origin)
	if err != nil {
		return BaiduConfig{}, err
	}
	site = strings.TrimSpace(site)
	if site == "" {
		site = origin
	}
	normalizedSite, err := NormalizeOrigin(site)
	if err != nil {
		return BaiduConfig{}, err
	}
	if normalizedSite != origin {
		return BaiduConfig{}, fmt.Errorf("Baidu site must match %s", origin)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return BaiduConfig{}, errors.New("Baidu push token is required")
	}
	if strings.ContainsAny(token, "\r\n") {
		return BaiduConfig{}, errors.New("Baidu push token is invalid")
	}
	return BaiduConfig{Endpoint: BaiduSubmitEndpoint, Site: normalizedSite, Token: token}, nil
}

func SubmitBaidu(ctx context.Context, client *http.Client, urls []string, config BaiduConfig) (BaiduResult, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if len(urls) == 0 {
		return BaiduResult{Complete: true, Skipped: true}, nil
	}
	origin, err := NormalizeOrigin(config.Site)
	if err != nil {
		return BaiduResult{}, err
	}
	unique := map[string]struct{}{}
	for _, rawURL := range urls {
		normalized, err := NormalizeURL(rawURL, origin, "submitted page URL")
		if err != nil {
			return BaiduResult{}, err
		}
		unique[normalized] = struct{}{}
	}
	ordered := make([]string, 0, len(unique))
	for rawURL := range unique {
		ordered = append(ordered, rawURL)
	}
	sort.Strings(ordered)

	result := BaiduResult{Complete: true}
	for start := 0; start < len(ordered); start += MaxBaiduURLs {
		end := minInt(start+MaxBaiduURLs, len(ordered))
		batch, err := submitBaiduBatch(ctx, client, ordered[start:end], config)
		if batch.Submitted > 0 {
			result.Results = append(result.Results, batch)
		}
		result.URLCount += batch.Submitted
		result.BatchCount = len(result.Results)
		result.SuccessCount += batch.Success
		result.Remain = batch.Remain
		if err != nil {
			result.Complete = false
			return result, err
		}
		if !batch.Complete {
			result.Complete = false
		}
	}
	if !result.Complete {
		return result, fmt.Errorf("Baidu accepted %d of %d submitted URLs; snapshot was not advanced because accepted URLs are ambiguous", result.SuccessCount, result.URLCount)
	}
	return result, nil
}

func submitBaiduBatch(ctx context.Context, client *http.Client, urls []string, config BaiduConfig) (BaiduBatchResult, error) {
	started := time.Now()
	endpointValue := strings.TrimSpace(config.Endpoint)
	if endpointValue == "" {
		endpointValue = BaiduSubmitEndpoint
	}
	endpoint, err := url.Parse(endpointValue)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return BaiduBatchResult{}, errors.New("Baidu submit endpoint is invalid")
	}
	siteURL, err := url.Parse(config.Site)
	if err != nil || siteURL.Hostname() == "" {
		return BaiduBatchResult{}, errors.New("Baidu site is invalid")
	}
	query := endpoint.Query()
	query.Set("site", siteURL.Hostname())
	query.Set("token", config.Token)
	endpoint.RawQuery = query.Encode()

	body := []byte(strings.Join(urls, "\n"))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return BaiduBatchResult{}, err
	}
	request.Header.Set("content-type", "text/plain")
	result := BaiduBatchResult{Submitted: len(urls)}
	slog.Info("Baidu submission started", "operation", "baidu-submit", "site", siteURL.Hostname(), "count", len(urls))
	response, err := client.Do(request)
	if err != nil {
		slog.Warn("Baidu submission transport failed", "operation", "baidu-submit", "site", siteURL.Hostname(), "durationMs", time.Since(started).Milliseconds())
		return result, fmt.Errorf("Baidu submit request failed: %s", redactBaiduResponse(err.Error(), config.Token))
	}
	result.HTTPStatus = response.StatusCode
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	response.Body.Close()
	if readErr != nil {
		return result, readErr
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		slog.Warn("Baidu submission rejected", "operation", "baidu-submit", "site", siteURL.Hostname(), "httpStatus", response.StatusCode, "count", len(urls), "durationMs", time.Since(started).Milliseconds())
		if strings.Contains(strings.ToLower(string(payload)), "over quota") {
			return result, fmt.Errorf("Baidu submit returned HTTP %d: over quota; the current batch exceeds the available submission quota. Check the quota for %s in Baidu Search Resource; the submission snapshot was not advanced", response.StatusCode, siteURL.Hostname())
		}
		return result, fmt.Errorf("Baidu submit returned HTTP %d: %s", response.StatusCode, redactBaiduResponse(string(payload), config.Token))
	}

	var decoded struct {
		Remain      int      `json:"remain"`
		Success     int      `json:"success"`
		NotSameSite []string `json:"not_same_site"`
		NotValid    []string `json:"not_valid"`
		Message     string   `json:"message"`
		Error       int      `json:"error"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return result, fmt.Errorf("decode Baidu response: %w", err)
	}
	if decoded.Error != 0 {
		return result, fmt.Errorf("Baidu submit error %d: %s", decoded.Error, redactBaiduResponse(decoded.Message, config.Token))
	}
	result.Success = decoded.Success
	result.Remain = decoded.Remain
	result.NotSameSite = decoded.NotSameSite
	result.NotValid = decoded.NotValid
	result.Complete = decoded.Success == len(urls)
	slog.Info("Baidu submission completed", "operation", "baidu-submit", "site", siteURL.Hostname(), "httpStatus", result.HTTPStatus, "submitted", result.Submitted, "accepted", result.Success, "remain", result.Remain, "durationMs", time.Since(started).Milliseconds())
	return result, nil
}

func redactBaiduResponse(value, token string) string {
	if token == "" {
		return value
	}
	return strings.ReplaceAll(value, token, "[REDACTED]")
}

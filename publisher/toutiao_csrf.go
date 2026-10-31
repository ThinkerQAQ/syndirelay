package publisher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// toutiaoCSRFCredential is held only in memory for a single publication
// operation. Never persist or log Token.
type toutiaoCSRFCredential struct {
	Token     string
	ExpiresAt time.Time
}

// parseToutiaoWareCSRF parses a creator API response header:
// "0,<token>,<ttl-ms>[,...]". It does not accept HTTP cookies or HAR
// signatures as substitutes for a fresh server-provided token.
func parseToutiaoWareCSRF(raw string, now time.Time) (toutiaoCSRFCredential, error) {
	parts := strings.Split(raw, ",")
	if len(parts) < 3 || strings.TrimSpace(parts[0]) != "0" {
		return toutiaoCSRFCredential{}, errors.New("Toutiao CSRF header not supplied or unrecognized")
	}
	token := strings.TrimSpace(parts[1])
	if len(token) < 12 || len(token) > 4096 || strings.ContainsAny(token, "\r\n ,;\t") {
		return toutiaoCSRFCredential{}, errors.New("Toutiao CSRF token has an invalid format")
	}
	millis, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
	if err != nil || millis < 1000 {
		return toutiaoCSRFCredential{}, errors.New("Toutiao CSRF token has no usable expiry")
	}
	const maxTTLMillis = int64(24 * 60 * 60 * 1000)
	if millis > maxTTLMillis {
		millis = maxTTLMillis
	}
	valid := time.Duration(millis) * time.Millisecond
	return toutiaoCSRFCredential{Token: token, ExpiresAt: now.Add(valid)}, nil
}

// fetchToutiaoCSRF probes the official creator API directly via HTTP.
// It never executes page JavaScript and never submits article content.
// The caller must supply an authenticated, cookie-jar-backed HTTP client.
func fetchToutiaoCSRF(ctx context.Context, client *http.Client, agent string, now time.Time) (toutiaoCSRFCredential, error) {
	if client == nil {
		return toutiaoCSRFCredential{}, errors.New("HTTP client is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://mp.toutiao.com/spice/image", nil)
	if err != nil {
		return toutiaoCSRFCredential{}, err
	}
	req.Header.Set("x-secsdk-csrf-request", "1")
	req.Header.Set("x-secsdk-csrf-version", "1.2.22")
	if strings.TrimSpace(agent) != "" {
		req.Header.Set("User-Agent", agent)
	}
	resp, err := client.Do(req)
	if err != nil {
		return toutiaoCSRFCredential{}, fmt.Errorf("Toutiao CSRF preflight: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.Request == nil || resp.Request.URL == nil ||
		resp.Request.URL.Scheme != "https" || resp.Request.URL.Hostname() != "mp.toutiao.com" {
		return toutiaoCSRFCredential{}, errors.New("Toutiao CSRF preflight redirected outside creator origin")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return toutiaoCSRFCredential{}, fmt.Errorf("Toutiao CSRF preflight rejected with HTTP %d", resp.StatusCode)
	}
	return parseToutiaoWareCSRF(resp.Header.Get("x-ware-csrf-token"), now)
}

package search

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	GoogleWebmastersScope             = "https://www.googleapis.com/auth/webmasters"
	GoogleDefaultTokenURI             = "https://oauth2.googleapis.com/token"
	GoogleSearchConsoleAPI            = "https://www.googleapis.com/webmasters/v3"
	GoogleURLInspectionAPI            = "https://searchconsole.googleapis.com/v1/urlInspection/index:inspect"
	GoogleURLInspectionDailySiteLimit = 2000
	GoogleURLInspectionDefaultDelay   = 110 * time.Millisecond
)

var GoogleDefaultSitemaps = []string{"sitemap-index.xml", "sitemap-all.txt"}

// The browser and API must address the same verified domain property.
func DefaultGoogleSiteURL() string {
	return "sc-domain:" + mustURL(DefaultSiteOrigin).Hostname()
}

type GoogleServiceAccount struct {
	ClientEmail string
	PrivateKey  string
	TokenURI    string
}

type GoogleAccessToken struct {
	AccessToken string
	TokenType   string
	ExpiresIn   int
}

type GoogleSitemapResult struct {
	SiteURL    string `json:"siteUrl"`
	FeedPath   string `json:"feedPath"`
	HTTPStatus int    `json:"httpStatus"`
}

type GoogleInspectionResult struct {
	URL             string   `json:"url"`
	Verdict         string   `json:"verdict"`
	CoverageState   string   `json:"coverageState"`
	RobotsTxtState  string   `json:"robotsTxtState"`
	IndexingState   string   `json:"indexingState"`
	LastCrawlTime   string   `json:"lastCrawlTime"`
	PageFetchState  string   `json:"pageFetchState"`
	UserCanonical   string   `json:"userCanonical"`
	GoogleCanonical string   `json:"googleCanonical"`
	CrawledAs       string   `json:"crawledAs"`
	ReferringURLs   []string `json:"referringUrls"`
	Sitemap         []string `json:"sitemap"`
}

type GoogleInspectionSummary struct {
	Total          int            `json:"total"`
	Verdicts       map[string]int `json:"verdicts"`
	CoverageStates map[string]int `json:"coverageStates"`
	WithSitemap    int            `json:"withSitemap"`
	WithoutSitemap int            `json:"withoutSitemap"`
	NeverCrawled   int            `json:"neverCrawled"`
}

type GoogleInspectionReport struct {
	SiteURL        string                   `json:"siteUrl"`
	Offset         int                      `json:"offset"`
	Limit          int                      `json:"limit"`
	Inspected      int                      `json:"inspected"`
	TotalAvailable int                      `json:"totalAvailable"`
	Remaining      int                      `json:"remaining"`
	NextOffset     *int                     `json:"nextOffset,omitempty"`
	Summary        GoogleInspectionSummary  `json:"summary"`
	Results        []GoogleInspectionResult `json:"results"`
}

type GoogleInspectionProgress struct {
	Type           string
	Offset         int
	Inspected      int
	RequestNumber  int
	AbsoluteIndex  int
	Duration       time.Duration
	TotalAvailable int
	Remaining      int
	NextOffset     *int
	URL            string
	Result         *GoogleInspectionResult
	Message        string
}

func ParseGoogleServiceAccount(raw string) (GoogleServiceAccount, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return GoogleServiceAccount{}, errors.New("Google service-account JSON is required")
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return GoogleServiceAccount{}, fmt.Errorf("invalid Google service-account JSON: %w", err)
	}
	if _, ok := probe["installed"]; ok {
		return GoogleServiceAccount{}, errors.New("Google credentials are OAuth Desktop Client credentials, not a Service Account JSON key")
	}
	if _, ok := probe["web"]; ok {
		return GoogleServiceAccount{}, errors.New("Google credentials are OAuth Web Client credentials, not a Service Account JSON key")
	}
	var payload struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		TokenURI    string `json:"token_uri"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return GoogleServiceAccount{}, err
	}
	if payload.Type != "" && payload.Type != "service_account" {
		return GoogleServiceAccount{}, fmt.Errorf("Google credentials type is %q, expected service_account", payload.Type)
	}
	payload.ClientEmail = strings.TrimSpace(payload.ClientEmail)
	payload.PrivateKey = strings.TrimSpace(payload.PrivateKey)
	payload.TokenURI = strings.TrimSpace(payload.TokenURI)
	if payload.TokenURI == "" {
		payload.TokenURI = GoogleDefaultTokenURI
	}
	missing := []string{}
	if payload.ClientEmail == "" {
		missing = append(missing, "client_email")
	}
	if payload.PrivateKey == "" {
		missing = append(missing, "private_key")
	}
	if len(missing) > 0 {
		return GoogleServiceAccount{}, fmt.Errorf("Google Service Account JSON is missing %s", strings.Join(missing, ", "))
	}
	tokenURL, err := url.Parse(payload.TokenURI)
	if err != nil || tokenURL.Scheme != "https" || tokenURL.Host == "" {
		return GoogleServiceAccount{}, errors.New("Google token_uri must use HTTPS")
	}
	if _, err := parseRSAPrivateKey(payload.PrivateKey); err != nil {
		return GoogleServiceAccount{}, fmt.Errorf("Google service-account private_key is invalid: %w", err)
	}
	return GoogleServiceAccount{
		ClientEmail: payload.ClientEmail,
		PrivateKey:  payload.PrivateKey,
		TokenURI:    tokenURL.String(),
	}, nil
}

func parseRSAPrivateKey(value string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		return nil, errors.New("PEM block was not found")
	}
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not RSA")
		}
		return key, nil
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func base64URL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func CreateGoogleServiceAccountAssertion(credentials GoogleServiceAccount, scope string, now time.Time, lifetime time.Duration) (string, error) {
	if scope == "" {
		scope = GoogleWebmastersScope
	}
	if lifetime <= 0 || lifetime > time.Hour {
		return "", errors.New("Google OAuth assertion lifetime must be between 1 second and 1 hour")
	}
	key, err := parseRSAPrivateKey(credentials.PrivateKey)
	if err != nil {
		return "", err
	}
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	issuedAt := now.Unix()
	claims, _ := json.Marshal(map[string]any{
		"iss":   credentials.ClientEmail,
		"scope": scope,
		"aud":   credentials.TokenURI,
		"iat":   issuedAt,
		"exp":   issuedAt + int64(lifetime/time.Second),
	})
	signingInput := base64URL(header) + "." + base64URL(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64URL(signature), nil
}

func FetchGoogleAccessToken(ctx context.Context, client *http.Client, credentials GoogleServiceAccount) (GoogleAccessToken, error) {
	if client == nil {
		client = http.DefaultClient
	}
	assertion, err := CreateGoogleServiceAccountAssertion(credentials, GoogleWebmastersScope, time.Now(), time.Hour)
	if err != nil {
		return GoogleAccessToken{}, err
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, credentials.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return GoogleAccessToken{}, err
	}
	request.Header.Set("content-type", "application/x-www-form-urlencoded; charset=utf-8")
	response, err := client.Do(request)
	if err != nil {
		return GoogleAccessToken{}, err
	}
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if readErr != nil {
		return GoogleAccessToken{}, readErr
	}
	var decoded struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		_ = json.Unmarshal(payload, &decoded)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || strings.TrimSpace(decoded.AccessToken) == "" {
		return GoogleAccessToken{}, fmt.Errorf("Google OAuth token exchange failed with HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if decoded.TokenType == "" {
		decoded.TokenType = "Bearer"
	}
	return GoogleAccessToken{AccessToken: decoded.AccessToken, TokenType: decoded.TokenType, ExpiresIn: decoded.ExpiresIn}, nil
}

func NormalizeSearchConsoleSiteURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = DefaultGoogleSiteURL()
	}
	if strings.HasPrefix(value, "sc-domain:") {
		host := strings.TrimSpace(strings.TrimPrefix(value, "sc-domain:"))
		if host == "" {
			return "", errors.New("Search Console domain property is empty")
		}
		return "sc-domain:" + host, nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("unsupported Search Console property protocol: %s", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("Search Console property host is required")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return parsed.String(), nil
}

func GoogleSiteOrigin(siteURL string) (string, error) {
	normalized, err := NormalizeSearchConsoleSiteURL(siteURL)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(normalized, "sc-domain:") {
		return NormalizeOrigin("https://" + strings.TrimPrefix(normalized, "sc-domain:"))
	}
	return NormalizeOrigin(normalized)
}

func googleAPIRequest(ctx context.Context, client *http.Client, method, endpoint, accessToken string, body []byte) ([]byte, int, error) {
	started := time.Now()
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("authorization", "Bearer "+accessToken)
	if body != nil {
		request.Header.Set("content-type", "application/json; charset=utf-8")
	}
	response, err := client.Do(request)
	if err != nil {
		slog.Warn("Google Search Console transport failed", "operation", "google-api", "method", method, "endpoint", endpoint, "durationMs", time.Since(started).Milliseconds())
		return nil, 0, err
	}
	slog.Info("Google Search Console response", "operation", "google-api", "method", method, "endpoint", endpoint, "httpStatus", response.StatusCode, "durationMs", time.Since(started).Milliseconds())
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	response.Body.Close()
	if readErr != nil {
		return nil, response.StatusCode, readErr
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return payload, response.StatusCode, fmt.Errorf("Google API request failed with HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	return payload, response.StatusCode, nil
}

func googlePathSegment(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func CheckGoogleSearchConsoleSite(ctx context.Context, client *http.Client, siteURL, accessToken string) (map[string]any, error) {
	normalized, err := NormalizeSearchConsoleSiteURL(siteURL)
	if err != nil {
		return nil, err
	}
	endpoint := GoogleSearchConsoleAPI + "/sites/" + googlePathSegment(normalized)
	payload, status, err := googleAPIRequest(ctx, client, http.MethodGet, endpoint, accessToken, nil)
	if err != nil {
		return nil, err
	}
	var decoded struct {
		PermissionLevel string `json:"permissionLevel"`
	}
	_ = json.Unmarshal(payload, &decoded)
	return map[string]any{
		"siteUrl":         normalized,
		"permissionLevel": decoded.PermissionLevel,
		"httpStatus":      status,
	}, nil
}

func SubmitGoogleSitemaps(ctx context.Context, client *http.Client, siteURL, origin, accessToken string, feeds ...string) ([]GoogleSitemapResult, error) {
	normalizedSite, err := NormalizeSearchConsoleSiteURL(siteURL)
	if err != nil {
		return nil, err
	}
	origin, err = NormalizeOrigin(origin)
	if err != nil {
		return nil, err
	}
	if len(feeds) == 0 {
		for _, name := range GoogleDefaultSitemaps {
			feeds = append(feeds, origin+"/"+name)
		}
	}
	for _, feed := range feeds {
		if _, err := NormalizeURL(feed, origin, "Sitemap URL"); err != nil {
			return nil, err
		}
	}
	results := make([]GoogleSitemapResult, 0, len(feeds))
	for _, feedPath := range feeds {
		endpoint := GoogleSearchConsoleAPI + "/sites/" + googlePathSegment(normalizedSite) + "/sitemaps/" + googlePathSegment(feedPath)
		_, status, err := googleAPIRequest(ctx, client, http.MethodPut, endpoint, accessToken, nil)
		if err != nil {
			return results, err
		}
		results = append(results, GoogleSitemapResult{SiteURL: normalizedSite, FeedPath: feedPath, HTTPStatus: status})
	}
	return results, nil
}

func InspectGoogleURL(ctx context.Context, client *http.Client, siteURL, inspectionURL, accessToken string, selectedOrigin ...string) (GoogleInspectionResult, error) {
	normalizedSite, err := NormalizeSearchConsoleSiteURL(siteURL)
	if err != nil {
		return GoogleInspectionResult{}, err
	}
	origin, err := GoogleSiteOrigin(normalizedSite)
	if err != nil {
		return GoogleInspectionResult{}, err
	}
	if len(selectedOrigin) > 1 {
		return GoogleInspectionResult{}, errors.New("exactly one Inspection origin is allowed")
	}
	if len(selectedOrigin) == 1 {
		candidate, err := NormalizeOrigin(selectedOrigin[0])
		if err != nil {
			return GoogleInspectionResult{}, err
		}
		propertyHost := mustURL(origin).Hostname()
		candidateHost := mustURL(candidate).Hostname()
		covered := candidate == origin
		if strings.HasPrefix(normalizedSite, "sc-domain:") {
			covered = candidateHost == propertyHost || strings.HasSuffix(candidateHost, "."+propertyHost)
		}
		if !covered {
			return GoogleInspectionResult{}, errors.New("Inspection origin is outside the configured Google property")
		}
		origin = candidate
	}
	inspectionURL, err = NormalizeURL(inspectionURL, origin, "Inspection URL")
	if err != nil {
		return GoogleInspectionResult{}, err
	}
	body, _ := json.Marshal(map[string]string{
		"inspectionUrl": inspectionURL,
		"siteUrl":       normalizedSite,
		"languageCode":  "en-US",
	})
	payload, _, err := googleAPIRequest(ctx, client, http.MethodPost, GoogleURLInspectionAPI, accessToken, body)
	if err != nil {
		return GoogleInspectionResult{}, err
	}
	var decoded struct {
		InspectionResult struct {
			IndexStatusResult struct {
				Verdict         string   `json:"verdict"`
				CoverageState   string   `json:"coverageState"`
				RobotsTxtState  string   `json:"robotsTxtState"`
				IndexingState   string   `json:"indexingState"`
				LastCrawlTime   string   `json:"lastCrawlTime"`
				PageFetchState  string   `json:"pageFetchState"`
				UserCanonical   string   `json:"userCanonical"`
				GoogleCanonical string   `json:"googleCanonical"`
				CrawledAs       string   `json:"crawledAs"`
				ReferringURLs   []string `json:"referringUrls"`
				Sitemap         []string `json:"sitemap"`
			} `json:"indexStatusResult"`
		} `json:"inspectionResult"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return GoogleInspectionResult{}, fmt.Errorf("Google URL inspection returned invalid JSON: %w", err)
	}
	index := decoded.InspectionResult.IndexStatusResult
	return GoogleInspectionResult{
		URL:             inspectionURL,
		Verdict:         index.Verdict,
		CoverageState:   index.CoverageState,
		RobotsTxtState:  index.RobotsTxtState,
		IndexingState:   index.IndexingState,
		LastCrawlTime:   index.LastCrawlTime,
		PageFetchState:  index.PageFetchState,
		UserCanonical:   index.UserCanonical,
		GoogleCanonical: index.GoogleCanonical,
		CrawledAs:       index.CrawledAs,
		ReferringURLs:   append([]string{}, index.ReferringURLs...),
		Sitemap:         append([]string{}, index.Sitemap...),
	}, nil
}

func SummarizeGoogleInspection(results []GoogleInspectionResult) GoogleInspectionSummary {
	summary := GoogleInspectionSummary{
		Total:          len(results),
		Verdicts:       map[string]int{},
		CoverageStates: map[string]int{},
	}
	for _, result := range results {
		verdict := result.Verdict
		if verdict == "" {
			verdict = "UNKNOWN"
		}
		coverage := result.CoverageState
		if coverage == "" {
			coverage = "UNKNOWN"
		}
		summary.Verdicts[verdict]++
		summary.CoverageStates[coverage]++
		if len(result.Sitemap) > 0 {
			summary.WithSitemap++
		} else {
			summary.WithoutSitemap++
		}
		if result.LastCrawlTime == "" {
			summary.NeverCrawled++
		}
	}
	return summary
}

func AuditGoogleURLs(ctx context.Context, client *http.Client, urls []string, siteURL, origin, accessToken string, offset, limit int, requestDelay time.Duration, onProgress func(GoogleInspectionProgress) error) (GoogleInspectionReport, error) {
	if offset < 0 {
		return GoogleInspectionReport{}, errors.New("Google audit offset must be non-negative")
	}
	if limit <= 0 || limit > GoogleURLInspectionDailySiteLimit {
		return GoogleInspectionReport{}, fmt.Errorf("Google audit limit must be between 1 and %d", GoogleURLInspectionDailySiteLimit)
	}
	if requestDelay < 0 {
		return GoogleInspectionReport{}, errors.New("Google audit request delay must be non-negative")
	}
	origin, err := NormalizeOrigin(origin)
	if err != nil {
		return GoogleInspectionReport{}, err
	}
	end := offset + limit
	if end > len(urls) {
		end = len(urls)
	}
	if offset > len(urls) {
		offset = len(urls)
	}
	selected := make([]string, 0, end-offset)
	for index := offset; index < end; index++ {
		normalized, err := NormalizeURL(urls[index], origin, "Inspection URL")
		if err != nil {
			return GoogleInspectionReport{}, err
		}
		selected = append(selected, normalized)
	}

	results := make([]GoogleInspectionResult, 0, len(selected))
	for index, inspectionURL := range selected {
		requestNumber := index + 1
		absoluteIndex := offset + requestNumber
		if onProgress != nil {
			if err := onProgress(GoogleInspectionProgress{
				Type: "request_start", Offset: offset, Inspected: index,
				RequestNumber: requestNumber, AbsoluteIndex: absoluteIndex,
				TotalAvailable: len(urls), URL: inspectionURL,
				Message: "开始检查 " + strconv.Itoa(absoluteIndex) + " / " + strconv.Itoa(len(urls)),
			}); err != nil {
				return GoogleInspectionReport{}, err
			}
		}
		started := time.Now()
		result, err := InspectGoogleURL(ctx, client, siteURL, inspectionURL, accessToken, origin)
		if err != nil {
			return GoogleInspectionReport{}, err
		}
		results = append(results, result)
		inspected := index + 1
		next := offset + inspected
		var nextOffset *int
		if next < len(urls) {
			value := next
			nextOffset = &value
		}
		if onProgress != nil {
			copyResult := result
			if err := onProgress(GoogleInspectionProgress{
				Type: "request_complete", Offset: offset, Inspected: inspected,
				RequestNumber: requestNumber, AbsoluteIndex: absoluteIndex,
				Duration: time.Since(started), TotalAvailable: len(urls),
				Remaining: maxInt(0, len(urls)-next), NextOffset: nextOffset,
				Result:  &copyResult,
				Message: "完成 " + strconv.Itoa(absoluteIndex) + " / " + strconv.Itoa(len(urls)),
			}); err != nil {
				return GoogleInspectionReport{}, err
			}
		}
		if requestDelay > 0 && index+1 < len(selected) {
			timer := time.NewTimer(requestDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return GoogleInspectionReport{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	next := offset + len(results)
	var nextOffset *int
	if next < len(urls) {
		value := next
		nextOffset = &value
	}
	return GoogleInspectionReport{
		SiteURL:        mustNormalizeSearchConsoleSiteURL(siteURL),
		Offset:         offset,
		Limit:          limit,
		Inspected:      len(results),
		TotalAvailable: len(urls),
		Remaining:      maxInt(0, len(urls)-next),
		NextOffset:     nextOffset,
		Summary:        SummarizeGoogleInspection(results),
		Results:        results,
	}, nil
}

func mustNormalizeSearchConsoleSiteURL(value string) string {
	normalized, _ := NormalizeSearchConsoleSiteURL(value)
	return normalized
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DefaultSiteOrigin          = "https://thinkerqaq.com"
	DefaultSitemapIndex        = "sitemap-index.xml"
	DefaultTextSitemap         = "sitemap-all.txt"
	DefaultFingerprintManifest = "sitemap-inventory.json"
	MaxTextSitemapURLs         = 50_000
)

type Inventory struct {
	Source              string            `json:"source,omitempty"`
	FingerprintSource   string            `json:"fingerprintSource,omitempty"`
	FingerprintCoverage int               `json:"fingerprintCoverage,omitempty"`
	Origin              string            `json:"origin"`
	FetchedAt           string            `json:"fetchedAt,omitempty"`
	Total               int               `json:"total"`
	URLs                []string          `json:"urls,omitempty"`
	Fingerprints        map[string]string `json:"fingerprints,omitempty"`
}

type GeneratedInventory struct {
	Origin          string
	SitemapIndexURL string
	SitemapURLs     []string
	URLList         []string
}

type FingerprintManifest struct {
	Version      int               `json:"version"`
	Origin       string            `json:"origin"`
	GeneratedAt  string            `json:"generatedAt"`
	Fingerprints map[string]string `json:"fingerprints"`
}

type sitemapDocument struct {
	Locations []string `xml:"loc"`
	Sitemaps  []struct {
		Location string `xml:"loc"`
	} `xml:"sitemap"`
	URLs []struct {
		Location string `xml:"loc"`
	} `xml:"url"`
}

func NormalizeOrigin(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		value = DefaultSiteOrigin
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported site protocol: %s", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("site origin host is required")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func NormalizeURL(rawURL, origin, label string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("%s is invalid: %w", label, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%s must be absolute: %s", label, rawURL)
	}
	if parsed.Scheme+"://"+parsed.Host != origin {
		return "", fmt.Errorf("%s must use %s: %s", label, origin, rawURL)
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func parseSitemapLocations(data []byte) ([]string, error) {
	var doc sitemapDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	result := append([]string{}, doc.Locations...)
	for _, item := range doc.Sitemaps {
		result = append(result, item.Location)
	}
	for _, item := range doc.URLs {
		result = append(result, item.Location)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(result))
	for _, raw := range result {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}

func safePath(root, relative, label string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(filepath.Join(root, relative))
	if err != nil {
		return "", err
	}
	prefix := root + string(os.PathSeparator)
	if candidate != root && !strings.HasPrefix(candidate, prefix) {
		return "", fmt.Errorf("%s escapes root: %s", label, relative)
	}
	return candidate, nil
}

func sitemapFileForURL(distRoot, rawURL, origin string) (string, error) {
	normalized, err := NormalizeURL(rawURL, origin, "sitemap URL")
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(normalized)
	decoded, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", err
	}
	relative := strings.TrimLeft(decoded, "/")
	if relative == "" {
		return "", fmt.Errorf("sitemap URL does not identify a file: %s", rawURL)
	}
	return safePath(distRoot, filepath.FromSlash(relative), "sitemap path")
}

func LoadGeneratedInventory(distRoot, sitemapIndex, expectedOrigin string) (GeneratedInventory, error) {
	if sitemapIndex == "" {
		sitemapIndex = DefaultSitemapIndex
	}
	indexPath, err := safePath(distRoot, sitemapIndex, "sitemap index")
	if err != nil {
		return GeneratedInventory{}, err
	}
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return GeneratedInventory{}, err
	}
	sitemapURLs, err := parseSitemapLocations(indexData)
	if err != nil {
		return GeneratedInventory{}, err
	}
	if len(sitemapURLs) == 0 {
		return GeneratedInventory{}, fmt.Errorf("no child sitemaps were found in %s", sitemapIndex)
	}

	origin := expectedOrigin
	if strings.TrimSpace(origin) == "" {
		first, err := url.Parse(sitemapURLs[0])
		if err != nil {
			return GeneratedInventory{}, err
		}
		origin = first.Scheme + "://" + first.Host
	}
	origin, err = NormalizeOrigin(origin)
	if err != nil {
		return GeneratedInventory{}, err
	}

	urls := map[string]struct{}{}
	normalizedSitemaps := make([]string, 0, len(sitemapURLs))
	for _, sitemapURL := range sitemapURLs {
		normalized, err := NormalizeURL(sitemapURL, origin, "sitemap URL")
		if err != nil {
			return GeneratedInventory{}, err
		}
		normalizedSitemaps = append(normalizedSitemaps, normalized)
		file, err := sitemapFileForURL(distRoot, normalized, origin)
		if err != nil {
			return GeneratedInventory{}, err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return GeneratedInventory{}, err
		}
		locations, err := parseSitemapLocations(data)
		if err != nil {
			return GeneratedInventory{}, err
		}
		for _, raw := range locations {
			normalizedURL, err := NormalizeURL(raw, origin, "page URL")
			if err != nil {
				return GeneratedInventory{}, err
			}
			urls[normalizedURL] = struct{}{}
		}
	}
	if len(urls) == 0 {
		return GeneratedInventory{}, errors.New("no page URLs were found in generated sitemaps")
	}
	urlList := make([]string, 0, len(urls))
	for value := range urls {
		urlList = append(urlList, value)
	}
	sort.Strings(urlList)
	sort.Strings(normalizedSitemaps)
	return GeneratedInventory{
		Origin:          origin,
		SitemapIndexURL: origin + "/" + strings.TrimLeft(filepath.ToSlash(sitemapIndex), "/"),
		SitemapURLs:     normalizedSitemaps,
		URLList:         urlList,
	}, nil
}

func pageFileForURL(distRoot, rawURL, origin string) (string, error) {
	normalized, err := NormalizeURL(rawURL, origin, "page URL")
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(normalized)
	decoded, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", err
	}
	relative := strings.TrimLeft(decoded, "/")
	candidates := []string{}
	switch {
	case decoded == "/":
		candidates = []string{"index.html"}
	case strings.HasSuffix(decoded, "/"):
		candidates = []string{filepath.Join(filepath.FromSlash(relative), "index.html")}
	default:
		candidates = []string{filepath.FromSlash(relative) + ".html", filepath.Join(filepath.FromSlash(relative), "index.html")}
	}
	for _, candidate := range candidates {
		file, err := safePath(distRoot, candidate, "page path")
		if err != nil {
			return "", err
		}
		if info, err := os.Stat(file); err == nil && !info.IsDir() {
			return file, nil
		}
	}
	return "", fmt.Errorf("generated page file was not found for %s", rawURL)
}

func BuildFingerprintManifest(distRoot string, inventory GeneratedInventory, now time.Time) (FingerprintManifest, error) {
	fingerprints := make(map[string]string, len(inventory.URLList))
	for _, rawURL := range inventory.URLList {
		file, err := pageFileForURL(distRoot, rawURL, inventory.Origin)
		if err != nil {
			return FingerprintManifest{}, err
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return FingerprintManifest{}, err
		}
		digest := sha256.Sum256(data)
		fingerprints[rawURL] = hex.EncodeToString(digest[:])
	}
	return FingerprintManifest{
		Version:      1,
		Origin:       inventory.Origin,
		GeneratedAt:  now.UTC().Format(time.RFC3339Nano),
		Fingerprints: fingerprints,
	}, nil
}

func WriteGeneratedInventory(distRoot, textOutput, fingerprintOutput string, now time.Time) (GeneratedInventory, error) {
	inventory, err := LoadGeneratedInventory(distRoot, DefaultSitemapIndex, "")
	if err != nil {
		return GeneratedInventory{}, err
	}
	if len(inventory.URLList) > MaxTextSitemapURLs {
		return GeneratedInventory{}, fmt.Errorf("text sitemap has %d URLs; split it before exceeding %d", len(inventory.URLList), MaxTextSitemapURLs)
	}
	if textOutput == "" {
		textOutput = DefaultTextSitemap
	}
	textPath, err := safePath(distRoot, textOutput, "text sitemap output")
	if err != nil {
		return GeneratedInventory{}, err
	}
	if err := os.WriteFile(textPath, []byte(strings.Join(inventory.URLList, "\n")+"\n"), 0o644); err != nil {
		return GeneratedInventory{}, err
	}

	manifest, err := BuildFingerprintManifest(distRoot, inventory, now)
	if err != nil {
		return GeneratedInventory{}, err
	}
	if fingerprintOutput == "" {
		fingerprintOutput = DefaultFingerprintManifest
	}
	manifestPath, err := safePath(distRoot, fingerprintOutput, "fingerprint manifest output")
	if err != nil {
		return GeneratedInventory{}, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return GeneratedInventory{}, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return GeneratedInventory{}, err
	}
	return inventory, nil
}

func ParseURLList(text, origin string) ([]string, error) {
	origin, err := NormalizeOrigin(origin)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, line := range strings.Split(text, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		normalized, err := NormalizeURL(raw, origin, "submitted URL")
		if err != nil {
			return nil, err
		}
		seen[normalized] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func ReadURLFile(file, origin string) ([]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return ParseURLList(string(data), origin)
}

func FetchRemoteInventory(ctx context.Context, client *http.Client, origin string, configured ...SiteConfig) (Inventory, error) {
	if client == nil {
		client = http.DefaultClient
	}
	origin, err := NormalizeOrigin(origin)
	if err != nil {
		return Inventory{}, err
	}
	site := DefaultSiteConfig(origin)
	if len(configured) > 1 {
		return Inventory{}, errors.New("exactly one search site configuration is allowed")
	}
	if len(configured) == 1 {
		site = configured[0]
	}
	site, err = NormalizeSiteConfig(site)
	if err != nil {
		return Inventory{}, err
	}
	if site.Origin != origin {
		return Inventory{}, errors.New("search site configuration must match inventory origin")
	}
	source := site.URLListURL
	fingerprintSource := site.FingerprintURL

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return Inventory{}, err
	}
	request.Header.Set("accept", "text/plain,*/*;q=0.8")
	response, err := client.Do(request)
	if err != nil {
		return Inventory{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	response.Body.Close()
	if readErr != nil {
		return Inventory{}, readErr
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Inventory{}, fmt.Errorf("sitemap-all.txt fetch failed with HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	urls, err := ParseURLList(string(data), origin)
	if err != nil {
		return Inventory{}, err
	}
	if len(urls) == 0 {
		return Inventory{}, errors.New("sitemap-all.txt did not contain any valid URLs")
	}

	fingerprints := map[string]string{}
	resolvedFingerprintSource := ""
	fingerprintRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, fingerprintSource, nil)
	if fingerprintSource != "" && err == nil {
		fingerprintRequest.Header.Set("accept", "application/json,*/*;q=0.8")
		if fingerprintResponse, requestErr := client.Do(fingerprintRequest); requestErr == nil {
			payload, payloadErr := io.ReadAll(io.LimitReader(fingerprintResponse.Body, 32<<20))
			fingerprintResponse.Body.Close()
			if payloadErr == nil && fingerprintResponse.StatusCode >= 200 && fingerprintResponse.StatusCode < 300 {
				var manifest FingerprintManifest
				if json.Unmarshal(payload, &manifest) == nil {
					manifestOrigin, originErr := NormalizeOrigin(manifest.Origin)
					if originErr == nil && manifestOrigin == origin {
						allowed := make(map[string]struct{}, len(urls))
						for _, item := range urls {
							allowed[item] = struct{}{}
						}
						valid := true
						for rawURL, rawHash := range manifest.Fingerprints {
							normalized, urlErr := NormalizeURL(rawURL, origin, "fingerprint URL")
							hash := strings.ToLower(strings.TrimSpace(rawHash))
							if urlErr != nil || len(hash) != 64 {
								valid = false
								break
							}
							if _, decodeErr := hex.DecodeString(hash); decodeErr != nil {
								valid = false
								break
							}
							if _, ok := allowed[normalized]; ok {
								fingerprints[normalized] = hash
							}
						}
						if valid {
							resolvedFingerprintSource = fingerprintSource
						} else {
							fingerprints = map[string]string{}
						}
					}
				}
			}
		}
	}

	return Inventory{
		Source:              source,
		FingerprintSource:   resolvedFingerprintSource,
		FingerprintCoverage: len(fingerprints),
		Origin:              origin,
		FetchedAt:           time.Now().UTC().Format(time.RFC3339Nano),
		Total:               len(urls),
		URLs:                urls,
		Fingerprints:        fingerprints,
	}, nil
}

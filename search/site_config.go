package search

import (
	"fmt"
	"net/url"
	"strings"
)

// SiteConfig is the user-owned identity and public resources shared by providers.
type SiteConfig struct {
	Origin         string `json:"origin" toml:"site_origin"`
	URLListURL     string `json:"urlListURL" toml:"url_list_url"`
	SitemapURL     string `json:"sitemapURL" toml:"sitemap_url"`
	FingerprintURL string `json:"fingerprintURL" toml:"fingerprint_url"`
	GoogleProperty string `json:"googleProperty" toml:"google_property"`
}

func DefaultSiteConfig(origin string) SiteConfig {
	return SiteConfig{
		Origin: origin, URLListURL: origin + "/" + DefaultTextSitemap,
		SitemapURL:     origin + "/" + DefaultSitemapIndex,
		FingerprintURL: origin + "/" + DefaultFingerprintManifest,
		GoogleProperty: "sc-domain:" + mustURL(origin).Hostname(),
	}
}

func NormalizeSiteConfig(config SiteConfig) (SiteConfig, error) {
	parsed, err := url.Parse(strings.TrimSpace(config.Origin))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return config, fmt.Errorf("search.site_origin must be an HTTPS site origin without credentials, path, query or fragment")
	}
	config.Origin = parsed.Scheme + "://" + parsed.Host
	for _, field := range []struct {
		path     string
		value    *string
		optional bool
	}{
		{"search.url_list_url", &config.URLListURL, false},
		{"search.sitemap_url", &config.SitemapURL, false},
		{"search.fingerprint_url", &config.FingerprintURL, true},
	} {
		*field.value = strings.TrimSpace(*field.value)
		if field.optional && *field.value == "" {
			continue
		}
		resource, err := url.Parse(*field.value)
		if err != nil || resource.User != nil || resource.Fragment != "" || resource.Path == "" || resource.Path == "/" {
			return config, fmt.Errorf("%s must be a public resource URL without credentials or fragment", field.path)
		}
		normalized, err := NormalizeURL(*field.value, config.Origin, field.path)
		if err != nil {
			return config, err
		}
		*field.value = normalized
	}
	if config.URLListURL == config.SitemapURL {
		return config, fmt.Errorf("search.url_list_url and search.sitemap_url must identify different TXT and XML resources")
	}
	if strings.TrimSpace(config.GoogleProperty) == "" {
		return config, fmt.Errorf("search.google_property is required")
	}
	config.GoogleProperty, err = NormalizeSearchConsoleSiteURL(config.GoogleProperty)
	if err != nil {
		return config, fmt.Errorf("search.google_property: %w", err)
	}
	if strings.HasPrefix(config.GoogleProperty, "sc-domain:") {
		domain := strings.TrimPrefix(config.GoogleProperty, "sc-domain:")
		if domain == "" || strings.ContainsAny(domain, "/:@?# ") || (parsed.Hostname() != domain && !strings.HasSuffix(parsed.Hostname(), "."+domain)) {
			return config, fmt.Errorf("search.google_property must cover search.site_origin")
		}
	} else if config.GoogleProperty != config.Origin+"/" {
		return config, fmt.Errorf("search.google_property must match search.site_origin or be its domain property")
	}
	return config, nil
}

func (config SiteConfig) ConsoleLinks() map[string]string {
	return map[string]string{
		"indexnow": "https://www.bing.com/webmasters/home?siteUrl=" + url.QueryEscape(config.Origin+"/"),
		"baidu":    "https://ziyuan.baidu.com/linksubmit/index?site=" + url.QueryEscape(config.Origin),
		"google":   "https://search.google.com/search-console?resource_id=" + url.QueryEscape(config.GoogleProperty),
		"so360":    "https://zhanzhang.so.com/sitetool/site_manage",
		"sogou":    "https://zhanzhang.sogou.com/index.php/dashboard/index",
	}
}

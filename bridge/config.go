package bridge

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	blogpublishing "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publishing"
	blogsearch "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/search"
	"github.com/pelletier/go-toml/v2"
)

type publishingFooterConfig = blogpublishing.FooterConfig
type publishingCanonicalConfig = blogpublishing.CanonicalConfig
type publishingTrackingConfig = blogpublishing.TrackingConfig
type publishingPlatformConfig = blogpublishing.PlatformConfig
type publishingMermaidConfig = blogpublishing.MermaidConfig
type publishingCompilerConfig = blogpublishing.CompilerConfig
type publishingR2Config = blogpublishing.R2Config
type publishingAssetsConfig = blogpublishing.AssetsConfig
type publishingConfig = blogpublishing.Config

type SocialConfig struct {
	Buffer struct {
		APIKey string `json:"-" toml:"api_key"`
	} `json:"buffer" toml:"buffer"`
}

// ResolvedSocialHTTPClient uses the same proxy rules as other BlogCTL publishers.
func ResolvedSocialHTTPClient() (*http.Client, error) {
	config, err := loadBridgeConfigChecked()
	if err != nil {
		return nil, err
	}
	return httpClientForConfig(config)
}

func ResolvedSocialConfig() SocialConfig {
	config := loadBridgeConfig().Social
	if config.Buffer.APIKey == "" {
		config.Buffer.APIKey = strings.TrimSpace(os.Getenv("BUFFER_API_KEY"))
	}
	return config
}

type bridgeConfig struct {
	Search       *blogsearch.SiteConfig `json:"search" toml:"search"`
	ProxyEnabled bool                   `json:"proxyEnabled" toml:"proxy_enabled"`
	ProxyHost    string                 `json:"proxyHost" toml:"proxy_host"`
	ProxyPort    int                    `json:"proxyPort" toml:"proxy_port"`

	LogDirectory string `json:"logDirectory,omitempty" toml:"log_directory"`
	LogLevel     string `json:"logLevel,omitempty" toml:"log_level"`
	UILocale     string `json:"uiLocale" toml:"ui_locale"`

	ContentRoot                    string            `json:"contentRoot" toml:"content_root"`
	ImageUploadRoots               []string          `json:"-" toml:"image_upload_roots"`
	EngineRoot                     string            `json:"engineRoot" toml:"engine_root"`
	DistributionRoot               string            `json:"distributionRoot" toml:"distribution_root"`
	PublicationBindingsPath        string            `json:"publicationBindingsPath" toml:"publication_bindings_path"`
	ToolPaths                      map[string]string `json:"toolPaths" toml:"tool_paths"`
	DevtoAPIKey                    string            `json:"devtoApiKey,omitempty" toml:"devto_api_key"`
	IndexNowEndpoint               string            `json:"indexNowEndpoint,omitempty" toml:"indexnow_endpoint"`
	IndexNowKey                    string            `json:"indexNowKey,omitempty" toml:"indexnow_key"`
	IndexNowKeyLocation            string            `json:"indexNowKeyLocation,omitempty" toml:"indexnow_key_location"`
	BaiduToken                     string            `json:"baiduToken,omitempty" toml:"baidu_token"`
	GoogleSearchConsoleServiceJSON string            `json:"googleSearchConsoleServiceJson,omitempty" toml:"google_search_console_service_json"`
	Publishing                     publishingConfig  `json:"publishing" toml:"publishing"`
	Social                         SocialConfig      `json:"-" toml:"social"`
}

var publishingPlatformOrder = blogpublishing.PlatformIDs()

func defaultFooterTemplate(language string) string {
	return blogpublishing.DefaultFooterTemplate(language)
}

func defaultPlatformPublishingConfig(platform string) publishingPlatformConfig {
	return blogpublishing.DefaultPlatformConfig(platform)
}

// UILocalePreference is shared by the CLI, Extension and Web Console.
// It controls presentation only, never the language of syndicated content.
func UILocalePreference() string {
	return loadBridgeConfig().UILocale
}

func defaultPublishingConfig() publishingConfig {
	return blogpublishing.DefaultConfig()
}

func defaultBridgeConfig() bridgeConfig {
	site := blogsearch.DefaultSiteConfig(blogsearch.DefaultSiteOrigin)
	distributionRoot, _ := DistributionRoot()
	publicationBindingsPath, _ := PublicationBindingsPath()
	return bridgeConfig{
		Search:                  &site,
		LogLevel:                "info",
		UILocale:                "auto",
		ToolPaths:               map[string]string{},
		DistributionRoot:        distributionRoot,
		PublicationBindingsPath: publicationBindingsPath,
		IndexNowEndpoint:        "https://www.bing.com/indexnow",
		Publishing:              defaultPublishingConfig(),
	}
}

func ResolvedPublishingConfig() publishingConfig {
	return loadBridgeConfig().Publishing
}

func ResolvedDistributionRoot() string {
	return loadBridgeConfig().DistributionRoot
}

type SearchRuntimeConfig struct {
	Site                     blogsearch.SiteConfig
	IndexNowEndpoint         string
	IndexNowKey              string
	IndexNowKeyLocation      string
	BaiduToken               string
	GoogleServiceAccountJSON string
}

func ResolvedSearchRuntimeConfig() (SearchRuntimeConfig, error) {
	config, err := loadBridgeConfigChecked()
	if err != nil {
		return SearchRuntimeConfig{}, err
	}
	return SearchRuntimeConfig{
		Site:                     *config.Search,
		IndexNowEndpoint:         strings.TrimSpace(config.IndexNowEndpoint),
		IndexNowKey:              strings.TrimSpace(config.IndexNowKey),
		IndexNowKeyLocation:      strings.TrimSpace(config.IndexNowKeyLocation),
		BaiduToken:               strings.TrimSpace(config.BaiduToken),
		GoogleServiceAccountJSON: strings.TrimSpace(config.GoogleSearchConsoleServiceJSON),
	}, nil
}

func defaultConfigDir() (string, error) {
	if runtime.GOOS == "windows" {
		executable, err := os.Executable()
		if err != nil {
			return "", err
		}
		return filepath.Join(filepath.Dir(executable), "Data"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "BlogCTL"), nil
}

// ConfigDir returns the single directory used by configuration, logs, and runtime state.
// A data directory override is supported for isolated tests and custom deployments.
func ConfigDir() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("BLOGCTL_DATA_DIR")); configured != "" {
		if !filepath.IsAbs(configured) {
			return "", errors.New("BLOGCTL_DATA_DIR must be an absolute path")
		}
		return filepath.Clean(configured), nil
	}
	return defaultConfigDir()
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "blogctl.toml"), nil
}

func DistributionRoot() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "distribution"), nil
}

func PublicationBindingsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "publications.json"), nil
}

func bridgeConfigPath() (string, error) { return ConfigPath() }

func normalizeStoredPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return filepath.Clean(value)
}

func mergeConfigDefaults(config bridgeConfig) bridgeConfig {
	defaults := defaultBridgeConfig()
	if config.Search == nil {
		config.Search = defaults.Search
	}
	if config.IndexNowEndpoint == "" {
		config.IndexNowEndpoint = defaults.IndexNowEndpoint
	}
	if strings.TrimSpace(config.UILocale) == "" {
		config.UILocale = "auto"
	}
	if strings.TrimSpace(config.LogLevel) == "" {
		config.LogLevel = defaults.LogLevel
	}
	if strings.TrimSpace(config.DistributionRoot) == "" {
		config.DistributionRoot = defaults.DistributionRoot
	}
	if strings.TrimSpace(config.PublicationBindingsPath) == "" {
		config.PublicationBindingsPath = defaults.PublicationBindingsPath
	}
	if config.ToolPaths == nil {
		config.ToolPaths = map[string]string{}
	}
	if config.Publishing.Platforms == nil {
		config.Publishing.Platforms = map[string]publishingPlatformConfig{}
	}
	for platform, value := range defaults.Publishing.Platforms {
		configured, ok := config.Publishing.Platforms[platform]
		if !ok {
			config.Publishing.Platforms[platform] = value
			continue
		}
		if configured.Language == "" {
			configured.Language = value.Language
		}
		if configured.Footer.Template == "" {
			configured.Footer.Template = defaultFooterTemplate(configured.Language)
		}
		if configured.Canonical.Mode == "" {
			configured.Canonical.Mode = value.Canonical.Mode
		}
		if configured.Tracking.Source == "" {
			configured.Tracking.Source = value.Tracking.Source
		}
		if configured.Tracking.Medium == "" {
			configured.Tracking.Medium = value.Tracking.Medium
		}
		if configured.Tracking.Campaign == "" {
			configured.Tracking.Campaign = value.Tracking.Campaign
		}
		config.Publishing.Platforms[platform] = configured
	}
	return config
}

func normalizeBridgeConfig(config bridgeConfig) (bridgeConfig, error) {
	config = mergeConfigDefaults(config)
	site, err := blogsearch.NormalizeSiteConfig(*config.Search)
	if err != nil {
		return config, err
	}
	config.Search = &site
	config.ContentRoot = normalizeStoredPath(config.ContentRoot)
	config.EngineRoot = normalizeStoredPath(config.EngineRoot)
	if len(config.ImageUploadRoots) == 0 {
		config.ImageUploadRoots = nil
	}
	if len(config.ImageUploadRoots) > 16 {
		return config, errors.New("too many image_upload_roots")
	}
	for i, root := range config.ImageUploadRoots {
		root = normalizeStoredPath(root)
		if root == "" || !filepath.IsAbs(root) {
			return config, errors.New("image_upload_roots must contain absolute paths")
		}
		config.ImageUploadRoots[i] = root
	}

	config.DistributionRoot = normalizeStoredPath(config.DistributionRoot)
	config.PublicationBindingsPath = normalizeStoredPath(config.PublicationBindingsPath)
	config.DevtoAPIKey = strings.TrimSpace(config.DevtoAPIKey)
	config.Social.Buffer.APIKey = strings.TrimSpace(config.Social.Buffer.APIKey)
	config.IndexNowEndpoint = strings.TrimSpace(config.IndexNowEndpoint)
	config.IndexNowKey = strings.TrimSpace(config.IndexNowKey)
	config.IndexNowKeyLocation = strings.TrimSpace(config.IndexNowKeyLocation)
	config.BaiduToken = strings.TrimSpace(config.BaiduToken)
	config.GoogleSearchConsoleServiceJSON = strings.TrimSpace(config.GoogleSearchConsoleServiceJSON)
	config.LogDirectory = normalizeStoredPath(config.LogDirectory)
	config.UILocale = strings.TrimSpace(config.UILocale)
	switch config.UILocale {
	case "auto", "zh-CN", "en":
	default:
		return config, errors.New("ui_locale must be auto, zh-CN, or en")
	}
	config.LogLevel = strings.ToLower(strings.TrimSpace(config.LogLevel))
	switch config.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return config, errors.New("log level must be debug, info, warn, or error")
	}
	for name, value := range config.ToolPaths {
		config.ToolPaths[name] = normalizeStoredPath(value)
	}

	config.ProxyHost = strings.TrimSpace(config.ProxyHost)
	_, host, port, err := normalizeProxyAddress(config.ProxyHost, config.ProxyPort)
	if err != nil {
		return config, err
	}
	config.ProxyHost = host
	config.ProxyPort = port
	if config.ProxyEnabled && config.ProxyHost == "" {
		return config, errors.New("启用代理前请填写代理主机和端口")
	}
	if config.IndexNowEndpoint != "" {
		endpoint, err := url.Parse(config.IndexNowEndpoint)
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
			return config, errors.New("IndexNow endpoint must be an HTTPS URL")
		}
	}
	if config.IndexNowKeyLocation != "" {
		keyLocation, err := url.Parse(config.IndexNowKeyLocation)
		if err != nil || keyLocation.Scheme != "https" || keyLocation.Host == "" {
			return config, errors.New("IndexNow key location must be an HTTPS URL")
		}
	}
	normalizedPublishing, err := blogpublishing.Normalize(config.Publishing)
	if err != nil {
		return config, err
	}
	config.Publishing = normalizedPublishing
	return config, nil
}

func loadBridgeConfig() bridgeConfig {
	config, err := loadBridgeConfigChecked()
	if err != nil {
		return defaultBridgeConfig()
	}
	return config
}

func loadBridgeConfigChecked() (bridgeConfig, error) {
	path, err := bridgeConfigPath()
	if err != nil {
		return bridgeConfig{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultBridgeConfig(), nil
		}
		return bridgeConfig{}, err
	}
	var config bridgeConfig
	var document map[string]any
	if err := toml.Unmarshal(data, &document); err != nil {
		return config, fmt.Errorf("%s: %w", path, err)
	}
	if section, ok := document["search"].(map[string]any); ok {
		allowed := map[string]bool{"site_origin": true, "url_list_url": true, "sitemap_url": true, "fingerprint_url": true, "google_property": true}
		for key := range section {
			if !allowed[key] {
				return config, fmt.Errorf("%s: unknown configuration field search.%s", path, key)
			}
		}
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("%s: %w", path, err)
	}
	normalized, err := normalizeBridgeConfig(config)
	if err != nil {
		return config, fmt.Errorf("%s: %w", path, err)
	}
	return normalized, nil
}

func persistSearchConfigMigration(config bridgeConfig) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return saveBridgeConfig(config)
	}
	if err != nil {
		return err
	}
	var document map[string]any
	if err := toml.Unmarshal(data, &document); err != nil {
		return err
	}
	_, hasSearch := document["search"]
	_, hasLegacySite := document["baidu_site"]
	if !hasSearch || hasLegacySite {
		return saveBridgeConfig(config)
	}
	return nil
}

func saveBridgeConfig(config bridgeConfig) error {
	normalized, err := normalizeBridgeConfig(config)
	if err != nil {
		return err
	}
	path, err := bridgeConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := toml.Marshal(normalized)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func UpdateWorkspaceRoots(contentRoot, engineRoot string) error {
	config := loadBridgeConfig()
	config.ContentRoot = contentRoot
	config.EngineRoot = engineRoot
	return saveBridgeConfig(config)
}

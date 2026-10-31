package publishing

import (
	"errors"
	"net/url"
	"strings"

	blogplatform "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/platform"
)

const DefaultAssetBaseURL = "https://pub-366a15b6733345039775c083a1fffb3e.r2.dev/"

type FooterConfig struct {
	Enabled  bool   `json:"enabled" toml:"enabled"`
	Template string `json:"template" toml:"template"`
}

type CanonicalConfig struct {
	Mode string `json:"mode" toml:"mode"`
}

type TrackingConfig struct {
	Enabled  bool   `json:"enabled" toml:"enabled"`
	Source   string `json:"source" toml:"source"`
	Medium   string `json:"medium" toml:"medium"`
	Campaign string `json:"campaign" toml:"campaign"`
}

type PlatformConfig struct {
	Language    string          `json:"language" toml:"language"`
	ChangedOnly bool            `json:"changedOnly" toml:"changed_only"`
	Footer      FooterConfig    `json:"footer" toml:"footer"`
	Canonical   CanonicalConfig `json:"canonical" toml:"canonical"`
	Tracking    TrackingConfig  `json:"tracking" toml:"tracking"`
}

type MermaidConfig struct {
	Format string  `json:"format" toml:"format"`
	Width  int     `json:"width" toml:"width"`
	Scale  float64 `json:"scale" toml:"scale"`
}

type CompilerConfig struct {
	Mermaid MermaidConfig `json:"mermaid" toml:"mermaid"`
}

type R2Config struct {
	Bucket          string `json:"bucket,omitempty" toml:"bucket"`
	PublicBaseURL   string `json:"publicBaseUrl" toml:"public_base_url"`
	AccessKeyID     string `json:"accessKeyId,omitempty" toml:"access_key_id"`
	SecretAccessKey string `json:"secretAccessKey,omitempty" toml:"secret_access_key"`
	AccountID       string `json:"accountId,omitempty" toml:"account_id"`
	Endpoint        string `json:"endpoint,omitempty" toml:"endpoint"`
}

type AssetsConfig struct {
	Store string   `json:"store" toml:"store"`
	R2    R2Config `json:"r2" toml:"r2"`
}

type Config struct {
	Compiler  CompilerConfig            `json:"compiler" toml:"compiler"`
	Assets    AssetsConfig              `json:"assets" toml:"assets"`
	Platforms map[string]PlatformConfig `json:"platforms" toml:"platforms"`
}

func PlatformIDs() []string {
	return blogplatform.IDs()
}

func DefaultFooterTemplate(language string) string {
	if language == "en" {
		return "> This article was first published on [{site}]({url}) and syndicated here by the author. The original article may be revised over time; please refer to the personal blog for the latest version."
	}
	return "> 本文首发于 [{site}]({url})，由作者本人同步发布。原文可能持续修订，最新版本请以个人博客为准。"
}

func DefaultPlatformConfig(platform string) PlatformConfig {
	language := blogplatform.DefaultLanguage(platform)
	if language == "" {
		language = "zh-CN"
	}
	mode := "footer"
	if platform == "devto" || platform == "medium" || platform == "hashnode" {
		mode = "native"
	}
	return PlatformConfig{
		Language: language,
		Footer: FooterConfig{
			Enabled:  true,
			Template: DefaultFooterTemplate(language),
		},
		Canonical: CanonicalConfig{Mode: mode},
		Tracking: TrackingConfig{
			Enabled:  true,
			Source:   platform,
			Medium:   "referral",
			Campaign: "article_syndication",
		},
	}
}

func DefaultConfig() Config {
	result := Config{
		Compiler: CompilerConfig{
			Mermaid: MermaidConfig{Format: "png", Width: 1200, Scale: 2},
		},
		Assets: AssetsConfig{
			Store: "r2",
			R2: R2Config{
				PublicBaseURL: DefaultAssetBaseURL,
			},
		},
		Platforms: make(map[string]PlatformConfig),
	}
	for _, platform := range blogplatform.IDs() {
		result.Platforms[platform] = DefaultPlatformConfig(platform)
	}
	return result
}

func mergeDefaults(config Config) Config {
	defaults := DefaultConfig()
	if config.Compiler.Mermaid.Format == "" {
		config.Compiler.Mermaid.Format = defaults.Compiler.Mermaid.Format
	}
	if config.Compiler.Mermaid.Width <= 0 {
		config.Compiler.Mermaid.Width = defaults.Compiler.Mermaid.Width
	}
	if config.Compiler.Mermaid.Scale <= 0 {
		config.Compiler.Mermaid.Scale = defaults.Compiler.Mermaid.Scale
	}
	if config.Assets.Store == "" {
		config.Assets.Store = defaults.Assets.Store
	}
	if config.Assets.R2.PublicBaseURL == "" {
		config.Assets.R2.PublicBaseURL = defaults.Assets.R2.PublicBaseURL
	}
	if config.Platforms == nil {
		config.Platforms = map[string]PlatformConfig{}
	}
	for platform, value := range defaults.Platforms {
		configured, ok := config.Platforms[platform]
		if !ok {
			config.Platforms[platform] = value
			continue
		}
		if configured.Language == "" {
			configured.Language = value.Language
		}
		if configured.Footer.Template == "" {
			configured.Footer.Template = DefaultFooterTemplate(configured.Language)
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
		config.Platforms[platform] = configured
	}
	return config
}

func Normalize(config Config) (Config, error) {
	config = mergeDefaults(config)

	config.Compiler.Mermaid.Format = strings.ToLower(strings.TrimSpace(config.Compiler.Mermaid.Format))
	if config.Compiler.Mermaid.Format != "png" {
		return config, errors.New("publishing compiler Mermaid format must be png")
	}
	if config.Compiler.Mermaid.Width <= 0 || config.Compiler.Mermaid.Scale <= 0 {
		return config, errors.New("publishing compiler Mermaid width and scale must be positive")
	}

	config.Assets.Store = strings.ToLower(strings.TrimSpace(config.Assets.Store))
	if config.Assets.Store != "r2" {
		return config, errors.New("publishing asset store must be r2")
	}
	config.Assets.R2.Bucket = strings.TrimSpace(config.Assets.R2.Bucket)
	config.Assets.R2.PublicBaseURL = strings.TrimSpace(config.Assets.R2.PublicBaseURL)
	config.Assets.R2.AccessKeyID = strings.TrimSpace(config.Assets.R2.AccessKeyID)
	config.Assets.R2.SecretAccessKey = strings.TrimSpace(config.Assets.R2.SecretAccessKey)
	config.Assets.R2.AccountID = strings.TrimSpace(config.Assets.R2.AccountID)
	config.Assets.R2.Endpoint = strings.TrimSpace(config.Assets.R2.Endpoint)
	if config.Assets.R2.PublicBaseURL == "" {
		return config, errors.New("publishing R2 publicBaseUrl is required")
	}
	publicBase, err := url.Parse(config.Assets.R2.PublicBaseURL)
	if err != nil || publicBase.Scheme != "https" || publicBase.Host == "" {
		return config, errors.New("publishing R2 publicBaseUrl must be an HTTPS URL")
	}

	for platform, value := range config.Platforms {
		switch strings.ToLower(strings.TrimSpace(value.Language)) {
		case "en":
			value.Language = "en"
		case "zh", "zh-cn":
			value.Language = "zh-CN"
		default:
			return config, errors.New(platform + " language must be zh-CN or en")
		}
		value.Footer.Template = strings.TrimSpace(value.Footer.Template)
		value.Canonical.Mode = strings.ToLower(strings.TrimSpace(value.Canonical.Mode))
		value.Tracking.Source = strings.TrimSpace(value.Tracking.Source)
		value.Tracking.Medium = strings.TrimSpace(value.Tracking.Medium)
		value.Tracking.Campaign = strings.TrimSpace(value.Tracking.Campaign)
		if value.Footer.Enabled && value.Footer.Template == "" {
			return config, errors.New(platform + " footer template is required when footer is enabled")
		}
		switch value.Canonical.Mode {
		case "native", "footer", "none":
		default:
			return config, errors.New(platform + " canonical mode must be native, footer, or none")
		}
		if value.Tracking.Enabled && value.Tracking.Source == "" {
			return config, errors.New(platform + " tracking source is required when tracking is enabled")
		}
		config.Platforms[platform] = value
	}
	return config, nil
}

package bridge

import (
	publishing "github.com/ThinkerQAQ/syndirelay/publishing"
	"testing"
)

func TestPublishingAssetStatusReportsReadyWithoutExposingSecrets(t *testing.T) {
	config := defaultBridgeConfig()
	config.Publishing.Assets.R2 = publishingR2Config{
		Bucket: "thinkerqaq-assets", PublicBaseURL: "https://assets.example.com/",
		AccessKeyID: "access", SecretAccessKey: "secret", AccountID: "account",
	}
	status := publishingAssetStatus(config)
	if !status.Ready || len(status.Missing) != 0 {
		t.Fatalf("status = %#v", status)
	}
	view := publishingAssetsPublicView(config)
	if !view.R2.SecretAccessKeyConfigured {
		t.Fatal("secret configured flag is false")
	}
}

func TestPublishingAssetStatusNamesMissingRequirements(t *testing.T) {
	config := defaultBridgeConfig()
	config.Publishing.Assets.R2.Bucket = ""
	config.Publishing.Assets.R2.PublicBaseURL = ""
	status := publishingAssetStatus(config)
	if status.Ready {
		t.Fatalf("status unexpectedly ready: %#v", status)
	}
	if len(status.Missing) != 5 {
		t.Fatalf("missing = %#v", status.Missing)
	}
}

func TestSharedAssetUpdatePreservesPlatformSettings(t *testing.T) {
	config := defaultBridgeConfig()
	before := config.Publishing.Platforms["cnblogs"]
	compiler := publishingCompilerConfig{
		Mermaid: publishingMermaidConfig{Format: "png", Width: 1600, Scale: 2},
	}
	assets := publishingAssetsConfig{
		Store: "r2",
		R2: publishingR2Config{
			Bucket:        "shared-assets",
			PublicBaseURL: "https://assets.example.com/",
		},
	}

	updated, err := applyPublishingRuntimeConfig(config, &compiler, &assets)
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.Publishing.Platforms["cnblogs"]; got != before {
		t.Fatalf("platform config changed while saving shared assets: got %#v want %#v", got, before)
	}
}

func TestPlatformUpdatePreservesSharedAssetSettings(t *testing.T) {
	config := defaultBridgeConfig()
	config.Publishing.Assets.R2.Bucket = "shared-assets"
	config.Publishing.Assets.R2.PublicBaseURL = "https://assets.example.com/"
	view := publishingViews(config)[0]
	view.Tracking.Campaign = "changed-platform-setting"

	updated, err := updatePublishing(config, []publishingPlatformView{view})
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.Publishing.Assets.R2.Bucket; got != "shared-assets" {
		t.Fatalf("R2 bucket changed while saving platform config: %q", got)
	}
	if got := updated.Publishing.Assets.R2.PublicBaseURL; got != "https://assets.example.com/" {
		t.Fatalf("R2 public base URL changed while saving platform config: %q", got)
	}
}

func TestPublishingIdentityAndCopyrightAreConfigurableAndPersist(t *testing.T) {
	useIsolatedUserConfigDir(t)
	cfg := defaultBridgeConfig()
	cfg.Publishing.Site.Origin = "https://writer.example.org"
	cfg.Publishing.Site.NameZH = "独立站点"
	cfg.Publishing.Site.NameEN = "Independent Writer"
	cfg.Publishing.Site.LegacyOrigins = []string{"https://old.example.org"}
	cfg.Publishing.Copyright.Mode = "config"
	cfg.Publishing.Copyright.ZH.Syndication = "© 独立作者。遵循自定义许可。"
	cfg.Publishing.Copyright.ZH.Source = "原文"
	cfg.Publishing.Copyright.EN.Syndication = "© Independent Writer, original content under CC BY-NC-SA 4.0."
	cfg.Publishing.Copyright.EN.Source = "Original article"
	normalized, err := normalizeBridgeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveBridgeConfig(normalized); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadBridgeConfigChecked()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Publishing.Site.Origin != "https://writer.example.org" || loaded.Publishing.Site.NameZH != "独立站点" || loaded.Publishing.Copyright.ZH.Syndication != cfg.Publishing.Copyright.ZH.Syndication {
		t.Fatalf("TOML lost owner-specific policy: %+v", loaded.Publishing)
	}
	payload := publishingControlPayload(loaded)
	site, ok := payload["site"].(publishing.SiteConfig)
	if !ok || site.NameEN != "Independent Writer" {
		t.Fatalf("missing configured site in API payload: %#v", payload["site"])
	}
	policy, ok := payload["copyright"].(publishing.CopyrightConfig)
	if !ok || policy.Mode != "config" || policy.EN.Syndication == "" {
		t.Fatalf("missing configured copyright in API payload: %#v", payload["copyright"])
	}
}

package bridge

import "testing"

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

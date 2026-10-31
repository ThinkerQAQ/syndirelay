package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBridgeConfigDefaultsPublicationBindingsPath(t *testing.T) {
	dir := useIsolatedUserConfigDir(t)
	config, err := normalizeBridgeConfig(bridgeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "publications.json")
	if config.PublicationBindingsPath != want {
		t.Fatalf("publication bindings path = %q, want %q", config.PublicationBindingsPath, want)
	}
}

func TestBridgeConfigPersistsProxy(t *testing.T) {
	dir := useIsolatedUserConfigDir(t)

	want, err := normalizeBridgeConfig(bridgeConfig{ProxyEnabled: true, ProxyHost: "127.0.0.1", ProxyPort: 7890})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveBridgeConfig(want); err != nil {
		t.Fatal(err)
	}
	got := loadBridgeConfig()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "blogctl.toml")); err != nil {
		t.Fatalf("config file not written: %v", err)
	}
}

func TestBridgeConfigRejectsPartialProxy(t *testing.T) {
	for _, config := range []bridgeConfig{
		{ProxyEnabled: true, ProxyHost: "127.0.0.1"},
		{ProxyEnabled: true, ProxyPort: 7890},
	} {
		if _, err := normalizeBridgeConfig(config); err == nil {
			t.Fatalf("normalizeBridgeConfig(%#v) unexpectedly succeeded", config)
		}
	}
}

func TestBridgeConfigRetainsProxyAddressWhileDisabled(t *testing.T) {
	config, err := normalizeBridgeConfig(bridgeConfig{ProxyEnabled: false, ProxyHost: " 127.0.0.1 ", ProxyPort: 7890})
	if err != nil {
		t.Fatal(err)
	}
	if config.ProxyHost != "127.0.0.1" || config.ProxyPort != 7890 || config.ProxyEnabled {
		t.Fatalf("config = %#v", config)
	}
}

func TestDevtoAPIKeyPersistsAndToolRegistryMasksSecret(t *testing.T) {
	useIsolatedUserConfigDir(t)

	config, err := normalizeBridgeConfig(bridgeConfig{DevtoAPIKey: " secret-key "})
	if err != nil {
		t.Fatal(err)
	}
	if config.DevtoAPIKey != "secret-key" {
		t.Fatalf("normalized key = %q", config.DevtoAPIKey)
	}
	if err := saveBridgeConfig(config); err != nil {
		t.Fatal(err)
	}
	reloaded := loadBridgeConfig()
	if reloaded.DevtoAPIKey != "secret-key" {
		t.Fatalf("reloaded key = %q", reloaded.DevtoAPIKey)
	}

	var devto toolDescriptor
	for _, tool := range toolRegistry(reloaded) {
		if tool.Name == "devto-api" {
			devto = tool
			break
		}
	}
	if !devto.Health.OK || devto.Health.Summary != "已配置" {
		t.Fatalf("DEV.to health = %#v", devto.Health)
	}
	if _, exposed := devto.Config.Values["apiKey"]; exposed {
		t.Fatalf("DEV.to API key was exposed: %#v", devto.Config.Values)
	}
	if len(devto.Config.Schema) != 1 || devto.Config.Schema[0].Type != "secret" {
		t.Fatalf("DEV.to schema = %#v", devto.Config.Schema)
	}

	unchanged, err := updateToolConfig(reloaded, "devto-api", map[string]any{"apiKey": ""})
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.DevtoAPIKey != "secret-key" {
		t.Fatalf("blank secret save cleared key: %q", unchanged.DevtoAPIKey)
	}
}

func TestPublishingViewsExposeAndUpdateLanguage(t *testing.T) {
	config := defaultBridgeConfig()
	views := publishingViews(config)
	var medium publishingPlatformView
	for _, view := range views {
		if view.ID == "medium" {
			medium = view
			break
		}
	}
	if medium.Language != "en" {
		t.Fatalf("medium view language = %q", medium.Language)
	}
	if !medium.Capabilities.BrowserSession || !medium.Capabilities.DraftCreate || !medium.Capabilities.DraftUpdate ||
		!medium.Capabilities.ExplicitPublish {
		t.Fatalf("medium capabilities = %#v", medium.Capabilities)
	}
	medium.Language = "zh-CN"
	updated, err := updatePublishing(config, []publishingPlatformView{medium})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Publishing.Platforms["medium"].Language != "zh-CN" {
		t.Fatalf("updated medium language = %q", updated.Publishing.Platforms["medium"].Language)
	}
	var juejin publishingPlatformView
	for _, view := range publishingViews(updated) {
		if view.ID == "juejin" {
			juejin = view
			break
		}
	}
	juejin.ChangedOnly = true
	updated, err = updatePublishing(updated, []publishingPlatformView{juejin})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Publishing.Platforms["juejin"].ChangedOnly || updated.Publishing.Platforms["cnblogs"].ChangedOnly {
		t.Fatalf("changed-only policy leaked across platforms: %#v", updated.Publishing.Platforms)
	}
	medium.ChangedOnly = true
	updated, err = updatePublishing(updated, []publishingPlatformView{medium})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Publishing.Platforms["medium"].ChangedOnly {
		t.Fatal("Medium changed-only policy was not saved")
	}
}

func TestBridgeConfigDefaultFooterFollowsConfiguredLanguage(t *testing.T) {
	config, err := normalizeBridgeConfig(bridgeConfig{
		Publishing: publishingConfig{Platforms: map[string]publishingPlatformConfig{
			"cnblogs": {Language: "en"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(config.Publishing.Platforms["cnblogs"].Footer.Template, "This article was first published") {
		t.Fatalf("footer = %q", config.Publishing.Platforms["cnblogs"].Footer.Template)
	}
}

func TestBridgeConfigPublishingLanguageDefaultsAndValidation(t *testing.T) {
	config, err := normalizeBridgeConfig(bridgeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if config.Publishing.Platforms["cnblogs"].Language != "zh-CN" {
		t.Fatalf("cnblogs language = %q", config.Publishing.Platforms["cnblogs"].Language)
	}
	if config.Publishing.Platforms["medium"].Language != "en" {
		t.Fatalf("medium language = %q", config.Publishing.Platforms["medium"].Language)
	}

	config.Publishing.Platforms["medium"] = publishingPlatformConfig{
		Language:  " zh-cn ",
		Footer:    defaultPlatformPublishingConfig("medium").Footer,
		Canonical: defaultPlatformPublishingConfig("medium").Canonical,
		Tracking:  defaultPlatformPublishingConfig("medium").Tracking,
	}
	normalized, err := normalizeBridgeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Publishing.Platforms["medium"].Language != "zh-CN" {
		t.Fatalf("normalized medium language = %q", normalized.Publishing.Platforms["medium"].Language)
	}

	invalid := normalized
	profile := invalid.Publishing.Platforms["medium"]
	profile.Language = "auto"
	invalid.Publishing.Platforms["medium"] = profile
	if _, err := normalizeBridgeConfig(invalid); err == nil || !strings.Contains(err.Error(), "language must be zh-CN or en") {
		t.Fatalf("unexpected language validation error: %v", err)
	}
}

func TestBridgeConfigPublishingCompilerAndAssetsDefaults(t *testing.T) {
	config, err := normalizeBridgeConfig(bridgeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if config.Publishing.Compiler.Mermaid.Format != "png" ||
		config.Publishing.Compiler.Mermaid.Width != 1200 ||
		config.Publishing.Compiler.Mermaid.Scale != 2 {
		t.Fatalf("mermaid compiler config = %#v", config.Publishing.Compiler.Mermaid)
	}
	if config.Publishing.Assets.Store != "r2" {
		t.Fatalf("asset store = %q", config.Publishing.Assets.Store)
	}
	if config.Publishing.Assets.R2.PublicBaseURL != "https://pub-366a15b6733345039775c083a1fffb3e.r2.dev/" {
		t.Fatalf("R2 public base URL = %q", config.Publishing.Assets.R2.PublicBaseURL)
	}
}

func TestBridgeConfigRejectsInvalidPublishingCompilerPolicy(t *testing.T) {
	config := defaultBridgeConfig()
	config.Publishing.Compiler.Mermaid.Format = "svg"
	if _, err := normalizeBridgeConfig(config); err == nil || !strings.Contains(err.Error(), "Mermaid format must be png") {
		t.Fatalf("unexpected Mermaid format validation error: %v", err)
	}

	config = defaultBridgeConfig()
	config.Publishing.Assets.Store = "filesystem"
	if _, err := normalizeBridgeConfig(config); err == nil || !strings.Contains(err.Error(), "asset store must be r2") {
		t.Fatalf("unexpected asset store validation error: %v", err)
	}

	config = defaultBridgeConfig()
	config.Publishing.Assets.R2.PublicBaseURL = "http://assets.example.com/"
	if _, err := normalizeBridgeConfig(config); err == nil || !strings.Contains(err.Error(), "publicBaseUrl must be an HTTPS URL") {
		t.Fatalf("unexpected R2 URL validation error: %v", err)
	}
}

func TestBridgeConfigStoresR2CredentialsInTOML(t *testing.T) {
	dir := useIsolatedUserConfigDir(t)
	config := defaultBridgeConfig()
	config.Publishing.Assets.R2.Bucket = "thinkerqaq-assets"
	config.Publishing.Assets.R2.AccessKeyID = "access"
	config.Publishing.Assets.R2.SecretAccessKey = "secret"
	config.Publishing.Assets.R2.AccountID = "account"
	if err := saveBridgeConfig(config); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "blogctl.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"[publishing.assets.r2]", "access_key_id = 'access'", "secret_access_key = 'secret'", "account_id = 'account'"} {
		if !strings.Contains(text, want) {
			t.Fatalf("TOML missing %q:\n%s", want, text)
		}
	}
	reloaded := loadBridgeConfig()
	if reloaded.Publishing.Assets.R2.SecretAccessKey != "secret" {
		t.Fatalf("R2 secret was not reloaded")
	}
}

func TestBridgeToolShowsPublicationBindingsPath(t *testing.T) {
	dir := useIsolatedUserConfigDir(t)
	config, err := normalizeBridgeConfig(bridgeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range toolRegistry(config) {
		if tool.Name != "publication-bindings" {
			continue
		}
		want := filepath.Join(dir, "publications.json")
		if tool.Health.Path != want {
			t.Fatalf("publication bindings tool path = %q, want %q", tool.Health.Path, want)
		}
		if !tool.Required {
			t.Fatal("publication bindings tool must be required")
		}
		return
	}
	t.Fatal("publication bindings tool not found")
}

func TestBridgeToolShowsConfigPath(t *testing.T) {
	dir := useIsolatedUserConfigDir(t)
	for _, tool := range toolRegistry(defaultBridgeConfig()) {
		if tool.Name == "bridge" {
			if tool.Health.Path != filepath.Join(dir, "blogctl.toml") {
				t.Fatalf("bridge config path = %q", tool.Health.Path)
			}
			return
		}
	}
	t.Fatal("bridge tool not found")
}

func TestConfigDirOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Data")
	t.Setenv("BLOGCTL_DATA_DIR", dir)
	got, err := ConfigDir()
	if err != nil || got != dir {
		t.Fatalf("ConfigDir = %q, %v; want %q", got, err, dir)
	}
	t.Setenv("BLOGCTL_DATA_DIR", "relative-path")
	if _, err := ConfigDir(); err == nil {
		t.Fatal("expected an error for relative BLOGCTL_DATA_DIR")
	}
}

// UI locale is independent of the selected article publishing language.
func TestBridgeUILocaleContract(t *testing.T) {
	useIsolatedUserConfigDir(t)
	for _, locale := range []string{"auto", "zh-CN", "en"} {
		config, err := normalizeBridgeConfig(bridgeConfig{UILocale: locale})
		if err != nil {
			t.Fatalf("%s: %v", locale, err)
		}
		if err := saveBridgeConfig(config); err != nil {
			t.Fatal(err)
		}
		if got := loadBridgeConfig().UILocale; got != locale {
			t.Fatalf("got %s want %s", got, locale)
		}
	}
	if _, err := normalizeBridgeConfig(bridgeConfig{UILocale: "fr"}); err == nil {
		t.Fatal("accepted unsupported locale")
	}
	if config, err := normalizeBridgeConfig(bridgeConfig{}); err != nil || config.UILocale != "auto" {
		t.Fatalf("default = %q, %v", config.UILocale, err)
	}
}

func TestBufferAPIKeyPersistsAndNeverLeaks(t *testing.T) {
	useIsolatedUserConfigDir(t)
	config, err := normalizeBridgeConfig(bridgeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	config.Social.Buffer.APIKey = "private-test-token"
	if err := saveBridgeConfig(config); err != nil {
		t.Fatal(err)
	}
	loaded := loadBridgeConfig()
	if loaded.Social.Buffer.APIKey != "private-test-token" {
		t.Fatal("key was not loaded")
	}
	raw, err := os.ReadFile(func() string { d, _ := ConfigDir(); return filepath.Join(d, "blogctl.toml") }())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "private-test-token") {
		t.Fatal("key not persisted in TOML")
	}
	for _, item := range toolRegistry(loaded) {
		if item.Name != "buffer-api" {
			continue
		}
		if _, ok := item.Config.Values["apiKey"]; ok {
			t.Fatal("Buffer key leaked in tool registry")
		}
		if item.Config.Schema[0].Type != "secret" {
			t.Fatal("Buffer key is not masked")
		}
		if item.Kind != "social" || !strings.Contains(item.Config.Schema[0].Placeholder, "已配置") {
			t.Fatalf("Buffer must be a configured Social provider with masked key: %+v", item)
		}
		sanitized := publicBridgeConfig(loaded)
		serialized, err := json.Marshal(sanitized)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(serialized), "private-test-token") {
			t.Fatal("Buffer key exposed in public config")
		}
		return
	}
	t.Fatal("Buffer API tool not registered")
}

package search

import (
	"strings"
	"testing"
)

func TestIndexNowRequiresPerSiteKey(t *testing.T) {
	_, err := ResolveIndexNowConfig("https://example.com", "", "", "", "", false)
	if err == nil || !strings.Contains(err.Error(), "key is not configured") {
		t.Fatalf("missing key must fail closed, got %v", err)
	}
}

func TestIndexNowConfiguredKeyDoesNotRequireLocalPublicDirectory(t *testing.T) {
	config, err := ResolveIndexNowConfig("https://example.com", "/nonexistent/site/build", "", "site-owned-key", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if config.Key != "site-owned-key" || config.KeyLocation != "https://example.com/site-owned-key.txt" {
		t.Fatalf("unexpected site-specific config: %#v", config)
	}
}

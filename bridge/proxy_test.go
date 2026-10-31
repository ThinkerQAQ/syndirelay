package bridge

import (
	"net/http"
	"strings"
	"testing"
)

func TestNormalizeProxyAddress(t *testing.T) {
	address, host, port, err := normalizeProxyAddress("127.0.0.1", 7890)
	if err != nil {
		t.Fatal(err)
	}
	if address != "http://127.0.0.1:7890" || host != "127.0.0.1" || port != 7890 {
		t.Fatalf("got %q %q %d", address, host, port)
	}
}

func TestProxyTransportUsesConfiguredProxy(t *testing.T) {
	transport, err := proxyHTTPTransport(bridgeConfig{ProxyEnabled: true, ProxyHost: "127.0.0.1", ProxyPort: 7890})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	request, _ := http.NewRequest(http.MethodGet, "https://medium.com/", nil)
	proxyURL, err := transport.Proxy(request)
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:7890" {
		t.Fatalf("proxy = %v", proxyURL)
	}
}

func TestDirectTransportIgnoresEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9999")
	transport, err := proxyHTTPTransport(bridgeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	if transport.Proxy != nil {
		t.Fatal("direct transport must not inherit environment proxy settings")
	}
}

func envValue(values []string, key string) string {
	for _, value := range values {
		name, item, ok := strings.Cut(value, "=")
		if ok && strings.EqualFold(name, key) {
			return item
		}
	}
	return ""
}

func TestProcessEnvironmentUsesConfiguredProxyForChildRuntimes(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9999")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9999")
	t.Setenv("NO_PROXY", "example.com")
	t.Setenv("NODE_USE_ENV_PROXY", "")

	env, err := processEnvironmentForConfig(bridgeConfig{
		ProxyEnabled: true,
		ProxyHost:    "127.0.0.1",
		ProxyPort:    7890,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := envValue(env, "HTTP_PROXY"); got != "http://127.0.0.1:7890" {
		t.Fatalf("HTTP_PROXY = %q", got)
	}
	if got := envValue(env, "HTTPS_PROXY"); got != "http://127.0.0.1:7890" {
		t.Fatalf("HTTPS_PROXY = %q", got)
	}
	if got := envValue(env, "ALL_PROXY"); got != "http://127.0.0.1:7890" {
		t.Fatalf("ALL_PROXY = %q", got)
	}
	if got := envValue(env, "NODE_USE_ENV_PROXY"); got != "1" {
		t.Fatalf("NODE_USE_ENV_PROXY = %q", got)
	}
	if got := envValue(env, "BLOGCTL_PROXY_REQUIRED"); got != "1" {
		t.Fatalf("BLOGCTL_PROXY_REQUIRED = %q", got)
	}
	if got := envValue(env, "NO_PROXY"); !strings.Contains(got, "127.0.0.1") || !strings.Contains(got, "localhost") {
		t.Fatalf("NO_PROXY = %q", got)
	}
}

func TestProcessEnvironmentIsDirectWhenProxyDisabled(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9999")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9999")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:9999")
	t.Setenv("NODE_USE_ENV_PROXY", "1")

	env, err := processEnvironmentForConfig(bridgeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NODE_USE_ENV_PROXY", "BLOGCTL_PROXY_REQUIRED"} {
		if got := envValue(env, key); got != "" {
			t.Fatalf("%s = %q; child runtime should be direct", key, got)
		}
	}
}

package bridge

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func normalizeProxyAddress(host string, port int) (string, string, int, error) {
	host = strings.TrimSpace(host)
	if host == "" && port == 0 {
		return "", "", 0, nil
	}
	if host == "" || port == 0 {
		return "", "", 0, fmt.Errorf("代理主机和端口必须同时填写")
	}
	if strings.Contains(host, "://") || strings.ContainsAny(host, "/?#@ \t\r\n") {
		return "", "", 0, fmt.Errorf("代理主机只填写域名或 IP，不要包含协议、路径或端口")
	}
	if port < 1 || port > 65535 {
		return "", "", 0, fmt.Errorf("代理端口必须在 1 到 65535 之间")
	}
	address := "http://" + net.JoinHostPort(host, strconv.Itoa(port))
	return address, host, port, nil
}

func removeEnvironmentKeys(values []string, names ...string) []string {
	blocked := make(map[string]struct{}, len(names))
	for _, name := range names {
		blocked[strings.ToUpper(strings.TrimSpace(name))] = struct{}{}
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		name, _, ok := strings.Cut(value, "=")
		if !ok {
			result = append(result, value)
			continue
		}
		if _, blockedName := blocked[strings.ToUpper(strings.TrimSpace(name))]; blockedName {
			continue
		}
		result = append(result, value)
	}
	return result
}

func processEnvironmentForConfig(config bridgeConfig) ([]string, error) {
	env := removeEnvironmentKeys(
		os.Environ(),
		"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "NODE_USE_ENV_PROXY", "BLOGCTL_PROXY_REQUIRED",
	)
	if !config.ProxyEnabled {
		return env, nil
	}
	address, _, _, err := normalizeProxyAddress(config.ProxyHost, config.ProxyPort)
	if err != nil {
		return nil, err
	}
	env = append(env,
		"HTTP_PROXY="+address,
		"HTTPS_PROXY="+address,
		"ALL_PROXY="+address,
		"NO_PROXY=localhost,127.0.0.1,::1,[::1]",
		"NODE_USE_ENV_PROXY=1",
		"BLOGCTL_PROXY_REQUIRED=1",
	)
	return env, nil
}

func proxyHTTPTransport(config bridgeConfig) (*http.Transport, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if !config.ProxyEnabled {
		return transport, nil
	}
	address, _, _, err := normalizeProxyAddress(config.ProxyHost, config.ProxyPort)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	proxyURL, err := url.Parse(address)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	return transport, nil
}

func httpClientForConfig(config bridgeConfig) (*http.Client, error) {
	transport, err := proxyHTTPTransport(config)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport, Timeout: 45 * time.Second}, nil
}

func proxySummary(config bridgeConfig) string {
	if !config.ProxyEnabled {
		return "直连"
	}
	address, _, _, err := normalizeProxyAddress(config.ProxyHost, config.ProxyPort)
	if err != nil {
		return "代理配置无效"
	}
	return address
}

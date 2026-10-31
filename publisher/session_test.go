package publisher

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type sessionRoundTripFunc func(*http.Request) (*http.Response, error)

func (f sessionRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHTTPClientForSessionReplaysCapturedHeaderOnlyToAllowedPlatformHost(t *testing.T) {
	seen := map[string]string{}
	base := &http.Client{Transport: sessionRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		seen[request.URL.Hostname()] = request.Header.Get("Cookie")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    request,
		}, nil
	})}
	client, err := HTTPClientForSession(base, Session{
		Cookies: []BrowserCookie{{
			Name: "host-cookie", Value: "host-value", Domain: "segmentfault.com", Path: "/", Secure: true, HostOnly: true,
		}},
		RequestCookieHeader: "PHPSESSID=browser-value; sl-session=secondary",
		CookieHostSuffixes:  []string{"segmentfault.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, rawURL := range []string{
		"https://segmentfault.com/write",
		"https://api.segmentfault.com/example",
		"https://images.example.com/example.png",
	} {
		response, err := client.Get(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	if seen["segmentfault.com"] != "host-cookie=host-value; PHPSESSID=browser-value; sl-session=secondary" {
		t.Fatalf("platform cookie header = %q", seen["segmentfault.com"])
	}
	if seen["api.segmentfault.com"] != "PHPSESSID=browser-value; sl-session=secondary" {
		t.Fatalf("platform subdomain cookie header = %q", seen["api.segmentfault.com"])
	}
	if seen["images.example.com"] != "" {
		t.Fatalf("captured browser cookies leaked cross-site: %q", seen["images.example.com"])
	}
}

func TestHTTPClientForSessionDoesNotReplayUnscopedCapturedHeader(t *testing.T) {
	seen := ""
	base := &http.Client{Transport: sessionRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		seen = request.Header.Get("Cookie")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    request,
		}, nil
	})}
	client, err := HTTPClientForSession(base, Session{RequestCookieHeader: "sid=secret"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get("https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if seen != "" {
		t.Fatalf("unscoped captured cookies were replayed: %q", seen)
	}
}

func TestHTTPClientForSessionPreservesCookieDomainAndPath(t *testing.T) {
	future := float64(time.Now().Add(time.Hour).Unix())
	client, err := HTTPClientForSession(&http.Client{}, Session{
		Cookies: []BrowserCookie{
			{
				Name: "root", Value: "root-value", Domain: ".juejin.cn", Path: "/",
				Secure: true, ExpirationDate: &future,
			},
			{
				Name: "api-only", Value: "api-value", Domain: "api.juejin.cn", Path: "/content_api/",
				Secure: true, HostOnly: true, ExpirationDate: &future,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	apiURL, _ := url.Parse("https://api.juejin.cn/content_api/v1/article_draft/create")
	apiCookies := client.Jar.Cookies(apiURL)
	values := map[string]string{}
	for _, cookie := range apiCookies {
		values[cookie.Name] = cookie.Value
	}
	if values["root"] != "root-value" || values["api-only"] != "api-value" {
		t.Fatalf("api cookies = %#v", values)
	}

	userURL, _ := url.Parse("https://api.juejin.cn/user_api/v1/user/get")
	userCookies := client.Jar.Cookies(userURL)
	values = map[string]string{}
	for _, cookie := range userCookies {
		values[cookie.Name] = cookie.Value
	}
	if values["root"] != "root-value" {
		t.Fatalf("user cookies = %#v", values)
	}
	if _, ok := values["api-only"]; ok {
		t.Fatalf("path-scoped cookie leaked to user_api: %#v", values)
	}
}

func TestHTTPClientForSessionDropsExpiredCookies(t *testing.T) {
	past := float64(time.Now().Add(-time.Hour).Unix())
	client, err := HTTPClientForSession(&http.Client{}, Session{
		Cookies: []BrowserCookie{{
			Name: "expired", Value: "secret", Domain: ".juejin.cn", Path: "/",
			Secure: true, ExpirationDate: &past,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse("https://api.juejin.cn/")
	if got := client.Jar.Cookies(target); len(got) != 0 {
		t.Fatalf("expired cookies = %#v", got)
	}
}

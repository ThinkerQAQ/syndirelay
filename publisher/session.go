package publisher

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

type requestCookieTransport struct {
	base         http.RoundTripper
	header       string
	hostSuffixes []string
}

func (t requestCookieTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request != nil && request.URL != nil && t.header != "" && requestHostAllowed(request.URL.Hostname(), t.hostSuffixes) {
		clone := request.Clone(request.Context())
		clone.Header = request.Header.Clone()
		clone.Header.Set("Cookie", mergeCookieHeaders(request.Header.Get("Cookie"), t.header))
		request = clone
	}
	return t.base.RoundTrip(request)
}

func mergeCookieHeaders(existing, captured string) string {
	values := map[string]string{}
	order := []string{}
	add := func(header string) {
		for _, pair := range strings.Split(header, ";") {
			pair = strings.TrimSpace(pair)
			name, value, ok := strings.Cut(pair, "=")
			name = strings.TrimSpace(name)
			if !ok || name == "" {
				continue
			}
			if _, exists := values[name]; !exists {
				order = append(order, name)
			}
			values[name] = value
		}
	}
	add(existing)
	add(captured)
	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, name+"="+values[name])
	}
	return strings.Join(parts, "; ")
}

func requestHostAllowed(host string, suffixes []string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return false
	}
	for _, suffix := range suffixes {
		suffix = strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(suffix)), "."), ".")
		if suffix != "" && (host == suffix || strings.HasSuffix(host, "."+suffix)) {
			return true
		}
	}
	return false
}

func HTTPClientForSession(base *http.Client, session Session) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, cookie := range session.Cookies {
		if strings.TrimSpace(cookie.Name) == "" || cookie.Value == "" {
			continue
		}
		host := strings.TrimPrefix(strings.TrimSpace(cookie.Domain), ".")
		if host == "" {
			continue
		}
		path := cookie.Path
		if path == "" {
			path = "/"
		}
		scheme := "http"
		if cookie.Secure {
			scheme = "https"
		}
		seed := &url.URL{Scheme: scheme, Host: host, Path: path}
		value := &http.Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     path,
			Secure:   cookie.Secure,
			HttpOnly: cookie.HTTPOnly,
		}
		if !cookie.HostOnly {
			value.Domain = cookie.Domain
		}
		if cookie.ExpirationDate != nil {
			seconds, fraction := mathModf(*cookie.ExpirationDate)
			value.Expires = time.Unix(int64(seconds), int64(fraction*1e9))
			if value.Expires.Before(now) {
				continue
			}
		}
		jar.SetCookies(seed, []*http.Cookie{value})
	}
	if len(session.Cookies) == 0 && session.RequestCookieHeader == "" {
		return nil, errors.New("browser session contains no cookies")
	}
	if base == nil {
		base = http.DefaultClient
	}
	clone := *base
	clone.Jar = jar
	if session.RequestCookieHeader != "" && len(session.CookieHostSuffixes) > 0 {
		transport := clone.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		clone.Transport = requestCookieTransport{
			base: transport, header: session.RequestCookieHeader,
			hostSuffixes: append([]string{}, session.CookieHostSuffixes...),
		}
	}
	return &clone, nil
}

func mathModf(value float64) (float64, float64) {
	whole := float64(int64(value))
	return whole, value - whole
}

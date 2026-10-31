package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	blogapp "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/app"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/internal/version"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

func setExtensionAuth(request *http.Request, token string) {
	request.Header.Set("origin", "chrome-extension://test")
	request.Header.Set("x-thinkerqaq-token", token)
}

func TestBridgeAcceptsOnlyApprovedMediumCookies(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	server.SetNow(func() time.Time { return now })
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	body := map[string]any{
		"cookies": []map[string]string{
			{"name": "sid", "value": "secret"},
			{"name": "uid", "value": "123"},
			{"name": "unrelated", "value": "drop"},
		},
		"userAgent": "UA",
	}
	encoded, _ := json.Marshal(body)
	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/sessions/medium", bytes.NewReader(encoded))
	setExtensionAuth(request, "token")
	request.Header.Set("content-type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}

	server.mu.Lock()
	session := server.sessions["medium"]
	server.mu.Unlock()
	if session.Cookies["sid"] != "secret" || session.Cookies["uid"] != "123" {
		t.Fatalf("cookies = %#v", session.Cookies)
	}
	if _, ok := session.Cookies["unrelated"]; ok {
		t.Fatalf("unrelated cookie was retained: %#v", session.Cookies)
	}
}

func TestBridgeStoresJuejinCookieMetadataInMemory(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	expiry := float64(time.Now().Add(time.Hour).Unix())
	body := map[string]any{
		"cookies": []map[string]any{
			{
				"name": "sessionid", "value": "secret", "domain": ".juejin.cn", "path": "/",
				"secure": true, "httpOnly": true, "hostOnly": false, "sameSite": "lax",
				"expirationDate": expiry,
			},
		},
		"userAgent": "UA",
	}
	encoded, _ := json.Marshal(body)
	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/sessions/juejin", bytes.NewReader(encoded))
	setExtensionAuth(request, "token")
	request.Header.Set("content-type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}

	server.mu.Lock()
	session := server.sessions["juejin"]
	server.mu.Unlock()
	if len(session.BrowserCookies) != 1 {
		t.Fatalf("cookies = %#v", session.BrowserCookies)
	}
	cookie := session.BrowserCookies[0]
	if cookie.Domain != ".juejin.cn" || cookie.Path != "/" || !cookie.Secure || !cookie.HTTPOnly || cookie.Value != "secret" {
		t.Fatalf("cookie = %#v", cookie)
	}
	if session.UserAgent != "UA" {
		t.Fatalf("user agent = %q", session.UserAgent)
	}
}

func TestCNBlogsCookieReachesGoJarAndStatusRedactsValue(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	const cookieValue = "test-login-value"
	const requestCookieHeader = ".CNBlogsCookie=test-request-value"
	const uploadCookieHeader = ".CNBlogsUploadCookie=test-upload-value"
	body, _ := json.Marshal(map[string]any{
		"cookies": []map[string]any{{
			"name": ".CNBlogsCookie", "value": cookieValue, "domain": ".cnblogs.com",
			"path": "/", "secure": true, "storeId": "0",
			"partitionKey": map[string]string{"topLevelSite": "https://cnblogs.com"},
		}},
		"userAgent":           "UA",
		"requestCookieHeader": requestCookieHeader,
		"requestCookieHeaders": map[string]string{
			"upload.cnblogs.com": uploadCookieHeader,
		},
	})
	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/sessions/cnblogs", bytes.NewReader(body))
	setExtensionAuth(request, "token")
	request.Header.Set("content-type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("session status = %d", response.StatusCode)
	}

	session, base, err := (bridgeNativePublisher{server: server}).publisherSession("cnblogs")
	if err != nil {
		t.Fatal(err)
	}
	if session.RequestCookieHeader != requestCookieHeader {
		t.Fatal("captured browser Cookie header did not reach the publisher")
	}
	if session.RequestCookieHeaders["upload.cnblogs.com"] != uploadCookieHeader {
		t.Fatal("captured upload Cookie header did not reach the publisher")
	}
	client, err := publisher.HTTPClientForSession(base, session)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse("https://i.cnblogs.com/api/user")
	attached := client.Jar.Cookies(target)
	if len(attached) != 1 || attached[0].Name != ".CNBlogsCookie" || attached[0].Value != cookieValue {
		t.Fatal("CNBlogs login cookie was not attached to the Go request")
	}

	statusRequest, _ := http.NewRequest(http.MethodGet, handler.URL+"/v1/sessions/cnblogs/status", nil)
	statusRequest.Header.Set("origin", "chrome-extension://test")
	statusResponse, err := http.DefaultClient.Do(statusRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer statusResponse.Body.Close()
	statusBody, _ := io.ReadAll(statusResponse.Body)
	if statusResponse.StatusCode != http.StatusOK || strings.Contains(string(statusBody), cookieValue) || strings.Contains(string(statusBody), requestCookieHeader) || strings.Contains(string(statusBody), uploadCookieHeader) {
		t.Fatal("session status failed or exposed a cookie value")
	}
	var status struct {
		RequestCookieNames       []string `json:"requestCookieNames"`
		UploadRequestCookieNames []string `json:"uploadRequestCookieNames"`
		Cookies                  []struct {
			Name        string `json:"name"`
			StoreID     string `json:"storeId"`
			Partitioned bool   `json:"partitioned"`
		} `json:"cookies"`
	}
	if err := json.Unmarshal(statusBody, &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Cookies) != 1 || status.Cookies[0].Name != ".CNBlogsCookie" || status.Cookies[0].StoreID != "0" || !status.Cookies[0].Partitioned {
		t.Fatal("session status did not retain cookie metadata")
	}
	if len(status.RequestCookieNames) != 1 || status.RequestCookieNames[0] != ".CNBlogsCookie" {
		t.Fatal("session status did not report captured request Cookie names")
	}
	if len(status.UploadRequestCookieNames) != 1 || status.UploadRequestCookieNames[0] != ".CNBlogsUploadCookie" {
		t.Fatal("session status did not report captured upload Cookie names")
	}
}

func TestCNBlogsAcceptsCapturedRequestCookieWithoutCookieAPIEntries(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/cnblogs", strings.NewReader(`{"cookies":[],"requestCookieHeader":".CNBlogsCookie=test-login"}`))
	setExtensionAuth(request, "token")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	session, _, err := (bridgeNativePublisher{server: server}).publisherSession("cnblogs")
	if err != nil || session.RequestCookieHeader == "" {
		t.Fatal("captured request Cookie was not available to the Go publisher")
	}
}

func TestCNBlogsRejectsCapturedCookieHeaderForUnknownHost(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/cnblogs", strings.NewReader(`{
		"cookies":[],
		"requestCookieHeader":".CNBlogsCookie=test-login",
		"requestCookieHeaders":{"evil.example":"secret"}
	}`))
	setExtensionAuth(request, "token")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestBridgeAcceptsCapturedRequestCookieForSegmentFault(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/segmentfault",
		strings.NewReader(`{"cookies":[],"requestCookieHeader":"PHPSESSID=test-login; sl-session=test-secondary","userAgent":"UA"}`))
	setExtensionAuth(request, "token")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	session, _, err := (bridgeNativePublisher{server: server}).publisherSession("segmentfault")
	if err != nil {
		t.Fatal(err)
	}
	if session.RequestCookieHeader == "" || len(session.CookieHostSuffixes) != 1 || session.CookieHostSuffixes[0] != "segmentfault.com" {
		t.Fatalf("publisher session = %#v", session)
	}
}

func TestBridgeAcceptsMediumSidFromCapturedRequestHeader(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/medium",
		strings.NewReader(`{"cookies":[],"requestCookieHeader":"sid=test-login; xsrf=test-xsrf","userAgent":"UA"}`))
	setExtensionAuth(request, "token")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	server.mu.Lock()
	session := server.sessions["medium"]
	server.mu.Unlock()
	if session.RequestCookieHeader == "" {
		t.Fatal("Medium captured request Cookie header was not stored")
	}
}

func TestBridgeHealthReportsRuntimeVersion(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	request.Header.Set("origin", "chrome-extension://test")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Version != version.Current {
		t.Fatalf("health = %#v", payload)
	}
}

func TestBridgeReadOnlyStatusDoesNotRequireToken(t *testing.T) {
	server, _ := New("token")
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	for _, path := range []string{"/v1/health", "/v1/config", "/v1/sessions/medium/status", "/v1/sessions/juejin/status"} {
		request, _ := http.NewRequest(http.MethodGet, handler.URL+path, nil)
		request.Header.Set("origin", "chrome-extension://test")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, response.StatusCode)
		}
		if response.Header.Get("access-control-allow-origin") != "chrome-extension://test" {
			t.Fatalf("%s missing extension CORS header", path)
		}
	}
}

func TestBridgeConfigResponseNeverExposesDevtoAPIKey(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	server.mu.Lock()
	server.config.DevtoAPIKey = "top-secret-devto-key"
	server.config.IndexNowKey = "top-secret-indexnow-key"
	server.config.GoogleSearchConsoleServiceJSON = "top-secret-google-service-account"
	server.config.Publishing.Assets.R2.SecretAccessKey = "top-secret-r2-key"
	server.mu.Unlock()
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	request, _ := http.NewRequest(http.MethodGet, handler.URL+"/v1/config", nil)
	setExtensionAuth(request, "token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("top-secret-devto-key")) || bytes.Contains(raw, []byte("devtoApiKey")) {
		t.Fatalf("config response exposed DEV.to credential: %s", raw)
	}
	if bytes.Contains(raw, []byte("top-secret-indexnow-key")) || bytes.Contains(raw, []byte("indexNowKey")) {
		t.Fatalf("config response exposed IndexNow credential: %s", raw)
	}
	if bytes.Contains(raw, []byte("top-secret-google-service-account")) || bytes.Contains(raw, []byte("googleSearchConsoleServiceJson")) {
		t.Fatalf("config response exposed Google Search Console credential: %s", raw)
	}
	if bytes.Contains(raw, []byte("top-secret-r2-key")) {
		t.Fatalf("config response exposed R2 secret: %s", raw)
	}
}

func TestBridgeReadOnlyStatusAllowsCLIWithoutOrigin(t *testing.T) {
	server, _ := New("token")
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	for _, path := range []string{"/v1/config", "/v1/sessions/medium/status"} {
		response, err := http.Get(handler.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, response.StatusCode)
		}
	}
}

func TestBridgeReadOnlyStatusRejectsWebsiteOrigin(t *testing.T) {
	server, _ := New("token")
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	request, _ := http.NewRequest(http.MethodGet, handler.URL+"/v1/health", nil)
	request.Header.Set("origin", "https://example.com")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
}

func TestBridgeExtensionWriteRequiresToken(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	restarted := false
	server.SetRestart(func() { restarted = true })
	request := httptest.NewRequest(http.MethodPost, "/v1/restart", nil)
	request.Header.Set("origin", "chrome-extension://test")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
	if restarted {
		t.Fatal("extension write without bridge token reached the restart handler")
	}
}

func TestBridgeRejectsNonExtensionOrigin(t *testing.T) {
	server, _ := New("token")
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/sessions/medium", bytes.NewBufferString(`{"cookies":[]}`))
	request.Header.Set("origin", "https://example.com")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
}

func TestBridgeRejectsSessionPostWithoutExtensionOrigin(t *testing.T) {
	server, _ := New("token")
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/sessions/medium", bytes.NewBufferString(`{"cookies":[{"name":"sid","value":"secret"}]}`))
	request.Header.Set("content-type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
}

func TestBridgeProxyConfigUpdatesTransportAndPersists(t *testing.T) {
	useIsolatedUserConfigDir(t)
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	body := bytes.NewBufferString(`{"proxyEnabled":true,"proxyHost":"127.0.0.1","proxyPort":7890}`)
	request, _ := http.NewRequest(http.MethodPut, handler.URL+"/v1/config", body)
	setExtensionAuth(request, "token")
	request.Header.Set("content-type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}

	server.mu.Lock()
	config := server.config
	client := server.httpClient
	server.mu.Unlock()
	if !config.ProxyEnabled || config.ProxyHost != "127.0.0.1" || config.ProxyPort != 7890 {
		t.Fatalf("config = %#v", config)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy == nil {
		t.Fatalf("transport = %#v", client.Transport)
	}
	mediumRequest, _ := http.NewRequest(http.MethodGet, "https://medium.com/", nil)
	proxyURL, err := transport.Proxy(mediumRequest)
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:7890" {
		t.Fatalf("proxy = %v", proxyURL)
	}

	reloaded, err := New("token-2")
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.config.ProxyEnabled || reloaded.config.ProxyHost != "127.0.0.1" || reloaded.config.ProxyPort != 7890 {
		t.Fatalf("reloaded config = %#v", reloaded.config)
	}
}

func TestBridgeProxyConfigWriteRequiresExtensionOrigin(t *testing.T) {
	useIsolatedUserConfigDir(t)
	server, _ := New("token")
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	for _, origin := range []string{"", "https://example.com"} {
		request, _ := http.NewRequest(http.MethodPut, handler.URL+"/v1/config", bytes.NewBufferString(`{"proxyEnabled":false}`))
		request.Header.Set("content-type", "application/json")
		if origin != "" {
			request.Header.Set("origin", origin)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("origin %q status = %d, want 403", origin, response.StatusCode)
		}
	}
}

func TestSyncStartAllowsBridgeTokenWithoutExtensionOrigin(t *testing.T) {
	server, _ := New("token")
	server.syncRunner = func(_ context.Context, _ bridgeConfig, request syncRequest, emit func(blogapp.SyncEvent)) (string, error) {
		for _, platform := range request.Platforms {
			emit(blogapp.SyncEvent{Platform: platform, State: "completed", Result: "draft-created"})
		}
		return "ok", nil
	}
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/sync/jobs",
		bytes.NewBufferString(`{"article":"example","platforms":["devto"],"draft":true,"operation":"draft"}`))
	request.Header.Set("content-type", "application/json")
	request.Header.Set("x-thinkerqaq-token", "token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", response.StatusCode)
	}
	var payload struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Job.ID == "" {
		t.Fatal("sync job id is empty")
	}
}

func TestSyncStartRejectsInvalidBridgeTokenWithoutExtensionOrigin(t *testing.T) {
	server, _ := New("token")
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/sync/jobs",
		bytes.NewBufferString(`{"article":"example","platforms":["devto"],"draft":true,"operation":"draft"}`))
	request.Header.Set("content-type", "application/json")
	request.Header.Set("x-thinkerqaq-token", "wrong")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
}

func TestSyncStartReloadsPersistedWorkspaceRoots(t *testing.T) {
	useIsolatedUserConfigDir(t)
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	configSeen := make(chan bridgeConfig, 1)
	server.syncRunner = func(_ context.Context, config bridgeConfig, request syncRequest, emit func(blogapp.SyncEvent)) (string, error) {
		configSeen <- config
		for _, platform := range request.Platforms {
			emit(blogapp.SyncEvent{Platform: platform, State: "completed", Result: "draft-created"})
		}
		return "ok", nil
	}
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	contentRoot := t.TempDir()
	engineRoot := t.TempDir()
	if err := UpdateWorkspaceRoots(contentRoot, engineRoot); err != nil {
		t.Fatal(err)
	}

	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/sync/jobs",
		bytes.NewBufferString(`{"article":"example","platforms":["devto"],"draft":true,"operation":"draft"}`))
	request.Header.Set("content-type", "application/json")
	request.Header.Set("x-thinkerqaq-token", "token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", response.StatusCode)
	}

	select {
	case config := <-configSeen:
		if config.ContentRoot != contentRoot || config.EngineRoot != engineRoot {
			t.Fatalf("sync config roots = %q / %q", config.ContentRoot, config.EngineRoot)
		}
	case <-time.After(time.Second):
		t.Fatal("sync runner did not receive refreshed config")
	}
}

func TestSyncJobNotFoundReturnsStructuredError(t *testing.T) {
	server, _ := New("token")
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	request, _ := http.NewRequest(http.MethodGet, handler.URL+"/v1/sync/jobs/missing", nil)
	setExtensionAuth(request, "token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.StatusCode)
	}
	var payload struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "sync_job_not_found" || payload.Message != "sync job not found" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Details["id"] != "missing" {
		t.Fatalf("details = %#v", payload.Details)
	}
}

func TestMediumSessionRequiredStructuredError(t *testing.T) {
	server, _ := New("token")
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/platforms/medium/drafts", bytes.NewBufferString(`{}`))
	request.Header.Set("x-thinkerqaq-token", "token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("status = %d, want 428", response.StatusCode)
	}
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != "medium_session_required" || payload.Message != "medium_session_required" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestBridgeRestartEndpointRestartsWhenIdle(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	restarted := make(chan struct{}, 1)
	server.SetRestart(func() { restarted <- struct{}{} })
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/restart", nil)
	setExtensionAuth(request, "token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", response.StatusCode)
	}
	select {
	case <-restarted:
	case <-time.After(time.Second):
		t.Fatal("restart callback was not invoked")
	}
}

func TestBridgeRestartEndpointRejectsRunningSyncJob(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	restarted := make(chan struct{}, 1)
	server.SetRestart(func() { restarted <- struct{}{} })
	server.jobs["running"] = &syncJob{ID: "running", State: "running"}
	server.jobOrder = []string{"running"}
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/restart", nil)
	setExtensionAuth(request, "token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", response.StatusCode)
	}
	select {
	case <-restarted:
		t.Fatal("restart callback should not run while a sync job is active")
	case <-time.After(250 * time.Millisecond):
	}
}

func TestBridgeFiltersVerifiedPlatformCookiesBeforeStorage(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	handler := httptest.NewServer(server.Handler())
	defer handler.Close()

	body, _ := json.Marshal(map[string]any{
		"cookies": []map[string]any{
			{"name": "UserName", "value": "thinker", "domain": ".csdn.net", "path": "/", "secure": true},
			{"name": "UserToken", "value": "token", "domain": ".csdn.net", "path": "/", "secure": true},
			{"name": "tracking_cookie", "value": "drop-me", "domain": ".csdn.net", "path": "/", "secure": true},
		},
		"userAgent": "UA",
	})
	request, _ := http.NewRequest(http.MethodPost, handler.URL+"/v1/sessions/csdn", bytes.NewReader(body))
	setExtensionAuth(request, "token")
	request.Header.Set("content-type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}

	server.mu.Lock()
	session := server.sessions["csdn"]
	server.mu.Unlock()
	if len(session.BrowserCookies) != 2 {
		t.Fatalf("browser cookies = %#v", session.BrowserCookies)
	}
	if _, ok := session.Cookies["tracking_cookie"]; ok {
		t.Fatalf("unapproved cookie retained: %#v", session.Cookies)
	}
	for _, cookie := range session.BrowserCookies {
		if cookie.Name == "tracking_cookie" {
			t.Fatalf("unapproved browser cookie retained: %#v", session.BrowserCookies)
		}
	}
}

func TestBridgeMediumFilterAlsoAppliesToPublisherCookieMetadata(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/medium", strings.NewReader(`{
		"cookies":[
			{"name":"sid","value":"secret","domain":".medium.com","path":"/","secure":true},
			{"name":"tracking_cookie","value":"drop","domain":".medium.com","path":"/","secure":true}
		]
	}`))
	setExtensionAuth(request, "token")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	session, _, err := (bridgeNativePublisher{server: server}).publisherSession("medium")
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Cookies) != 1 || session.Cookies[0].Name != "sid" {
		t.Fatalf("publisher cookies = %#v", session.Cookies)
	}
}

func TestBridgeAcceptsOptionalDEVToBrowserSession(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	server.config.DevtoAPIKey = "api-key"
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/devto", strings.NewReader(`{
		"cookies":[
			{"name":"_Devto_Forem_Session","value":"session","domain":"dev.to","path":"/","secure":true},
			{"name":"tracking_cookie","value":"drop","domain":"dev.to","path":"/","secure":true}
		],
		"userAgent":"UA"
	}`))
	setExtensionAuth(request, "token")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	session, _, err := (bridgeNativePublisher{server: server}).publisherSession("devto")
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Cookies) != 1 || session.Cookies[0].Name != "_Devto_Forem_Session" {
		t.Fatalf("DEV.to publisher cookies = %#v", session.Cookies)
	}
}

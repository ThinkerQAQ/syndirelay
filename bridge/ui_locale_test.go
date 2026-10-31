package bridge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBridgeUILocaleEndpoint(t *testing.T) {
	useIsolatedUserConfigDir(t)
	server, err := New("secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()

	put := func(locale string, auth bool) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"uiLocale": locale})
		request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:32145/v1/ui-locale", bytes.NewReader(body))
		request.Header.Set("content-type", "application/json")
		if auth {
			setExtensionAuth(request, "secret")
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, request)
		return result
	}
	if got := put("en", false).Code; got == http.StatusOK {
		t.Fatal("unauthenticated locale write was accepted")
	}
	if got := put("fr", true).Code; got != http.StatusBadRequest {
		t.Fatalf("unsupported locale status = %d", got)
	}
	if got := put("en", true).Code; got != http.StatusOK {
		t.Fatalf("valid locale status = %d", got)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32145/v1/ui-locale", nil)
	setExtensionAuth(request, "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var result struct {
		UILocale string `json:"uiLocale"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.UILocale != "en" || loadBridgeConfig().UILocale != "en" {
		t.Fatalf("GET or TOML locale = %+v", result)
	}
}

package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	blogsearch "github.com/ThinkerQAQ/syndirelay/search"
)

func TestIndexNowBridgeSubmitsWithoutLocalWebsiteFiles(t *testing.T) {
	const key = "sample-site-owned-key"
	var posts int
	var remote *httptest.Server
	remote = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap-all.txt":
			fmt.Fprintln(w, remote.URL+"/articles/example/")
		case "/" + key + ".txt":
			fmt.Fprintln(w, key)
		case "/indexnow":
			if r.Method != http.MethodPost {
				t.Errorf("method = %s", r.Method)
			}
			var body struct {
				Host        string   `json:"host"`
				Key         string   `json:"key"`
				KeyLocation string   `json:"keyLocation"`
				URLList     []string `json:"urlList"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("invalid request body: %v", err)
			}
			if body.Key != key || body.KeyLocation != remote.URL+"/"+key+".txt" || len(body.URLList) != 1 || body.URLList[0] != remote.URL+"/articles/example/" {
				t.Errorf("unexpected IndexNow payload: %#v", body)
			}
			posts++
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	site := blogsearch.DefaultSiteConfig(remote.URL)
	site.FingerprintURL = ""
	bridge := &Server{
		httpClient: remote.Client(),
		config: bridgeConfig{
			Search:           &site,
			IndexNowEndpoint: remote.URL + "/indexnow",
			IndexNowKey:      key,
			EngineRoot:       "/path/that/does/not/exist",
		},
	}
	if _, err := bridge.checkIndexNowNative(context.Background()); err != nil {
		t.Fatalf("remote key check must not depend on local site files: %v", err)
	}
	result, err := bridge.submitIndexNow(context.Background(), "incremental", searchInventoryState{})
	if err != nil {
		t.Fatal(err)
	}
	if posts != 1 || result.Result.URLCount != 1 || result.Result.Results[0].HTTPStatus != http.StatusAccepted {
		t.Fatalf("submitted: posts=%d result=%#v", posts, result)
	}

	bridge.config.IndexNowKey = "wrong-ownership-key"
	bridge.config.IndexNowKeyLocation = remote.URL + "/" + key + ".txt"
	_, err = bridge.submitIndexNow(context.Background(), "full", searchInventoryState{})
	if err == nil || !strings.Contains(err.Error(), "does not match configured key") {
		t.Fatalf("mismatched remote ownership proof must block submit: %v", err)
	}
	if posts != 1 {
		t.Fatalf("mismatched key triggered a POST: %d", posts)
	}
}

func TestIndexNowWithoutKeyHasMissingHealth(t *testing.T) {
	config := defaultBridgeConfig()
	if config.IndexNowKey != "" || indexNowKey(config) != "" || indexNowKeyLocation(config) != "" {
		t.Fatal("default config must not contain any site-specific ownership key")
	}
	if health := indexNowHealth(config); health.OK || health.Status != "missing" {
		t.Fatalf("missing key must not be presented as configured: %#v", health)
	}
}

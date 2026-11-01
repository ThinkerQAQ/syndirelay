package search

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiffInventoriesSkipsDeletedForBaidu(t *testing.T) {
	previous := Inventory{
		URLs: []string{
			"https://example.invalid/a/",
			"https://example.invalid/deleted/",
		},
		Fingerprints: map[string]string{
			"https://example.invalid/a/":       "old",
			"https://example.invalid/deleted/": "old",
		},
	}
	current := Inventory{
		URLs: []string{
			"https://example.invalid/a/",
			"https://example.invalid/new/",
		},
		Fingerprints: map[string]string{
			"https://example.invalid/a/":   "new",
			"https://example.invalid/new/": "hash",
		},
	}
	diff, err := DiffInventories(previous, current, "incremental", false)
	if err != nil {
		t.Fatal(err)
	}
	if diff.AddedCount != 1 || diff.ChangedCount != 1 || diff.DeletedCount != 1 {
		t.Fatalf("unexpected diff: %#v", diff)
	}
	if len(diff.Selected) != 2 {
		t.Fatalf("selected = %#v", diff.Selected)
	}
	for _, value := range diff.Selected {
		if strings.Contains(value, "deleted") {
			t.Fatalf("deleted URL must not be submitted to Baidu: %#v", diff.Selected)
		}
	}
}

func TestSubmitBaiduUsesTextBodyAndOnlyCompletesFullBatch(t *testing.T) {
	var gotToken string
	var gotSite string
	var gotContentType string
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotToken = request.URL.Query().Get("token")
		gotSite = request.URL.Query().Get("site")
		gotContentType = request.Header.Get("content-type")
		body, _ := io.ReadAll(request.Body)
		gotBody = string(body)
		response.Header().Set("content-type", "application/json")
		_, _ = response.Write([]byte(`{"remain":9,"success":1}`))
	}))
	defer server.Close()

	config, err := ResolveBaiduConfig(
		"https://example.invalid",
		"https://example.invalid",
		"secret-token",
	)
	if err != nil {
		t.Fatal(err)
	}
	config.Endpoint = server.URL
	result, err := SubmitBaidu(context.Background(), server.Client(), []string{
		"https://example.invalid/a/",
		"https://example.invalid/b/",
	}, config)
	if err == nil {
		t.Fatal("partial Baidu acceptance must fail closed")
	}
	if result.Complete || result.SuccessCount != 1 || result.URLCount != 2 {
		t.Fatalf("result = %#v", result)
	}
	if gotToken != "secret-token" || gotSite != "example.invalid" {
		t.Fatalf("query token=%q site=%q", gotToken, gotSite)
	}
	if gotContentType != "text/plain" {
		t.Fatalf("content-type = %q", gotContentType)
	}
	if gotBody != "https://example.invalid/a/\nhttps://example.invalid/b/" {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestRedactBaiduResponse(t *testing.T) {
	got := redactBaiduResponse("request token=secret-token failed", "secret-token")
	if strings.Contains(got, "secret-token") {
		t.Fatalf("token leaked: %s", got)
	}
}

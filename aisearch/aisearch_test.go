package aisearch

import (
	"testing"
)

func TestContentRoutesMatchSiteLocales(t *testing.T) {
	tests := []struct {
		collection string
		id         string
		data       map[string]string
		wantLang   string
		wantURL    string
	}{
		{"articles", "concurrency-series-05-atomic-cas", map[string]string{}, "zh", DefaultBlogOrigin + "/articles/concurrency-series-05-atomic-cas/"},
		{"articles", "en/concurrency-series-05-atomic-cas", map[string]string{"language": "en"}, "en", DefaultBlogOrigin + "/en/articles/concurrency-series-05-atomic-cas/"},
		{"articles", "en/example", map[string]string{}, "en", DefaultBlogOrigin + "/en/articles/example/"},
		{"notes", "java/JUC/example", map[string]string{}, "zh", DefaultBlogOrigin + "/notes/java/JUC/example/"},
		{"notes", "example", map[string]string{"language": "en"}, "en", DefaultBlogOrigin + "/en/notes/example/"},
	}
	for _, test := range tests {
		language, err := ContentLanguage(test.collection, test.id, test.data)
		if err != nil {
			t.Fatalf("%s/%s: %v", test.collection, test.id, err)
		}
		if language != test.wantLang {
			t.Fatalf("%s/%s language=%q want=%q", test.collection, test.id, language, test.wantLang)
		}
		if got := ContentSourceURL(DefaultBlogOrigin, test.collection, test.id, language); got != test.wantURL {
			t.Fatalf("%s/%s URL=%q want=%q", test.collection, test.id, got, test.wantURL)
		}
	}
}

func TestUnsupportedContentLanguageFails(t *testing.T) {
	if _, err := ContentLanguage("articles", "example", map[string]string{"language": "fr"}); err == nil {
		t.Fatal("expected unsupported language to fail")
	}
}

func TestMutating7017Classification(t *testing.T) {
	if !mutating7017(503, []byte(`{"success":false,"errors":[{"code":7017,"message":"temporary"}]}`)) {
		t.Fatal("expected Cloudflare 7017 to be retryable")
	}
	if mutating7017(503, []byte(`{"success":false,"errors":[{"code":9999}]}`)) {
		t.Fatal("unrelated mutating 503 must not be retried")
	}
}

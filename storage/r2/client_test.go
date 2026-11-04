package r2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testUploadConfig(endpoint string) Config {
	return Config{
		AccessKeyID: "test-key", SecretAccessKey: "test-secret",
		Endpoint: endpoint, Bucket: "test-bucket",
		PublicBaseURL: "https://cdn.example.test/",
	}
}
func testImmutableKey(data []byte) string {
	sum := sha256.Sum256(data)
	return "images/v1/" + hex.EncodeToString(sum[:16]) + ".webp"
}

func TestImmutableUploadConditionalCreateAndDedup(t *testing.T) {
	body := []byte("fixed image")
	key := testImmutableKey(body)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/test-bucket/"+key {
			t.Errorf("unexpected method/path: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("If-None-Match") != "*" {
			t.Error("conditional create header missing")
		}
		if !strings.Contains(r.Header.Get("Authorization"), "SignedHeaders=content-type;host;if-none-match;x-amz-content-sha256;x-amz-date") {
			t.Error("conditional header must be signed")
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusPreconditionFailed)
		}
	}))
	defer server.Close()
	for i := 0; i < 2; i++ {
		got, err := UploadImmutable(context.Background(), server.Client(), testUploadConfig(server.URL), key, body, "image/webp")
		if err != nil || got != "https://cdn.example.test/"+key {
			t.Fatalf("attempt %d: url=%q error=%v", i, got, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("got %d requests, expected first create and second 412", calls.Load())
	}
}

func TestImmutableUploadRetriesTransientResponses(t *testing.T) {
	body := []byte("same payload")
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()
	_, err := UploadImmutable(context.Background(), server.Client(), testUploadConfig(server.URL), testImmutableKey(body), body, "image/webp")
	if err != nil || calls.Load() != 3 {
		t.Fatalf("retry result: calls=%d err=%v", calls.Load(), err)
	}
}

func TestImmutableUploadAmbiguousCommitCanReturn412(t *testing.T) {
	body := []byte("same payload")
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer server.Close()
	_, err := UploadImmutable(context.Background(), server.Client(), testUploadConfig(server.URL), testImmutableKey(body), body, "image/webp")
	if err != nil || calls.Load() != 2 {
		t.Fatalf("ambiguous commit: calls=%d err=%v", calls.Load(), err)
	}
}

func TestImmutableUploadPermanentFailureNotRetried(t *testing.T) {
	body := []byte("payload")
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	_, err := UploadImmutable(context.Background(), server.Client(), testUploadConfig(server.URL), testImmutableKey(body), body, "image/webp")
	if err == nil || !strings.Contains(err.Error(), "403") || calls.Load() != 1 {
		t.Fatalf("permanent error: calls=%d err=%v", calls.Load(), err)
	}
}

func TestImmutableUploadBoundedRetries(t *testing.T) {
	body := []byte("payload")
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	_, err := UploadImmutable(context.Background(), server.Client(), testUploadConfig(server.URL), testImmutableKey(body), body, "image/webp")
	if err == nil || !strings.Contains(err.Error(), "503") || calls.Load() != maxUploadAttempts {
		t.Fatalf("retry budget: calls=%d err=%v", calls.Load(), err)
	}
}

func TestImmutableUploadHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body := []byte("payload")
	_, err := UploadImmutable(ctx, &http.Client{Timeout: time.Second}, testUploadConfig("https://example.test"), testImmutableKey(body), body, "image/webp")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestImmutableUploadRetriesNetworkFailure(t *testing.T) {
	body := []byte("payload")
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("synthetic transient connection failure")
		}
		return server.Client().Transport.RoundTrip(r)
	})}
	_, err := UploadImmutable(context.Background(), client, testUploadConfig(server.URL), testImmutableKey(body), body, "image/webp")
	if err != nil || calls.Load() != 2 {
		t.Fatalf("network retry: calls=%d err=%v", calls.Load(), err)
	}
}

func TestImmutableUploadRejectsWrongDigest(t *testing.T) {
	_, err := UploadImmutable(context.Background(), nil, testUploadConfig("https://example.test"), "images/v1/wrong.webp", []byte("payload"), "image/webp")
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("wrong immutable key should be rejected: %v", err)
	}
}

func TestMutableUploadRetainsOverwriteAndSingleAttempt(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("If-None-Match") != "" {
			t.Error("mutable upload unexpectedly became conditional")
		}
		if strings.Contains(r.Header.Get("Authorization"), "if-none-match") {
			t.Error("mutable signature must not include conditional header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	for i := 0; i < 2; i++ {
		_, err := Upload(context.Background(), server.Client(), testUploadConfig(server.URL), "legacy/existing.webp", []byte("image bytes"), "image/webp")
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("legacy upload calls=%d", calls.Load())
	}

	var failed atomic.Int32
	fail := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failed.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer fail.Close()
	_, err := Upload(context.Background(), fail.Client(), testUploadConfig(fail.URL), "legacy/existing.webp", []byte("image bytes"), "image/webp")
	if err == nil || failed.Load() != 1 {
		t.Fatalf("legacy upload retried ambiguous write: calls=%d err=%v", failed.Load(), err)
	}
}

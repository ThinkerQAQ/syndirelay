package r2

import (
	"context"
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
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
		Endpoint:        endpoint,
		Bucket:          "test-bucket",
		PublicBaseURL:   "https://cdn.example.test/",
	}
}

func TestUploadConditionalCreateAndDedup(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/test-bucket/images/v1/hash.webp" {
			t.Errorf("wrong request method or path: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("If-None-Match") != "*" {
			t.Error("missing conditional create header")
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
	config := testUploadConfig(server.URL)
	for i := 0; i < 2; i++ {
		got, err := Upload(context.Background(), server.Client(), config, "images/v1/hash.webp", []byte("fixed image"), "image/webp")
		if err != nil || got != "https://cdn.example.test/images/v1/hash.webp" {
			t.Fatalf("attempt %d: url=%q error=%v", i, got, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("got %d attempts, want first create and then conditional reuse", calls.Load())
	}
}

func TestUploadRetriesTransientResponses(t *testing.T) {
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
	_, err := Upload(context.Background(), server.Client(), testUploadConfig(server.URL), "retry.webp", []byte("same payload"), "image/webp")
	if err != nil || calls.Load() != 3 {
		t.Fatalf("retry result: calls=%d err=%v", calls.Load(), err)
	}
}

func TestUploadRetryAfterAmbiguousCommitReceivesPreconditionFailed(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer server.Close()
	_, err := Upload(context.Background(), server.Client(), testUploadConfig(server.URL), "retry.webp", []byte("same payload"), "image/webp")
	if err != nil || calls.Load() != 2 {
		t.Fatalf("ambiguous commit: calls=%d err=%v", calls.Load(), err)
	}
}

func TestUploadPermanentFailuresDoNotRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	_, err := Upload(context.Background(), server.Client(), testUploadConfig(server.URL), "failed.webp", []byte("payload"), "image/webp")
	if err == nil || !strings.Contains(err.Error(), "403") || calls.Load() != 1 {
		t.Fatalf("permanent error: calls=%d err=%v", calls.Load(), err)
	}
}

func TestUploadBoundedRetries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	_, err := Upload(context.Background(), server.Client(), testUploadConfig(server.URL), "failed.webp", []byte("payload"), "image/webp")
	if err == nil || !strings.Contains(err.Error(), "503") || calls.Load() != maxUploadAttempts {
		t.Fatalf("retry budget: calls=%d err=%v", calls.Load(), err)
	}
}

func TestUploadHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Upload(ctx, &http.Client{Timeout: time.Second}, testUploadConfig("https://example.test"), "noop.webp", []byte("payload"), "image/webp")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func TestUploadRetriesNetworkFailure(t *testing.T) {
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
	_, err := Upload(context.Background(), client, testUploadConfig(server.URL), "retry.webp", []byte("payload"), "image/webp")
	if err != nil || calls.Load() != 2 {
		t.Fatalf("network retry result: calls=%d err=%v", calls.Load(), err)
	}
}

package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type bridgeRecoveryRoundTripper func(*http.Request) (*http.Response, error)

func (f bridgeRecoveryRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestMCPReadRecoversAfterBridgeTokenRotation(t *testing.T) {
	calls, refreshed := 0, 0
	c := &mcpBridgeClient{
		ready: true,
		state: bridgeState{BaseURL: "http://127.0.0.1:32146", Token: "previous"},
		stateProvider: func() (bridgeState, error) {
			refreshed++
			return bridgeState{BaseURL: "http://127.0.0.1:32146", Token: "current"}, nil
		},
		http: &http.Client{Transport: bridgeRecoveryRoundTripper(func(r *http.Request) (*http.Response, error) {
			calls++
			status, payload := http.StatusOK, `{"ok":true}`
			if r.Header.Get("x-thinkerqaq-token") != "current" {
				status, payload = http.StatusUnauthorized, `{"error":{"message":"invalid bridge token"}}`
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header), Request: r}, nil
		})},
	}
	result, err := c.call(context.Background(), http.MethodGet, "/v1/articles", nil)
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("read did not recover: result=%s err=%v", result, err)
	}
	if calls != 2 || refreshed != 1 {
		t.Fatalf("attempts=%d refreshes=%d; expected one transparent retry", calls, refreshed)
	}
	if _, err = c.call(context.Background(), http.MethodGet, "/v1/articles", nil); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || refreshed != 1 {
		t.Fatalf("read after recovery attempts=%d refreshes=%d", calls, refreshed)
	}
}

func TestMCPWriteNeverReplayedOnBridgeTokenRotation(t *testing.T) {
	calls, writes, refreshed := 0, 0, 0
	c := &mcpBridgeClient{
		ready: true,
		state: bridgeState{BaseURL: "http://127.0.0.1:32146", Token: "previous"},
		stateProvider: func() (bridgeState, error) {
			refreshed++
			return bridgeState{BaseURL: "http://127.0.0.1:32146", Token: "current"}, nil
		},
		http: &http.Client{Transport: bridgeRecoveryRoundTripper(func(r *http.Request) (*http.Response, error) {
			calls++
			status, payload := http.StatusOK, `{"job":{"id":"safe"}}`
			if r.Header.Get("x-thinkerqaq-token") != "current" {
				status, payload = http.StatusUnauthorized, `{"error":{"message":"invalid bridge token"}}`
			} else {
				writes++
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header), Request: r}, nil
		})},
	}
	_, err := c.call(context.Background(), http.MethodPost, "/v1/sync/jobs", map[string]any{"requestId": "test-unique-request"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("write must report rejected attempt, got %v", err)
	}
	if calls != 1 || writes != 0 || refreshed != 0 {
		t.Fatalf("unsafe retry on write: calls=%d writes=%d refreshes=%d", calls, writes, refreshed)
	}
	_, err = c.call(context.Background(), http.MethodPost, "/v1/sync/jobs", map[string]any{"requestId": "test-unique-request"})
	if err != nil || calls != 2 || writes != 1 || refreshed != 1 {
		t.Fatalf("explicit second write did not recover safely: calls=%d writes=%d refreshes=%d err=%v", calls, writes, refreshed, err)
	}
}

func TestMCPTokenRefreshRejectsUnexpectedLocalOrigin(t *testing.T) {
	c := &mcpBridgeClient{stateProvider: func() (bridgeState, error) {
		return bridgeState{BaseURL: "https://attacker.invalid", Token: "must-not-send"}, nil
	}}
	if _, err := c.call(context.Background(), http.MethodGet, "/v1/articles", nil); err == nil || !strings.Contains(err.Error(), "invalid local SyndiRelay Bridge origin") {
		t.Fatalf("untrusted origin was not rejected: %v", err)
	}
}

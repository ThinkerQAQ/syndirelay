package publisher

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestToutiaoCSRFPreflightUsesPlainHTTPAndValidatesExpiry(t *testing.T) {
	now := time.Date(2026, 10, 9, 22, 0, 0, 0, time.UTC)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "HEAD" || r.URL.Host != "mp.toutiao.com" || r.URL.Path != "/spice/image" ||
			r.Header.Get("x-secsdk-csrf-request") != "1" || r.Header.Get("x-secsdk-csrf-version") != "1.2.22" {
			t.Fatalf("unexpected preflight %s %s", r.Method, r.URL)
		}
		resp := jsonResponse(r, 200, "", map[string]string{"x-ware-csrf-token": "0,synthetic-opaque-token-123,90000,any,extra"})
		return resp, nil
	})}
	result, err := fetchToutiaoCSRF(context.Background(), client, "synthetic UA", now)
	if err != nil || result.Token != "synthetic-opaque-token-123" || calls != 1 {
		t.Fatalf("invalid credential metadata: err=%v length=%d calls=%d", err, len(result.Token), calls)
	}
	if result.ExpiresAt.Sub(now) != 90*time.Second {
		t.Fatal("wrong CSRF token TTL")
	}
}

func TestToutiaoCSRFPreflightRejectsMissingExpiredOrMalformedToken(t *testing.T) {
	now := time.Now()
	for _, input := range []string{"", "1,synthetic-token-value,90000", "0,short,90000", "0,synthetic-token-value,bad", "0,synthetic-token-value,0", "0,synthetic-token-value\nforged,10000"} {
		if _, err := parseToutiaoWareCSRF(input, now); err == nil {
			t.Fatal("malformed CSRF response accepted")
		}
	}
	valid, err := parseToutiaoWareCSRF("0,synthetic-token-value,999999999,extra", now)
	if err != nil || valid.ExpiresAt.Sub(now) != 24*time.Hour {
		t.Fatalf("TTL cap: %v", err)
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(r, 403, "", nil), nil
	})}
	_, err = fetchToutiaoCSRF(context.Background(), client, "", now)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("failed to reject invalid status: %v", err)
	}
}

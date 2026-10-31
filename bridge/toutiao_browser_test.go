package bridge

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestToutiaoBrowserTransportClaimsWriteOnceAndReturnsResponse(t *testing.T) {
	hub := newToutiaoBrowserHub()
	transport := toutiaoBrowserTransport{hub: hub, base: mediumRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatal("article write escaped to native transport")
		return nil, nil
	})}
	done := make(chan error, 1)
	go func() {
		req, _ := http.NewRequest(http.MethodPost, "https://mp.toutiao.com/mp/agw/article/publish", strings.NewReader("save=1&pgc_id=123"))
		res, err := transport.RoundTrip(req)
		if err == nil {
			defer res.Body.Close()
			b, _ := io.ReadAll(res.Body)
			if res.StatusCode != 200 || string(b) != `{"code":0}` {
				err = io.ErrUnexpectedEOF
			}
		}
		done <- err
	}()
	var item *toutiaoBrowserRequest
	deadline := time.Now().Add(time.Second)
	for item == nil && time.Now().Before(deadline) {
		item = hub.next()
		if item == nil {
			time.Sleep(time.Millisecond)
		}
	}
	if item == nil || item.Body != "save=1&pgc_id=123" {
		t.Fatal("write was not queued")
	}
	if hub.next() != nil {
		t.Fatal("ambiguous write was redelivered")
	}
	if !hub.complete(toutiaoBrowserReply{ID: item.ID, Status: 200, Body: `{"code":0}`}) {
		t.Fatal("completion rejected")
	}
	if hub.complete(toutiaoBrowserReply{ID: item.ID}) {
		t.Fatal("duplicate completion accepted")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestToutiaoBrowserTransportKeepsReadsNative(t *testing.T) {
	calls := 0
	transport := toutiaoBrowserTransport{hub: newToutiaoBrowserHub(), base: mediumRoundTripFunc(func(r *http.Request) (*http.Response, error) { calls++; return mediumResponse(r, 200, `{}`, nil), nil })}
	req, _ := http.NewRequest(http.MethodGet, "https://mp.toutiao.com/mp/agw/media/get_media_info", nil)
	res, err := transport.RoundTrip(req)
	if err != nil || calls != 1 {
		t.Fatalf("read transport: %v", err)
	}
	res.Body.Close()
}
func TestToutiaoBrowserEndpointRejectsUntrustedOrMissingToken(t *testing.T) {
	server, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ origin, token string }{{"https://evil.example", "test-token"}, {"chrome-extension://test", ""}} {
		req := httptest.NewRequest(http.MethodGet, "/v1/toutiao/browser/next", nil)
		req.Header.Set("origin", tc.origin)
		req.Header.Set("x-thinkerqaq-token", tc.token)
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Fatalf("unauthorized claim: %d", rec.Code)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := server.toutiaoRequests.send(ctx, "save=1"); err == nil {
		t.Fatal("cancelled write succeeded")
	}
	if server.toutiaoRequests.next() != nil {
		t.Fatal("cancelled request remained pending")
	}
}

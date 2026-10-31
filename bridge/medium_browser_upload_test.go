package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

func TestMediumBrowserUploadHandoff(t *testing.T) {
	hub := newMediumBrowserUploadHub()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type result struct {
		out mediumUploadResult
		err error
	}
	done := make(chan result, 1)
	go func() {
		v, e := hub.upload(ctx, "2b52b8609f8e", publisher.RehostImage{Source: "cover.png", Payload: []byte("\x89PNG\r\n\x1a\n"), ContentType: "image/png"})
		done <- result{v, e}
	}()
	var task map[string]any
	for attempt := 0; attempt < 100; attempt++ {
		task = hub.next()
		if task != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if task == nil {
		t.Fatal("no pending Medium browser task")
	}
	if task["draftId"] != "2b52b8609f8e" || task["mimeType"] != "image/png" || task["filename"] != "cover.png" {
		t.Fatalf("invalid safe task metadata: %+v", task)
	}
	if hub.next() != nil {
		t.Fatal("pending task was claimed twice")
	}
	if err := hub.complete(task["id"].(string), "1*sample", 300, 200, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err != nil || got.out.FileID != "1*sample" || got.out.Width != 300 {
			t.Fatalf("result: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("upload did not complete")
	}
	if hub.next() != nil {
		t.Fatal("completed upload remains pending")
	}
}
func TestMediumBrowserUploadFailsClosed(t *testing.T) {
	hub := newMediumBrowserUploadHub()
	if _, err := hub.upload(context.Background(), "../evil", publisher.RehostImage{Payload: []byte("png"), ContentType: "image/png"}); err == nil {
		t.Fatal("invalid draft ID accepted")
	}
	if err := hub.complete("made-up", "1*id", 1, 1, ""); err == nil {
		t.Fatal("unsolicited result accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := hub.upload(ctx, "2b52b8609f8e", publisher.RehostImage{Payload: []byte("png"), ContentType: "image/png"}); err == nil {
		t.Fatal("timeout must not silently succeed")
	}
}
func TestMediumBrowserUploadBridgeRequiresExtensionAndToken(t *testing.T) {
	s, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	handler := httptest.NewServer(s.Handler())
	defer handler.Close()
	for _, tc := range []struct {
		origin, token string
		status        int
	}{
		{"https://thinkerqaq.com", "test-token", http.StatusForbidden},
		{"", "wrong", http.StatusForbidden},
		{"chrome-extension://approved", "wrong", http.StatusForbidden},
		// Real Edge extension GET requests can have no Origin header.
		{"", "test-token", http.StatusOK},
		{"chrome-extension://approved", "test-token", http.StatusOK},
	} {
		req, err := http.NewRequest(http.MethodGet, handler.URL+"/v1/medium/browser-upload/next", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("x-thinkerqaq-token", tc.token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		_ = res.Body.Close()
		if res.StatusCode != tc.status {
			t.Fatalf("wrong authorization status: got %d want %d", res.StatusCode, tc.status)
		}
	}
}

func TestMediumBrowserUploadClaimRecoversAfterWorkerSuspension(t *testing.T) {
	hub := newMediumBrowserUploadHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := hub.upload(ctx, "2b52b8609f8e", publisher.RehostImage{Source: "cover.png", Payload: []byte("sample"), ContentType: "image/png"})
		done <- err
	}()
	var task map[string]any
	for attempt := 0; attempt < 100; attempt++ {
		task = hub.next()
		if task != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if task == nil {
		t.Fatal("missing image task")
	}
	if second := hub.next(); second != nil {
		t.Fatal("claimed image issued twice without lease expiry")
	}
	hub.mu.Lock()
	hub.pending[task["id"].(string)].claimedAt = time.Now().Add(-46 * time.Second)
	hub.mu.Unlock()
	retried := hub.next()
	if retried == nil || retried["id"] != task["id"] {
		t.Fatal("crashed browser worker never recovered the upload")
	}
	if err := hub.complete(task["id"].(string), "1*safe", 640, 480, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("upload result not delivered")
	}
}

func TestMediumBrowserUploadResultRequiresExtensionOrigin(t *testing.T) {
	server, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	web := httptest.NewServer(server.Handler())
	defer web.Close()
	for _, tc := range []struct {
		origin, token string
		want          int
	}{
		{"", "test-token", http.StatusForbidden},
		{"https://untrusted.example", "test-token", http.StatusForbidden},
		{"chrome-extension://approved", "wrong", http.StatusForbidden},
		{"chrome-extension://approved", "test-token", http.StatusConflict},
	} {
		request, err := http.NewRequest(http.MethodPost, web.URL+"/v1/medium/browser-upload/result", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", tc.origin)
		request.Header.Set("x-thinkerqaq-token", tc.token)
		res, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != tc.want {
			t.Errorf("POST result origin=%q status=%d want=%d", tc.origin, res.StatusCode, tc.want)
		}
	}
}

package bridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	blogcompiler "github.com/ThinkerQAQ/syndirelay/compiler"
)

func impactFixture(t *testing.T) (bridgeConfig, blogcompiler.CompiledArticle) {
	t.Helper()
	dir := useIsolatedUserConfigDir(t)
	c := defaultBridgeConfig()
	c.Publishing.Site.Origin = "https://example.org"
	c.Impact = ImpactConfig{Enabled: true, Endpoint: "http://127.0.0.1:19452", TokenFile: filepath.Join(dir, "synthetic-token"), SourceID: "fixture", PrimaryLanguage: "zh-CN", MonitorNew: true}
	if err := os.WriteFile(c.Impact.TokenFile, []byte("synthetic-local-only-token"), 0600); err != nil {
		t.Fatal(err)
	}
	a := blogcompiler.CompiledArticle{Slug: "article", Title: "Public title", Platform: "medium", Language: "en", CanonicalURL: "https://example.org/en/articles/article/", Markdown: "private content must stay local", HTML: "private HTML", ContentHash: "private-content-hash", SourceDir: "private-local-path"}
	return c, a
}
func TestImpactPublicMetadataAndStableIdentity(t *testing.T) {
	c, a := impactFixture(t)
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	first, err := makeImpactEvent(c, a, "https://medium.com/@fixture/article", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := makeImpactEvent(c, a, "https://medium.com/@fixture/article", now.Add(time.Hour))
	if err != nil || first.ID != second.ID || first.Payload.RequestID != second.Payload.RequestID {
		t.Fatal("replay identity changed")
	}
	raw, _ := json.Marshal(first)
	for _, private := range []string{a.Markdown, a.HTML, a.ContentHash, a.SourceDir, "synthetic-local-only-token", c.Impact.TokenFile} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private data in public metadata")
		}
	}
	work := first.Payload.Manifest.Works[0]
	if work.CanonicalURL != "https://example.org/articles/article/" || work.Channels[1].URL != a.CanonicalURL || work.Channels[1].Platform != "example.org" || work.ExternalID != "article" {
		t.Fatalf("primary language or provenance lost: %+v", work)
	}
	if first.Payload.Manifest.SourceID != "fixture" || first.Payload.Manifest.Producer != "SyndiRelay" {
		t.Fatal("provenance missing")
	}
}
func TestImpactRejectsRemoteReceiverAndRedirects(t *testing.T) {
	c, a := impactFixture(t)
	for _, endpoint := range []string{"https://example.org", "http://localhost", "http://127.0.0.1:1/path", "http://user@127.0.0.1:1", "http://127.0.0.1:1?token=x"} {
		c.Impact.Endpoint = endpoint
		if c.Impact.validate() == nil {
			t.Fatal("unsafe receiver accepted", endpoint)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.org/credential-sink", 302)
	}))
	defer server.Close()
	c.Impact.Endpoint = server.URL
	event, err := makeImpactEvent(c, a, "https://medium.com/@fixture/article", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if problem := sendImpact(c.Impact, event.Payload); problem != "receiver_http_302" {
		t.Fatal("redirect followed", problem)
	}
}
func TestImpactDurableRetryWithoutPublishingAgain(t *testing.T) {
	c, a := impactFixture(t)
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer synthetic-local-only-token" || r.URL.Path != "/api/actions/import_catalog" {
			t.Error("receiver auth or path")
		}
		var payload impactPayload
		if json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload.SelectedIDs) != 1 {
			t.Error("invalid catalog payload")
		}
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"dry_run":false,"rows":[{"work_id":42,"state":"new"}]}`))
	}))
	defer server.Close()
	c.Impact.Endpoint = server.URL
	event, err := makeImpactEvent(c, a, "https://medium.com/@fixture/article", now)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := impactDir()
	if err = writeImpactEvent(dir, event); err != nil {
		t.Fatal(err)
	}
	first := &Server{config: c, now: func() time.Time { return now }}
	first.deliverImpact(false)
	rows, err := readImpactEvents(dir)
	if err != nil || len(rows) != 1 || rows[0].Attempts != 1 || rows[0].Problem != "receiver_http_503" || rows[0].DeliveredAt != nil {
		t.Fatal("failure not retained", rows, err)
	}
	// A new Server object simulates restart; delivery uses only saved public metadata.
	restarted := &Server{config: c, now: func() time.Time { return now.Add(10 * time.Minute) }}
	restarted.deliverImpact(false)
	restarted.deliverImpact(true)
	rows, err = readImpactEvents(dir)
	if err != nil || rows[0].DeliveredAt == nil || rows[0].Attempts != 2 || calls != 2 {
		t.Fatal("restart retry or replay failed", rows, err, calls)
	}
}
func TestImpactInvalidAcknowledgementStaysPending(t *testing.T) {
	c, a := impactFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ok":true}`)) }))
	defer server.Close()
	c.Impact.Endpoint = server.URL
	e, err := makeImpactEvent(c, a, "https://medium.com/@fixture/article", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if problem := sendImpact(c.Impact, e.Payload); problem != "receiver_response_invalid" {
		t.Fatal("invalid acknowledgement accepted", problem)
	}
}

func TestImpactSlowReceiverDoesNotBlockPublicationEnqueue(t *testing.T) {
	c, a := impactFixture(t)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		w.Write([]byte(`{"dry_run":false,"rows":[{"work_id":42}]}`))
	}))
	defer receiver.Close()
	c.Impact.Endpoint = receiver.URL
	now := time.Now()
	event, err := makeImpactEvent(c, a, "https://medium.com/@fixture/first", now)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := impactDir()
	if err = writeImpactEvent(dir, event); err != nil {
		t.Fatal(err)
	}
	s := &Server{config: c, now: func() time.Time { return now }}
	done := make(chan struct{})
	go func() { s.deliverImpact(false); close(done) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("delivery did not start")
	}
	enqueued := make(chan struct{})
	go func() { s.queueImpact(c, a, "https://medium.com/@fixture/second"); close(enqueued) }()
	select {
	case <-enqueued:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("publication waited for receiver network")
	}
	close(release)
	<-done
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("second queued publication was lost")
	}
	s.impactDeliveryMu.Lock()
	s.impactDeliveryMu.Unlock()
	rows, err := readImpactEvents(dir)
	if err != nil || len(rows) != 2 {
		t.Fatal("publications not durable", rows, err)
	}
	for _, row := range rows {
		if row.DeliveredAt == nil {
			t.Fatal("queued publication not delivered")
		}
	}
}

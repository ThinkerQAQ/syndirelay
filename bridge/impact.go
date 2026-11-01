package bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	blogcompiler "github.com/ThinkerQAQ/syndirelay/compiler"
)

// Only public publication metadata enters this outbox. Never compiled content,
// cookies, configuration secrets, or draft URLs.
type ImpactConfig struct {
	Enabled         bool   `json:"enabled" toml:"enabled"`
	Endpoint        string `json:"endpoint" toml:"endpoint"`
	TokenFile       string `json:"tokenFile" toml:"token_file"`
	SourceID        string `json:"sourceId" toml:"source_id"`
	PrimaryLanguage string `json:"primaryLanguage" toml:"primary_language"`
	MonitorNew      bool   `json:"monitorNew" toml:"monitor_new"`
}

func (c ImpactConfig) validate() error {
	if !c.Enabled {
		return nil
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		return errors.New("Reverfolio endpoint must be an HTTP loopback origin")
	}
	if !filepath.IsAbs(c.TokenFile) {
		return errors.New("Reverfolio token_file must reference an existing absolute local path")
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`).MatchString(c.SourceID) {
		return errors.New("invalid Reverfolio source_id")
	}
	if c.PrimaryLanguage != "zh-CN" && c.PrimaryLanguage != "en" {
		return errors.New("choose Reverfolio primary_language: zh-CN or en")
	}
	return nil
}

type impactChannel struct {
	Platform string `json:"platform"`
	Language string `json:"language"`
	URL      string `json:"url"`
}
type impactWork struct {
	ExternalID   string          `json:"external_id"`
	Kind         string          `json:"kind"`
	Title        string          `json:"title"`
	CanonicalURL string          `json:"canonical_url"`
	Enabled      bool            `json:"enabled"`
	Channels     []impactChannel `json:"channels"`
}
type impactManifest struct {
	SchemaVersion int          `json:"schema_version"`
	SourceID      string       `json:"source_id"`
	Producer      string       `json:"producer"`
	Works         []impactWork `json:"works"`
}
type impactPayload struct {
	RequestID   string         `json:"request_id"`
	SelectedIDs []string       `json:"selected_ids"`
	Manifest    impactManifest `json:"manifest"`
}
type impactEvent struct {
	ID          string        `json:"id"`
	CreatedAt   time.Time     `json:"created_at"`
	Attempts    int           `json:"attempts"`
	NextAttempt time.Time     `json:"next_attempt"`
	DeliveredAt *time.Time    `json:"delivered_at,omitempty"`
	Problem     string        `json:"problem,omitempty"`
	Payload     impactPayload `json:"payload"`
}

func publicImpactURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
func makeImpactEvent(c bridgeConfig, a blogcompiler.CompiledArticle, publicURL string, now time.Time) (impactEvent, error) {
	if err := c.Impact.validate(); err != nil {
		return impactEvent{}, err
	}
	if !publicImpactURL(publicURL) || a.Slug == "" || strings.Contains(a.Slug, "..") || a.Title == "" {
		return impactEvent{}, errors.New("invalid public publication metadata")
	}
	parts := strings.Split(a.Slug, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	canonical := strings.TrimRight(c.Publishing.Site.Origin, "/") + c.Publishing.Site.ArticlePath(c.Impact.PrimaryLanguage) + strings.Join(parts, "/") + "/"
	if !publicImpactURL(canonical) || !publicImpactURL(a.CanonicalURL) {
		return impactEvent{}, errors.New("configured public canonical URLs are required")
	}
	channels := []impactChannel{{Platform: a.Platform, Language: a.Language, URL: publicURL}}
	if a.CanonicalURL != canonical {
		// Match the existing site's channel identity by its public origin.
		// A generic "blog" label conflicts with previously imported channels
		// (e.g. platform "example.org") for the exact same URL and language.
		canonicalHost, err := url.Parse(a.CanonicalURL)
		if err != nil || canonicalHost.Hostname() == "" || len(canonicalHost.Hostname()) > 80 {
			return impactEvent{}, errors.New("invalid canonical channel hostname")
		}
		channels = append(channels, impactChannel{Platform: strings.ToLower(canonicalHost.Hostname()), Language: a.Language, URL: a.CanonicalURL})
	}
	work := impactWork{ExternalID: a.Slug, Kind: "article", Title: a.Title, CanonicalURL: canonical, Enabled: c.Impact.MonitorNew, Channels: channels}
	payload := impactPayload{SelectedIDs: []string{a.Slug}, Manifest: impactManifest{SchemaVersion: 1, SourceID: c.Impact.SourceID, Producer: "SyndiRelay", Works: []impactWork{work}}}
	raw, _ := json.Marshal(payload)
	hash := sha256.Sum256(raw)
	id := hex.EncodeToString(hash[:])
	payload.RequestID = "syndirelay-" + id
	return impactEvent{ID: id, CreatedAt: now, NextAttempt: now, Payload: payload}, nil
}
func impactDir() (string, error) {
	dir, err := ConfigDir()
	return filepath.Join(dir, "reverfolio-outbox"), err
}
func writeImpactEvent(dir string, event impactEvent) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".event-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return replaceImpactFile(name, filepath.Join(dir, event.ID+".json"))
}
func replaceImpactFile(from, to string) error { return os.Rename(from, to) }
func readImpactEvents(dir string) ([]impactEvent, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []impactEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	events := []impactEvent{}
	for _, entry := range entries {
		if entry.IsDir() || !regexp.MustCompile(`^[a-f0-9]{64}\.json$`).MatchString(entry.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var event impactEvent
		if json.Unmarshal(raw, &event) != nil || event.ID+".json" != entry.Name() {
			return nil, errors.New("invalid outbox event")
		}
		events = append(events, event)
	}
	return events, nil
}
func (s *Server) queueImpact(c bridgeConfig, a blogcompiler.CompiledArticle, publicURL string) {
	if !c.Impact.Enabled {
		return
	}
	event, err := makeImpactEvent(c, a, publicURL, s.now())
	s.impactMu.Lock()
	defer s.impactMu.Unlock()
	if err != nil {
		s.impactProblem = "public_metadata_invalid"
		return
	}
	dir, err := impactDir()
	if err != nil {
		s.impactProblem = "outbox_path_failed"
		return
	}
	if _, err = os.Stat(filepath.Join(dir, event.ID+".json")); err == nil {
		return
	}
	if err = writeImpactEvent(dir, event); err != nil {
		s.impactProblem = "outbox_write_failed"
		return
	}
	s.impactProblem = ""
	go s.deliverImpact(false)
}
func (s *Server) impactIssue(problem string) {
	s.impactMu.Lock()
	s.impactProblem = problem
	s.impactMu.Unlock()
}
func (s *Server) deliverImpact(force bool) {
	// Sending never holds the file mutex: publishing can enqueue while a receiver is offline.
	s.impactDeliveryMu.Lock()
	defer s.impactDeliveryMu.Unlock()
	s.mu.Lock()
	c := s.config.Impact
	s.mu.Unlock()
	if !c.Enabled {
		return
	}
	if c.validate() != nil {
		s.impactIssue("configuration_invalid")
		return
	}
	dir, err := impactDir()
	if err != nil {
		s.impactIssue("outbox_path_failed")
		return
	}
	s.impactMu.Lock()
	events, err := readImpactEvents(dir)
	s.impactMu.Unlock()
	if err != nil {
		s.impactIssue("outbox_read_failed")
		return
	}
	for _, event := range events {
		if event.DeliveredAt != nil || (!force && s.now().Before(event.NextAttempt)) {
			continue
		}
		s.mu.Lock()
		current := s.config.Impact
		s.mu.Unlock()
		if current != c {
			return
		}
		event.Attempts++
		event.Problem = sendImpact(c, event.Payload)
		if event.Problem == "" {
			at := s.now()
			event.DeliveredAt = &at
		} else {
			power := event.Attempts
			if power > 8 {
				power = 8
			}
			event.NextAttempt = s.now().Add(time.Duration(1<<power) * time.Minute)
		}
		s.impactMu.Lock()
		err = writeImpactEvent(dir, event)
		s.impactMu.Unlock()
		if err != nil {
			s.impactIssue("outbox_write_failed")
			return
		}
	}
	s.impactIssue("")
}
func sendImpact(c ImpactConfig, payload impactPayload) string {
	// The application reads its configured local token; the agent and UI never do.
	file, err := os.Open(c.TokenFile)
	if err != nil {
		return "credential_file_unavailable"
	}
	token, err := io.ReadAll(io.LimitReader(file, 65537))
	file.Close()
	if err != nil || len(token) > 65536 || len(bytes.TrimSpace(token)) == 0 {
		return "credential_file_invalid"
	}
	raw, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.Endpoint, "/")+"/api/actions/import_catalog", bytes.NewReader(raw))
	if err != nil {
		return "request_invalid"
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return "receiver_unavailable"
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Sprintf("receiver_http_%d", response.StatusCode)
	}
	var result struct {
		DryRun bool `json:"dry_run"`
		Rows   []struct {
			WorkID int64 `json:"work_id"`
		} `json:"rows"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&result) != nil || result.DryRun || len(result.Rows) != 1 || result.Rows[0].WorkID < 1 {
		return "receiver_response_invalid"
	}
	return ""
}
func (s *Server) startImpactWorker() {
	s.impactMu.Lock()
	if s.impactCancel != nil {
		s.impactMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.impactCancel = cancel
	s.impactMu.Unlock()
	go func() {
		s.deliverImpact(false)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.deliverImpact(false)
			}
		}
	}()
}
func (s *Server) handleImpact(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if !allowReadOnlyBridgeStatus(w, r) {
			return
		}
		s.mu.Lock()
		config := s.config.Impact
		s.mu.Unlock()
		s.impactMu.Lock()
		dir, err := impactDir()
		var events []impactEvent
		if err == nil {
			events, err = readImpactEvents(dir)
		}
		problem := s.impactProblem
		s.impactMu.Unlock()
		if err != nil {
			problem = "outbox_read_failed"
		}
		writeJSON(w, 200, map[string]any{"ok": true, "config": config, "events": events, "problem": problem})
		return
	}
	if _, ok := allowExtensionWrite(w, r); !ok {
		return
	}
	if r.URL.Path == "/v1/impact/retry" && r.Method == http.MethodPost {
		go s.deliverImpact(true)
		writeJSON(w, 202, map[string]any{"ok": true})
		return
	}
	if r.Method != http.MethodPut {
		writeAPIError(w, 405, "method_not_allowed", "Use GET or PUT", nil)
		return
	}
	var patch ImpactConfig
	if err := readJSON(r, 64*1024, &patch); err != nil {
		writeError(w, err)
		return
	}
	if err := patch.validate(); err != nil {
		writeAPIError(w, 400, "invalid_request", err.Error(), nil)
		return
	}
	s.mu.Lock()
	current := s.config
	s.mu.Unlock()
	current.Impact = patch
	if err := saveBridgeConfig(current); err != nil {
		writeAPIError(w, 500, "internal_error", "Could not save integration configuration", nil)
		return
	}
	s.mu.Lock()
	s.config.Impact = patch
	s.mu.Unlock()
	s.startImpactWorker()
	writeJSON(w, 200, map[string]any{"ok": true, "config": patch})
}

func (s *Server) queueCurrentImpact(a blogcompiler.CompiledArticle, publicURL string) {
	s.mu.Lock()
	c := s.config
	s.mu.Unlock()
	s.queueImpact(c, a, publicURL)
}

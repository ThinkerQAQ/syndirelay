package bridge

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	blogsearch "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/search"
)

type searchInventoryState struct {
	Source              string            `json:"source"`
	FingerprintSource   string            `json:"fingerprintSource,omitempty"`
	FingerprintCoverage int               `json:"fingerprintCoverage,omitempty"`
	Origin              string            `json:"origin"`
	FetchedAt           string            `json:"fetchedAt"`
	Total               int               `json:"total"`
	URLs                []string          `json:"urls,omitempty"`
	Fingerprints        map[string]string `json:"fingerprints,omitempty"`
}

type searchOperationState struct {
	State          string `json:"state"`
	Mode           string `json:"mode,omitempty"`
	StartedAt      string `json:"startedAt,omitempty"`
	FinishedAt     string `json:"finishedAt,omitempty"`
	Count          int    `json:"count,omitempty"`
	NewCount       int    `json:"newCount,omitempty"`
	ChangedCount   int    `json:"changedCount,omitempty"`
	DeletedCount   int    `json:"deletedCount,omitempty"`
	UnchangedCount int    `json:"unchangedCount,omitempty"`
	HTTPStatus     int    `json:"httpStatus,omitempty"`
	Error          string `json:"error,omitempty"`
}

type searchInspectionResult struct {
	URL             string   `json:"url"`
	Verdict         string   `json:"verdict"`
	CoverageState   string   `json:"coverageState"`
	RobotsTxtState  string   `json:"robotsTxtState"`
	IndexingState   string   `json:"indexingState"`
	LastCrawlTime   string   `json:"lastCrawlTime"`
	PageFetchState  string   `json:"pageFetchState"`
	UserCanonical   string   `json:"userCanonical"`
	GoogleCanonical string   `json:"googleCanonical"`
	CrawledAs       string   `json:"crawledAs"`
	ReferringURLs   []string `json:"referringUrls"`
	Sitemap         []string `json:"sitemap"`
	CheckedAt       string   `json:"checkedAt,omitempty"`
}

type searchInspectionState struct {
	State      string                   `json:"state"`
	StartedAt  string                   `json:"startedAt,omitempty"`
	FinishedAt string                   `json:"finishedAt,omitempty"`
	Offset     int                      `json:"offset"`
	Limit      int                      `json:"limit"`
	Inspected  int                      `json:"inspected"`
	Total      int                      `json:"total"`
	Remaining  int                      `json:"remaining"`
	NextOffset *int                     `json:"nextOffset,omitempty"`
	Results    []searchInspectionResult `json:"results,omitempty"`
	Error      string                   `json:"error,omitempty"`
}

type googleIndexRequestItem struct {
	URL           string `json:"url"`
	Status        string `json:"status"`
	LastAttemptAt string `json:"lastAttemptAt,omitempty"`
	RequestedAt   string `json:"requestedAt,omitempty"`
	Error         string `json:"error,omitempty"`
}

type googleIndexRequestQueue struct {
	JobID             string                   `json:"jobId,omitempty"`
	State             string                   `json:"state"`
	CreatedAt         string                   `json:"createdAt,omitempty"`
	UpdatedAt         string                   `json:"updatedAt,omitempty"`
	CurrentIndex      int                      `json:"currentIndex"`
	ConsecutiveErrors int                      `json:"consecutiveErrors"`
	LastError         string                   `json:"lastError,omitempty"`
	Items             []googleIndexRequestItem `json:"items,omitempty"`
}

type searchIndexState struct {
	Site            blogsearch.SiteConfig `json:"site"`
	ConsoleLinks    map[string]string     `json:"consoleLinks"`
	Inventory       searchInventoryState  `json:"inventory"`
	IndexNow        searchOperationState  `json:"indexNow"`
	Baidu           searchOperationState  `json:"baidu"`
	So360           searchOperationState  `json:"so360"`
	Sogou           searchOperationState  `json:"sogou"`
	BaiduConfigured bool                  `json:"baiduConfigured"`
	Google          struct {
		CredentialsConfigured bool                    `json:"credentialsConfigured"`
		CredentialsError      string                  `json:"credentialsError,omitempty"`
		Sitemaps              searchOperationState    `json:"sitemaps"`
		Inspection            searchInspectionState   `json:"inspection"`
		RequestQueue          googleIndexRequestQueue `json:"requestQueue"`
	} `json:"google"`
}

func defaultSearchIndexState() searchIndexState {
	state := searchIndexState{}
	state.IndexNow.State = "idle"
	state.Baidu.State = "idle"
	state.So360.State = "unsupported"
	state.Sogou.State = "unsupported"
	state.Google.Sitemaps.State = "idle"
	state.Google.Inspection.State = "idle"
	state.Google.RequestQueue.State = "idle"
	return state
}

func searchIndexStatePath() (string, error) {
	configPath, err := ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), "search-index.json"), nil
}

func searchProviderSnapshotPath(filename string) (string, error) {
	configPath, err := ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), filename), nil
}

func loadSearchProviderSnapshot(filename string) searchInventoryState {
	path, err := searchProviderSnapshotPath(filename)
	if err != nil {
		return searchInventoryState{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return searchInventoryState{}
	}
	var snapshot searchInventoryState
	if json.Unmarshal(data, &snapshot) != nil {
		return searchInventoryState{}
	}
	// A previous site's URL list must not be interpreted as removed URLs
	// and submitted to the new domain's IndexNow endpoint.
	return snapshot
}

func saveSearchProviderSnapshot(filename string, snapshot searchInventoryState) error {
	path, err := searchProviderSnapshotPath(filename)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadIndexNowSnapshot() searchInventoryState {
	return loadSearchProviderSnapshot("indexnow-snapshot.json")
}

func loadBaiduIndexSnapshot() searchInventoryState {
	return loadSearchProviderSnapshot("baidu-snapshot.json")
}

func saveIndexNowSnapshot(snapshot searchInventoryState) error {
	return saveSearchProviderSnapshot("indexnow-snapshot.json", snapshot)
}

func saveBaiduIndexSnapshot(snapshot searchInventoryState) error {
	return saveSearchProviderSnapshot("baidu-snapshot.json", snapshot)
}

func compactSearchInventory(inventory searchInventoryState) searchInventoryState {
	inventory.URLs = nil
	inventory.Fingerprints = nil
	return inventory
}

func loadSearchIndexState() searchIndexState {
	state := defaultSearchIndexState()
	path, err := searchIndexStatePath()
	if err != nil {
		return state
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	if json.Unmarshal(data, &state) != nil {
		return defaultSearchIndexState()
	}
	// Search inspection and request queues belong to an origin. A domain
	// migration starts with a new inventory rather than resuming old URLs.
	if state.IndexNow.State == "" {
		state.IndexNow.State = "idle"
	}
	if state.Baidu.State == "" {
		state.Baidu.State = "idle"
	}
	if state.Google.Sitemaps.State == "" {
		state.Google.Sitemaps.State = "idle"
	}
	if state.Google.Inspection.State == "" {
		state.Google.Inspection.State = "idle"
	}
	if state.Google.RequestQueue.State == "" {
		state.Google.RequestQueue.State = "idle"
	}
	return state
}

func saveSearchIndexState(state searchIndexState) error {
	path, err := searchIndexStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func googleInspectionQuotaExceeded(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "http 429") ||
		strings.Contains(message, "resource_exhausted") ||
		strings.Contains(message, "quota exceeded")
}

func googleInspectionQuotaMessage() string {
	return "Google URL Inspection 今日配额已用尽；已保留当前进度，配额恢复后重试即可继续。"
}

func refreshSearchCredentialsFlag(state *searchIndexState, config bridgeConfig) {
	state.BaiduConfigured = baiduToken(config) != ""
	raw := googleSearchConsoleServiceJSON(config)
	state.Google.CredentialsConfigured = false
	state.Google.CredentialsError = ""
	if raw == "" {
		return
	}
	if err := validateGoogleServiceAccountJSON(raw); err != nil {
		state.Google.CredentialsError = err.Error()
		return
	}
	state.Google.CredentialsConfigured = true
}

func applyDurableTaskToOperation(state *searchOperationState, job *durableTaskJob) {
	if state == nil || job == nil {
		return
	}
	switch job.State {
	case "queued", "running":
		state.State = job.State
		state.Error = ""
		state.FinishedAt = ""
		if job.StartedAt != "" {
			state.StartedAt = job.StartedAt
		}
	case "failed":
		state.State = "failed"
		state.Error = job.Error
		state.FinishedAt = job.FinishedAt
	case "completed":
		state.State = "completed"
		state.Error = ""
		state.FinishedAt = job.FinishedAt
	}
}

func applyDurableTaskToInspection(state *searchInspectionState, job *durableTaskJob) {
	if state == nil || job == nil {
		return
	}
	if job.Progress.Total > 0 {
		state.Total = job.Progress.Total
		state.Remaining = max(0, state.Total-state.Inspected)
	}
	var payload googleInspectionTaskPayload
	if len(job.Payload) > 0 && json.Unmarshal(job.Payload, &payload) == nil {
		state.Offset = payload.Offset
		state.Limit = payload.Limit
		if payload.Offset < state.Total {
			next := payload.Offset
			state.NextOffset = &next
		} else if state.Total > 0 {
			state.NextOffset = nil
		}
	}
	switch job.State {
	case "queued", "running":
		state.State = job.State
		state.Error = ""
		state.FinishedAt = ""
		if job.StartedAt != "" {
			state.StartedAt = job.StartedAt
		}
	case "failed":
		state.State = "failed"
		state.Error = job.Error
		state.FinishedAt = job.FinishedAt
	case "paused":
		if reason, _ := job.Detail["reason"].(string); reason == "quota_blocked" {
			state.State = "quota_blocked"
			state.Error = job.Progress.Message
		}
	case "completed":
		state.State = "completed"
		state.Error = ""
		state.FinishedAt = job.FinishedAt
	}
}

func (s *Server) reconcileSearchStateWithDurableTasks(state *searchIndexState) {
	if state == nil {
		return
	}
	applyDurableTaskToOperation(&state.IndexNow, s.latestDurableSearchTask("indexnow-submit"))
	applyDurableTaskToOperation(&state.Baidu, s.latestDurableSearchTask("baidu-submit"))
	applyDurableTaskToOperation(&state.Google.Sitemaps, s.latestDurableSearchTask("google-sitemaps"))
	applyDurableTaskToInspection(&state.Google.Inspection, s.latestDurableSearchTask("google-inspection"))

	if job := s.latestDurableSearchTask("google-request-indexing"); job != nil && job.State == "running" {
		state.Google.RequestQueue.State = "running"
		state.Google.RequestQueue.LastError = ""
		if state.Google.RequestQueue.JobID == "" {
			state.Google.RequestQueue.JobID = job.ID
		}
	}
}

func (s *Server) loadSearchState() searchIndexState {
	state := loadSearchIndexState()
	site := s.searchSite()
	if state.Inventory.Origin != "" && strings.TrimRight(state.Inventory.Origin, "/") != site.Origin {
		state = defaultSearchIndexState()
	}
	if state.Site.GoogleProperty != "" && state.Site.GoogleProperty != site.GoogleProperty {
		state.Google.Inspection = searchInspectionState{State: "idle"}
		state.Google.RequestQueue = googleIndexRequestQueue{State: "idle"}
	}
	state.Site = site
	state.ConsoleLinks = site.ConsoleLinks()
	results := state.Google.Inspection.Results[:0]
	for _, result := range state.Google.Inspection.Results {
		if _, err := blogsearch.NormalizeURL(result.URL, site.Origin, "Inspection URL"); err == nil {
			results = append(results, result)
		}
	}
	state.Google.Inspection.Results = results
	items := state.Google.RequestQueue.Items[:0]
	for _, item := range state.Google.RequestQueue.Items {
		if _, err := blogsearch.NormalizeURL(item.URL, site.Origin, "Request Indexing URL"); err == nil {
			items = append(items, item)
		}
	}
	if len(items) != len(state.Google.RequestQueue.Items) {
		state.Google.RequestQueue.CurrentIndex = 0
	}
	state.Google.RequestQueue.Items = items
	return state
}

func (s *Server) searchState() searchIndexState {
	state := s.loadSearchState()
	refreshSearchCredentialsFlag(&state, s.config)
	s.reconcileSearchStateWithDurableTasks(&state)
	return state
}

func (s *Server) handleSearchIndexGet(response http.ResponseWriter, request *http.Request) {
	if !allowReadOnlyBridgeStatus(response, request) {
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"ok": true, "index": s.searchState()})
}

func (s *Server) handleSearchInventoryRefresh(response http.ResponseWriter, request *http.Request) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	inventory, err := s.fetchSearchInventory(request.Context())
	if err != nil {
		writeError(response, err)
		return
	}
	state := s.loadSearchState()
	reconcileInspectionInventory(&state.Google.Inspection, inventory)
	state.Inventory = compactSearchInventory(inventory)
	refreshSearchCredentialsFlag(&state, s.config)
	if err := saveSearchIndexState(state); err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"ok": true, "index": state})
}

func reconcileInspectionInventory(inspection *searchInspectionState, inventory searchInventoryState) {
	if inspection == nil || len(inventory.URLs) == 0 {
		return
	}
	total := inventory.Total
	if total <= 0 {
		total = len(inventory.URLs)
	}
	allowed := make(map[string]struct{}, len(inventory.URLs))
	for _, url := range inventory.URLs {
		if strings.TrimSpace(url) != "" {
			allowed[url] = struct{}{}
		}
	}
	filtered := make([]searchInspectionResult, 0, min(len(inspection.Results), len(allowed)))
	checked := make(map[string]struct{}, len(inspection.Results))
	for _, result := range inspection.Results {
		if _, ok := allowed[result.URL]; !ok {
			continue
		}
		filtered = append(filtered, result)
		checked[result.URL] = struct{}{}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].URL < filtered[j].URL })

	nextOffset := 0
	for index, url := range inventory.URLs {
		if _, ok := checked[url]; !ok {
			nextOffset = index
			break
		}
		nextOffset = index + 1
	}
	inspection.Results = filtered
	inspection.Inspected = len(filtered)
	inspection.Total = total
	inspection.Remaining = max(0, total-len(filtered))
	if nextOffset < total {
		value := nextOffset
		inspection.NextOffset = &value
	} else {
		inspection.NextOffset = nil
	}
}

func mergeInspectionResults(existing, incoming []searchInspectionResult) []searchInspectionResult {
	byURL := make(map[string]searchInspectionResult, len(existing)+len(incoming))
	for _, result := range existing {
		if result.URL != "" {
			byURL[result.URL] = result
		}
	}
	for _, result := range incoming {
		if result.URL != "" {
			byURL[result.URL] = result
		}
	}
	urls := make([]string, 0, len(byURL))
	for url := range byURL {
		urls = append(urls, url)
	}
	sort.Strings(urls)
	result := make([]searchInspectionResult, 0, len(urls))
	for _, url := range urls {
		result = append(result, byURL[url])
	}
	return result
}

func googleRequestCandidate(result searchInspectionResult) bool {
	if strings.TrimSpace(result.URL) == "" || strings.EqualFold(result.Verdict, "PASS") {
		return false
	}
	if strings.EqualFold(result.RobotsTxtState, "DISALLOWED") {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(result.IndexingState)) {
	case "BLOCKED_BY_META_TAG", "BLOCKED_BY_HTTP_HEADER", "BLOCKED_BY_ROBOTS_TXT":
		return false
	default:
		return true
	}
}

func buildGoogleRequestQueue(results []searchInspectionResult, previous googleIndexRequestQueue, now time.Time) googleIndexRequestQueue {
	previousByURL := make(map[string]googleIndexRequestItem, len(previous.Items))
	for _, item := range previous.Items {
		previousByURL[item.URL] = item
	}
	items := make([]googleIndexRequestItem, 0)
	for _, result := range results {
		if !googleRequestCandidate(result) {
			continue
		}
		item := previousByURL[result.URL]
		if item.URL == "" {
			item = googleIndexRequestItem{URL: result.URL, Status: "queued"}
		}
		switch item.Status {
		case "processing", "quota_blocked":
			item.Status = "queued"
			item.Error = ""
		case "requested", "indexed":
			// Preserve successful outcomes indefinitely. Rebuilding the queue
			// must never spend Google Request Indexing quota on the same URL
			// again unless a future explicit reset/retry feature asks for it.
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].URL < items[j].URL })

	currentIndex := len(items)
	for index, item := range items {
		if item.Status == "queued" || item.Status == "failed" {
			currentIndex = index
			break
		}
	}
	queue := googleIndexRequestQueue{
		JobID: previous.JobID, State: "idle", CreatedAt: previous.CreatedAt, UpdatedAt: now.UTC().Format(time.RFC3339),
		CurrentIndex: currentIndex, Items: items,
	}
	if queue.CreatedAt == "" {
		queue.CreatedAt = queue.UpdatedAt
	}
	if len(items) == 0 || currentIndex >= len(items) {
		queue.State = "completed"
	}
	return queue
}

func (s *Server) handleSearchGoogleRequestQueueCreate(response http.ResponseWriter, request *http.Request) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	state := s.loadSearchState()
	state.Google.RequestQueue = buildGoogleRequestQueue(
		state.Google.Inspection.Results,
		state.Google.RequestQueue,
		s.now(),
	)
	if err := saveSearchIndexState(state); err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"ok": true, "index": state})
}

func (s *Server) handleSearchGoogleRequestQueueUpdate(response http.ResponseWriter, request *http.Request) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	var input struct {
		URL    string `json:"url"`
		Result string `json:"result"`
		Error  string `json:"error"`
	}
	if err := readJSON(request, maxBodyBytes, &input); err != nil {
		writeError(response, err)
		return
	}
	input.URL = strings.TrimSpace(input.URL)
	input.Result = strings.TrimSpace(input.Result)
	if input.URL == "" || input.Result == "" {
		writeAPIError(response, http.StatusBadRequest, "invalid_google_index_result", "url and result are required", nil)
		return
	}
	state := s.loadSearchState()
	queue := &state.Google.RequestQueue
	found := -1
	for index := range queue.Items {
		if queue.Items[index].URL == input.URL {
			found = index
			break
		}
	}
	if found < 0 {
		writeAPIError(response, http.StatusNotFound, "google_index_queue_item_not_found", "Google request-indexing queue item not found", map[string]any{"url": input.URL})
		return
	}
	item := &queue.Items[found]
	item.LastAttemptAt = s.now().UTC().Format(time.RFC3339)
	item.Error = strings.TrimSpace(input.Error)
	advance := true
	switch input.Result {
	case "processing":
		item.Status = "processing"
		queue.LastError = ""
		advance = false
	case "requested_indexing":
		item.Status = "requested"
		item.RequestedAt = item.LastAttemptAt
		queue.ConsecutiveErrors = 0
	case "already_indexed":
		item.Status = "indexed"
		queue.ConsecutiveErrors = 0
	case "quota_blocked":
		item.Status = "quota_blocked"
		queue.State = "quota_blocked"
		queue.LastError = item.Error
		advance = false
	case "rate_limited":
		item.Status = "failed"
		queue.State = "paused"
		queue.LastError = item.Error
		advance = false
	case "not_logged_in":
		item.Status = "failed"
		queue.State = "paused"
		queue.LastError = item.Error
		advance = false
	case "ui_changed", "timeout":
		item.Status = "failed"
		queue.ConsecutiveErrors++
		queue.State = "paused"
		queue.LastError = item.Error
		advance = false
	case "failed":
		item.Status = "failed"
		queue.ConsecutiveErrors++
		queue.State = "paused"
		queue.LastError = item.Error
		advance = false
	default:
		writeAPIError(response, http.StatusBadRequest, "invalid_google_index_result", "unsupported Google request-indexing result", map[string]any{"result": input.Result})
		return
	}
	if advance && found >= queue.CurrentIndex {
		queue.CurrentIndex = found + 1
	}
	if queue.CurrentIndex >= len(queue.Items) && queue.State != "quota_blocked" && queue.State != "paused" {
		queue.State = "completed"
	}
	queue.UpdatedAt = s.now().UTC().Format(time.RFC3339)
	slog.Info("google request-indexing queue updated",
		"operation", "request-indexing-queue",
		"jobId", queue.JobID,
		"url", input.URL,
		"result", input.Result,
		"queueState", queue.State,
		"current", queue.CurrentIndex,
		"total", len(queue.Items),
		"consecutiveErrors", queue.ConsecutiveErrors,
	)
	if err := saveSearchIndexState(state); err != nil {
		writeError(response, err)
		return
	}
	var job *durableTaskJob
	if queue.JobID != "" {
		job, _ = s.updateGoogleRequestTaskFromQueue(*queue)
	}
	payload := map[string]any{"ok": true, "index": state}
	if job != nil {
		payload["job"] = durableTaskView(job)
	}
	writeJSON(response, http.StatusOK, payload)
}

func (s *Server) handleSearchGoogleRequestQueueControl(response http.ResponseWriter, request *http.Request, action string) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	state := s.loadSearchState()
	queue := &state.Google.RequestQueue
	switch action {
	case "start", "resume":
		if action == "resume" {
			for index, item := range queue.Items {
				if item.Status == "queued" || item.Status == "failed" || item.Status == "quota_blocked" || item.Status == "processing" {
					queue.CurrentIndex = index
					break
				}
			}
		}
		if len(queue.Items) == 0 || queue.CurrentIndex >= len(queue.Items) {
			writeAPIError(response, http.StatusConflict, "google_index_queue_empty", "Google request-indexing queue has no pending URLs", nil)
			return
		}
		current := &queue.Items[queue.CurrentIndex]
		if current.Status == "quota_blocked" || current.Status == "processing" {
			current.Status = "queued"
			current.Error = ""
		}
		queue.State = "running"
		queue.LastError = ""
		if _, err := s.ensureGoogleRequestTask(queue); err != nil {
			writeError(response, err)
			return
		}
	case "pause":
		queue.State = "paused"
	default:
		writeAPIError(response, http.StatusBadRequest, "invalid_google_index_queue_action", "unsupported Google request-indexing queue action", nil)
		return
	}
	queue.UpdatedAt = s.now().UTC().Format(time.RFC3339)
	if err := saveSearchIndexState(state); err != nil {
		writeError(response, err)
		return
	}
	var job *durableTaskJob
	if queue.JobID != "" {
		job, _ = s.updateGoogleRequestTaskFromQueue(*queue)
	}
	payload := map[string]any{"ok": true, "index": state}
	if job != nil {
		payload["job"] = durableTaskView(job)
	}
	writeJSON(response, http.StatusOK, payload)
}

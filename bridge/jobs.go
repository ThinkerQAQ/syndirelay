package bridge

import (
	"encoding/json"
	"errors"
	blogsearch "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/search"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type taskProgress struct {
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Unit    string `json:"unit,omitempty"`
	Message string `json:"message,omitempty"`
}

type durableTaskJob struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Type       string          `json:"type"`
	Title      string          `json:"title"`
	State      string          `json:"state"`
	CreatedAt  string          `json:"createdAt"`
	StartedAt  string          `json:"startedAt,omitempty"`
	UpdatedAt  string          `json:"updatedAt"`
	FinishedAt string          `json:"finishedAt,omitempty"`
	RetryAt    string          `json:"retryAt,omitempty"`
	Error      string          `json:"error,omitempty"`
	CanRetry   bool            `json:"canRetry,omitempty"`
	CanPause   bool            `json:"canPause,omitempty"`
	CanResume  bool            `json:"canResume,omitempty"`
	Progress   taskProgress    `json:"progress"`
	Detail     map[string]any  `json:"detail,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`

	Article   string                        `json:"article,omitempty"`
	Platforms []string                      `json:"platforms,omitempty"`
	Operation string                        `json:"operation,omitempty"`
	Results   map[string]syncPlatformResult `json:"results,omitempty"`
	Events    []syncJobEvent                `json:"events,omitempty"`
	Output    string                        `json:"output,omitempty"`
	DryRun    bool                          `json:"dryRun,omitempty"`
}

type durableTaskStore struct {
	Version int               `json:"version"`
	Order   []string          `json:"order"`
	Jobs    []*durableTaskJob `json:"jobs"`
}

type taskJobView struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Type       string         `json:"type,omitempty"`
	Title      string         `json:"title,omitempty"`
	State      string         `json:"state"`
	CreatedAt  string         `json:"createdAt,omitempty"`
	StartedAt  string         `json:"startedAt,omitempty"`
	UpdatedAt  string         `json:"updatedAt,omitempty"`
	FinishedAt string         `json:"finishedAt,omitempty"`
	RetryAt    string         `json:"retryAt,omitempty"`
	Error      string         `json:"error,omitempty"`
	CanRetry   bool           `json:"canRetry,omitempty"`
	CanPause   bool           `json:"canPause,omitempty"`
	CanResume  bool           `json:"canResume,omitempty"`
	Progress   taskProgress   `json:"progress"`
	Detail     map[string]any `json:"detail,omitempty"`

	Article   string                        `json:"article,omitempty"`
	Platforms []string                      `json:"platforms,omitempty"`
	Operation string                        `json:"operation,omitempty"`
	Results   map[string]syncPlatformResult `json:"results,omitempty"`
	Events    []syncJobEvent                `json:"events,omitempty"`
	Output    string                        `json:"output,omitempty"`
	DryRun    bool                          `json:"dryRun,omitempty"`
}

func durableTaskStorePath() (string, error) {
	configPath, err := ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), "jobs.json"), nil
}

func loadDurableTaskStore() (map[string]*durableTaskJob, []string) {
	jobs := map[string]*durableTaskJob{}
	order := []string{}
	path, err := durableTaskStorePath()
	if err != nil {
		return jobs, order
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return jobs, order
	}
	var store durableTaskStore
	if json.Unmarshal(data, &store) != nil {
		return jobs, order
	}
	seen := map[string]bool{}
	for _, job := range store.Jobs {
		if job == nil || strings.TrimSpace(job.ID) == "" || seen[job.ID] {
			continue
		}
		clone := *job
		clone.Payload = append(json.RawMessage(nil), job.Payload...)
		if job.Detail != nil {
			clone.Detail = make(map[string]any, len(job.Detail))
			for key, value := range job.Detail {
				clone.Detail[key] = value
			}
		}
		jobs[job.ID] = &clone
		seen[job.ID] = true
	}
	for _, id := range store.Order {
		if jobs[id] != nil {
			order = append(order, id)
			delete(seen, id)
		}
	}
	extra := make([]string, 0, len(seen))
	for id := range seen {
		extra = append(extra, id)
	}
	sort.Strings(extra)
	order = append(order, extra...)
	return jobs, order
}

func (s *Server) persistDurableTasksLocked() error {
	path, err := durableTaskStorePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	store := durableTaskStore{Version: 1, Order: append([]string{}, s.taskJobOrder...)}
	for _, id := range s.taskJobOrder {
		if job := s.taskJobs[id]; job != nil {
			clone := *job
			clone.Payload = append(json.RawMessage(nil), job.Payload...)
			store.Jobs = append(store.Jobs, &clone)
		}
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func normalizeRecoveredDurableTaskJobs(jobs map[string]*durableTaskJob, now time.Time) bool {
	changed := false
	stamp := now.UTC().Format(time.RFC3339)
	for _, job := range jobs {
		if job == nil {
			continue
		}
		switch job.State {
		case "running", "queued":
			if job.Type == "google-request-indexing" {
				job.State = "paused"
				job.CanResume = true
				job.CanPause = false
				job.Error = ""
				if job.Detail == nil {
					job.Detail = map[string]any{}
				}
				job.Detail["recovered"] = "Bridge restarted; waiting for browser resume"
			} else if job.Kind == "search" && job.Type == "google-inspection" {
				job.State = "queued"
				job.CanRetry = true
				job.CanPause = false
				job.CanResume = false
				job.Error = ""
				job.FinishedAt = ""
				job.Progress.Message = "等待从断点继续"
				if job.Detail == nil {
					job.Detail = map[string]any{}
				}
				job.Detail["recovered"] = "Bridge restarted; resuming from persisted inspection cursor"
			} else {
				job.State = "failed"
				var request syncRequest
				if job.Kind == "publishing" {
					_ = json.Unmarshal(job.Payload, &request)
				}
				job.CanRetry = job.Kind != "publishing" || canRetrySyncJobOperation(job.Operation, request)
				job.Error = "Bridge restarted before the task completed"
				if job.Kind == "publishing" {
					for platform, result := range job.Results {
						if result.State == "completed" || result.State == "failed" {
							continue
						}
						result.State = "failed"
						result.Error = job.Error
						result.Message = job.Error
						job.Results[platform] = result
					}
				}
			}
			job.UpdatedAt = stamp
			changed = true
		}
	}
	return changed
}

func (s *Server) latestDurableSearchTask(taskType string) *durableTaskJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := 0; index < len(s.taskJobOrder); index++ {
		job := s.taskJobs[s.taskJobOrder[index]]
		if job == nil || job.Kind != "search" || job.Type != taskType {
			continue
		}
		var binding struct {
			Site *blogsearch.SiteConfig `json:"site"`
		}
		if json.Unmarshal(job.Payload, &binding) != nil {
			continue
		}
		if binding.Site != nil && s.config.Search != nil && *binding.Site != *s.config.Search {
			continue
		}
		return cloneDurableTaskJob(job)
	}
	return nil
}

func cloneDurableTaskJob(job *durableTaskJob) *durableTaskJob {
	if job == nil {
		return nil
	}
	clone := *job
	clone.Payload = append(json.RawMessage(nil), job.Payload...)
	if job.Detail != nil {
		clone.Detail = make(map[string]any, len(job.Detail))
		for key, value := range job.Detail {
			clone.Detail[key] = value
		}
	}
	clone.Platforms = append([]string{}, job.Platforms...)
	clone.Events = append([]syncJobEvent{}, job.Events...)
	if job.Results != nil {
		clone.Results = make(map[string]syncPlatformResult, len(job.Results))
		for platform, result := range job.Results {
			clone.Results[platform] = result
		}
	}
	return &clone
}

func durableTaskView(job *durableTaskJob) taskJobView {
	if job == nil {
		return taskJobView{}
	}
	return taskJobView{
		ID: job.ID, Kind: job.Kind, Type: job.Type, Title: job.Title, State: job.State,
		CreatedAt: job.CreatedAt, StartedAt: job.StartedAt, UpdatedAt: job.UpdatedAt,
		FinishedAt: job.FinishedAt, RetryAt: job.RetryAt, Error: job.Error,
		CanRetry: job.CanRetry, CanPause: job.CanPause, CanResume: job.CanResume,
		Progress: job.Progress, Detail: job.Detail,
		Article: job.Article, Platforms: append([]string{}, job.Platforms...), Operation: job.Operation,
		Results: job.Results, Events: append([]syncJobEvent{}, job.Events...), Output: job.Output, DryRun: job.DryRun,
	}
}

func canRetrySyncJobOperation(operation string, request syncRequest) bool {
	if operation == "create" || operation == "publish" || operation == "publish-draft" {
		return false
	}
	if operation == "update" && (request.PublishAfter || len(request.Targets) != 1) {
		return false
	}
	return true
}

// A failed Medium create can resume its allocated draft. An explicitly
// selected published-editor Update that reached the text-write phase can
// resume its pictures without overwriting the already saved staged revision.
// Never create a new story or retry blind writes on a partial failure.
func mediumRetryDraftID(job syncJob) string {
	if job.Operation == "update" && len(job.Platforms) == 1 && job.Platforms[0] == "medium" &&
		!job.Request.PublishAfter && len(job.Request.Targets) == 1 {
		target := job.Request.Targets[0]
		if target.Platform == "medium" && mediumEditorPostID.MatchString(target.ID) &&
			(strings.Contains(job.Error, "Medium browser draft full text verification failed") ||
				strings.Contains(job.Error, "Medium draft "+target.ID+" exists but browser creation is incomplete") ||
				strings.Contains(job.Error, "medium: "+target.ID+": Medium content saved but canonical settings incomplete:")) {
			return target.ID
		}
	}
	if job.Operation != "create" || len(job.Platforms) != 1 ||
		job.Platforms[0] != "medium" || job.Request.PublishAfter {
		return ""
	}
	for index := len(job.Events) - 1; index >= 0; index-- {
		event := job.Events[index]
		if event.Platform == "medium" && event.Result == "draft-allocated" &&
			mediumEditorPostID.MatchString(event.TargetID) {
			return event.TargetID
		}
	}
	if mediumEditorPostID.MatchString(job.Request.ResumeMediumDraftID) {
		return job.Request.ResumeMediumDraftID
	}
	return ""
}

func syncTaskView(job syncJob) taskJobView {
	title := job.Article
	if title == "" {
		title = "发布任务"
	}
	return taskJobView{
		ID: job.ID, Kind: "publishing", Type: job.Operation, Title: title, State: job.State,
		StartedAt: job.StartedAt, UpdatedAt: job.FinishedAt, FinishedAt: job.FinishedAt,
		Error: job.Error, CanRetry: job.State == "failed" && (canRetrySyncJobOperation(job.Operation, job.Request) || mediumRetryDraftID(job) != ""),
		Progress: taskProgress{Current: completedSyncPlatforms(job), Total: len(job.Platforms), Unit: "platform"},
		Article:  job.Article, Platforms: append([]string{}, job.Platforms...), Operation: job.Operation,
		Results: job.Results, Events: job.Events, Output: job.Output, DryRun: job.DryRun,
	}
}

func publishingDurableTask(job *syncJob, existing *durableTaskJob, now time.Time) *durableTaskJob {
	if job == nil {
		return nil
	}
	createdAt := job.StartedAt
	if existing != nil && existing.CreatedAt != "" {
		createdAt = existing.CreatedAt
	}
	payload, _ := json.Marshal(job.Request)
	title := job.Article
	if title == "" {
		title = "发布任务"
	}
	result := &durableTaskJob{
		ID: job.ID, Kind: "publishing", Type: job.Operation, Title: title, State: job.State,
		CreatedAt: createdAt, StartedAt: job.StartedAt, UpdatedAt: now.UTC().Format(time.RFC3339),
		FinishedAt: job.FinishedAt, Error: job.Error,
		CanRetry: job.State == "failed" && (canRetrySyncJobOperation(job.Operation, job.Request) || mediumRetryDraftID(*job) != ""),
		Progress: taskProgress{Current: completedSyncPlatforms(*job), Total: len(job.Platforms), Unit: "platform"},
		Payload:  payload,
		Article:  job.Article, Platforms: append([]string{}, job.Platforms...), Operation: job.Operation,
		Results: make(map[string]syncPlatformResult, len(job.Results)),
		Events:  append([]syncJobEvent{}, job.Events...), Output: job.Output, DryRun: job.DryRun,
	}
	for platform, value := range job.Results {
		result.Results[platform] = value
	}
	return result
}

func syncJobFromDurable(job *durableTaskJob) *syncJob {
	if job == nil || job.Kind != "publishing" || len(job.Payload) == 0 {
		return nil
	}
	var request syncRequest
	if json.Unmarshal(job.Payload, &request) != nil ||
		strings.TrimSpace(request.Article) == "" ||
		len(request.Platforms) == 0 ||
		strings.TrimSpace(request.Operation) == "" {
		return nil
	}
	restored := &syncJob{
		ID: job.ID, Article: request.Article, Platforms: append([]string{}, request.Platforms...),
		Operation: request.Operation, Request: request, Results: make(map[string]syncPlatformResult, len(job.Results)),
		Events: append([]syncJobEvent{}, job.Events...), State: job.State, StartedAt: job.StartedAt,
		FinishedAt: job.FinishedAt, Output: job.Output, Error: job.Error, DryRun: request.DryRun,
	}
	for platform, value := range job.Results {
		restored.Results[platform] = value
	}
	return restored
}

func restoreSyncJobsFromDurable(jobs map[string]*durableTaskJob, order []string) (map[string]*syncJob, []string) {
	syncJobs := map[string]*syncJob{}
	syncOrder := []string{}
	for _, id := range order {
		if restored := syncJobFromDurable(jobs[id]); restored != nil {
			syncJobs[id] = restored
			syncOrder = append(syncOrder, id)
		}
	}
	return syncJobs, syncOrder
}

func (s *Server) restoreSyncJobLocked(id string) *syncJob {
	if job := s.jobs[id]; job != nil {
		return job
	}
	restored := syncJobFromDurable(s.taskJobs[id])
	if restored == nil {
		return nil
	}
	s.jobs[id] = restored
	s.jobOrder = moveJobToFront(s.jobOrder, id)
	return restored
}

func (s *Server) mirrorSyncJobLocked(job *syncJob) {
	if job == nil {
		return
	}
	if s.taskJobs == nil {
		s.taskJobs = map[string]*durableTaskJob{}
	}
	s.taskJobs[job.ID] = publishingDurableTask(job, s.taskJobs[job.ID], s.now())
	s.taskJobOrder = moveJobToFront(s.taskJobOrder, job.ID)
	s.pruneDurableTaskHistoryLocked(100)
	_ = s.persistDurableTasksLocked()
}

func (s *Server) removeMirroredSyncJobLocked(id string) {
	if s.taskJobs[id] == nil {
		return
	}
	delete(s.taskJobs, id)
	filtered := s.taskJobOrder[:0]
	for _, candidate := range s.taskJobOrder {
		if candidate != id {
			filtered = append(filtered, candidate)
		}
	}
	s.taskJobOrder = filtered
	_ = s.persistDurableTasksLocked()
}

func completedSyncPlatforms(job syncJob) int {
	completed := 0
	for _, result := range job.Results {
		if result.State == "completed" || result.State == "failed" {
			completed++
		}
	}
	return completed
}

func (s *Server) taskViews() []taskJobView {
	s.mu.Lock()
	durableOrder := append([]string{}, s.taskJobOrder...)
	durable := make(map[string]*durableTaskJob, len(s.taskJobs))
	for id, job := range s.taskJobs {
		durable[id] = cloneDurableTaskJob(job)
	}
	s.mu.Unlock()

	syncJobs := s.syncJobs()
	views := make([]taskJobView, 0, len(durableOrder)+len(syncJobs))
	for _, id := range durableOrder {
		if job := durable[id]; job != nil {
			views = append(views, durableTaskView(job))
		}
	}
	for _, job := range syncJobs {
		if durable[job.ID] != nil {
			continue
		}
		views = append(views, syncTaskView(job))
	}
	sort.SliceStable(views, func(i, j int) bool {
		left := views[i].UpdatedAt
		if left == "" {
			left = views[i].StartedAt
		}
		right := views[j].UpdatedAt
		if right == "" {
			right = views[j].StartedAt
		}
		return left > right
	})
	return views
}

func (s *Server) pruneDurableTaskHistoryLocked(limit int) {
	if limit < 1 || len(s.taskJobOrder) <= limit {
		return
	}
	kept := make([]string, 0, len(s.taskJobOrder))
	for _, id := range s.taskJobOrder {
		job := s.taskJobs[id]
		active := job != nil && (job.State == "running" || job.State == "queued" || job.State == "paused")
		if len(kept) < limit || active {
			kept = append(kept, id)
			continue
		}
		delete(s.taskJobs, id)
	}
	s.taskJobOrder = kept
}

func (s *Server) createDurableTaskJob(jobType, title string, payload any, progress taskProgress, capabilities struct {
	Retry  bool
	Pause  bool
	Resume bool
}) (*durableTaskJob, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	job := &durableTaskJob{
		ID: newJobID(), Kind: "search", Type: jobType, Title: title, State: "queued",
		CreatedAt: now, UpdatedAt: now, Payload: raw, Progress: progress,
		CanRetry: capabilities.Retry, CanPause: capabilities.Pause, CanResume: capabilities.Resume,
	}
	s.mu.Lock()
	if s.taskJobs == nil {
		s.taskJobs = map[string]*durableTaskJob{}
	}
	s.taskJobs[job.ID] = job
	s.taskJobOrder = append([]string{job.ID}, s.taskJobOrder...)
	s.pruneDurableTaskHistoryLocked(100)
	err = s.persistDurableTasksLocked()
	result := cloneDurableTaskJob(job)
	s.mu.Unlock()
	return result, err
}

func (s *Server) updateDurableTaskJob(id string, mutate func(*durableTaskJob)) (*durableTaskJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.taskJobs[id]
	if job == nil {
		return nil, errors.New("task job not found")
	}
	mutate(job)
	job.UpdatedAt = s.now().UTC().Format(time.RFC3339)
	if err := s.persistDurableTasksLocked(); err != nil {
		return nil, err
	}
	return cloneDurableTaskJob(job), nil
}

func (s *Server) durableTaskJob(id string) *durableTaskJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneDurableTaskJob(s.taskJobs[id])
}

func (s *Server) deleteDurableTaskJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.taskJobs[id]
	if job == nil {
		return errors.New("task job not found")
	}
	if job.State == "running" {
		return errors.New("running task cannot be deleted")
	}
	delete(s.taskJobs, id)
	filtered := s.taskJobOrder[:0]
	for _, candidate := range s.taskJobOrder {
		if candidate != id {
			filtered = append(filtered, candidate)
		}
	}
	s.taskJobOrder = filtered
	return s.persistDurableTasksLocked()
}

func (s *Server) clearFinishedDurableTaskJobs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.taskJobOrder[:0]
	removed := 0
	for _, id := range s.taskJobOrder {
		job := s.taskJobs[id]
		if job != nil && (job.State == "running" || job.State == "queued" || job.State == "paused") {
			kept = append(kept, id)
			continue
		}
		delete(s.taskJobs, id)
		removed++
	}
	s.taskJobOrder = kept
	_ = s.persistDurableTasksLocked()
	return removed
}

func (s *Server) handleTaskJobsGet(response http.ResponseWriter, request *http.Request) {
	if !allowReadOnlyBridgeStatus(response, request) {
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"jobs": s.taskViews()})
}

func (s *Server) handleTaskJobGet(response http.ResponseWriter, request *http.Request, id string) {
	if !allowReadOnlyBridgeStatus(response, request) {
		return
	}
	if job := s.durableTaskJob(id); job != nil {
		writeJSON(response, http.StatusOK, map[string]any{"job": durableTaskView(job)})
		return
	}
	s.handleSyncJobGet(response, id)
}

func (s *Server) handleTaskJobsClear(response http.ResponseWriter, request *http.Request) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	removed := s.clearFinishedSyncJobs() + s.clearFinishedDurableTaskJobs()
	writeJSON(response, http.StatusOK, map[string]any{"removed": removed, "jobs": s.taskViews()})
}

func (s *Server) handleTaskJobDelete(response http.ResponseWriter, request *http.Request, id string) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	if job := s.durableTaskJob(id); job != nil {
		var err error
		if job.Kind == "publishing" {
			err = s.deleteSyncJob(id)
		} else {
			err = s.deleteDurableTaskJob(id)
		}
		if err != nil {
			writeAPIError(response, http.StatusConflict, "task_delete_failed", err.Error(), nil)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"ok": true})
		return
	}
	s.handleSyncJobDelete(response, request, id)
}

func (s *Server) handleTaskJobRetry(response http.ResponseWriter, request *http.Request, id string) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	if job := s.durableTaskJob(id); job != nil {
		if job.Kind == "publishing" {
			if _, err := s.retrySyncJob(id); err != nil {
				writeAPIError(response, http.StatusConflict, "task_retry_failed", err.Error(), nil)
				return
			}
			refreshed := s.durableTaskJob(id)
			writeJSON(response, http.StatusAccepted, map[string]any{"job": durableTaskView(refreshed)})
			return
		}
		retried, err := s.retrySearchTaskJob(job)
		if err != nil {
			writeAPIError(response, http.StatusConflict, "task_retry_failed", err.Error(), nil)
			return
		}
		writeJSON(response, http.StatusAccepted, map[string]any{"job": durableTaskView(retried)})
		return
	}
	s.handleSyncJobRetry(response, request, id)
}

func (s *Server) handleTaskJobPause(response http.ResponseWriter, request *http.Request, id string) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	job := s.durableTaskJob(id)
	if job == nil {
		writeAPIError(response, http.StatusNotFound, "task_not_found", "task job not found", nil)
		return
	}
	paused, err := s.pauseSearchTaskJob(job)
	if err != nil {
		writeAPIError(response, http.StatusConflict, "task_pause_failed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"job": durableTaskView(paused)})
}

func (s *Server) handleTaskJobResume(response http.ResponseWriter, request *http.Request, id string) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	job := s.durableTaskJob(id)
	if job == nil {
		writeAPIError(response, http.StatusNotFound, "task_not_found", "task job not found", nil)
		return
	}
	resumed, err := s.resumeSearchTaskJob(job)
	if err != nil {
		writeAPIError(response, http.StatusConflict, "task_resume_failed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"job": durableTaskView(resumed)})
}

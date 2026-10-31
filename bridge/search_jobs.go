package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	blogsearch "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/search"
)

type searchSubmissionTaskPayload struct {
	Mode string                 `json:"mode"`
	Site *blogsearch.SiteConfig `json:"site,omitempty"`
}

type googleInspectionTaskPayload struct {
	Site   *blogsearch.SiteConfig `json:"site,omitempty"`
	Offset int                    `json:"offset"`
	Limit  int                    `json:"limit"`
	Done   bool                   `json:"done,omitempty"`
}

func reconcileRecoveredGoogleInspectionPayload(payload googleInspectionTaskPayload, inspection searchInspectionState) googleInspectionTaskPayload {
	if payload.Done || payload.Limit <= 0 || inspection.NextOffset == nil {
		return payload
	}
	nextOffset := *inspection.NextOffset
	if nextOffset <= payload.Offset {
		return payload
	}
	batchEnd := payload.Offset + payload.Limit
	if nextOffset >= batchEnd {
		payload.Offset = batchEnd
		payload.Limit = 0
		payload.Done = true
		return payload
	}
	payload.Offset = nextOffset
	payload.Limit = batchEnd - nextOffset
	return payload
}

func taskCapabilities(retry, pause, resume bool) struct {
	Retry  bool
	Pause  bool
	Resume bool
} {
	return struct {
		Retry  bool
		Pause  bool
		Resume bool
	}{Retry: retry, Pause: pause, Resume: resume}
}

func appendTaskOutput(job *durableTaskJob, line string) {
	if job == nil {
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	if job.Output != "" {
		job.Output += "\n"
	}
	job.Output += stamp + " " + line
	const maxBytes = 32 * 1024
	if len(job.Output) > maxBytes {
		trimmed := job.Output[len(job.Output)-maxBytes:]
		if index := strings.IndexByte(trimmed, '\n'); index >= 0 {
			trimmed = trimmed[index+1:]
		}
		job.Output = trimmed
	}
	if strings.HasPrefix(line, "[stage]") {
		slog.Info("search task stage", "jobId", job.ID, "taskType", job.Type, "detail", line)
	} else {
		slog.Debug("search task detail", "jobId", job.ID, "taskType", job.Type, "detail", line)
	}
}

func (s *Server) markDurableTaskRunning(id string) (*durableTaskJob, error) {
	return s.updateDurableTaskJob(id, func(job *durableTaskJob) {
		now := s.now().UTC().Format(time.RFC3339)
		job.State = "running"
		job.Error = ""
		job.RetryAt = ""
		job.CanPause = job.Type == "google-request-indexing"
		job.CanResume = false
		if job.Progress.Message == "" || job.Progress.Message == "等待执行" || job.Progress.Message == "等待重试" {
			job.Progress.Message = "正在执行"
		}
		if job.StartedAt == "" {
			job.StartedAt = now
		}
		job.FinishedAt = ""
	})
}

func (s *Server) completeDurableTask(id string, progress taskProgress, detail map[string]any) {
	_, _ = s.updateDurableTaskJob(id, func(job *durableTaskJob) {
		job.State = "completed"
		job.Error = ""
		job.Progress = progress
		job.Detail = detail
		job.FinishedAt = s.now().UTC().Format(time.RFC3339)
		job.CanRetry = false
		job.CanPause = false
		job.CanResume = false
		slog.Info("task completed", "jobId", job.ID, "kind", job.Kind, "taskType", job.Type, "current", progress.Current, "total", progress.Total)
	})
}

func (s *Server) failDurableTask(id string, err error) {
	_, _ = s.updateDurableTaskJob(id, func(job *durableTaskJob) {
		job.State = "failed"
		job.Error = err.Error()
		job.FinishedAt = s.now().UTC().Format(time.RFC3339)
		job.CanRetry = true
		job.CanPause = false
		job.CanResume = false
		slog.Error("task failed", "jobId", job.ID, "kind", job.Kind, "taskType", job.Type, "error", err.Error())
	})
}

func (s *Server) launchSearchTaskJob(jobID string) {
	go func() {
		job := s.durableTaskJob(jobID)
		if job == nil {
			return
		}
		s.searchMu.Lock()
		defer s.searchMu.Unlock()
		var binding struct {
			Site *blogsearch.SiteConfig `json:"site"`
		}
		if err := json.Unmarshal(job.Payload, &binding); err != nil {
			s.failDurableTask(jobID, err)
			return
		}
		if binding.Site != nil && *binding.Site != s.searchSite() {
			s.failDurableTask(jobID, errors.New("索引站点配置已变更；请创建新任务，旧任务不会按新站点重放"))
			return
		}
		if binding.Site == nil {
			previous := loadSearchIndexState()
			if previous.Inventory.Origin != "" && previous.Inventory.Origin != s.searchSite().Origin {
				s.failDurableTask(jobID, errors.New("旧索引任务属于其他站点；请创建新任务"))
				return
			}
		}
		if _, err := s.markDurableTaskRunning(jobID); err != nil {
			slog.Error("task start failed", "jobId", jobID, "taskType", job.Type, "error", err.Error())
			return
		}
		slog.Info("task started", "jobId", jobID, "kind", job.Kind, "taskType", job.Type)

		var err error
		switch job.Type {
		case "indexnow-submit":
			err = s.executeIndexNowTask(context.Background(), jobID, job.Payload)
		case "baidu-submit":
			err = s.executeBaiduIndexTask(context.Background(), jobID, job.Payload)
		case "google-sitemaps":
			err = s.executeGoogleSitemapsTask(context.Background(), jobID)
		case "google-inspection":
			err = s.executeGoogleInspectionTask(context.Background(), jobID, job.Payload)
		default:
			err = fmt.Errorf("unsupported search task type: %s", job.Type)
		}
		if err != nil {
			s.failDurableTask(jobID, err)
		}
	}()
}

func (s *Server) executeIndexNowTask(ctx context.Context, jobID string, rawPayload json.RawMessage) error {
	var input searchSubmissionTaskPayload
	if err := json.Unmarshal(rawPayload, &input); err != nil {
		return err
	}
	input.Mode = strings.TrimSpace(input.Mode)
	if input.Mode == "" {
		input.Mode = "incremental"
	}
	if input.Mode != "incremental" && input.Mode != "full" {
		return errors.New("IndexNow submission mode must be incremental or full")
	}

	state := s.loadSearchState()
	started := s.now().UTC()
	state.IndexNow = searchOperationState{
		State: "running", Mode: input.Mode, StartedAt: started.Format(time.RFC3339),
	}
	_ = saveSearchIndexState(state)

	previous := loadIndexNowSnapshot()
	payload, err := s.submitIndexNow(ctx, input.Mode, previous)
	if err != nil {
		state.IndexNow.State = "failed"
		state.IndexNow.FinishedAt = s.now().UTC().Format(time.RFC3339)
		state.IndexNow.Error = err.Error()
		_ = saveSearchIndexState(state)
		return err
	}
	if err := saveIndexNowSnapshot(payload.Inventory); err != nil {
		state.IndexNow.State = "failed"
		state.IndexNow.FinishedAt = s.now().UTC().Format(time.RFC3339)
		state.IndexNow.Error = err.Error()
		_ = saveSearchIndexState(state)
		return err
	}

	state.Inventory = compactSearchInventory(payload.Inventory)
	state.IndexNow = searchOperationState{
		State: "completed", Mode: payload.Diff.Mode, StartedAt: started.Format(time.RFC3339),
		FinishedAt: s.now().UTC().Format(time.RFC3339), Count: payload.Result.URLCount,
		NewCount: payload.Diff.AddedCount, ChangedCount: payload.Diff.ChangedCount,
		DeletedCount: payload.Diff.DeletedCount, UnchangedCount: payload.Diff.UnchangedCount,
		HTTPStatus: maxSearchHTTPStatus(payload.Result.Results),
	}
	refreshSearchCredentialsFlag(&state, s.config)
	if err := saveSearchIndexState(state); err != nil {
		return err
	}

	detail := map[string]any{
		"mode":       payload.Diff.Mode,
		"submitted":  payload.Result.URLCount,
		"added":      payload.Diff.AddedCount,
		"changed":    payload.Diff.ChangedCount,
		"deleted":    payload.Diff.DeletedCount,
		"unchanged":  payload.Diff.UnchangedCount,
		"httpStatus": state.IndexNow.HTTPStatus,
	}
	s.completeDurableTask(jobID, taskProgress{
		Current: payload.Result.URLCount,
		Total:   payload.Result.URLCount,
		Unit:    "URL",
		Message: "IndexNow submitted",
	}, detail)
	return nil
}

func (s *Server) executeBaiduIndexTask(ctx context.Context, jobID string, rawPayload json.RawMessage) error {
	var input searchSubmissionTaskPayload
	if err := json.Unmarshal(rawPayload, &input); err != nil {
		return err
	}
	input.Mode = strings.TrimSpace(input.Mode)
	if input.Mode == "" {
		input.Mode = "incremental"
	}
	if input.Mode != "incremental" && input.Mode != "full" {
		return errors.New("Baidu submission mode must be incremental or full")
	}

	state := s.loadSearchState()
	started := s.now().UTC()
	state.Baidu = searchOperationState{
		State: "running", Mode: input.Mode, StartedAt: started.Format(time.RFC3339),
	}
	_ = saveSearchIndexState(state)

	previous := loadBaiduIndexSnapshot()
	payload, err := s.submitBaidu(ctx, input.Mode, previous)
	if err != nil {
		// Baidu reports only aggregate accepted counts. Keep the provider
		// snapshot unchanged on partial acceptance, but persist the observed
		// submission statistics so Bridge/UI state reflects what actually
		// happened and the whole batch can be retried safely.
		if payload.Inventory.Total > 0 || payload.Result.URLCount > 0 {
			state.Inventory = compactSearchInventory(payload.Inventory)
			state.Baidu = searchOperationState{
				State: "failed", Mode: payload.Diff.Mode, StartedAt: started.Format(time.RFC3339),
				FinishedAt: s.now().UTC().Format(time.RFC3339), Count: payload.Result.URLCount,
				NewCount: payload.Diff.AddedCount, ChangedCount: payload.Diff.ChangedCount,
				DeletedCount: payload.Diff.DeletedCount, UnchangedCount: payload.Diff.UnchangedCount,
				HTTPStatus: maxSearchHTTPStatus(payload.Result.Results), Error: err.Error(),
			}
		} else {
			state.Baidu.State = "failed"
			state.Baidu.FinishedAt = s.now().UTC().Format(time.RFC3339)
			state.Baidu.Error = err.Error()
		}
		_ = saveSearchIndexState(state)
		return err
	}
	if err := saveBaiduIndexSnapshot(payload.Inventory); err != nil {
		state.Baidu.State = "failed"
		state.Baidu.FinishedAt = s.now().UTC().Format(time.RFC3339)
		state.Baidu.Error = err.Error()
		_ = saveSearchIndexState(state)
		return err
	}

	state.Inventory = compactSearchInventory(payload.Inventory)
	state.Baidu = searchOperationState{
		State: "completed", Mode: payload.Diff.Mode, StartedAt: started.Format(time.RFC3339),
		FinishedAt: s.now().UTC().Format(time.RFC3339), Count: payload.Result.SuccessCount,
		NewCount: payload.Diff.AddedCount, ChangedCount: payload.Diff.ChangedCount,
		DeletedCount: payload.Diff.DeletedCount, UnchangedCount: payload.Diff.UnchangedCount,
		HTTPStatus: maxSearchHTTPStatus(payload.Result.Results),
	}
	if err := saveSearchIndexState(state); err != nil {
		return err
	}
	s.completeDurableTask(jobID, taskProgress{
		Current: payload.Result.SuccessCount,
		Total:   payload.Result.URLCount,
		Unit:    "URL",
		Message: "Baidu URLs submitted",
	}, map[string]any{
		"mode":             payload.Diff.Mode,
		"submitted":        payload.Result.URLCount,
		"accepted":         payload.Result.SuccessCount,
		"remain":           payload.Result.Remain,
		"added":            payload.Diff.AddedCount,
		"changed":          payload.Diff.ChangedCount,
		"deleted":          payload.Diff.DeletedCount,
		"deletedSubmitted": 0,
		"unchanged":        payload.Diff.UnchangedCount,
		"httpStatus":       state.Baidu.HTTPStatus,
	})
	return nil
}

func (s *Server) executeGoogleSitemapsTask(ctx context.Context, jobID string) error {
	state := s.loadSearchState()
	started := s.now().UTC()
	state.Google.Sitemaps = searchOperationState{State: "running", StartedAt: started.Format(time.RFC3339)}
	_ = saveSearchIndexState(state)

	payload, err := s.submitGoogleSitemapsNative(ctx)
	if err != nil {
		state.Google.Sitemaps.State = "failed"
		state.Google.Sitemaps.FinishedAt = s.now().UTC().Format(time.RFC3339)
		state.Google.Sitemaps.Error = err.Error()
		refreshSearchCredentialsFlag(&state, s.config)
		_ = saveSearchIndexState(state)
		return err
	}
	httpStatus := 0
	for _, result := range payload.Result {
		if result.HTTPStatus > httpStatus {
			httpStatus = result.HTTPStatus
		}
	}
	state.Inventory = compactSearchInventory(payload.Inventory)
	state.Google.Sitemaps = searchOperationState{
		State: "completed", StartedAt: started.Format(time.RFC3339),
		FinishedAt: s.now().UTC().Format(time.RFC3339), Count: len(payload.Result),
		HTTPStatus: httpStatus,
	}
	refreshSearchCredentialsFlag(&state, s.config)
	if err := saveSearchIndexState(state); err != nil {
		return err
	}
	detail := map[string]any{
		"submitted":  len(payload.Result),
		"httpStatus": state.Google.Sitemaps.HTTPStatus,
	}
	s.completeDurableTask(jobID, taskProgress{
		Current: len(payload.Result), Total: 2, Unit: "sitemap", Message: "Search Console sitemap submit completed",
	}, detail)
	return nil
}

func (s *Server) executeGoogleInspectionTask(ctx context.Context, jobID string, rawPayload json.RawMessage) error {
	var input googleInspectionTaskPayload
	if err := json.Unmarshal(rawPayload, &input); err != nil {
		return err
	}
	if input.Limit == 0 && input.Offset > 0 {
		input.Done = true
	}
	if input.Done {
		state := s.loadSearchState()
		state.Google.Inspection.State = "completed"
		state.Google.Inspection.FinishedAt = s.now().UTC().Format(time.RFC3339)
		state.Google.Inspection.Offset = input.Offset
		state.Google.Inspection.Limit = 0
		state.Google.Inspection.Error = ""
		refreshSearchCredentialsFlag(&state, s.config)
		if err := saveSearchIndexState(state); err != nil {
			return err
		}
		total := state.Google.Inspection.Total
		current := min(input.Offset, total)
		s.completeDurableTask(jobID, taskProgress{
			Current: current, Total: total, Unit: "URL", Message: "URL Inspection batch completed",
		}, map[string]any{
			"nextOffset":   input.Offset,
			"checkedCount": state.Google.Inspection.Inspected,
			"remaining":    state.Google.Inspection.Remaining,
		})
		return nil
	}
	if input.Limit == 0 {
		input.Limit = blogsearch.GoogleURLInspectionDailySiteLimit
	}
	if input.Offset < 0 || input.Limit < 1 || input.Limit > blogsearch.GoogleURLInspectionDailySiteLimit {
		return fmt.Errorf(
			"Google inspection requires offset >= 0 and 1 <= limit <= %d",
			blogsearch.GoogleURLInspectionDailySiteLimit,
		)
	}

	state := s.loadSearchState()
	started := s.now().UTC()
	if state.Google.Inspection.StartedAt == "" {
		state.Google.Inspection.StartedAt = started.Format(time.RFC3339)
	}
	state.Google.Inspection.State = "running"
	state.Google.Inspection.FinishedAt = ""
	state.Google.Inspection.Offset = input.Offset
	state.Google.Inspection.Limit = input.Limit
	state.Google.Inspection.Error = ""
	_ = saveSearchIndexState(state)

	inventory, err := s.fetchSearchInventory(ctx)
	if err != nil {
		return err
	}
	state = s.loadSearchState()
	reconcileInspectionInventory(&state.Google.Inspection, inventory)
	state.Inventory = compactSearchInventory(inventory)
	state.Google.Inspection.State = "running"
	state.Google.Inspection.Offset = input.Offset
	state.Google.Inspection.Limit = input.Limit
	state.Google.Inspection.Total = inventory.Total
	state.Google.Inspection.Remaining = max(0, inventory.Total-len(state.Google.Inspection.Results))
	refreshSearchCredentialsFlag(&state, s.config)
	if err := saveSearchIndexState(state); err != nil {
		return err
	}
	_, err = s.updateDurableTaskJob(jobID, func(job *durableTaskJob) {
		job.Progress = taskProgress{
			Current: min(input.Offset, inventory.Total),
			Total:   inventory.Total,
			Unit:    "URL",
			Message: fmt.Sprintf("URL inventory 已读取：%d URLs", inventory.Total),
		}
		job.Detail = map[string]any{"phase": "inventory_ready"}
		appendTaskOutput(job, fmt.Sprintf("[stage] URL inventory 已读取：%d URLs", inventory.Total))
	})
	if err != nil {
		return err
	}

	_, err = s.updateDurableTaskJob(jobID, func(job *durableTaskJob) {
		job.Progress.Message = "正在获取 Google OAuth token"
		job.Detail = map[string]any{"phase": "oauth_start"}
		appendTaskOutput(job, "[stage] 正在获取 Google OAuth token")
	})
	if err != nil {
		return err
	}
	accessToken, err := s.googleAccessToken(ctx)
	if err != nil {
		return err
	}
	_, err = s.updateDurableTaskJob(jobID, func(job *durableTaskJob) {
		job.Progress.Message = "Google OAuth token 已获取"
		job.Detail = map[string]any{"phase": "oauth_ready"}
		appendTaskOutput(job, "[stage] Google OAuth token 已获取")
	})
	if err != nil {
		return err
	}

	report, err := blogsearch.AuditGoogleURLs(
		ctx,
		s.httpClient,
		inventory.URLs,
		s.searchSite().GoogleProperty,
		inventory.Origin,
		accessToken,
		input.Offset,
		input.Limit,
		blogsearch.GoogleURLInspectionDefaultDelay,
		func(event blogsearch.GoogleInspectionProgress) error {
			switch event.Type {
			case "request_start":
				if strings.TrimSpace(event.URL) == "" {
					return errors.New("Google URL Inspection request_start missing URL")
				}
				_, err := s.updateDurableTaskJob(jobID, func(job *durableTaskJob) {
					cursor := max(0, event.AbsoluteIndex-1)
					job.Progress.Current = min(cursor, event.TotalAvailable)
					job.Progress.Total = event.TotalAvailable
					job.Progress.Unit = "URL"
					job.Progress.Message = fmt.Sprintf(
						"正在检查 %d / %d · %s",
						event.AbsoluteIndex,
						event.TotalAvailable,
						event.URL,
					)
					job.Detail = map[string]any{
						"phase":      "request",
						"currentUrl": event.URL,
						"nextOffset": cursor,
					}
					appendTaskOutput(job, fmt.Sprintf("[request] start #%d %s", event.AbsoluteIndex, event.URL))
				})
				return err

			case "request_complete":
				if event.Result == nil || strings.TrimSpace(event.Result.URL) == "" || event.Inspected <= 0 {
					return errors.New("Google URL Inspection progress record is invalid")
				}
				result := coreInspectionResult(*event.Result)
				result.CheckedAt = s.now().UTC().Format(time.RFC3339)

				currentState := s.loadSearchState()
				currentState.Google.Inspection.Results = mergeInspectionResults(
					currentState.Google.Inspection.Results,
					[]searchInspectionResult{result},
				)
				currentState.Google.Inspection.State = "running"
				currentState.Google.Inspection.FinishedAt = ""
				currentState.Google.Inspection.Offset = input.Offset
				currentState.Google.Inspection.Limit = input.Limit
				currentState.Google.Inspection.Inspected = len(currentState.Google.Inspection.Results)
				currentState.Google.Inspection.Total = event.TotalAvailable
				currentState.Google.Inspection.Remaining = max(0, event.TotalAvailable-len(currentState.Google.Inspection.Results))
				currentState.Google.Inspection.NextOffset = event.NextOffset
				currentState.Google.Inspection.Error = ""
				refreshSearchCredentialsFlag(&currentState, s.config)
				if err := saveSearchIndexState(currentState); err != nil {
					return err
				}

				nextOffset := input.Offset + event.Inspected
				remainingBatch := max(0, input.Limit-event.Inspected)
				nextPayload, _ := json.Marshal(googleInspectionTaskPayload{
					Site:   input.Site,
					Offset: nextOffset,
					Limit:  remainingBatch,
					Done:   remainingBatch == 0,
				})
				_, err := s.updateDurableTaskJob(jobID, func(job *durableTaskJob) {
					job.Progress = taskProgress{
						Current: min(event.AbsoluteIndex, event.TotalAvailable),
						Total:   event.TotalAvailable,
						Unit:    "URL",
						Message: fmt.Sprintf(
							"已检查 %d / %d · %s",
							event.AbsoluteIndex,
							event.TotalAvailable,
							result.URL,
						),
					}
					job.Payload = nextPayload
					job.Detail = map[string]any{
						"phase":          "completed_request",
						"currentUrl":     result.URL,
						"nextOffset":     nextOffset,
						"checkedCount":   currentState.Google.Inspection.Inspected,
						"durationMs":     event.Duration.Milliseconds(),
						"totalAvailable": event.TotalAvailable,
						"remaining":      currentState.Google.Inspection.Remaining,
					}
					appendTaskOutput(job, fmt.Sprintf(
						"[request] done #%d %s duration=%dms verdict=%s",
						event.AbsoluteIndex,
						result.URL,
						event.Duration.Milliseconds(),
						result.Verdict,
					))
				})
				return err

			default:
				return fmt.Errorf("unknown Google URL Inspection progress event: %s", event.Type)
			}
		},
	)
	if err != nil {
		state = s.loadSearchState()
		if googleInspectionQuotaExceeded(err) {
			message := googleInspectionQuotaMessage()
			state.Google.Inspection.State = "quota_blocked"
			state.Google.Inspection.FinishedAt = ""
			state.Google.Inspection.Error = message
			refreshSearchCredentialsFlag(&state, s.config)
			_ = saveSearchIndexState(state)
			_, _ = s.updateDurableTaskJob(jobID, func(job *durableTaskJob) {
				job.State = "paused"
				job.Error = ""
				job.FinishedAt = ""
				job.Progress.Message = message
				job.CanRetry = true
				job.CanPause = false
				job.CanResume = false
				job.Detail = map[string]any{
					"phase":  "quota_blocked",
					"reason": "quota_blocked",
				}
				appendTaskOutput(job, "[quota] "+message)
			})
			return nil
		}
		state.Google.Inspection.State = "failed"
		state.Google.Inspection.FinishedAt = s.now().UTC().Format(time.RFC3339)
		state.Google.Inspection.Error = err.Error()
		refreshSearchCredentialsFlag(&state, s.config)
		_ = saveSearchIndexState(state)
		return err
	}

	if report.Inspected == 0 && input.Offset < report.TotalAvailable {
		return errors.New("Google URL Inspection returned no progress")
	}
	state = s.loadSearchState()
	state.Google.Inspection.State = "completed"
	state.Google.Inspection.FinishedAt = s.now().UTC().Format(time.RFC3339)
	state.Google.Inspection.Total = report.TotalAvailable
	state.Google.Inspection.Remaining = report.Remaining
	state.Google.Inspection.NextOffset = report.NextOffset
	state.Google.Inspection.Error = ""
	refreshSearchCredentialsFlag(&state, s.config)
	if err := saveSearchIndexState(state); err != nil {
		return err
	}

	current := min(input.Offset+report.Inspected, report.TotalAvailable)
	s.completeDurableTask(jobID, taskProgress{
		Current: current,
		Total:   report.TotalAvailable,
		Unit:    "URL",
		Message: "URL Inspection batch completed",
	}, map[string]any{
		"nextOffset":     current,
		"checkedCount":   state.Google.Inspection.Inspected,
		"totalAvailable": report.TotalAvailable,
		"remaining":      report.Remaining,
	})
	return nil
}

func (s *Server) startIndexNowTask(mode string) (*durableTaskJob, error) {
	site := s.searchSite()
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "incremental"
	}
	if mode != "incremental" && mode != "full" {
		return nil, errors.New("IndexNow submission mode must be incremental or full")
	}
	title := "IndexNow 增量提交"
	if mode == "full" {
		title = "IndexNow 全量提交"
	}
	job, err := s.createDurableTaskJob(
		"indexnow-submit", title, searchSubmissionTaskPayload{Mode: mode, Site: &site},
		taskProgress{Unit: "URL", Message: "等待执行"},
		taskCapabilities(true, false, false),
	)
	if err != nil {
		return nil, err
	}
	s.launchSearchTaskJob(job.ID)
	return job, nil
}

func (s *Server) startBaiduIndexTask(mode string) (*durableTaskJob, error) {
	site := s.searchSite()
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "incremental"
	}
	if mode != "incremental" && mode != "full" {
		return nil, errors.New("Baidu submission mode must be incremental or full")
	}
	if baiduToken(s.config) == "" {
		return nil, errors.New("Baidu push token is not configured")
	}
	title := "Baidu 增量索引"
	if mode == "full" {
		title = "Baidu 全量索引"
	}
	job, err := s.createDurableTaskJob(
		"baidu-submit", title, searchSubmissionTaskPayload{Mode: mode, Site: &site},
		taskProgress{Unit: "URL", Message: "等待执行"},
		taskCapabilities(true, false, false),
	)
	if err != nil {
		return nil, err
	}
	s.launchSearchTaskJob(job.ID)
	return job, nil
}

func (s *Server) startGoogleSitemapsTask() (*durableTaskJob, error) {
	job, err := s.createDurableTaskJob(
		"google-sitemaps", "Google Sitemap 提交", map[string]any{"site": s.searchSite()},
		taskProgress{Total: 2, Unit: "sitemap", Message: "等待执行"},
		taskCapabilities(true, false, false),
	)
	if err != nil {
		return nil, err
	}
	s.launchSearchTaskJob(job.ID)
	return job, nil
}

func (s *Server) startGoogleInspectionTask(offset, limit int) (*durableTaskJob, error) {
	site := s.searchSite()
	if limit == 0 {
		limit = 2000
	}
	if offset < 0 || limit < 1 || limit > 2000 {
		return nil, errors.New("Google inspection requires offset >= 0 and 1 <= limit <= 2000")
	}
	state := s.loadSearchState()
	total := state.Inventory.Total
	if total <= 0 {
		total = offset + limit
	}
	job, err := s.createDurableTaskJob(
		"google-inspection", "Google URL Inspection",
		googleInspectionTaskPayload{Offset: offset, Limit: limit, Site: &site},
		taskProgress{Current: min(offset, total), Total: total, Unit: "URL", Message: "等待执行"},
		taskCapabilities(true, false, false),
	)
	if err != nil {
		return nil, err
	}
	s.launchSearchTaskJob(job.ID)
	return job, nil
}

func (s *Server) handleSearchIndexNowJobStart(response http.ResponseWriter, request *http.Request) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	var input searchSubmissionTaskPayload
	if err := readJSON(request, maxBodyBytes, &input); err != nil {
		writeError(response, err)
		return
	}
	job, err := s.startIndexNowTask(input.Mode)
	if err != nil {
		writeAPIError(response, http.StatusBadRequest, "invalid_indexnow_submit_mode", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{
		"ok": true, "job": durableTaskView(job), "index": s.searchState(),
	})
}

func (s *Server) handleSearchBaiduJobStart(response http.ResponseWriter, request *http.Request) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	var input searchSubmissionTaskPayload
	if err := readJSON(request, maxBodyBytes, &input); err != nil {
		writeError(response, err)
		return
	}
	job, err := s.startBaiduIndexTask(input.Mode)
	if err != nil {
		writeAPIError(response, http.StatusBadRequest, "invalid_baidu_submit", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{
		"ok": true, "job": durableTaskView(job), "index": s.searchState(),
	})
}

func (s *Server) handleSearchGoogleSitemapsJobStart(response http.ResponseWriter, request *http.Request) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	job, err := s.startGoogleSitemapsTask()
	if err != nil {
		writeError(response, err)
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{
		"ok": true, "job": durableTaskView(job), "index": s.searchState(),
	})
}

func (s *Server) handleSearchGoogleInspectionJobStart(response http.ResponseWriter, request *http.Request) {
	if !s.allowSyncControlWrite(response, request) {
		return
	}
	var input googleInspectionTaskPayload
	if err := readJSON(request, maxBodyBytes, &input); err != nil {
		writeError(response, err)
		return
	}
	job, err := s.startGoogleInspectionTask(input.Offset, input.Limit)
	if err != nil {
		writeAPIError(response, http.StatusBadRequest, "invalid_search_inspection_range", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{
		"ok": true, "job": durableTaskView(job), "index": s.searchState(),
	})
}

func (s *Server) retrySearchTaskJob(job *durableTaskJob) (*durableTaskJob, error) {
	if job == nil || job.Kind != "search" {
		return nil, errors.New("task is not a search job")
	}
	switch job.Type {
	case "indexnow-submit", "baidu-submit", "google-sitemaps", "google-inspection":
		updated, err := s.updateDurableTaskJob(job.ID, func(current *durableTaskJob) {
			current.State = "queued"
			current.Error = ""
			current.FinishedAt = ""
			if current.Type != "google-inspection" {
				current.Progress.Current = 0
			}
			current.Progress.Message = "等待重试"
			current.CanRetry = true
			current.CanPause = false
			current.CanResume = false
		})
		if err != nil {
			return nil, err
		}
		s.launchSearchTaskJob(job.ID)
		return updated, nil
	case "google-request-indexing":
		return s.resumeSearchTaskJob(job)
	default:
		return nil, fmt.Errorf("unsupported search task type: %s", job.Type)
	}
}

func (s *Server) pauseSearchTaskJob(job *durableTaskJob) (*durableTaskJob, error) {
	if job == nil || job.Type != "google-request-indexing" {
		return nil, errors.New("this task does not support pause")
	}
	state := s.loadSearchState()
	queue := &state.Google.RequestQueue
	if queue.JobID != job.ID {
		return nil, errors.New("request-indexing task is no longer active")
	}
	queue.State = "paused"
	queue.UpdatedAt = s.now().UTC().Format(time.RFC3339)
	if err := saveSearchIndexState(state); err != nil {
		return nil, err
	}
	return s.updateGoogleRequestTaskFromQueue(*queue)
}

func (s *Server) resumeSearchTaskJob(job *durableTaskJob) (*durableTaskJob, error) {
	if job == nil || job.Type != "google-request-indexing" {
		return nil, errors.New("this task does not support resume")
	}
	state := s.loadSearchState()
	queue := &state.Google.RequestQueue
	if queue.JobID != job.ID {
		return nil, errors.New("request-indexing task is no longer active")
	}
	for index, item := range queue.Items {
		if item.Status == "queued" || item.Status == "failed" || item.Status == "quota_blocked" || item.Status == "processing" {
			queue.CurrentIndex = index
			break
		}
	}
	if len(queue.Items) == 0 || queue.CurrentIndex >= len(queue.Items) {
		return nil, errors.New("request-indexing queue has no pending URLs")
	}
	current := &queue.Items[queue.CurrentIndex]
	if current.Status == "quota_blocked" || current.Status == "processing" {
		current.Status = "queued"
		current.Error = ""
	}
	queue.State = "running"
	queue.LastError = ""
	queue.UpdatedAt = s.now().UTC().Format(time.RFC3339)
	if err := saveSearchIndexState(state); err != nil {
		return nil, err
	}
	return s.updateGoogleRequestTaskFromQueue(*queue)
}

func (s *Server) ensureGoogleRequestTask(queue *googleIndexRequestQueue) (*durableTaskJob, error) {
	if queue == nil {
		return nil, errors.New("request-indexing queue is required")
	}
	if queue.JobID != "" {
		if existing := s.durableTaskJob(queue.JobID); existing != nil && existing.State != "completed" {
			return existing, nil
		}
	}
	job, err := s.createDurableTaskJob(
		"google-request-indexing", "Google Request Indexing", map[string]any{"site": s.searchSite()},
		taskProgress{Current: queue.CurrentIndex, Total: len(queue.Items), Unit: "URL", Message: "等待浏览器执行"},
		taskCapabilities(false, true, true),
	)
	if err != nil {
		return nil, err
	}
	queue.JobID = job.ID
	return job, nil
}

func (s *Server) updateGoogleRequestTaskFromQueue(queue googleIndexRequestQueue) (*durableTaskJob, error) {
	if queue.JobID == "" {
		return nil, errors.New("request-indexing queue has no task job")
	}
	return s.updateDurableTaskJob(queue.JobID, func(job *durableTaskJob) {
		currentURL := ""
		currentStatus := ""
		if queue.CurrentIndex >= 0 && queue.CurrentIndex < len(queue.Items) {
			currentURL = queue.Items[queue.CurrentIndex].URL
			currentStatus = queue.Items[queue.CurrentIndex].Status
		}
		job.Progress = taskProgress{
			Current: min(queue.CurrentIndex, len(queue.Items)),
			Total:   len(queue.Items),
			Unit:    "URL",
		}
		job.Detail = map[string]any{
			"queueState":        queue.State,
			"consecutiveErrors": queue.ConsecutiveErrors,
			"lastError":         queue.LastError,
			"currentUrl":        currentURL,
			"currentItemStatus": currentStatus,
		}
		switch queue.State {
		case "running":
			job.State = "running"
			if currentStatus == "processing" {
				job.Progress.Message = fmt.Sprintf("正在处理第 %d / %d 个 URL", queue.CurrentIndex+1, len(queue.Items))
			} else {
				job.Progress.Message = "Search Console 正在请求编入索引"
			}
			job.CanPause = true
			job.CanResume = false
			job.CanRetry = false
			if job.StartedAt == "" {
				job.StartedAt = s.now().UTC().Format(time.RFC3339)
			}
		case "paused":
			job.State = "paused"
			job.Progress.Message = "已暂停"
			job.CanPause = false
			job.CanResume = true
			job.CanRetry = false
		case "quota_blocked":
			job.State = "paused"
			job.Progress.Message = "Google 配额已用尽，等待下次继续"
			job.CanPause = false
			job.CanResume = true
			job.CanRetry = false
			job.Detail["reason"] = "quota_blocked"
		case "completed":
			job.State = "completed"
			job.Progress.Current = len(queue.Items)
			job.Progress.Message = "Request Indexing 已完成"
			job.FinishedAt = s.now().UTC().Format(time.RFC3339)
			job.CanPause = false
			job.CanResume = false
			job.CanRetry = false
		default:
			job.State = queue.State
		}
		job.Error = queue.LastError
	})
}

func recoverSearchTasksAfterRestart(s *Server) {
	state := s.loadSearchState()
	queue := &state.Google.RequestQueue
	queueChanged := false
	if queue.State == "running" {
		queue.State = "paused"
		queue.LastError = ""
		queue.UpdatedAt = s.now().UTC().Format(time.RFC3339)
		queueChanged = true
	}

	if (queue.State == "paused" || queue.State == "quota_blocked") && len(queue.Items) > 0 {
		if queue.JobID == "" || s.durableTaskJob(queue.JobID) == nil {
			queue.JobID = ""
			if _, err := s.ensureGoogleRequestTask(queue); err != nil {
				slog.Error("request-indexing task recovery failed", "error", err.Error())
			} else {
				queueChanged = true
			}
		}
		if queue.JobID != "" {
			if _, err := s.updateGoogleRequestTaskFromQueue(*queue); err != nil {
				slog.Error("request-indexing task state recovery failed", "jobId", queue.JobID, "error", err.Error())
			}
		}
	}

	if queueChanged {
		if err := saveSearchIndexState(state); err != nil {
			slog.Error("request-indexing queue recovery persistence failed", "error", err.Error())
		}
	}

	if job := s.latestDurableSearchTask("google-inspection"); job != nil && job.State == "queued" {
		var payload googleInspectionTaskPayload
		if len(job.Payload) > 0 && json.Unmarshal(job.Payload, &payload) == nil {
			resumed := reconcileRecoveredGoogleInspectionPayload(payload, state.Google.Inspection)
			if resumed != payload {
				raw, err := json.Marshal(resumed)
				if err == nil {
					_, _ = s.updateDurableTaskJob(job.ID, func(current *durableTaskJob) {
						current.Payload = raw
						current.Progress.Current = min(resumed.Offset, current.Progress.Total)
						current.Detail = map[string]any{
							"phase":      "recovered",
							"nextOffset": resumed.Offset,
						}
					})
				}
			}
		}
		s.launchSearchTaskJob(job.ID)
	}
}

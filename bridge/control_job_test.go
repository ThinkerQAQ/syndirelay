package bridge

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	blogapp "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/app"
)

func TestApplySyncEventToJobUpdatesStructuredResult(t *testing.T) {
	job := &syncJob{
		ID: "job-1",
		Results: map[string]syncPlatformResult{
			"devto": {State: "queued"},
		},
	}
	at := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	applySyncEventToJob(job, blogapp.SyncEvent{
		Platform: "devto",
		State:    "completed",
		Result:   "updated",
		URL:      "https://dev.to/example",
	}, at)

	got := job.Results["devto"]
	if got.State != "completed" || got.Result != "updated" || got.URL != "https://dev.to/example" {
		t.Fatalf("result = %#v", got)
	}
	if len(job.Events) != 1 || job.Events[0].At != "2026-09-17T12:00:00Z" {
		t.Fatalf("events = %#v", job.Events)
	}
}

func TestCloneSyncJobDeepCopiesCollections(t *testing.T) {
	original := &syncJob{
		Platforms: []string{"medium"},
		Results:   map[string]syncPlatformResult{"medium": {State: "running"}},
		Events:    []syncJobEvent{{Platform: "medium", State: "running"}},
	}
	clone := cloneSyncJob(original)
	clone.Platforms[0] = "devto"
	clone.Results["medium"] = syncPlatformResult{State: "failed"}
	clone.Events[0].State = "failed"
	if original.Platforms[0] != "medium" || original.Results["medium"].State != "running" || original.Events[0].State != "running" {
		t.Fatalf("original mutated: %#v", original)
	}
}

func TestDeleteSyncJobRejectsRunningAndRemovesFinished(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	server.jobs["running"] = &syncJob{ID: "running", State: "running"}
	server.jobs["done"] = &syncJob{ID: "done", State: "completed"}
	server.jobOrder = []string{"running", "done"}

	if err := server.deleteSyncJob("running"); err == nil || err.Error() != "running sync job cannot be deleted" {
		t.Fatalf("running delete error = %v", err)
	}
	if err := server.deleteSyncJob("done"); err != nil {
		t.Fatal(err)
	}
	if server.jobs["done"] != nil {
		t.Fatal("finished job was not deleted")
	}
	if len(server.jobOrder) != 1 || server.jobOrder[0] != "running" {
		t.Fatalf("jobOrder = %#v", server.jobOrder)
	}
}

func TestPruneSyncJobHistoryNeverEvictsRunningJobs(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	server.jobs = map[string]*syncJob{}
	server.jobOrder = nil

	for index := 0; index < 22; index++ {
		id := fmt.Sprintf("job-%02d", index)
		state := "completed"
		if index == 21 {
			state = "running"
		}
		server.jobs[id] = &syncJob{ID: id, State: state}
		server.jobOrder = append(server.jobOrder, id)
	}

	server.pruneSyncJobHistoryLocked(20)
	if server.jobs["job-21"] == nil || server.jobs["job-21"].State != "running" {
		t.Fatal("running job was evicted from history")
	}
	if len(server.jobOrder) != 21 {
		t.Fatalf("jobOrder length = %d, want 21 while an old job is still running", len(server.jobOrder))
	}

	server.jobs["job-21"].State = "completed"
	server.pruneSyncJobHistoryLocked(20)
	if len(server.jobOrder) != 20 {
		t.Fatalf("jobOrder length = %d, want 20 after all jobs finish", len(server.jobOrder))
	}
	if server.jobs["job-21"] != nil {
		t.Fatal("old completed job was not pruned")
	}
}

func TestClearFinishedSyncJobsKeepsRunningJobs(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	server.jobs["running"] = &syncJob{ID: "running", State: "running"}
	server.jobs["failed"] = &syncJob{ID: "failed", State: "failed"}
	server.jobs["done"] = &syncJob{ID: "done", State: "completed"}
	server.jobOrder = []string{"running", "failed", "done"}

	removed := server.clearFinishedSyncJobs()
	if removed != 2 {
		t.Fatalf("removed = %d", removed)
	}
	if len(server.jobOrder) != 1 || server.jobOrder[0] != "running" {
		t.Fatalf("jobOrder = %#v", server.jobOrder)
	}
}

func TestCloneSyncJobPreservesRetryRequest(t *testing.T) {
	original := &syncJob{
		ID: "job-1",
		Request: syncRequest{
			Article:   "example",
			Platforms: []string{"juejin"},
			Changed:   true,
			Draft:     true,
		},
	}
	clone := cloneSyncJob(original)
	if clone.Request.Article != "example" || !clone.Request.Changed || !clone.Request.Draft {
		t.Fatalf("request = %#v", clone.Request)
	}
}

func TestMoveJobToFrontDoesNotDuplicateID(t *testing.T) {
	got := moveJobToFront([]string{"newer", "job-1", "older"}, "job-1")
	want := []string{"job-1", "newer", "older"}
	if len(got) != len(want) {
		t.Fatalf("jobOrder = %#v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("jobOrder = %#v", got)
		}
	}
}

func TestRetrySyncJobReusesIDAndReplacesFailedAttempt(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	server.syncRunner = func(_ context.Context, _ bridgeConfig, request syncRequest, emit func(blogapp.SyncEvent)) (string, error) {
		for _, platform := range request.Platforms {
			emit(blogapp.SyncEvent{Platform: platform, State: "completed", Result: "created"})
		}
		return "ok", nil
	}
	request := syncRequest{
		Article:   "example",
		Platforms: []string{"juejin"},
		Changed:   true,
		Draft:     true,
	}
	server.jobs["job-1"] = &syncJob{
		ID: "job-1", Article: request.Article, Platforms: request.Platforms,
		Request: request, State: "failed", Error: "old failure",
	}
	server.jobs["other"] = &syncJob{ID: "other", State: "completed"}
	server.jobOrder = []string{"other", "job-1"}

	retried, err := server.retrySyncJob("job-1")
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID != "job-1" {
		t.Fatalf("retry id = %q, want job-1", retried.ID)
	}
	if len(server.jobOrder) != 2 || server.jobOrder[0] != "job-1" || server.jobOrder[1] != "other" {
		t.Fatalf("jobOrder = %#v", server.jobOrder)
	}
	count := 0
	for _, id := range server.jobOrder {
		if id == "job-1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("job-1 appears %d times in %#v", count, server.jobOrder)
	}
	if retried.State != "running" || retried.Error != "" || retried.Output != "" || retried.FinishedAt != "" {
		t.Fatalf("retried job was not reset: %#v", retried)
	}
	if retried.Results["juejin"].State != "queued" {
		t.Fatalf("retry result = %#v", retried.Results["juejin"])
	}
	if len(retried.Events) != 1 || retried.Events[0].Platform != "juejin" || retried.Events[0].State != "queued" {
		t.Fatalf("retry events = %#v", retried.Events)
	}
}

func TestRetrySyncJobRejectsPublishAttempts(t *testing.T) {
	server, err := New("token")
	if err != nil {
		t.Fatal(err)
	}
	server.jobs["publish-job"] = &syncJob{
		ID: "publish-job", State: "failed", Operation: "publish",
		Request: syncRequest{
			Article: "example", Platforms: []string{"oschina"}, Operation: "publish",
		},
	}
	server.jobOrder = []string{"publish-job"}

	if _, err := server.retrySyncJob("publish-job"); err == nil || !strings.Contains(err.Error(), "cannot be retried safely") {
		t.Fatalf("error = %v", err)
	}
	if server.jobs["publish-job"].State != "failed" {
		t.Fatalf("publish job was mutated: %#v", server.jobs["publish-job"])
	}

}

func TestChangedPoliciesForRequestUsesSelectedPlatformConfig(t *testing.T) {
	config := defaultBridgeConfig()
	profile := config.Publishing.Platforms["cnblogs"]
	profile.ChangedOnly = true
	config.Publishing.Platforms["cnblogs"] = profile
	request := syncRequest{Platforms: []string{"cnblogs", "juejin"}, Operation: "draft", UsePlatformChangedOnly: true}
	policies := changedPoliciesForRequest(config, request)
	if len(policies) != 2 || !policies["cnblogs"] || policies["juejin"] {
		t.Fatalf("policies = %#v", policies)
	}
	request.UsePlatformChangedOnly = false
	if changedPoliciesForRequest(config, request) != nil {
		t.Fatal("request without platform policy must keep its global changed flag")
	}
	request.UsePlatformChangedOnly = true
	request.Operation = "publish"
	if changedPoliciesForRequest(config, request) != nil {
		t.Fatal("publish operation must ignore draft policy")
	}
}

func TestRetryFailedMediumCreateKeepsPreviouslyAllocatedDraft(t *testing.T) {
	server, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan syncRequest, 1)
	server.syncRunner = func(_ context.Context, _ bridgeConfig, request syncRequest, _ func(blogapp.SyncEvent)) (string, error) {
		received <- request
		return "ok", nil
	}
	request := syncRequest{Article: "test", Platforms: []string{"medium"}, Operation: "create"}
	server.jobs["resume-job"] = &syncJob{
		ID: "resume-job", Article: "test", Operation: "create", Platforms: []string{"medium"}, Request: request,
		State: "failed", Events: []syncJobEvent{{Platform: "medium", State: "running", Result: "draft-allocated", TargetID: "89da2be2e4d5"}},
	}
	server.jobOrder = []string{"resume-job"}
	got, err := server.retrySyncJob("resume-job")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "resume-job" || got.Request.ResumeMediumDraftID != "89da2be2e4d5" {
		t.Fatalf("Medium retry allocated new remote story: %+v", got)
	}
	select {
	case forwarded := <-received:
		if forwarded.ResumeMediumDraftID != "89da2be2e4d5" {
			t.Fatal("failed to pass allocated remote ID")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("retry runner was not invoked")
	}
}

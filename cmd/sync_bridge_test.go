package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestBridgeLiveArticlesFiltersPublishedArticles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/articles" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		if request.Header.Get("x-thinkerqaq-token") != "token" {
			t.Fatalf("token = %q", request.Header.Get("x-thinkerqaq-token"))
		}
		response.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{
			"articles": []map[string]any{
				{"slug": "published-a", "status": "published"},
				{"slug": "draft-b", "status": "draft"},
				{"slug": "published-c", "status": "published"},
			},
		})
	}))
	defer server.Close()

	articles, err := bridgeLiveArticles(context.Background(), server.Client(), bridgeState{
		BaseURL: server.URL,
		Token:   "token",
		PID:     1,
	}, syncOptions{all: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(articles, []string{"published-a", "published-c"}) {
		t.Fatalf("articles = %#v", articles)
	}
}

func TestWaitBridgeSyncJobReturnsCompletedJob(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/sync/jobs/job-1" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		state := "running"
		result := ""
		url := ""
		if calls.Add(1) >= 2 {
			state = "completed"
			result = "draft-created"
			url = "https://example.com/draft"
		}
		response.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{
			"job": map[string]any{
				"id":      "job-1",
				"article": "example",
				"state":   state,
				"results": map[string]any{
					"devto": map[string]any{
						"state":  state,
						"result": result,
						"url":    url,
					},
				},
			},
		})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	job, err := waitBridgeSyncJob(ctx, server.Client(), bridgeState{
		BaseURL: server.URL,
		Token:   "token",
		PID:     1,
	}, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if job.State != "completed" || job.Results["devto"].URL != "https://example.com/draft" {
		t.Fatalf("job = %#v", job)
	}
}

func TestBridgeRequestJSONSurfacesStructuredError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("content-type", "application/json")
		response.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(response).Encode(map[string]any{
			"error":   "invalid request",
			"code":    "invalid_request",
			"message": "browser session is required",
		})
	}))
	defer server.Close()

	err := bridgeRequestJSON(context.Background(), server.Client(), bridgeState{
		BaseURL: server.URL,
		Token:   "token",
		PID:     1,
	}, http.MethodPost, "/v1/sync/jobs", map[string]any{"article": "example"}, nil)
	if err == nil || err.Error() != "Bridge 400 Bad Request: browser session is required" {
		t.Fatalf("error = %v", err)
	}
}

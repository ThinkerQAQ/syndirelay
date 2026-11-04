package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Exercise the actual first request against an uninitialized Bridge client,
// not just concurrent calls against a pre-warmed ready=true client.
func TestMCPColdStartConcurrentReadOnly(t *testing.T) {
	var starts atomic.Int32
	client := &mcpBridgeClient{
		stateProvider: func() (bridgeState, error) {
			starts.Add(1)
			time.Sleep(30 * time.Millisecond)
			return bridgeState{BaseURL: "http://127.0.0.1:32146", Token: "synthetic-startup-token"}, nil
		},
		http: &http.Client{Transport: bridgeRecoveryRoundTripper(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("x-thinkerqaq-token") != "synthetic-startup-token" {
				t.Error("incorrect initial Bridge token")
			}
			body := "{\"ok\":true}"
			if strings.Contains(r.URL.Path, "/v1/articles") {
				body = "{\"articles\":[]}"
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
		})},
	}
	server := newBlogMCPServer(client)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 30)
	for index := 0; index < 10; index++ {
		st, ct := mcp.NewInMemoryTransports()
		ss, err := server.Connect(ctx, st, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ss.Close()
		mc := mcp.NewClient(&mcp.Implementation{Name: "cold-start-check", Version: "1.0"}, nil)
		session, err := mc.Connect(ctx, ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		for _, name := range []string{"list_articles", "list_platforms", "list_tasks"} {
			wg.Add(1)
			go func(session *mcp.ClientSession, name string) {
				defer wg.Done()
				response, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
				if err != nil {
					errs <- err
					return
				}
				if response.IsError {
					errs <- io.ErrUnexpectedEOF
				}
			}(session, name)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if count := starts.Load(); count != 1 {
		t.Errorf("expected exactly one Bridge cold start, got %d", count)
	}
}

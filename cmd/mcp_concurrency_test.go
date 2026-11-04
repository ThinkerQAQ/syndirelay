package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The first read-only calls often arrive together immediately after MCP
// initialization. They must not cause shared Bridge startup races.
func TestMCPFirstConcurrentReadOnlyCalls(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-thinkerqaq-token") != "synthetic-test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/platforms":
			_, _ = w.Write([]byte(`{"platforms":["devto"]}`))
		case "/v1/articles":
			_, _ = w.Write([]byte(`{"articles":[{"slug":"unit-test"}]}`))
		case "/v1/sync/jobs":
			_, _ = w.Write([]byte(`{"jobs":[]}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer mock.Close()
	bridge := &mcpBridgeClient{ready: true, state: bridgeState{BaseURL: mock.URL, Token: "synthetic-test-token"}}
	server := newBlogMCPServer(bridge)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errors := make(chan error, 50)
	for sessionIndex := 0; sessionIndex < 5; sessionIndex++ {
		st, ct := mcp.NewInMemoryTransports()
		ss, e := server.Connect(ctx, st, nil)
		if e != nil {
			t.Fatal(e)
		}
		defer ss.Close()
		client := mcp.NewClient(&mcp.Implementation{Name: "concurrency-probe", Version: "1.0"}, nil)
		cs, e := client.Connect(ctx, ct, nil)
		if e != nil {
			t.Fatal(e)
		}
		defer cs.Close()
		for _, name := range []string{"list_platforms", "list_articles", "list_tasks"} {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
				if err != nil {
					errors <- err
					return
				}
				if result.IsError {
					raw, _ := json.Marshal(result)
					errors <- fmt.Errorf("%s result failed: %s", name, raw)
				}
			}(name)
		}
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

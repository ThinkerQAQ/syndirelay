package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	runtimeversion "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The MCP server is an adapter to the already-running local BlogCTL Bridge.
// Credentials remain in the browser Extension and local Bridge, never in tool results.
type mcpBridgeClient struct {
	mu    sync.Mutex
	state bridgeState
	ready bool
	http  *http.Client
}

func (c *mcpBridgeClient) call(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	c.mu.Lock()
	if !c.ready {
		state, err := ensureBridgeProcess()
		if err != nil {
			c.mu.Unlock()
			return nil, err
		}
		// Never send the local Bridge token to an arbitrary origin, even if a
		// stale/tampered runtime-state file advertises a different address.
		if state.BaseURL != "http://127.0.0.1:32145" && state.BaseURL != "http://localhost:32145" {
			c.mu.Unlock()
			return nil, errors.New("invalid local BlogCTL Bridge origin")
		}
		c.state = state
		c.ready = true
	}
	state := c.state
	if c.http == nil {
		c.http = &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	}
	client := c.http
	c.mu.Unlock()
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		input = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, state.BaseURL+path, input)
	if err != nil {
		return nil, err
	}
	request.Header.Set("x-thinkerqaq-token", state.Token)
	if body != nil {
		request.Header.Set("content-type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		c.mu.Lock()
		c.ready = false
		c.mu.Unlock() // do not automatically retry any operation
		return nil, fmt.Errorf("local BlogCTL Bridge unavailable: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, errors.New("BlogCTL response exceeds 4MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var failure struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		// Some API errors are wrapped in {error:{code,message}}.
		var wrap struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &failure)
		_ = json.Unmarshal(raw, &wrap)
		message := failure.Message
		if message == "" {
			message = wrap.Error.Message
		}
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return nil, fmt.Errorf("BlogCTL: %s (HTTP %d)", message, response.StatusCode)
	}
	return json.RawMessage(raw), nil
}

func (c *mcpBridgeClient) refreshSessions(ctx context.Context, platforms []string) error {
	raw, err := c.call(ctx, http.MethodPost, "/v1/mcp/session-refresh", map[string]any{"platforms": platforms})
	if err != nil {
		return err
	}
	var queue struct {
		Pending []string `json:"pending"`
	}
	if err := json.Unmarshal(raw, &queue); err != nil {
		return err
	}
	if len(queue.Pending) == 0 {
		return nil
	}
	deadline := time.NewTimer(80 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready := true
		for _, platform := range queue.Pending {
			status, err := c.call(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(platform)+"/status", nil)
			if err != nil {
				return err
			}
			var result struct {
				Authenticated bool `json:"authenticated"`
			}
			if err := json.Unmarshal(status, &result); err != nil {
				return err
			}
			if !result.Authenticated {
				ready = false
				break
			}
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("browser session refresh timed out: keep the BlogCTL Edge/Chrome extension enabled and signed in")
		case <-ticker.C:
		}
	}
}

type mcpArticleID struct {
	Slug     string `json:"slug"`
	Language string `json:"language,omitempty"`
}
type mcpTarget struct {
	Platform  string `json:"platform"`
	ID        string `json:"id"`
	URL       string `json:"url,omitempty"`
	State     string `json:"state"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}
type mcpPublishInput struct {
	Article   string      `json:"article"`
	Platforms []string    `json:"platforms"`
	RequestID string      `json:"request_id" jsonschema:"Unique 8-128 character ID. Reuse the same ID only when retrying the same intended operation."`
	Targets   []mcpTarget `json:"targets,omitempty"`
}

type mcpResult struct {
	Data any `json:"data"`
}

func mcpTool(name, desc string, readOnly bool) *mcp.Tool {
	no := false
	return &mcp.Tool{Name: name, Description: desc, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, DestructiveHint: &no, OpenWorldHint: &no}}
}

func newBlogMCPServer(c *mcpBridgeClient) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "blogctl", Version: runtimeversion.Current}, &mcp.ServerOptions{Instructions: "BlogCTL MCP calls the existing local Bridge, never receives browser cookies or executes arbitrary commands. Read tools are safe. Create always creates a new draft; update requires an explicit remote target; publish-draft requires a draft ID. Ask for explicit publishing intent before side effects. Do not blindly retry a timed-out publishing action; use the same request_id to check the original task, inspect get_task and the remote platform state. Browser extension must be installed and signed in; session refresh is automatic. Images may be sanitized and uploaded only from explicitly authorized local folders. The MCP is not a content editor or Git/deployment tool."})
	addRead := func(name, desc, method, path string) {
		mcp.AddTool(server, mcpTool(name, desc, true), func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, mcpResult, error) {
			raw, err := c.call(ctx, method, path, nil)
			return nil, mcpResult{raw}, err
		})
	}
	addRead("list_articles", "List locally published Chinese articles and whether English mirrors exist.", "GET", "/v1/articles")
	addRead("list_platforms", "List supported platforms and BlogCTL capabilities; not a guarantee that user is signed in.", "GET", "/v1/mcp/platforms")
	addRead("get_publications", "List locally recorded article distribution bindings; does not prove current remote state.", "GET", "/v1/publications")
	addRead("list_tasks", "List saved BlogCTL tasks and publishing results.", "GET", "/v1/jobs")
	mcp.AddTool(server, mcpTool("get_article", "Read Markdown from the configured local content checkout by slug and language. No arbitrary paths.", true), func(ctx context.Context, _ *mcp.CallToolRequest, in mcpArticleID) (*mcp.CallToolResult, mcpResult, error) {
		lang := in.Language
		if lang == "" {
			lang = "zh-CN"
		}
		if strings.TrimSpace(in.Slug) == "" {
			return nil, mcpResult{}, errors.New("slug required")
		}
		raw, err := c.call(ctx, "GET", "/v1/mcp/article/"+url.PathEscape(in.Slug)+"?language="+url.QueryEscape(lang), nil)
		return nil, mcpResult{raw}, err
	})
	mcp.AddTool(server, mcpTool("get_article_links", "Get recorded per-platform URLs and IDs for a local article slug. Links do not prove the current remote account state.", true), func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		Article string `json:"article"`
	}) (*mcp.CallToolResult, mcpResult, error) {
		if in.Article == "" {
			return nil, mcpResult{}, errors.New("article required")
		}
		raw, err := c.call(ctx, "GET", "/v1/article-links?article="+url.QueryEscape(in.Article), nil)
		return nil, mcpResult{raw}, err
	})
	imageTool := mcpTool("upload_image_to_r2", "Sanitize a local JPEG/PNG/WebP image, auto-orient, strip metadata, content-version, then upload cleaned image to R2. dry_run never uploads. Source must be in configured content_root or image_upload_roots.", false)
	remoteWrite := true
	imageTool.Annotations.OpenWorldHint = &remoteWrite
	mcp.AddTool(server, imageTool, func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		LocalPath string `json:"local_path" jsonschema:"Absolute local image path on BlogCTL host."`
		Format    string `json:"format,omitempty" jsonschema:"webp (default), jpeg or png."`
		Alt       string `json:"alt,omitempty" jsonschema:"Optional Markdown image alt text."`
		DryRun    bool   `json:"dry_run,omitempty" jsonschema:"Sanitize locally, without uploading to R2."`
	}) (*mcp.CallToolResult, mcpResult, error) {
		if strings.TrimSpace(in.LocalPath) == "" {
			return nil, mcpResult{}, errors.New("local_path required")
		}
		raw, err := c.call(ctx, "POST", "/v1/mcp/images/upload", map[string]any{"localPath": in.LocalPath, "format": in.Format, "alt": in.Alt, "dryRun": in.DryRun})
		return nil, mcpResult{raw}, err
	})
	mcp.AddTool(server, mcpTool("get_task", "Get a publishing task by the ID returned from create/update/publish; includes per-platform result links.", true), func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		ID string `json:"id"`
	}) (*mcp.CallToolResult, mcpResult, error) {
		if in.ID == "" {
			return nil, mcpResult{}, errors.New("id required")
		}
		raw, err := c.call(ctx, "GET", "/v1/sync/jobs/"+url.PathEscape(in.ID), nil)
		return nil, mcpResult{raw}, err
	})
	// Preview is local-only. Existing Bridge dry-run compiles but never creates remote articles.
	mcp.AddTool(server, mcpTool("preview_publish", "Run BlogCTL dry-run for an explicitly selected article and platform; creates a local task but makes no remote writes.", false), func(ctx context.Context, _ *mcp.CallToolRequest, in struct {
		Article   string   `json:"article"`
		Platforms []string `json:"platforms"`
	}) (*mcp.CallToolResult, mcpResult, error) {
		raw, err := c.call(ctx, "POST", "/v1/sync/jobs", map[string]any{"article": in.Article, "platforms": in.Platforms, "operation": "create", "dryRun": true})
		return nil, mcpResult{raw}, err
	})
	publish := func(name, desc, operation string, after bool) {
		mcp.AddTool(server, mcpTool(name, desc, false), func(ctx context.Context, _ *mcp.CallToolRequest, in mcpPublishInput) (*mcp.CallToolResult, mcpResult, error) {
			if len(in.RequestID) < 8 {
				return nil, mcpResult{}, errors.New("request_id with 8 or more characters is required for safe retries")
			}
			if strings.TrimSpace(in.Article) == "" || len(in.Platforms) == 0 {
				return nil, mcpResult{}, errors.New("article and platforms are required")
			}
			if operation == "create" && len(in.Targets) > 0 {
				return nil, mcpResult{}, errors.New("create cannot take existing targets")
			}
			if (operation == "update" || operation == "publish-draft") && len(in.Targets) == 0 {
				return nil, mcpResult{}, errors.New("explicit remote targets are required")
			}
			if err := c.refreshSessions(ctx, in.Platforms); err != nil {
				return nil, mcpResult{}, err
			}
			payload := map[string]any{"article": in.Article, "platforms": in.Platforms, "operation": operation, "requestId": in.RequestID, "publishAfter": after}
			if len(in.Targets) > 0 {
				payload["targets"] = in.Targets
			}
			raw, err := c.call(ctx, "POST", "/v1/sync/jobs", payload)
			return nil, mcpResult{raw}, err
		})
	}
	publish("create_draft", "Create a new remote draft for the explicitly selected article/platforms; creates another draft even if a previous one exists. Do not use to update existing articles.", "create", false)
	publish("create_and_publish", "Create a new draft and explicitly publish it on supported platforms. This is a public side effect; duplicates are possible if used for an existing article.", "create", true)
	publish("update_article", "Update only selected remote article targets; never creates a replacement draft. Provide platform/id/state and any known URL/version.", "update", false)
	publish("update_and_publish", "Update explicitly selected remote drafts and publish on supported platforms; does not silently create a new draft.", "update", true)
	publish("publish_draft", "Publish only selected existing remote draft IDs; no new draft creation.", "publish-draft", false)
	return server
}

func runMCPStdio() error {
	c := &mcpBridgeClient{}
	return newBlogMCPServer(c).Run(context.Background(), &mcp.StdioTransport{})
}

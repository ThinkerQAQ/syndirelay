package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	blogapp "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/app"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/bridge"
	blogplatform "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/platform"
)

type syncOptions struct {
	articles  []string
	all       bool
	platforms []string
	dryRun    bool
	changed   bool
	draft     bool
}

func parseSyncArgs(args []string) (syncOptions, error) {
	var options syncOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--article":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return options, errors.New("--article requires a value")
			}
			options.articles = append(options.articles, args[i+1])
			i++
		case "--all":
			options.all = true
		case "--platforms":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return options, errors.New("--platforms requires a value")
			}
			for _, value := range strings.Split(args[i+1], ",") {
				value = strings.TrimSpace(value)
				if value != "" && !slices.Contains(options.platforms, value) {
					options.platforms = append(options.platforms, value)
				}
			}
			i++
		case "--dry-run":
			options.dryRun = true
		case "--changed":
			options.changed = true
		case "--draft":
			options.draft = true
		default:
			return options, fmt.Errorf("unknown sync option %q", args[i])
		}
	}
	if options.all && len(options.articles) > 0 {
		return options, errors.New("choose either explicit --article selections or explicit --all, not both")
	}
	if !options.all && len(options.articles) == 0 {
		return options, errors.New("explicit article selection is required: use --article <slug>, or --all only when full syndication is intentional")
	}
	if len(options.platforms) == 0 {
		return options, errors.New("explicit platform selection is required: use --platforms <list>")
	}
	for _, platform := range options.platforms {
		if !blogplatform.Supported(platform) {
			return options, fmt.Errorf("unsupported platform: %s", platform)
		}
	}
	return options, nil
}

type bridgeSyncResult struct {
	State   string `json:"state"`
	Result  string `json:"result,omitempty"`
	URL     string `json:"url,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type bridgeSyncJob struct {
	ID      string                      `json:"id"`
	Article string                      `json:"article"`
	State   string                      `json:"state"`
	Error   string                      `json:"error,omitempty"`
	Results map[string]bridgeSyncResult `json:"results"`
	Output  string                      `json:"output,omitempty"`
}

type bridgeAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func bridgeRequestJSON(ctx context.Context, client *http.Client, state bridgeState, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(state.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("content-type", "application/json")
	}
	request.Header.Set("x-thinkerqaq-token", state.Token)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		var decoded bridgeAPIError
		if json.Unmarshal(payload, &decoded) == nil {
			message := strings.TrimSpace(decoded.Message)
			if message != "" {
				return fmt.Errorf("Bridge %s: %s", response.Status, message)
			}
		}
		return fmt.Errorf("Bridge %s", response.Status)
	}
	if output == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(output); err != nil {
		return fmt.Errorf("decode Bridge response: %w", err)
	}
	return nil
}

func bridgeLiveArticles(ctx context.Context, client *http.Client, state bridgeState, options syncOptions) ([]string, error) {
	if !options.all {
		return append([]string{}, options.articles...), nil
	}
	var response struct {
		Articles []struct {
			Slug   string `json:"slug"`
			Status string `json:"status"`
		} `json:"articles"`
	}
	if err := bridgeRequestJSON(ctx, client, state, http.MethodGet, "/v1/articles", nil, &response); err != nil {
		return nil, err
	}
	articles := make([]string, 0, len(response.Articles))
	for _, article := range response.Articles {
		if strings.EqualFold(strings.TrimSpace(article.Status), "published") && strings.TrimSpace(article.Slug) != "" {
			articles = append(articles, article.Slug)
		}
	}
	if len(articles) == 0 {
		return nil, errors.New("Bridge returned no published articles for --all")
	}
	return articles, nil
}

func waitBridgeSyncJob(ctx context.Context, client *http.Client, state bridgeState, id string) (bridgeSyncJob, error) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		var response struct {
			Job bridgeSyncJob `json:"job"`
		}
		if err := bridgeRequestJSON(ctx, client, state, http.MethodGet, "/v1/sync/jobs/"+id, nil, &response); err != nil {
			return bridgeSyncJob{}, err
		}
		switch response.Job.State {
		case "completed":
			return response.Job, nil
		case "failed":
			message := strings.TrimSpace(response.Job.Error)
			if message == "" {
				message = "sync job failed"
			}
			return response.Job, errors.New(message)
		}
		select {
		case <-ctx.Done():
			return bridgeSyncJob{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func printBridgeSyncJob(out io.Writer, job bridgeSyncJob) {
	for platform, result := range job.Results {
		line := fmt.Sprintf("[%s] %s", platform, result.Result)
		if strings.TrimSpace(result.Result) == "" {
			line = fmt.Sprintf("[%s] %s", platform, result.State)
		}
		if result.URL != "" {
			line += " " + result.URL
		}
		if result.Message != "" {
			line += " · " + result.Message
		}
		fmt.Fprintln(out, line)
	}
}

func (a app) runLiveSyncViaBridge(ctx context.Context, options syncOptions) error {
	state, err := ensureBridgeProcess()
	if err != nil {
		return fmt.Errorf("start BlogCTL Bridge: %w", err)
	}
	fmt.Fprintf(a.out, "[bridge] ready on %s\n", state.BaseURL)

	client := &http.Client{Timeout: 30 * time.Second}
	articles, err := bridgeLiveArticles(ctx, client, state, options)
	if err != nil {
		return err
	}
	for _, article := range articles {
		body := map[string]any{
			"article":   article,
			"platforms": append([]string{}, options.platforms...),
			"dryRun":    false,
			"changed":   options.changed,
			"draft":     options.draft,
			"operation": "draft",
		}
		var response struct {
			Job bridgeSyncJob `json:"job"`
		}
		if err := bridgeRequestJSON(ctx, client, state, http.MethodPost, "/v1/sync/jobs", body, &response); err != nil {
			return fmt.Errorf("%s: %w", article, err)
		}
		if response.Job.ID == "" {
			return fmt.Errorf("%s: Bridge returned an empty sync job id", article)
		}
		fmt.Fprintf(a.out, "[job %s] %s\n", response.Job.ID, article)
		job, err := waitBridgeSyncJob(ctx, client, state, response.Job.ID)
		printBridgeSyncJob(a.out, job)
		if err != nil {
			return fmt.Errorf("%s: %w", article, err)
		}
	}
	return nil
}

func (a app) runSync(args []string) error {
	options, err := parseSyncArgs(args)
	if err != nil {
		return err
	}
	if a.contentRoot == "" {
		return errors.New("content repository root is required for sync")
	}
	if err := bridge.UpdateWorkspaceRoots(a.contentRoot, a.root); err != nil {
		fmt.Fprintf(a.out, "[config] unable to persist workspace roots: %v\n", err)
	}

	if !options.dryRun {
		return a.runLiveSyncViaBridge(context.Background(), options)
	}

	config := blogapp.SyncConfig{
		EngineRoot:       a.root,
		ContentRoot:      a.contentRoot,
		DistributionRoot: bridge.ResolvedDistributionRoot(),
		Publishing:       bridge.ResolvedPublishingConfig(),
	}
	service := blogapp.NewSyncService()
	output, err := service.Run(context.Background(), config, blogapp.SyncRequest{
		Articles:  append([]string{}, options.articles...),
		All:       options.all,
		Platforms: append([]string{}, options.platforms...),
		DryRun:    true,
		Changed:   options.changed,
		Draft:     options.draft,
	})
	if output != "" {
		fmt.Fprint(a.out, output)
	}
	return err
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	blogaisearch "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/aisearch"
)

type aiSearchStats struct {
	Queued    int `json:"queued"`
	Running   int `json:"running"`
	Outdated  int `json:"outdated"`
	Completed int `json:"completed"`
	Error     int `json:"error"`
	Engine    struct {
		R2 struct {
			ObjectCount int `json:"objectCount"`
		} `json:"r2"`
		Vectorize struct {
			VectorsCount int `json:"vectorsCount"`
		} `json:"vectorize"`
	} `json:"engine"`
}

type cloudflareEnvelope struct {
	Success *bool           `json:"success"`
	Result  json.RawMessage `json:"result"`
	Errors  []struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

func (a app) runAISearch(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: blogctl ai-search <prepare|sync|verify> [options]")
	}
	switch args[0] {
	case "prepare":
		return a.runAISearchPrepare(args[1:])
	case "sync":
		return a.runAISearchSync(args[1:])
	case "verify":
		return a.runAISearchVerify(args[1:])
	default:
		return errors.New("usage: blogctl ai-search <prepare|sync|verify> [options]")
	}
}

func optionArg(args []string, name, fallback string) (string, error) {
	for index := 0; index < len(args); index++ {
		if args[index] != name {
			continue
		}
		if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
			return "", fmt.Errorf("%s requires a value", name)
		}
		return args[index+1], nil
	}
	return fallback, nil
}

func hasArg(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func isZeroSHA(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && strings.Trim(value, "0") == ""
}

func runGit(dir string, args ...string) error {
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func gitOutput(dir string, args ...string) ([]byte, error) {
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Stderr = os.Stderr
	return command.Output()
}

func (a app) runAISearchPrepare(args []string) error {
	output, err := optionArg(args, "--output", "")
	if err != nil {
		return err
	}
	if strings.TrimSpace(output) == "" {
		return errors.New("--output is required")
	}
	contentRoot, err := optionArg(args, "--content-root", "")
	if err != nil {
		return err
	}
	contentSHA, err := optionArg(args, "--content-sha", os.Getenv("CONTENT_SHA"))
	if err != nil {
		return err
	}
	beforeSHA, err := optionArg(args, "--content-before-sha", os.Getenv("CONTENT_BEFORE_SHA"))
	if err != nil {
		return err
	}
	forceFull := hasArg(args, "--force-full") || strings.TrimSpace(os.Getenv("AI_SEARCH_FORCE_FULL_SYNC")) == "1"

	resolvedOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	// After the engine split, content may live in its own checkout rather than
	// beneath the legacy site repository. Honor the explicitly provided source.
	sourceRoot := filepath.Join(a.root, "src", "content")
	if strings.TrimSpace(contentRoot) != "" {
		resolvedContent, err := filepath.Abs(contentRoot)
		if err != nil {
			return err
		}
		sourceRoot = filepath.Join(resolvedContent, "src", "content")
	}
	prepared, err := blogaisearch.PrepareInput(sourceRoot, resolvedOutput)
	if err != nil {
		return fmt.Errorf("prepare AI Search input: %w", err)
	}
	fmt.Fprintf(
		a.out,
		"[ai-search] prepared articles=%d notes=%d translations=%d\n",
		prepared.Articles,
		prepared.Notes,
		prepared.NoteTranslations,
	)

	manifest := filepath.Join(resolvedOutput, "ai-search-changed-paths.txt")
	if forceFull {
		if err := os.Remove(manifest); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		fmt.Fprintln(a.out, "[ai-search] full reconcile requested")
		return nil
	}
	contentSHA = strings.TrimSpace(contentSHA)
	beforeSHA = strings.TrimSpace(beforeSHA)
	if contentSHA == "" {
		return os.WriteFile(manifest, nil, 0o644)
	}
	if beforeSHA == "" || isZeroSHA(beforeSHA) {
		return errors.New("content-before-sha is required for incremental AI Search sync; use --force-full for a full reconcile")
	}
	if strings.TrimSpace(contentRoot) == "" {
		return errors.New("--content-root is required when content-sha is set")
	}
	resolvedContent, err := filepath.Abs(contentRoot)
	if err != nil {
		return err
	}
	if err := runGit(resolvedContent, "fetch", "--no-tags", "--depth=1", "origin", beforeSHA); err != nil {
		return fmt.Errorf("fetch previous content revision: %w", err)
	}
	diff, err := gitOutput(
		resolvedContent,
		"diff", "--no-renames", "--name-only", beforeSHA, contentSHA, "--",
		"src/content/articles", "src/content/notes", "src/content/note-translations",
	)
	if err != nil {
		return fmt.Errorf("compute AI Search change manifest: %w", err)
	}
	if err := os.WriteFile(manifest, diff, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "[ai-search] change manifest written: %s\n", manifest)
	return nil
}

func (a app) runAISearchSync(args []string) error {
	verify := hasArg(args, "--verify")
	manifest, err := optionArg(args, "--manifest", filepath.Join(a.root, "src", "content", "ai-search-changed-paths.txt"))
	if err != nil {
		return err
	}
	forceFull := hasArg(args, "--force-full") || strings.TrimSpace(os.Getenv("AI_SEARCH_FORCE_FULL_SYNC")) == "1"

	accountID := strings.TrimSpace(os.Getenv("CLOUDFLARE_ACCOUNT_ID"))
	token := strings.TrimSpace(os.Getenv("CLOUDFLARE_AI_SEARCH_TOKEN"))
	instance := strings.TrimSpace(os.Getenv("CLOUDFLARE_AI_SEARCH_INSTANCE"))
	if instance == "" {
		instance = "thinkerqaq-blog"
	}
	blogOrigin := strings.TrimSpace(os.Getenv("BLOG_ORIGIN"))
	if blogOrigin == "" {
		blogOrigin = blogaisearch.DefaultBlogOrigin
	}
	requestTimeout, err := durationFromMillisecondsEnv("AI_SEARCH_REQUEST_TIMEOUT_MS", 30*time.Second)
	if err != nil {
		return err
	}
	maxRetries, err := intFromEnv("AI_SEARCH_REQUEST_RETRIES", 8)
	if err != nil {
		return err
	}
	retryBase, err := durationFromMillisecondsEnv("AI_SEARCH_RETRY_BASE_MS", 2*time.Second)
	if err != nil {
		return err
	}
	retryMax, err := durationFromMillisecondsEnv("AI_SEARCH_RETRY_MAX_MS", 15*time.Second)
	if err != nil {
		return err
	}

	result, err := blogaisearch.Sync(
		context.Background(),
		&http.Client{},
		blogaisearch.SyncConfig{
			AccountID:          accountID,
			APIToken:           token,
			InstanceName:       instance,
			BlogOrigin:         blogOrigin,
			ContentRoot:        filepath.Join(a.root, "src", "content"),
			ChangeManifestPath: manifest,
			ForceFull:          forceFull,
			RequestTimeout:     requestTimeout,
			MaxRetries:         maxRetries,
			RetryBase:          retryBase,
			RetryMax:           retryMax,
		},
	)
	if err != nil {
		return fmt.Errorf("sync AI Search: %w", err)
	}
	payload, _ := json.Marshal(result)
	fmt.Fprintf(a.out, "[ai-search] sync %s\n", payload)

	if !verify {
		return nil
	}
	return a.runAISearchVerify(args)
}

func intFromEnv(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return value, nil
}

func durationFromMillisecondsEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative millisecond value", name)
	}
	return time.Duration(value) * time.Millisecond, nil
}

func aiSearchDuration(args []string, name string, fallback time.Duration) (time.Duration, error) {
	value, err := optionArg(args, name, "")
	if err != nil {
		return 0, err
	}
	if value == "" {
		return fallback, nil
	}
	if seconds, parseErr := strconv.Atoi(value); parseErr == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		return 0, fmt.Errorf("invalid %s value %q", name, value)
	}
	return duration, nil
}

func readAISearchStats(client *http.Client, accountID, token, instance string) (aiSearchStats, error) {
	url := "https://api.cloudflare.com/client/v4/accounts/" + accountID + "/ai-search/instances/" + instance + "/stats"
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return aiSearchStats{}, err
	}
	request.Header.Set("authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return aiSearchStats{}, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return aiSearchStats{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return aiSearchStats{}, fmt.Errorf("Cloudflare AI Search stats returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}

	var envelope cloudflareEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return aiSearchStats{}, err
	}
	if envelope.Success != nil && !*envelope.Success {
		messages := make([]string, 0, len(envelope.Errors))
		for _, entry := range envelope.Errors {
			if strings.TrimSpace(entry.Message) != "" {
				messages = append(messages, entry.Message)
			} else if entry.Code != nil {
				messages = append(messages, fmt.Sprint(entry.Code))
			}
		}
		return aiSearchStats{}, fmt.Errorf("Cloudflare AI Search stats failed: %s", strings.Join(messages, "; "))
	}

	statsPayload := payload
	if len(envelope.Result) > 0 && string(envelope.Result) != "null" {
		statsPayload = envelope.Result
	}
	var stats aiSearchStats
	if err := json.Unmarshal(statsPayload, &stats); err != nil {
		return aiSearchStats{}, err
	}
	return stats, nil
}

func (a app) runAISearchVerify(args []string) error {
	timeout, err := aiSearchDuration(args, "--timeout", 3*time.Minute)
	if err != nil {
		return err
	}
	interval, err := aiSearchDuration(args, "--interval", 10*time.Second)
	if err != nil {
		return err
	}
	accountID := strings.TrimSpace(os.Getenv("CLOUDFLARE_ACCOUNT_ID"))
	token := strings.TrimSpace(os.Getenv("CLOUDFLARE_AI_SEARCH_TOKEN"))
	instance := strings.TrimSpace(os.Getenv("CLOUDFLARE_AI_SEARCH_INSTANCE"))
	if instance == "" {
		instance = "thinkerqaq-blog"
	}
	if accountID == "" || token == "" {
		return errors.New("CLOUDFLARE_ACCOUNT_ID and CLOUDFLARE_AI_SEARCH_TOKEN are required")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	deadline := time.Now().Add(timeout)
	for {
		stats, err := readAISearchStats(client, accountID, token, instance)
		if err != nil {
			return err
		}
		if stats.Error > 0 {
			return fmt.Errorf("Cloudflare AI Search reports %d indexing error(s)", stats.Error)
		}
		indexing := stats.Queued+stats.Running+stats.Outdated > 0
		ready := !indexing && (stats.Completed > 0 || stats.Engine.R2.ObjectCount > 0 || stats.Engine.Vectorize.VectorsCount > 0)
		fmt.Fprintf(a.out, "[ai-search] queued=%d running=%d outdated=%d completed=%d errors=%d ready=%t\n",
			stats.Queued, stats.Running, stats.Outdated, stats.Completed, stats.Error, ready)
		if ready {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("AI Search did not become ready within %s", timeout)
		}
		time.Sleep(interval)
	}

	evalTimeout, err := aiSearchDuration(args, "--eval-timeout", 3*time.Minute)
	if err != nil {
		return err
	}
	evalInterval, err := aiSearchDuration(args, "--eval-interval", 10*time.Second)
	if err != nil {
		return err
	}
	return blogaisearch.Evaluate(
		context.Background(),
		client,
		blogaisearch.EvalConfig{
			AccountID:    accountID,
			APIToken:     token,
			InstanceName: instance,
			Timeout:      evalTimeout,
			RetryDelay:   evalInterval,
		},
		nil,
		func(diagnostic blogaisearch.EvalDiagnostic) {
			if payload, marshalErr := json.Marshal(diagnostic); marshalErr == nil {
				fmt.Fprintln(a.out, string(payload))
			}
		},
	)
}

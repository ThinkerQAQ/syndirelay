package assets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	blogcompiler "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/compiler"
)

var assetIDPattern = regexp.MustCompile(`^[a-f0-9]{16,64}$`)

// Render the same shared output at most once across concurrent platforms.
// The reference count keeps the lock table bounded across article uploads.
var renderLocks = struct {
	sync.Mutex
	entries map[string]*renderLock
}{entries: make(map[string]*renderLock)}

type renderLock struct {
	mutex sync.Mutex
	users int
}

func lockRenderOutput(path string) func() {
	renderLocks.Lock()
	entry := renderLocks.entries[path]
	if entry == nil {
		entry = &renderLock{}
		renderLocks.entries[path] = entry
	}
	entry.users++
	renderLocks.Unlock()
	entry.mutex.Lock()
	return func() {
		entry.mutex.Unlock()
		renderLocks.Lock()
		entry.users--
		if entry.users == 0 {
			delete(renderLocks.entries, path)
		}
		renderLocks.Unlock()
	}
}

// A small bound prevents ten platform tasks from launching ten simultaneous
// headless Chromium processes when a diagram is first published.
var rendererSlots = make(chan struct{}, 2)

type Config struct {
	EngineRoot       string
	DistributionRoot string
	Node             string
	Env              []string
	MermaidWidth     int
	MermaidScale     float64
}

type Stats struct {
	Assets   int
	Rendered int
	Cached   int
}

func Prepare(ctx context.Context, articles []blogcompiler.CompiledArticle, config Config) (Stats, error) {
	if strings.TrimSpace(config.EngineRoot) == "" {
		return Stats{}, errors.New("asset preparation requires engine root")
	}
	distributionRoot := strings.TrimSpace(config.DistributionRoot)
	if distributionRoot == "" {
		return Stats{}, errors.New("asset preparation requires distribution root")
	}

	unique := make(map[string]blogcompiler.Asset)
	order := make([]string, 0)
	for _, article := range articles {
		for _, asset := range article.Assets {
			key := asset.Kind + ":" + asset.ID
			if _, exists := unique[key]; exists {
				continue
			}
			unique[key] = asset
			order = append(order, key)
		}
	}

	// Independent diagram files can render concurrently; the global renderer
	// slots bound headless Chromium fan-out, while per-file locks deduplicate
	// the same diagram requested by different platform plans.
	type itemResult struct {
		cached bool
		err    error
	}
	results := make([]itemResult, len(order))
	var tasks sync.WaitGroup
	for index, key := range order {
		index, asset := index, unique[key]
		tasks.Add(1)
		go func() {
			defer tasks.Done()
			if err := validate(asset); err != nil {
				results[index].err = err
				return
			}
			output := filepath.Join(distributionRoot, "assets", asset.Kind, asset.ID+".png")
			results[index].cached, results[index].err = prepareOne(ctx, config, asset, output)
		}()
	}
	tasks.Wait()
	stats := Stats{Assets: len(order)}
	for index, item := range results {
		if item.err != nil {
			return stats, fmt.Errorf("publishing asset %s: %w", order[index], item.err)
		}
		if item.cached {
			stats.Cached++
		} else {
			stats.Rendered++
		}
	}
	return stats, nil
}

func prepareOne(ctx context.Context, config Config, asset blogcompiler.Asset, output string) (bool, error) {
	unlock := lockRenderOutput(output)
	defer unlock()

	// Recheck under the per-output lock, because another platform may have
	// rendered this file after the caller first requested it.
	if info, err := os.Stat(output); err == nil && !info.IsDir() && info.Size() > 0 {
		return true, nil
	}
	if strings.TrimSpace(asset.Content) == "" {
		return false, errors.New("renderer content is empty")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return false, err
	}
	node := strings.TrimSpace(config.Node)
	if node == "" {
		var err error
		node, err = exec.LookPath("node")
		if err != nil {
			return false, errors.New("node was not found for publishing renderer")
		}
	}
	renderer := filepath.Join(config.EngineRoot, "tools", "blogctl", "renderers", "node", "publishing-image.mjs")
	if info, err := os.Stat(renderer); err != nil || info.IsDir() {
		return false, fmt.Errorf("publishing renderer was not found: %s", renderer)
	}
	input, err := os.CreateTemp("", "blogctl-publishing-asset-*")
	if err != nil {
		return false, err
	}
	inputPath := input.Name()
	defer os.Remove(inputPath)
	if _, err := input.WriteString(asset.Content); err != nil {
		_ = input.Close()
		return false, err
	}
	if err := input.Close(); err != nil {
		return false, err
	}

	// The renderer needs a .png suffix. Render to a private file and atomically
	// publish it only after validating a nonempty result.
	temporary, err := os.CreateTemp(filepath.Dir(output), ".blogctl-render-*.png")
	if err != nil {
		return false, err
	}
	tempPath := temporary.Name()
	_ = temporary.Close()
	defer os.Remove(tempPath)

	args := []string{renderer, "--kind", asset.Kind, "--input", inputPath, "--output", tempPath}
	if strings.TrimSpace(asset.Renderer) != "" {
		args = append(args, "--renderer", asset.Renderer)
	}
	if asset.Kind == "mermaid" {
		width, scale := config.MermaidWidth, config.MermaidScale
		if width <= 0 {
			width = 1200
		}
		if scale <= 0 {
			scale = 2
		}
		args = append(args, "--width", fmt.Sprintf("%d", width), "--scale", fmt.Sprintf("%g", scale))
	}
	select {
	case rendererSlots <- struct{}{}:
		defer func() { <-rendererSlots }()
	case <-ctx.Done():
		return false, ctx.Err()
	}
	command := exec.CommandContext(ctx, node, args...)
	command.Dir = config.EngineRoot
	if len(config.Env) > 0 {
		command.Env = config.Env
	} else {
		command.Env = os.Environ()
	}
	combined, err := command.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("render publishing asset: %w: %s", err, strings.TrimSpace(string(combined)))
	}
	if info, err := os.Stat(tempPath); err != nil || info.IsDir() || info.Size() == 0 {
		return false, fmt.Errorf("renderer completed without publishing asset output: %s", output)
	}
	if err := os.Rename(tempPath, output); err != nil {
		// Windows won't rename over an existing file.
		if err := os.Remove(output); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		if err := os.Rename(tempPath, output); err != nil {
			return false, err
		}
	}
	return false, nil
}

func validate(asset blogcompiler.Asset) error {
	switch asset.Kind {
	case "mermaid", "plantuml", "ascii", "table":
	default:
		return fmt.Errorf("unsupported publishing asset kind: %s", asset.Kind)
	}
	if !assetIDPattern.MatchString(asset.ID) {
		return fmt.Errorf("invalid publishing asset id: %s", asset.ID)
	}
	return nil
}

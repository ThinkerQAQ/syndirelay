package assets

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	blogcompiler "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/compiler"
)

func TestPrepareConcurrentPlatformsRenderEachSharedAssetOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a local POSIX renderer fixture")
	}
	root := t.TempDir()
	renderer := filepath.Join(root, "tools", "blogctl", "renderers", "node", "publishing-image.mjs")
	if err := os.MkdirAll(filepath.Dir(renderer), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(renderer, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(root, "calls.log")
	executable := filepath.Join(root, "node-fixture")
	script := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then out="$2"; shift 2; else shift; fi
done
printf 'render\n' >> "` + calls + `"
sleep 0.1
printf 'PNG-content' > "$out"
`
	if err := os.WriteFile(executable, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	article := []blogcompiler.CompiledArticle{{Assets: []blogcompiler.Asset{
		{Kind: "mermaid", ID: strings.Repeat("a", 24), Content: "graph TD; A-->B"},
		{Kind: "mermaid", ID: strings.Repeat("b", 24), Content: "graph TD; C-->D"},
	}}}
	const platforms = 10
	results := make(chan Stats, platforms)
	errors := make(chan error, platforms)
	var wg sync.WaitGroup
	wg.Add(platforms)
	for i := 0; i < platforms; i++ {
		go func() {
			defer wg.Done()
			stats, err := Prepare(context.Background(), article, Config{EngineRoot: root, DistributionRoot: filepath.Join(root, "data"), Node: executable})
			if err != nil {
				errors <- err
				return
			}
			results <- stats
		}()
	}
	wg.Wait()
	close(errors)
	close(results)
	for err := range errors {
		t.Error(err)
	}
	rendered := 0
	for result := range results {
		if result.Rendered+result.Cached != 2 {
			t.Fatalf("unexpected stats %+v", result)
		}
		rendered += result.Rendered
	}
	if rendered != 2 {
		t.Fatalf("expected two total renders across ten platforms; actual=%d", rendered)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(b), "render") != 2 {
		t.Fatalf("renderer started %d times, want 2", strings.Count(string(b), "render"))
	}
	entries, err := os.ReadDir(filepath.Join(root, "data", "assets", "mermaid"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("cache contains %d entries, wanted two final PNGs", len(entries))
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".png") || strings.HasPrefix(entry.Name(), ".blogctl-render-") {
			t.Fatalf("temporary output leaked: %s", entry.Name())
		}
	}
}

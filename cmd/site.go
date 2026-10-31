package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	blogsearch "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/search"
	blogsite "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/site"
)

func (a app) runSite(args []string) error {
	if len(args) == 0 || (args[0] != "build" && args[0] != "assemble") {
		return errors.New("usage: blogctl site <build|assemble> --content-root <path>")
	}
	operation := args[0]
	contentRoot := ""
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--content-root":
			if index+1 >= len(args) {
				return errors.New("--content-root requires a value")
			}
			contentRoot = args[index+1]
			index++
		default:
			return fmt.Errorf("unknown site option %q", args[index])
		}
	}
	if operation == "assemble" && contentRoot == "" {
		return errors.New("site assemble requires --content-root <path>")
	}
	if contentRoot != "" {
		resolved, err := filepath.Abs(contentRoot)
		if err != nil {
			return err
		}
		if !directoryExists(resolved) {
			return fmt.Errorf("content root does not exist: %s", resolved)
		}
		result, err := blogsite.Assemble(a.root, resolved)
		if err != nil {
			return fmt.Errorf("assemble content: %w", err)
		}
		fmt.Fprintf(a.out, "[site] assembled content=%s media=%t manifest=%t\n", result.TargetContent, result.MediaCopied, result.ManifestCopied)
	}
	if operation == "assemble" {
		return nil
	}

	if _, _, err := a.prepareNode(true); err != nil {
		return err
	}
	if err := a.runNPM(false, "run", "build:astro"); err != nil {
		return fmt.Errorf("build Astro site: %w", err)
	}
	distRoot := filepath.Join(a.root, "dist")
	inventory, err := blogsearch.WriteGeneratedInventory(
		distRoot,
		blogsearch.DefaultTextSitemap,
		blogsearch.DefaultFingerprintManifest,
		time.Now(),
	)
	if err != nil {
		return fmt.Errorf("build search inventory: %w", err)
	}
	fmt.Fprintf(a.out, "[search] generated %d URLs\n", len(inventory.URLList))

	node, _, err := a.prepareNode(false)
	if err != nil {
		return err
	}
	pagefind := filepath.Join(a.root, "scripts", "build-pagefind.mjs")
	if !fileExists(pagefind) {
		return fmt.Errorf("Pagefind build script was not found: %s", pagefind)
	}
	if err := a.runner.Run(node, []string{pagefind}, os.Environ()); err != nil {
		return fmt.Errorf("build Pagefind search: %w", err)
	}
	return nil
}

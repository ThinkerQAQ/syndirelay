package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	contentRootEnvironment = "BLOG_CONTENT_ROOT"
	engineRootEnvironment  = "BLOGCTL_ENGINE_ROOT"
)

func resolveSyncWorkspace() (string, string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	return resolveSyncWorkspaceFrom(current, os.Getenv)
}

func resolveSyncWorkspaceFrom(current string, getenv func(string) string) (string, string, error) {
	current, err := cleanAbsolutePath(current)
	if err != nil {
		return "", "", err
	}

	engineAncestor := findAncestor(current, isEngineRoot)
	contentAncestor := findAncestor(current, isContentRoot)

	contentRoot, configured, err := configuredRoot(getenv(contentRootEnvironment), isContentRoot)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", contentRootEnvironment, err)
	}
	if !configured {
		contentRoot = contentAncestor
		if contentRoot == "" && engineAncestor != "" {
			candidate := filepath.Join(filepath.Dir(engineAncestor), "blog-content")
			if isContentRoot(candidate) {
				contentRoot = candidate
			}
		}
	}
	if contentRoot == "" {
		return "", "", fmt.Errorf("content repository not found; run blogctl sync from blog-content or set %s", contentRootEnvironment)
	}

	engineRoot, configured, err := configuredRoot(getenv(engineRootEnvironment), isEngineRoot)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", engineRootEnvironment, err)
	}
	if !configured {
		engineRoot = engineAncestor
		if engineRoot == "" {
			candidate := filepath.Join(filepath.Dir(contentRoot), "ThinkerQAQ.github.io")
			if isEngineRoot(candidate) {
				engineRoot = candidate
			}
		}
	}
	if engineRoot == "" {
		return "", "", fmt.Errorf("public engine repository not found; set %s to your ThinkerQAQ.github.io checkout", engineRootEnvironment)
	}

	return engineRoot, contentRoot, nil
}

func configuredRoot(value string, predicate func(string) bool) (string, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false, nil
	}
	root, err := cleanAbsolutePath(value)
	if err != nil {
		return "", true, err
	}
	if !predicate(root) {
		return "", true, fmt.Errorf("%q is not a valid repository root", root)
	}
	return root, true, nil
}

func findAncestor(start string, predicate func(string) bool) string {
	current := filepath.Clean(start)
	for {
		if predicate(current) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func isEngineRoot(root string) bool {
	return fileExists(filepath.Join(root, "package.json")) &&
		fileExists(filepath.Join(root, "astro.config.mjs")) &&
		directoryExists(filepath.Join(root, "tools", "blogctl"))
}

func isContentRoot(root string) bool {
	return directoryExists(filepath.Join(root, "src", "content", "articles")) &&
		fileExists(filepath.Join(root, "src", "data", "content-manifest.json"))
}

func withEnvironment(env []string, key, value string) []string {
	updated := append([]string(nil), env...)
	for index, entry := range updated {
		name, _, found := strings.Cut(entry, "=")
		if found && strings.EqualFold(name, key) {
			updated[index] = key + "=" + value
			return updated
		}
	}
	return append(updated, key+"="+value)
}

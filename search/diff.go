package search

import (
	"errors"
	"log/slog"
	"sort"
	"strings"
)

type Diff struct {
	Mode           string   `json:"mode"`
	Selected       []string `json:"selected,omitempty"`
	Added          []string `json:"added,omitempty"`
	Changed        []string `json:"changed,omitempty"`
	Deleted        []string `json:"deleted,omitempty"`
	Unchanged      []string `json:"unchanged,omitempty"`
	SelectedCount  int      `json:"selectedCount"`
	AddedCount     int      `json:"addedCount"`
	ChangedCount   int      `json:"changedCount"`
	DeletedCount   int      `json:"deletedCount"`
	UnchangedCount int      `json:"unchangedCount"`
}

func DiffInventories(previous, current Inventory, mode string, includeDeleted bool) (Diff, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "incremental"
	}
	if mode != "incremental" && mode != "full" {
		return Diff{}, errors.New("submission mode must be incremental or full")
	}
	origin, err := NormalizeOrigin(current.Origin)
	if err != nil {
		return Diff{}, err
	}
	// A snapshot from before a domain migration must never turn the old
	// domain's URLs into deletion notifications to the current provider.
	if previous.Origin != "" {
		previousOrigin, err := NormalizeOrigin(previous.Origin)
		if err != nil {
			return Diff{}, err
		}
		if previousOrigin != origin {
			slog.Info("search snapshot reset after origin change", "operation", "search-diff", "previousOrigin", previousOrigin, "origin", origin, "mode", mode)
			previous = Inventory{}
		}
	}
	previousURLs := make(map[string]struct{}, len(previous.URLs))
	currentURLs := make(map[string]struct{}, len(current.URLs))
	ignored := 0
	for _, item := range previous.URLs {
		normalized, err := NormalizeURL(item, origin, "snapshot URL")
		if err != nil {
			ignored++
			continue
		}
		previousURLs[normalized] = struct{}{}
	}
	if ignored > 0 {
		slog.Warn("search snapshot excluded URLs outside current origin", "operation", "search-diff", "origin", origin, "count", ignored)
	}
	for _, item := range current.URLs {
		normalized, err := NormalizeURL(item, origin, "inventory URL")
		if err != nil {
			return Diff{}, err
		}
		currentURLs[normalized] = struct{}{}
	}

	result := Diff{Mode: mode}
	for rawURL := range currentURLs {
		if _, ok := previousURLs[rawURL]; !ok {
			result.Added = append(result.Added, rawURL)
			continue
		}
		before := strings.TrimSpace(previous.Fingerprints[rawURL])
		after := strings.TrimSpace(current.Fingerprints[rawURL])
		if before == "" || after == "" || before != after {
			result.Changed = append(result.Changed, rawURL)
		} else {
			result.Unchanged = append(result.Unchanged, rawURL)
		}
	}
	for rawURL := range previousURLs {
		if _, ok := currentURLs[rawURL]; !ok {
			result.Deleted = append(result.Deleted, rawURL)
		}
	}
	for _, items := range [][]string{result.Added, result.Changed, result.Deleted, result.Unchanged} {
		sort.Strings(items)
	}
	selected := append([]string{}, current.URLs...)
	if mode == "incremental" {
		selected = append([]string{}, result.Added...)
		selected = append(selected, result.Changed...)
	}
	if includeDeleted {
		selected = append(selected, result.Deleted...)
	}
	seen := map[string]struct{}{}
	result.Selected = result.Selected[:0]
	for _, rawURL := range selected {
		if _, ok := seen[rawURL]; ok {
			continue
		}
		seen[rawURL] = struct{}{}
		result.Selected = append(result.Selected, rawURL)
	}
	sort.Strings(result.Selected)
	result.SelectedCount = len(result.Selected)
	result.AddedCount = len(result.Added)
	result.ChangedCount = len(result.Changed)
	result.DeletedCount = len(result.Deleted)
	result.UnchangedCount = len(result.Unchanged)
	return result, nil
}

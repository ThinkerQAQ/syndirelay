package bridge

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Fallback contains only confirmed local task records. It never asserts that
// the provider still holds these drafts; updates remain explicitly selected.
var safe51CTOSlug = regexp.MustCompile("^[A-Za-z0-9_/-]{1,200}$")
var safe51CTODraftID = regexp.MustCompile("^[0-9]{1,20}$")

func cached51CTODraftPosts(jobs []*durableTaskJob, contentRoot string) []remoteInventoryPost {
	root := filepath.Join(contentRoot, "src", "content", "articles")
	posts := []remoteInventoryPost{}
	seen := map[string]bool{}
	for _, job := range jobs {
		if job == nil || job.DryRun || !safe51CTOSlug.MatchString(job.Article) ||
			strings.Contains(job.Article, "..") || strings.HasPrefix(job.Article, "/") {
			continue
		}
		title := readFrontmatterScalar(filepath.Join(root, filepath.FromSlash(job.Article)+".md"), "title")
		if title == "" {
			continue
		}
		for _, evt := range job.Events {
			if evt.Platform != "51cto" || (evt.Result != "draft-created" && evt.Result != "draft-updated") {
				continue
			}
			id := strings.TrimSpace(evt.TargetID)
			if !safe51CTODraftID.MatchString(id) || seen[id] {
				continue
			}
			expected := "https://blog.51cto.com/blogger/draft/" + id
			if evt.URL != "" && evt.URL != expected {
				continue
			}
			seen[id] = true
			posts = append(posts, remoteInventoryPost{ID: id, Title: title, URL: expected})
		}
	}
	return posts
}

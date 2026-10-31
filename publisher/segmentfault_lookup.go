package publisher

import (
	"context"
	"errors"
	htmlstd "html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type SegmentFaultPost struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published bool   `json:"published"`
}

var (
	segmentFaultDraftLinkPattern = regexp.MustCompile(`(?is)<a\b[^>]*href=["'](?:https://segmentfault\.com)?/write\?[^"']*\bdraftId=([0-9]+)[^"']*["'][^>]*>(.*?)</a>`)
	segmentFaultPostLinkPattern  = regexp.MustCompile(`(?is)<a\b[^>]*href=["'](?:https://segmentfault\.com)?/a/([0-9]+)(?:[?#][^"']*)?["'][^>]*>(.*?)</a>`)
	segmentFaultHTMLTagPattern   = regexp.MustCompile(`(?s)<[^>]+>`)
	segmentFaultPagePattern      = regexp.MustCompile(`(?:\?|&)page=([0-9]+)`)
)

func normalizeSegmentFaultTitle(value string) string {
	value = htmlstd.UnescapeString(segmentFaultHTMLTagPattern.ReplaceAllString(value, " "))
	return strings.Join(strings.Fields(value), " ")
}

func SegmentFaultTitleMatches(local, remote string) bool {
	local = normalizeSegmentFaultTitle(local)
	remote = normalizeSegmentFaultTitle(remote)
	if local == "" || remote == "" {
		return false
	}
	if local == remote {
		return true
	}
	for _, separator := range []string{" · ", " - ", " — "} {
		if strings.HasPrefix(remote, local+separator) {
			return true
		}
	}
	return false
}

func parseSegmentFaultPosts(raw []byte, published bool) []SegmentFaultPost {
	pattern := segmentFaultDraftLinkPattern
	if published {
		pattern = segmentFaultPostLinkPattern
	}
	seen := map[string]struct{}{}
	posts := []SegmentFaultPost{}
	for _, match := range pattern.FindAllSubmatch(raw, -1) {
		if len(match) != 3 {
			continue
		}
		id := strings.TrimSpace(string(match[1]))
		title := normalizeSegmentFaultTitle(string(match[2]))
		if id == "" || title == "" {
			continue
		}
		key := map[bool]string{true: "published:", false: "draft:"}[published] + id
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		postURL := segmentFaultOrigin + "/write?draftId=" + url.QueryEscape(id)
		if published {
			postURL = segmentFaultOrigin + "/a/" + url.PathEscape(id)
		}
		posts = append(posts, SegmentFaultPost{
			ID: id, Title: title, URL: postURL, Published: published,
		})
	}
	return posts
}

func maxSegmentFaultPage(raw []byte) int {
	maxPage := 1
	for _, match := range segmentFaultPagePattern.FindAllSubmatch(raw, -1) {
		if len(match) != 2 {
			continue
		}
		value, err := strconv.Atoi(string(match[1]))
		if err == nil && value > maxPage {
			maxPage = value
		}
	}
	if maxPage > 10 {
		maxPage = 10
	}
	return maxPage
}

func (s *segmentFaultAdapter) listPage(ctx context.Context, path string, published bool) ([]SegmentFaultPost, error) {
	posts := []SegmentFaultPost{}
	seen := map[string]struct{}{}
	maxPage := 1
	for page := 1; page <= maxPage; page++ {
		rawURL := segmentFaultOrigin + path
		if page > 1 {
			separator := "?"
			if strings.Contains(rawURL, "?") {
				separator = "&"
			}
			rawURL += separator + "page=" + strconv.Itoa(page)
		}
		req, err := s.request(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		response, err := s.client.Do(req)
		if err != nil {
			return nil, platformError(ErrUpstream, s.ID(), "list-posts", 0, err.Error(), true)
		}
		raw, readErr := readBounded(response, 4<<20)
		response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, classifyHTTP(s.ID(), "list-posts", response.StatusCode, string(raw))
		}
		if page == 1 {
			maxPage = maxSegmentFaultPage(raw)
		}
		for _, post := range parseSegmentFaultPosts(raw, published) {
			key := map[bool]string{true: "published:", false: "draft:"}[published] + post.ID
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			posts = append(posts, post)
		}
	}
	return posts, nil
}

// SegmentFaultListPosts reads the signed-in user's draft list and published article list.
// SegmentFault does not expose a dedicated search endpoint, so callers should match locally.
func SegmentFaultListPosts(ctx context.Context, base *http.Client, session Session) (string, []SegmentFaultPost, error) {
	adapterValue, err := NewSegmentFaultAdapter(base, session)
	if err != nil {
		return "", nil, err
	}
	adapter := adapterValue.(*segmentFaultAdapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return "", nil, err
	}
	if !auth.Authenticated || strings.TrimSpace(auth.Username) == "" {
		return "", nil, errors.New("SegmentFault browser session is not authenticated")
	}

	drafts, err := adapter.listPage(ctx, "/user/draft", false)
	if err != nil {
		return "", nil, err
	}
	published, err := adapter.listPage(ctx, "/u/"+url.PathEscape(auth.Username)+"/articles", true)
	if err != nil {
		return "", nil, err
	}
	return auth.Username, append(drafts, published...), nil
}

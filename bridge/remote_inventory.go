package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

// Remote inventory is account-scoped rather than associated with any local
// article. Binding and mutation remain in the Update workflow.
type remoteInventoryPost struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published bool   `json:"published"`
}

func (s *Server) handleRemoteInventory(response http.ResponseWriter, request *http.Request) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	platform := strings.TrimSpace(request.URL.Query().Get("platform"))
	allowed := map[string]bool{"cnblogs": true, "juejin": true, "csdn": true, "segmentfault": true, "zhihu": true, "51cto": true, "oschina": true, "toutiao": true, "devto": true, "hashnode": true, "medium": true}
	if !allowed[platform] {
		writeAPIError(response, http.StatusBadRequest, "invalid_platform", "platform inventory is unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 40*time.Second)
	defer cancel()
	account := ""
	posts := make([]remoteInventoryPost, 0)
	partial := ""
	appendPost := func(id, title, url string, published bool) {
		if strings.TrimSpace(id) == "" {
			return
		}
		posts = append(posts, remoteInventoryPost{ID: id, Title: title, URL: url, Published: published})
	}
	if platform == "devto" {
		s.mu.Lock()
		apiKey := devtoAPIKey(s.config)
		client := s.httpClient
		s.mu.Unlock()
		if apiKey == "" {
			writeAPIError(response, http.StatusBadRequest, "api_key_required", "DEV.to API Key is not configured", nil)
			return
		}
		for page := 1; page <= 5; page++ {
			url := fmt.Sprintf("https://dev.to/api/articles/me/all?page=%d&per_page=100", page)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				writeAPIError(response, http.StatusInternalServerError, "request_failed", err.Error(), nil)
				return
			}
			req.Header.Set("api-key", apiKey)
			req.Header.Set("accept", "application/vnd.forem.api-v1+json")
			result, err := client.Do(req)
			if err != nil {
				writeAPIError(response, http.StatusBadGateway, "lookup_failed", "DEV.to inventory request failed", nil)
				return
			}
			var batch []devtoArticleCandidate
			decErr := json.NewDecoder(http.MaxBytesReader(response, result.Body, 4<<20)).Decode(&batch)
			_ = result.Body.Close()
			if result.StatusCode != http.StatusOK || decErr != nil {
				writeAPIError(response, http.StatusBadGateway, "lookup_failed", "DEV.to inventory response invalid", nil)
				return
			}
			for _, item := range batch {
				appendPost(fmt.Sprint(item.ID), item.Title, item.URL, devtoCandidatePublished(item))
			}
			if len(batch) < 100 {
				break
			}
			if page == 5 {
				partial = "DEV.to 最多读取最近 500 篇"
			}
		}
	} else {
		session, client, err := (bridgeNativePublisher{server: s}).publisherSession(platform)
		if err != nil {
			writeAPIError(response, http.StatusBadRequest, "session_required", err.Error(), nil)
			return
		}
		switch platform {
		case "toutiao":
			var items []publisher.ToutiaoPost
			account, items, err = publisher.ToutiaoListPosts(ctx, client, session)
			for _, item := range items {
				appendPost(item.ID, item.Title, item.URL, item.Published)
			}
		case "medium":
			var items []mediumPost
			account, items, err = (mediumClient{httpClient: client}).listPosts(ctx, mediumPlatformSession(session))
			for _, item := range items {
				appendPost(item.ID, item.Title, item.URL, item.Published)
			}
		case "hashnode":
			var items []publisher.HashnodePost
			var incomplete bool
			account, items, incomplete, err = publisher.HashnodeListPosts(ctx, client, session)
			for _, item := range items {
				appendPost(item.ID, item.Title, item.URL, item.Published)
			}
			if incomplete {
				partial = "Hashnode 列表存在更多文章；当前查询结果不完整"
			}
		case "cnblogs":
			var items []publisher.CNBlogsPost
			account, items, err = publisher.CNBlogsSearchPosts(ctx, client, session, "")
			for _, item := range items {
				appendPost(item.ID, item.Title, item.URL, item.Published)
			}
			if len(items) >= 50 {
				partial = "博客园当前最多读取 50 篇"
			}
		case "juejin":
			var items []publisher.JuejinPost
			var truncated bool
			account, items, truncated, err = publisher.JuejinListAllPosts(ctx, client, session)
			for _, item := range items {
				appendPost(item.ID, item.Title, item.URL, item.Published)
			}
			if truncated {
				partial = "掘金草稿最多读取最近 500 篇；可在「更新」页按 ID 定位更早草稿"
			}
		case "csdn":
			var items []publisher.CSDNPost
			account, items, err = publisher.CSDNListPosts(ctx, client, session)
			for _, item := range items {
				appendPost(item.ID, item.Title, item.URL, item.Published)
			}
		case "segmentfault":
			var items []publisher.SegmentFaultPost
			account, items, err = publisher.SegmentFaultListPosts(ctx, client, session)
			for _, item := range items {
				appendPost(item.ID, item.Title, item.URL, item.Published)
			}
		case "zhihu":
			var items []publisher.ZhihuPost
			account, items, err = publisher.ZhihuListPosts(ctx, client, session)
			for _, item := range items {
				appendPost(item.ID, item.Title, item.URL, item.Published)
			}
		case "51cto":
			var items []publisher.Cto51Post
			account, items, err = publisher.Cto51ListPosts(ctx, client, session)
			for _, item := range items {
				appendPost(item.ID, item.Title, item.URL, item.Published)
			}
		case "oschina":
			var items []publisher.OSChinaPost
			account, items, err = publisher.OSChinaListPosts(ctx, client, session)
			for _, item := range items {
				appendPost(item.ID, item.Title, item.URL, item.Published)
			}
		}
		if err != nil {
			writeAPIError(response, http.StatusBadGateway, "lookup_failed", err.Error(), nil)
			return
		}
	}
	writeJSON(response, http.StatusOK, map[string]any{"platform": platform, "account": account, "items": posts, "partial": partial})
}

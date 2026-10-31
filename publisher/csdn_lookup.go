package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const csdnBlogOrigin = "https://blog.csdn.net"

type CSDNPost struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published bool   `json:"published"`
}

func CSDNTitleMatches(local, remote string) bool {
	local = strings.Join(strings.Fields(strings.TrimSpace(local)), " ")
	remote = strings.Join(strings.Fields(strings.TrimSpace(remote)), " ")
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

func CSDNArticleID(reference string) string {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return ""
	}
	if _, err := strconv.ParseInt(reference, 10, 64); err == nil {
		return reference
	}
	parsed, err := url.Parse(reference)
	if err != nil {
		return ""
	}
	if id := strings.TrimSpace(parsed.Query().Get("articleId")); id != "" {
		if _, err := strconv.ParseInt(id, 10, 64); err == nil {
			return id
		}
	}
	base := path.Base(strings.TrimRight(parsed.Path, "/"))
	if _, err := strconv.ParseInt(base, 10, 64); err != nil {
		return ""
	}
	return base
}

func (c *csdnAdapter) listPublished(ctx context.Context) ([]CSDNPost, error) {
	if strings.TrimSpace(c.userID) == "" {
		auth, err := c.CheckAuth(ctx)
		if err != nil {
			return nil, err
		}
		if !auth.Authenticated || strings.TrimSpace(c.userID) == "" {
			return nil, errors.New("CSDN browser session is not authenticated")
		}
	}

	const pageSize = 100
	result := []CSDNPost{}
	seen := map[string]struct{}{}
	for pageNumber := 1; pageNumber <= 10; pageNumber++ {
		query := url.Values{}
		query.Set("page", strconv.Itoa(pageNumber))
		query.Set("size", strconv.Itoa(pageSize))
		query.Set("businessType", "lately")
		query.Set("noMore", "false")
		query.Set("username", c.userID)
		rawURL := csdnBlogOrigin + "/community/home-api/v1/get-business-list?" + query.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("accept", "application/json")
		if c.userAgent != "" {
			req.Header.Set("user-agent", c.userAgent)
		}
		response, err := c.client.Do(req)
		if err != nil {
			return nil, platformError(ErrUpstream, c.ID(), "list-published", 0, err.Error(), true)
		}
		raw, readErr := readBounded(response, 4<<20)
		response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, classifyHTTP(c.ID(), "list-published", response.StatusCode, string(raw))
		}
		var decoded struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				List []struct {
					Title string `json:"title"`
					URL   string `json:"url"`
				} `json:"list"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, platformError(ErrUpstream, c.ID(), "list-published", response.StatusCode, "invalid JSON response", false)
		}
		if decoded.Code != 200 {
			return nil, platformError(ErrUpstream, c.ID(), "list-published", decoded.Code, responseMessage(decoded.Message), false)
		}
		for _, item := range decoded.Data.List {
			id := CSDNArticleID(item.URL)
			title := strings.TrimSpace(item.Title)
			if id == "" || title == "" {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, CSDNPost{
				ID: id, Title: title, URL: strings.TrimSpace(item.URL), Published: true,
			})
		}
		if len(decoded.Data.List) < pageSize {
			break
		}
	}
	return result, nil
}

func (c *csdnAdapter) listConsolePosts(ctx context.Context, listStatus string, published bool) ([]CSDNPost, error) {
	started := time.Now()
	const pageSize = 20
	result := []CSDNPost{}
	seen := map[string]struct{}{}
	for pageNumber := 1; pageNumber <= 100; pageNumber++ {
		query := url.Values{}
		query.Set("page", strconv.Itoa(pageNumber))
		query.Set("pageSize", strconv.Itoa(pageSize))
		query.Set("status", listStatus)
		apiPath := "/blog/phoenix/console/v1/article/list?" + query.Encode()
		pageStarted := time.Now()
		req, err := c.apiRequest(ctx, http.MethodGet, apiPath, nil)
		if err != nil {
			return nil, err
		}
		response, err := c.client.Do(req)
		if err != nil {
			slog.Warn("csdn article list request failed", "operation", "article-list", "listStatus", listStatus, "page", pageNumber, "durationMs", time.Since(pageStarted).Milliseconds(), "errorType", "http-request")
			return nil, platformError(ErrUpstream, c.ID(), "list-"+listStatus, 0, err.Error(), true)
		}
		raw, readErr := readBounded(response, 4<<20)
		response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			slog.Warn("csdn article list rejected", "operation", "article-list", "listStatus", listStatus, "page", pageNumber, "status", response.StatusCode, "durationMs", time.Since(pageStarted).Milliseconds())
			return nil, classifyHTTP(c.ID(), "list-"+listStatus, response.StatusCode, string(raw))
		}
		var decoded struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				List []struct {
					ArticleID any    `json:"articleId"`
					Title     string `json:"title"`
				} `json:"list"`
				Page  int `json:"page"`
				Size  int `json:"size"`
				Total int `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, platformError(ErrUpstream, c.ID(), "list-"+listStatus, response.StatusCode, "invalid JSON response", false)
		}
		if decoded.Code != 200 {
			return nil, platformError(ErrUpstream, c.ID(), "list-"+listStatus, decoded.Code, responseMessage(decoded.Message), false)
		}
		for _, item := range decoded.Data.List {
			id := valueString(item.ArticleID)
			title := strings.TrimSpace(item.Title)
			if id == "" || title == "" {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			target := csdnOrigin + "/md?articleId=" + url.QueryEscape(id)
			if published {
				target = csdnBlogOrigin + "/" + url.PathEscape(c.userID) + "/article/details/" + url.PathEscape(id)
			}
			result = append(result, CSDNPost{ID: id, Title: title, URL: target, Published: published})
		}
		slog.Info("csdn article list page completed", "operation", "article-list", "listStatus", listStatus, "page", pageNumber, "resultCount", len(decoded.Data.List), "total", decoded.Data.Total, "durationMs", time.Since(pageStarted).Milliseconds())
		effectiveSize := decoded.Data.Size
		if effectiveSize <= 0 {
			effectiveSize = pageSize
		}
		if len(decoded.Data.List) == 0 || len(decoded.Data.List) < effectiveSize || (decoded.Data.Total > 0 && pageNumber*effectiveSize >= decoded.Data.Total) {
			break
		}
	}
	slog.Info("csdn article list completed", "operation", "article-list", "listStatus", listStatus, "resultCount", len(result), "durationMs", time.Since(started).Milliseconds())
	return result, nil
}

func (c *csdnAdapter) fetchPost(ctx context.Context, postID string) (CSDNPost, error) {
	postID = strings.TrimSpace(postID)
	if postID == "" {
		return CSDNPost{}, platformError(ErrValidation, c.ID(), "get-article", 0, "article id is required", false)
	}
	query := url.Values{}
	query.Set("id", postID)
	query.Set("model_type", "")
	path := "/blog-console-api/v3/editor/getArticle?" + query.Encode()
	req, err := c.apiRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return CSDNPost{}, err
	}
	var decoded struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ArticleID any    `json:"article_id"`
			Title     string `json:"title"`
			Status    int    `json:"status"`
		} `json:"data"`
	}
	if err := doJSON(c.client, req, c.ID(), "get-article", &decoded); err != nil {
		return CSDNPost{}, err
	}
	if decoded.Code != 200 {
		return CSDNPost{}, platformError(ErrUpstream, c.ID(), "get-article", decoded.Code, responseMessage(decoded.Message), false)
	}
	id := valueString(decoded.Data.ArticleID)
	if id == "" {
		id = postID
	}
	title := strings.TrimSpace(decoded.Data.Title)
	if title == "" {
		return CSDNPost{}, platformError(ErrUpstream, c.ID(), "get-article", decoded.Code, "article title is missing", false)
	}
	published := decoded.Data.Status == 0 || decoded.Data.Status == 1
	target := csdnOrigin + "/md?articleId=" + url.QueryEscape(id)
	if published {
		if strings.TrimSpace(c.userID) == "" {
			_, _ = c.CheckAuth(ctx)
		}
		if c.userID != "" {
			target = csdnBlogOrigin + "/" + url.PathEscape(c.userID) + "/article/details/" + url.PathEscape(id)
		}
	}
	return CSDNPost{ID: id, Title: title, URL: target, Published: published}, nil
}

func CSDNAccount(ctx context.Context, base *http.Client, session Session) (string, error) {
	adapterValue, err := NewCSDNAdapter(base, session)
	if err != nil {
		return "", err
	}
	adapter := adapterValue.(*csdnAdapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return "", err
	}
	if !auth.Authenticated || strings.TrimSpace(adapter.userID) == "" {
		return "", errors.New("CSDN browser session is not authenticated")
	}
	return adapter.userID, nil
}

// CSDNListPosts reads the signed-in user's draft and published article lists.
func CSDNListPosts(ctx context.Context, base *http.Client, session Session) (string, []CSDNPost, error) {
	adapterValue, err := NewCSDNAdapter(base, session)
	if err != nil {
		return "", nil, err
	}
	adapter := adapterValue.(*csdnAdapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return "", nil, err
	}
	if !auth.Authenticated || strings.TrimSpace(adapter.userID) == "" {
		return "", nil, errors.New("CSDN browser session is not authenticated")
	}
	drafts, err := adapter.listConsolePosts(ctx, "draft", false)
	if err != nil {
		return "", nil, err
	}
	published, err := adapter.listConsolePosts(ctx, "all_v3", true)
	if err != nil {
		slog.Warn("csdn console published list failed; using public fallback", "operation", "article-list-fallback", "errorType", "console-list")
		published, err = adapter.listPublished(ctx)
		if err != nil {
			return "", nil, err
		}
	}
	posts := make([]CSDNPost, 0, len(drafts)+len(published))
	posts = append(posts, drafts...)
	for _, post := range published {
		posts = appendCSDNPost(posts, post)
	}
	return adapter.userID, posts, nil
}

func appendCSDNPost(posts []CSDNPost, candidate CSDNPost) []CSDNPost {
	for index := range posts {
		if posts[index].ID == candidate.ID {
			posts[index] = candidate
			return posts
		}
	}
	return append(posts, candidate)
}

func CSDNLookupPost(ctx context.Context, base *http.Client, session Session, postID string) (string, CSDNPost, error) {
	adapterValue, err := NewCSDNAdapter(base, session)
	if err != nil {
		return "", CSDNPost{}, err
	}
	adapter := adapterValue.(*csdnAdapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return "", CSDNPost{}, err
	}
	if !auth.Authenticated || strings.TrimSpace(adapter.userID) == "" {
		return "", CSDNPost{}, errors.New("CSDN browser session is not authenticated")
	}
	post, err := adapter.fetchPost(ctx, postID)
	return adapter.userID, post, err
}

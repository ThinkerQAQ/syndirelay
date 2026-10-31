package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Cto51Post is a remotely discovered 51CTO draft or published article.
type Cto51Post struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published bool   `json:"published"`
}

// Cto51TitleMatches compares a local title against a 51CTO remote title. 51CTO
// appends the author name after " · " on some articles, which the shared CSDN
// matcher already tolerates.
func Cto51TitleMatches(local, remote string) bool {
	return CSDNTitleMatches(local, remote)
}

type cto51ListItem struct {
	BlogID  any    `json:"blog_id"`
	Title   string `json:"title"`
	BlogURL string `json:"blog_url"`
	EditURL string `json:"edit_url"`
}

type cto51ListPage struct {
	Status int    `json:"status"`
	Msg    string `json:"msg"`
	Data   struct {
		Pagination string          `json:"pagination"`
		List       []cto51ListItem `json:"list"`
	} `json:"data"`
}

func cto51PostsFromList(items []cto51ListItem, published bool, username string) []Cto51Post {
	result := make([]Cto51Post, 0, len(items))
	for _, item := range items {
		id := valueString(item.BlogID)
		title := strings.TrimSpace(item.Title)
		if id == "" || id == "0" || title == "" {
			continue
		}

		target := strings.TrimSpace(item.EditURL)
		if published {
			target = strings.TrimSpace(item.BlogURL)
			if target == "" && strings.TrimSpace(username) != "" {
				target = cto51Origin + "/" + url.PathEscape(strings.TrimSpace(username)) + "/" + url.PathEscape(id)
			}
		} else if target == "" {
			target = cto51Origin + "/blogger/draft/" + url.PathEscape(id)
		}

		result = append(result, Cto51Post{
			ID: id, Title: title, URL: target, Published: published,
		})
	}
	return result
}

func (c *cto51Adapter) listPosts(ctx context.Context, listType int, published bool, operation string) ([]Cto51Post, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, err
	}

	const maxPages = 50
	result := []Cto51Post{}
	seen := map[string]struct{}{}
	for page := 1; page <= maxPages; page++ {
		payload := map[string]any{
			"type": listType, "blog_type": 0, "year": "", "month": "", "ccid": "",
			"flag": false, "level": "",
		}
		if page > 1 {
			payload["page"] = page
		}
		body, _ := json.Marshal(payload)
		values := url.Values{}
		values.Set("data", string(body))
		req, err := c.request(ctx, http.MethodPost, cto51Origin+"/creative-center-ajax/list", strings.NewReader(values.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("content-type", "application/x-www-form-urlencoded; charset=UTF-8")
		req.Header.Set("accept", "*/*")
		req.Header.Set("x-requested-with", "XMLHttpRequest")

		var decoded cto51ListPage
		if err := doJSON(c.client, req, c.ID(), operation, &decoded); err != nil {
			return nil, err
		}
		if decoded.Status != 1 {
			return nil, platformError(ErrUpstream, c.ID(), operation, 0, responseMessage(decoded.Msg), false)
		}

		added := 0
		for _, post := range cto51PostsFromList(decoded.Data.List, published, c.username) {
			key := "draft:" + post.ID
			if published {
				key = "published:" + post.ID
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, post)
			added++
		}

		if len(decoded.Data.List) == 0 || added == 0 || strings.TrimSpace(decoded.Data.Pagination) == "" {
			break
		}
	}
	return result, nil
}

func (c *cto51Adapter) listDrafts(ctx context.Context) ([]Cto51Post, error) {
	// 51CTO's draft page initializes the same list endpoint with type=4.
	return c.listPosts(ctx, 4, false, "list-drafts")
}

func (c *cto51Adapter) listPublished(ctx context.Context) ([]Cto51Post, error) {
	// The captured article-manager page declares attr_type=2 as "公开" and
	// submits it to /creative-center-ajax/list. These are public articles.
	return c.listPosts(ctx, 2, true, "list-published")
}

func newAuthenticated51CTOAdapter(ctx context.Context, base *http.Client, session Session) (*cto51Adapter, error) {
	adapterValue, err := New51CTOAdapter(base, session)
	if err != nil {
		return nil, err
	}
	adapter := adapterValue.(*cto51Adapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return nil, err
	}
	if !auth.Authenticated || strings.TrimSpace(adapter.username) == "" {
		return nil, errors.New("51CTO browser session is not authenticated")
	}
	return adapter, nil
}

// Cto51ListPosts reads the signed-in user's 51CTO drafts and public articles.
// The bridge performs final title matching locally.
func Cto51ListPosts(ctx context.Context, base *http.Client, session Session) (string, []Cto51Post, error) {
	adapter, err := newAuthenticated51CTOAdapter(ctx, base, session)
	if err != nil {
		return "", nil, err
	}
	drafts, err := adapter.listDrafts(ctx)
	if err != nil {
		return "", nil, err
	}
	published, err := adapter.listPublished(ctx)
	if err != nil {
		return "", nil, err
	}
	posts := make([]Cto51Post, 0, len(drafts)+len(published))
	posts = append(posts, drafts...)
	posts = append(posts, published...)
	return adapter.username, posts, nil
}

// Cto51ListDrafts is retained for callers that intentionally need draft-only
// discovery.
func Cto51ListDrafts(ctx context.Context, base *http.Client, session Session) (string, []Cto51Post, error) {
	adapter, err := newAuthenticated51CTOAdapter(ctx, base, session)
	if err != nil {
		return "", nil, err
	}
	posts, err := adapter.listDrafts(ctx)
	if err != nil {
		return "", nil, err
	}
	return adapter.username, posts, nil
}

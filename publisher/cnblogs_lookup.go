package publisher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type CNBlogsPost struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published bool   `json:"published"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

var cnBlogsNumericID = regexp.MustCompile(`^[0-9]+$`)

// CNBlogsDraftEditorURL is the creator editor route, not the article preview.
func CNBlogsDraftEditorURL(id string) string {
	return cnBlogsOrigin + "/posts/edit;postId=" + url.QueryEscape(strings.TrimSpace(id))
}

func cnBlogsDraftURL(id, stored string) string {
	if cnBlogsNumericID.MatchString(strings.TrimSpace(id)) {
		return CNBlogsDraftEditorURL(id)
	}
	return stored
}

func ParseCNBlogsPostReference(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if cnBlogsNumericID.MatchString(reference) {
		return reference, nil
	}
	parsed, err := url.Parse(reference)
	if err != nil || parsed.Scheme != "https" || parsed.Port() != "" {
		return "", errors.New("CNBlogs reference must be a numeric ID or HTTPS article URL")
	}
	var id string
	switch strings.ToLower(parsed.Hostname()) {
	case "www.cnblogs.com":
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(segments) == 3 && segments[1] == "p" {
			id = strings.TrimSuffix(segments[2], ".html")
		}
	case "i.cnblogs.com":
		for _, prefix := range []string{"/posts/edit;postId=", "/articles/edit;postId="} {
			if strings.HasPrefix(parsed.Path, prefix) {
				id = strings.TrimPrefix(parsed.Path, prefix)
				break
			}
		}
	}
	if !cnBlogsNumericID.MatchString(id) {
		return "", errors.New("CNBlogs article URL does not contain a valid post ID")
	}
	return id, nil
}

func cnBlogsPostFromMap(value map[string]any) CNBlogsPost {
	post := CNBlogsPost{
		ID: valueString(value["id"]), Title: valueString(value["title"]),
		URL: valueString(value["url"]), UpdatedAt: valueString(value["dateUpdated"]),
	}
	post.Published, _ = value["isPublished"].(bool)
	if !post.Published {
		post.URL = cnBlogsDraftURL(post.ID, post.URL)
	}
	return post
}

// CNBlogsSearchPosts reads a bounded set of the signed-in account's editor list.
func CNBlogsSearchPosts(ctx context.Context, base *http.Client, session Session, search string) (string, []CNBlogsPost, error) {
	adapterValue, err := NewCNBlogsAdapter(base, session)
	if err != nil {
		return "", nil, err
	}
	adapter := adapterValue.(*cnBlogsAdapter)
	if _, err := adapter.CheckAuth(ctx); err != nil {
		return "", nil, err
	}
	posts := []CNBlogsPost{}
	for page := 0; page < 5; page++ {
		values := url.Values{"p": {fmt.Sprint(page)}, "cid": {""}, "t": {"1"}, "cfg": {"0"}, "search": {search}, "orderBy": {""}, "s": {""}, "scid": {""}}
		req, err := adapter.request(ctx, http.MethodGet, cnBlogsOrigin+"/api/posts/list?"+values.Encode(), nil)
		if err != nil {
			return "", nil, err
		}
		var decoded struct {
			PostList   []map[string]any `json:"postList"`
			PostsCount int              `json:"postsCount"`
		}
		if err := doJSON(adapter.client, req, adapter.ID(), "list-posts", &decoded); err != nil {
			return "", nil, err
		}
		for _, value := range decoded.PostList {
			post := cnBlogsPostFromMap(value)
			if post.ID != "" {
				posts = append(posts, post)
			}
		}
		if len(decoded.PostList) == 0 || len(posts) >= decoded.PostsCount || len(posts) >= 50 {
			break
		}
	}
	return adapter.username, posts, nil
}

// CNBlogsGetPost verifies that the current editor account can read the post.
func CNBlogsGetPost(ctx context.Context, base *http.Client, session Session, id string) (string, CNBlogsPost, error) {
	if !cnBlogsNumericID.MatchString(id) {
		return "", CNBlogsPost{}, errors.New("invalid CNBlogs post ID")
	}
	adapterValue, err := NewCNBlogsAdapter(base, session)
	if err != nil {
		return "", CNBlogsPost{}, err
	}
	adapter := adapterValue.(*cnBlogsAdapter)
	if _, err := adapter.CheckAuth(ctx); err != nil {
		return "", CNBlogsPost{}, err
	}
	value, err := adapter.fetchPost(ctx, id)
	if err != nil {
		return "", CNBlogsPost{}, err
	}
	if author := valueString(value["author"]); author != "" && !strings.EqualFold(author, adapter.username) {
		return "", CNBlogsPost{}, errors.New("CNBlogs post belongs to a different account")
	}
	post := cnBlogsPostFromMap(value)
	if post.ID != id {
		return "", CNBlogsPost{}, errors.New("CNBlogs returned a different post ID")
	}
	return adapter.username, post, nil
}

package publisher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// HashnodePost is a read-only remote draft or published article. IDs are
// dashboard IDs, not the public cuid or a BlogCTL binding ID.
type HashnodePost struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published bool   `json:"published"`
}

// HashnodeListPosts reads the publication's editor tabs using the logged-in
// browser session. Hashnode's internal REST endpoints are undocumented.
// Incomplete lists are flagged; callers must never interpret an incomplete
// list as proof that an article does not already exist.
func HashnodeListPosts(ctx context.Context, base *http.Client, session Session) (string, []HashnodePost, bool, error) {
	a, err := NewHashnodeAdapter(base, session)
	if err != nil {
		return "", nil, false, err
	}
	h := a.(*hashnodeAdapter)
	return h.listPosts(ctx, "")
}

// HashnodeSearchPosts asks the editor to search both tabs, then returns
// candidate titles to be verified locally against the selected English post.
func HashnodeSearchPosts(ctx context.Context, base *http.Client, session Session, term string) (string, []HashnodePost, bool, error) {
	a, err := NewHashnodeAdapter(base, session)
	if err != nil {
		return "", nil, false, err
	}
	return a.(*hashnodeAdapter).listPosts(ctx, strings.TrimSpace(term))
}

func (h *hashnodeAdapter) listPosts(ctx context.Context, search string) (string, []HashnodePost, bool, error) {
	if auth, err := h.CheckAuth(ctx); err != nil {
		return "", nil, false, err
	} else if !auth.Authenticated {
		return "", nil, false, errors.New("Hashnode browser session is not authenticated")
	}
	username := h.username
	if username == "" {
		return "", nil, false, errors.New("Hashnode publication username is missing")
	}
	posts := []HashnodePost{}
	truncated := false
	seen := map[string]bool{}
	for _, tab := range []string{"drafts", "published"} {
		// The editor uses `page` for published entries, and `nextCursor` for
		// drafts. Only the initial draft page shape is captured, so stop and
		// flag truncation rather than guessing an undocumented cursor shape.
		for page := 0; page < 10; page++ {
			if tab == "drafts" && page > 0 {
				break
			}
			params := url.Values{"tab": {tab}, "publicationId": {h.publicationID}, "limit": {"50"}}
			if tab == "published" || search != "" {
				params.Set("page", fmt.Sprint(page))
			}
			if search != "" {
				params.Set("search", search)
			}
			var response struct {
				Success bool `json:"success"`
				Drafts  []struct {
					ID    string `json:"_id"`
					Title string `json:"title"`
				} `json:"drafts"`
				Published []struct {
					ID    string `json:"_id"`
					Title string `json:"title"`
					Slug  string `json:"slug"`
				} `json:"published"`
				HasMore bool `json:"hasMore"`
			}
			if err := h.request(ctx, http.MethodGet, "/api/drafts?"+params.Encode(), nil, &response, "list-"+tab); err != nil {
				return "", nil, false, err
			}
			if !response.Success {
				return "", nil, false, fmt.Errorf("Hashnode %s list was not successful", tab)
			}
			if tab == "drafts" {
				for _, item := range response.Drafts {
					if item.ID == "" || seen["draft:"+item.ID] {
						continue
					}
					seen["draft:"+item.ID] = true
					posts = append(posts, HashnodePost{ID: item.ID, Title: item.Title, URL: hashnodeOrigin + "/draft/" + url.PathEscape(item.ID)})
				}
				truncated = truncated || response.HasMore
				break
			}
			for _, item := range response.Published {
				if item.ID == "" || seen["published:"+item.ID] {
					continue
				}
				seen["published:"+item.ID] = true
				publicURL := ""
				if item.Slug != "" {
					publicURL = "https://" + username + ".hashnode.dev/" + url.PathEscape(item.Slug)
				}
				posts = append(posts, HashnodePost{ID: item.ID, Title: item.Title, URL: publicURL, Published: true})
			}
			if !response.HasMore {
				break
			}
			if page == 9 {
				truncated = true
			}
		}
	}
	return username, posts, truncated, nil
}

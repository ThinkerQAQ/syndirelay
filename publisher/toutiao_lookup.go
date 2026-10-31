package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ToutiaoPost is an article owned by the currently authenticated creator.
// Search feed results are intentionally excluded: they may belong to other authors.
type ToutiaoPost struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Published  bool   `json:"published"`
	FeedCovers string `json:"-"`
	ModifiedAt string `json:"-"`
}

func ToutiaoTitleMatches(local, remote string) bool {
	return CSDNTitleMatches(local, remote) || CSDNTitleMatches(truncateToutiaoTitle(local), remote)
}

func (t *toutiaoAdapter) listDrafts(ctx context.Context) ([]ToutiaoPost, error) {
	query := url.Values{
		"type": {"0"}, "count": {"100"}, "app_id": {"1231"},
	}
	req, err := t.request(ctx, http.MethodGet, toutiaoOrigin+"/mp/agw/creator_center/draft_list?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var decoded struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
		Drafts  []struct {
			GID          any    `json:"gid"`
			Title        string `json:"title"`
			GraphicExtra string `json:"graphic_extra"`
		} `json:"draft_list"`
	}
	if err := doJSON(t.client, req, t.ID(), "list-drafts", &decoded); err != nil {
		return nil, err
	}
	if decoded.Code == nil || *decoded.Code != 0 {
		return nil, platformError(ErrUpstream, t.ID(), "list-drafts", 0, responseMessage(decoded.Message), false)
	}
	posts := make([]ToutiaoPost, 0, len(decoded.Drafts))
	for _, draft := range decoded.Drafts {
		id := valueString(draft.GID)
		title := strings.TrimSpace(draft.Title)
		if id == "" || id == "0" || title == "" {
			continue
		}
		covers := "[]"
		if strings.TrimSpace(draft.GraphicExtra) != "" {
			var extra struct {
				FeedCovers string `json:"pgc_feed_covers"`
			}
			if err := json.Unmarshal([]byte(draft.GraphicExtra), &extra); err != nil {
				return nil, platformError(ErrUpstream, t.ID(), "list-drafts", 0, "invalid draft metadata", false)
			}
			if extra.FeedCovers != "" {
				var parsed []json.RawMessage
				if err := json.Unmarshal([]byte(extra.FeedCovers), &parsed); err != nil {
					return nil, platformError(ErrUpstream, t.ID(), "list-drafts", 0, "invalid draft cover metadata", false)
				}
				covers = extra.FeedCovers
			}
		}
		posts = append(posts, ToutiaoPost{
			ID: id, Title: title, FeedCovers: covers,
			URL: toutiaoOrigin + "/profile_v4/graphic/publish?pgc_id=" + url.QueryEscape(id),
		})
	}
	return posts, nil
}

func (t *toutiaoAdapter) listPublished(ctx context.Context) ([]ToutiaoPost, error) {
	const maxPages = 50
	query := url.Values{
		"status": {"2"}, "type": {"0"}, "page_size": {"20"},
		"need_stat": {"true"}, "wenda_type": {"1"}, "app_id": {"1231"},
	}
	posts := []ToutiaoPost{}
	seen := make(map[string]struct{})
	var cursor int64
	for page := 0; page < maxPages; page++ {
		if page > 0 {
			query.Set("cursor", strconv.FormatInt(cursor, 10))
		}
		req, err := t.request(ctx, http.MethodGet, toutiaoOrigin+"/mp/agw/creator_center/list/v2?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var decoded struct {
			Code      *int   `json:"code"`
			Message   string `json:"message"`
			HasMore   bool   `json:"has_more"`
			EndCursor int64  `json:"end_cursor"`
			Contents  []struct {
				Article struct {
					GID        any    `json:"gid"`
					Title      string `json:"title"`
					Status     int    `json:"status"`
					ModifyTime int64  `json:"modify_time"`
				} `json:"article_attr"`
			} `json:"contents"`
		}
		if err := doJSON(t.client, req, t.ID(), "list-published", &decoded); err != nil {
			return nil, err
		}
		if decoded.Code == nil || *decoded.Code != 0 {
			return nil, platformError(ErrUpstream, t.ID(), "list-published", 0, responseMessage(decoded.Message), false)
		}
		for _, item := range decoded.Contents {
			if item.Article.Status != 2 {
				continue
			}
			id := valueString(item.Article.GID)
			title := strings.TrimSpace(item.Article.Title)
			if id == "" || id == "0" || title == "" {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			posts = append(posts, ToutiaoPost{
				ID: id, Title: title, Published: true,
				ModifiedAt: func() string {
					if item.Article.ModifyTime <= 0 {
						return ""
					}
					return strconv.FormatInt(item.Article.ModifyTime, 10)
				}(),
				URL: toutiaoPublic + "/article/" + url.PathEscape(id) + "/",
			})
		}
		if !decoded.HasMore {
			return posts, nil
		}
		if len(decoded.Contents) == 0 || decoded.EndCursor == cursor {
			return nil, errors.New("Toutiao published article pagination did not advance")
		}
		cursor = decoded.EndCursor
	}
	return nil, errors.New("Toutiao published article list exceeds pagination limit")
}

// ToutiaoListPosts fetches the creator's own drafts and published articles.
// Unlike /api/feed/mp_search/v1/, these endpoints are scoped to the signed-in creator.
func ToutiaoListPosts(ctx context.Context, base *http.Client, session Session) (string, []ToutiaoPost, error) {
	adapterValue, err := NewToutiaoAdapter(base, session)
	if err != nil {
		return "", nil, err
	}
	adapter := adapterValue.(*toutiaoAdapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return "", nil, err
	}
	if !auth.Authenticated {
		return "", nil, errors.New("Toutiao browser session is not authenticated")
	}
	drafts, err := adapter.listDrafts(ctx)
	if err != nil {
		return "", nil, err
	}
	published, err := adapter.listPublished(ctx)
	if err != nil {
		return "", nil, err
	}
	return auth.UserID, append(drafts, published...), nil
}

package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// JuejinPost is a remotely discovered Juejin article (published) or draft.
// For a published post, ID is the article_id and DraftID carries the draft_id;
// for a draft, both hold the draft_id.
type JuejinPost struct {
	ID        string `json:"id"`
	DraftID   string `json:"draftId,omitempty"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published bool   `json:"published"`
}

func stripJuejinMarkup(value string) string {
	value = strings.ReplaceAll(value, "<em>", "")
	value = strings.ReplaceAll(value, "</em>", "")
	return strings.TrimSpace(value)
}

// JuejinTitleMatches compares a local title against a Juejin remote title,
// stripping the <em> highlight markers Juejin search responses embed around the
// matched keyword.
func JuejinTitleMatches(local, remote string) bool {
	return CSDNTitleMatches(local, stripJuejinMarkup(remote))
}

type juejinPublishedItem struct {
	ArticleID   juejinID `json:"article_id"`
	ArticleInfo struct {
		ArticleID juejinID `json:"article_id"`
		DraftID   juejinID `json:"draft_id"`
		Title     string   `json:"title"`
	} `json:"article_info"`
}

type juejinPublishedResult struct {
	Data    []juejinPublishedItem `json:"data"`
	Cursor  string                `json:"cursor"`
	HasMore bool                  `json:"has_more"`
	ErrNo   int                   `json:"err_no"`
	ErrMsg  string                `json:"err_msg"`
}

func (j *juejinAdapter) listPublished(ctx context.Context, userID string) ([]JuejinPost, error) {
	result := []JuejinPost{}
	seen := map[string]struct{}{}
	cursor := "0"
	for page := 0; page < 100; page++ {
		encoded, _ := json.Marshal(map[string]any{
			"user_id":   userID,
			"sort_type": 2,
			"cursor":    cursor,
		})
		csrf, err := j.csrf(ctx)
		if err != nil {
			return nil, err
		}
		rawURL := j.apiBase + "/content_api/v1/article/query_list?aid=2608&uuid=" + url.QueryEscape(j.uuid) + "&spider=0"
		req, err := j.request(ctx, http.MethodPost, rawURL, bytes.NewReader(encoded))
		if err != nil {
			return nil, err
		}
		req.Header.Set("content-type", "application/json")
		req.Header.Set("x-secsdk-csrf-token", csrf)
		response, err := j.client.Do(req)
		if err != nil {
			return nil, err
		}
		raw, readErr := readBounded(response, 4<<20)
		response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, classifyHTTP("juejin", "list-published", response.StatusCode, string(raw))
		}
		var decoded juejinPublishedResult
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, platformError(ErrUpstream, "juejin", "list-published", response.StatusCode, "invalid JSON response", false)
		}
		if decoded.ErrNo != 0 {
			return nil, platformError(ErrUpstream, "juejin", "list-published", decoded.ErrNo, decoded.ErrMsg, false)
		}
		for _, item := range decoded.Data {
			articleID := strings.TrimSpace(string(item.ArticleID))
			if articleID == "" || articleID == "0" {
				articleID = strings.TrimSpace(string(item.ArticleInfo.ArticleID))
			}
			draftID := strings.TrimSpace(string(item.ArticleInfo.DraftID))
			title := stripJuejinMarkup(item.ArticleInfo.Title)
			if articleID == "" || articleID == "0" || title == "" {
				continue
			}
			if _, exists := seen[articleID]; exists {
				continue
			}
			seen[articleID] = struct{}{}
			result = append(result, JuejinPost{
				ID: articleID, DraftID: draftID, Title: title,
				URL: juejinOrigin + "/post/" + url.PathEscape(articleID), Published: true,
			})
		}
		if !decoded.HasMore || strings.TrimSpace(decoded.Cursor) == "" || decoded.Cursor == cursor {
			break
		}
		cursor = decoded.Cursor
	}
	return result, nil
}

// Juejin's author draft screen uses list_by_user (captured from the browser
// editor). Keep the browser's documented page size and fetch subsequent pages
// without persisting its credentials or any article body.
const juejinDraftPageSize = 10
const juejinDraftMaxPages = 50

type juejinDraftListItem struct {
	ID        juejinID `json:"id"`
	ArticleID juejinID `json:"article_id"`
	UserID    juejinID `json:"user_id"`
	Title     string   `json:"title"`
	Status    int      `json:"status"`
}

type juejinDraftListResult struct {
	Data   []juejinDraftListItem `json:"data"`
	Count  int                   `json:"count"`
	ErrNo  int                   `json:"err_no"`
	ErrMsg string                `json:"err_msg"`
}

func (j *juejinAdapter) listDrafts(ctx context.Context, userID string) ([]JuejinPost, bool, error) {
	result := make([]JuejinPost, 0)
	seen := make(map[string]struct{})
	csrf, err := j.csrf(ctx)
	if err != nil {
		return nil, false, err
	}
	for page := 1; page <= juejinDraftMaxPages; page++ {
		payload, _ := json.Marshal(map[string]any{
			"keyword": "", "page_size": juejinDraftPageSize, "page_no": page,
		})
		rawURL := j.apiBase + "/content_api/v1/article_draft/list_by_user?aid=2608&uuid=" +
			url.QueryEscape(j.uuid) + "&spider=0"
		req, err := j.request(ctx, http.MethodPost, rawURL, bytes.NewReader(payload))
		if err != nil {
			return nil, false, err
		}
		req.Header.Set("content-type", "application/json")
		req.Header.Set("accept", "application/json")
		req.Header.Set("referer", juejinOrigin+"/editor/drafts")
		req.Header.Set("x-secsdk-csrf-token", csrf)
		response, err := j.client.Do(req)
		if err != nil {
			return nil, false, err
		}
		raw, readErr := readBounded(response, 8<<20)
		response.Body.Close()
		if readErr != nil {
			return nil, false, readErr
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, false, classifyHTTP("juejin", "list-drafts", response.StatusCode, string(raw))
		}
		var decoded juejinDraftListResult
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, false, platformError(ErrUpstream, "juejin", "list-drafts",
				response.StatusCode, "invalid JSON response", false)
		}
		if decoded.ErrNo != 0 {
			return nil, false, platformError(ErrUpstream, "juejin", "list-drafts",
				decoded.ErrNo, decoded.ErrMsg, false)
		}
		for _, item := range decoded.Data {
			if id := strings.TrimSpace(string(item.UserID)); id != "" && id != userID {
				return nil, false, platformError(ErrValidation, "juejin", "list-drafts", 0,
					"draft list contains posts from a different account", false)
			}
			// article_id can be present on an unsubmitted draft. Draft list entries
			// are identified by their own ID, not article_id.
			draftID := strings.TrimSpace(string(item.ID))
			title := stripJuejinMarkup(item.Title)
			if item.Status != 0 || draftID == "" || draftID == "0" || title == "" {
				continue
			}
			if _, exists := seen[draftID]; exists {
				continue
			}
			seen[draftID] = struct{}{}
			result = append(result, JuejinPost{
				ID: draftID, DraftID: draftID, Title: title,
				URL:       juejinOrigin + "/editor/drafts/" + url.PathEscape(draftID),
				Published: false,
			})
		}
		if len(decoded.Data) == 0 || page*juejinDraftPageSize >= decoded.Count {
			return result, false, nil
		}
		if len(decoded.Data) < juejinDraftPageSize {
			return result, decoded.Count > page*juejinDraftPageSize, nil
		}
	}
	return result, true, nil
}

// JuejinListAllPosts uses the same authenticated Go adapter for published
// posts and drafts. A fixed pagination limit is reported to callers.
func JuejinListAllPosts(ctx context.Context, base *http.Client, session Session) (
	string, []JuejinPost, bool, error,
) {
	adapterValue, err := NewJuejinAdapter(base, session)
	if err != nil {
		return "", nil, false, err
	}
	adapter := adapterValue.(*juejinAdapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return "", nil, false, err
	}
	if !auth.Authenticated || strings.TrimSpace(auth.UserID) == "" {
		return "", nil, false, errors.New("Juejin browser session is not authenticated")
	}
	published, err := adapter.listPublished(ctx, auth.UserID)
	if err != nil {
		return "", nil, false, err
	}
	drafts, truncated, err := adapter.listDrafts(ctx, auth.UserID)
	if err != nil {
		return "", nil, false, err
	}
	// Distinct remote states have distinct IDs. Deduplicate only exact
	// platform/state/ID matches rather than merging draft into published posts.
	posts := make([]JuejinPost, 0, len(drafts)+len(published))
	seen := make(map[string]struct{})
	for _, post := range append(drafts, published...) {
		key := post.ID
		if post.Published {
			key = "published:" + key
		} else {
			key = "draft:" + key
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		posts = append(posts, post)
	}
	return auth.UserID, posts, truncated, nil
}

func (j *juejinAdapter) lookupDraft(ctx context.Context, draftID string) (JuejinPost, error) {
	detail, err := j.draftDetail(ctx, draftID)
	if err != nil {
		return JuejinPost{}, err
	}
	article := detail.Data.ArticleDraft
	draft := strings.TrimSpace(article.ID)
	if draft == "" || draft == "0" {
		draft = strings.TrimSpace(draftID)
	}
	articleID := strings.TrimSpace(article.ArticleID)
	if articleID == "0" {
		articleID = ""
	}
	title := stripJuejinMarkup(article.Title)
	if title == "" {
		return JuejinPost{}, platformError(ErrUpstream, "juejin", "draft-detail", 0, "draft title is missing", false)
	}
	if articleID != "" {
		return JuejinPost{
			ID: articleID, DraftID: draft, Title: title,
			URL: juejinOrigin + "/post/" + url.PathEscape(articleID), Published: true,
		}, nil
	}
	return JuejinPost{
		ID: draft, DraftID: draft, Title: title,
		URL: juejinOrigin + "/editor/drafts/" + url.PathEscape(draft), Published: false,
	}, nil
}

// JuejinListPosts reads the signed-in user's published article list. The
// response includes both article_id and draft_id, which lets the bridge
// reconcile a locally recorded draft after it has been published.
func JuejinListPosts(ctx context.Context, base *http.Client, session Session) (string, []JuejinPost, error) {
	adapterValue, err := NewJuejinAdapter(base, session)
	if err != nil {
		return "", nil, err
	}
	adapter := adapterValue.(*juejinAdapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return "", nil, err
	}
	if !auth.Authenticated || strings.TrimSpace(auth.UserID) == "" {
		return "", nil, errors.New("Juejin browser session is not authenticated")
	}
	posts, err := adapter.listPublished(ctx, auth.UserID)
	if err != nil {
		return "", nil, err
	}
	return auth.UserID, posts, nil
}

// JuejinLookupDraft verifies a known Juejin draft ID and reports whether it has
// since been published.
func JuejinLookupDraft(ctx context.Context, base *http.Client, session Session, draftID string) (JuejinPost, error) {
	adapterValue, err := NewJuejinAdapter(base, session)
	if err != nil {
		return JuejinPost{}, err
	}
	adapter := adapterValue.(*juejinAdapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return JuejinPost{}, err
	}
	if !auth.Authenticated || strings.TrimSpace(auth.UserID) == "" {
		return JuejinPost{}, errors.New("Juejin browser session is not authenticated")
	}
	return adapter.lookupDraft(ctx, draftID)
}

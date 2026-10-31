package publisher

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type OSChinaPost struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published bool   `json:"published"`
}

// OSChinaDraftEditorURL resolves a draft to the numeric account's editing UI.
// An account username is not interchangeable with the creator's numeric ID.
func OSChinaDraftEditorURL(userID, draftID string) string {
	if !cnBlogsNumericID.MatchString(strings.TrimSpace(userID)) ||
		!cnBlogsNumericID.MatchString(strings.TrimSpace(draftID)) {
		return ""
	}
	return osChinaOrigin + "/u/" + url.PathEscape(strings.TrimSpace(userID)) +
		"/blog/ai-write/draft/" + url.PathEscape(strings.TrimSpace(draftID))
}

// OSChinaDraftURL corrects persisted legacy preview URLs when the numeric
// creator ID is known. Otherwise retain the existing link rather than
// hardcoding an account or emitting a malformed editor URL.
func OSChinaDraftURL(userID, draftID, stored string) string {
	if normalized := OSChinaDraftEditorURL(userID, draftID); normalized != "" {
		return normalized
	}
	parsed, err := url.Parse(strings.TrimSpace(stored))
	if err == nil && parsed.Scheme == "https" && parsed.Hostname() == "my.oschina.net" {
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(segments) >= 2 && segments[0] == "u" {
			if normalized := OSChinaDraftEditorURL(segments[1], draftID); normalized != "" {
				return normalized
			}
		}
	}
	return stored
}

func OSChinaTitleMatches(local, remote string) bool {
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

func (o *osChinaAdapter) listDrafts(ctx context.Context) ([]OSChinaPost, error) {
	result := []OSChinaPost{}
	for page := 1; page <= 10; page++ {
		rawURL := osChinaAPIOrigin + "/oschinapi/api/draft/list?pageNum=" + strconv.Itoa(page) + "&pageSize=20"
		req, err := o.request(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		var decoded struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Result  struct {
				Records []struct {
					ID    any    `json:"id"`
					Title string `json:"title"`
				} `json:"records"`
				Current int `json:"current"`
				Pages   int `json:"pages"`
			} `json:"result"`
		}
		if err := doJSON(o.client, req, o.ID(), "list-drafts", &decoded); err != nil {
			return nil, err
		}
		if !decoded.Success {
			return nil, platformError(ErrUpstream, o.ID(), "list-drafts", 0, responseMessage(decoded.Message), false)
		}
		for _, record := range decoded.Result.Records {
			id := valueString(record.ID)
			title := strings.TrimSpace(record.Title)
			if id == "" || title == "" {
				continue
			}
			result = append(result, OSChinaPost{
				ID: id, Title: title,
				URL: OSChinaDraftEditorURL(o.userID, id),
			})
		}
		if len(decoded.Result.Records) == 0 || decoded.Result.Pages <= page {
			break
		}
	}
	return result, nil
}

func (o *osChinaAdapter) listPublished(ctx context.Context) ([]OSChinaPost, error) {
	result := []OSChinaPost{}
	for page := 1; page <= 10; page++ {
		rawURL := osChinaAPIOrigin + "/oschinapi/blog/my/web?pageNum=" + strconv.Itoa(page) + "&pageSize=20"
		req, err := o.request(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		var decoded struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Result  struct {
				Records []struct {
					ID    any    `json:"id"`
					Title string `json:"title"`
				} `json:"records"`
				Current int `json:"current"`
				Pages   int `json:"pages"`
			} `json:"result"`
		}
		if err := doJSON(o.client, req, o.ID(), "list-published", &decoded); err != nil {
			return nil, err
		}
		if !decoded.Success {
			return nil, platformError(ErrUpstream, o.ID(), "list-published", 0, responseMessage(decoded.Message), false)
		}
		for _, record := range decoded.Result.Records {
			id := valueString(record.ID)
			title := strings.TrimSpace(record.Title)
			if id == "" || title == "" {
				continue
			}
			result = append(result, OSChinaPost{
				ID: id, Title: title,
				URL:       osChinaOrigin + "/u/" + url.PathEscape(o.userID) + "/blog/" + url.PathEscape(id),
				Published: true,
			})
		}
		if len(decoded.Result.Records) == 0 || decoded.Result.Pages <= page {
			break
		}
	}
	return result, nil
}

// OSChinaListPosts reads the signed-in user's draft list and published blog list.
// OSChina does not expose a dedicated title search endpoint in the captured flow,
// so callers match the local article title against these lists.
func OSChinaListPosts(ctx context.Context, base *http.Client, session Session) (string, []OSChinaPost, error) {
	adapterValue, err := NewOSChinaAdapter(base, session)
	if err != nil {
		return "", nil, err
	}
	adapter := adapterValue.(*osChinaAdapter)
	if err := adapter.ensureUser(ctx); err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(adapter.userID) == "" {
		return "", nil, errors.New("OSChina user id is unavailable")
	}
	drafts, err := adapter.listDrafts(ctx)
	if err != nil {
		return "", nil, err
	}
	published, err := adapter.listPublished(ctx)
	if err != nil {
		return "", nil, err
	}
	return adapter.userID, append(drafts, published...), nil
}

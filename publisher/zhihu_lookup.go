package publisher

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type ZhihuPost struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Published bool   `json:"published"`
}

type zhihuListPage struct {
	Paging struct {
		IsEnd bool `json:"is_end"`
	} `json:"paging"`
	Data []map[string]any `json:"data"`
}

func ZhihuTitleMatches(local, remote string) bool {
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

func (z *zhihuAdapter) accountURLToken(ctx context.Context) (string, error) {
	req, err := z.request(ctx, http.MethodGet, zhihuMeURL+"?include=url_token,name,id", nil)
	if err != nil {
		return "", err
	}
	var decoded struct {
		ID       string `json:"id"`
		URLToken string `json:"url_token"`
	}
	if err := doJSON(z.client, req, z.ID(), "account", &decoded); err != nil {
		return "", err
	}
	if strings.TrimSpace(decoded.ID) == "" || strings.TrimSpace(decoded.URLToken) == "" {
		return "", errors.New("Zhihu account url_token is unavailable")
	}
	return strings.TrimSpace(decoded.URLToken), nil
}

func (z *zhihuAdapter) listDrafts(ctx context.Context) ([]ZhihuPost, error) {
	if strings.TrimSpace(z.dc0) == "" {
		return nil, errors.New("Zhihu d_c0 cookie is required for signed article lookup")
	}
	result := []ZhihuPost{}
	for offset, page := 0, 0; page < 10; page, offset = page+1, offset+10 {
		rawURL := "https://www.zhihu.com/api/v4/articles/my_drafts?offset=" + strconv.Itoa(offset) +
			"&limit=10&include=data%5B*%5D.schedule"
		req, err := z.request(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		var decoded zhihuListPage
		if err := doJSON(z.client, req, z.ID(), "list-drafts", &decoded); err != nil {
			return nil, err
		}
		for _, value := range decoded.Data {
			id := valueString(value["url_token"])
			if id == "" {
				id = valueString(value["id"])
			}
			title := strings.TrimSpace(valueString(value["title"]))
			if id == "" || title == "" {
				continue
			}
			result = append(result, ZhihuPost{
				ID: id, Title: title,
				URL:       zhihuOrigin + "/p/" + url.PathEscape(id) + "/edit",
				Published: false,
			})
		}
		if decoded.Paging.IsEnd || len(decoded.Data) == 0 {
			break
		}
	}
	return result, nil
}

func (z *zhihuAdapter) listPublished(ctx context.Context, urlToken string) ([]ZhihuPost, error) {
	if strings.TrimSpace(z.dc0) == "" {
		return nil, errors.New("Zhihu d_c0 cookie is required for signed article lookup")
	}
	result := []ZhihuPost{}
	for offset, page := 0, 0; page < 10; page, offset = page+1, offset+20 {
		rawURL := "https://www.zhihu.com/api/v4/members/" + url.PathEscape(urlToken) +
			"/articles?offset=" + strconv.Itoa(offset) + "&limit=20&sort_by=created&ws_qiangzhisafe=0"
		req, err := z.request(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		var decoded zhihuListPage
		if err := doJSON(z.client, req, z.ID(), "list-published", &decoded); err != nil {
			return nil, err
		}
		for _, value := range decoded.Data {
			id := valueString(value["id"])
			title := strings.TrimSpace(valueString(value["title"]))
			if id == "" || title == "" {
				continue
			}
			target := strings.TrimSpace(valueString(value["url"]))
			if target == "" {
				target = zhihuOrigin + "/p/" + url.PathEscape(id)
			} else if strings.HasPrefix(target, "http://") {
				target = "https://" + strings.TrimPrefix(target, "http://")
			}
			result = append(result, ZhihuPost{
				ID: id, Title: title, URL: target, Published: true,
			})
		}
		if decoded.Paging.IsEnd || len(decoded.Data) == 0 {
			break
		}
	}
	return result, nil
}

func ZhihuAccountURLToken(ctx context.Context, base *http.Client, session Session) (string, error) {
	adapterValue, err := NewZhihuAdapter(base, session)
	if err != nil {
		return "", err
	}
	adapter := adapterValue.(*zhihuAdapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return "", err
	}
	if !auth.Authenticated {
		return "", errors.New("Zhihu browser session is not authenticated")
	}
	return adapter.accountURLToken(ctx)
}

// ZhihuListPosts reads the signed-in user's draft list and published article list.
// Binding candidates are matched locally; BlogCTL does not depend on global search.
func ZhihuListPosts(ctx context.Context, base *http.Client, session Session) (string, []ZhihuPost, error) {
	adapterValue, err := NewZhihuAdapter(base, session)
	if err != nil {
		return "", nil, err
	}
	adapter := adapterValue.(*zhihuAdapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return "", nil, err
	}
	if !auth.Authenticated {
		return "", nil, errors.New("Zhihu browser session is not authenticated")
	}
	urlToken, err := adapter.accountURLToken(ctx)
	if err != nil {
		return "", nil, err
	}
	drafts, draftErr := adapter.listDrafts(ctx)
	published, publishedErr := adapter.listPublished(ctx, urlToken)
	if len(drafts) == 0 && len(published) == 0 && (draftErr != nil || publishedErr != nil) {
		return "", nil, errors.Join(draftErr, publishedErr)
	}
	return urlToken, append(drafts, published...), nil
}

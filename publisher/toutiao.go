package publisher

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	toutiaoOrigin = "https://mp.toutiao.com"
	toutiaoPublic = "https://www.toutiao.com"
)

type toutiaoAdapter struct {
	client    *http.Client
	session   Session
	userAgent string
	userID    string
	mediaID   string
}

func NewToutiaoAdapter(base *http.Client, session Session) (Adapter, error) {
	client, err := HTTPClientForSession(base, session)
	if err != nil {
		return nil, err
	}
	return &toutiaoAdapter{client: client, session: session, userAgent: session.UserAgent}, nil
}

func (t *toutiaoAdapter) ID() string { return "toutiao" }

func (t *toutiaoAdapter) request(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	req, err := browserRequest(ctx, method, rawURL, toutiaoOrigin, toutiaoOrigin+"/profile_v4/graphic/publish", t.userAgent, body)
	if err != nil {
		return nil, err
	}
	if method == http.MethodPost && (req.URL.Path == "/spice/image" || req.URL.Path == "/mp/agw/article/publish") {
		for _, name := range []string{"x-secsdk-csrf-token", "tt-anti-token"} {
			if value := strings.TrimSpace(t.session.RequestHeaders[name]); value != "" {
				req.Header.Set(name, value)
			}
		}
	}
	return req, nil
}

func (t *toutiaoAdapter) CheckAuth(ctx context.Context) (AuthResult, error) {
	req, err := t.request(ctx, http.MethodGet, toutiaoOrigin+"/mp/agw/media/get_media_info", nil)
	if err != nil {
		return AuthResult{}, err
	}
	response, err := t.client.Do(req)
	if err != nil {
		return AuthResult{}, platformError(ErrUpstream, t.ID(), "auth", 0, err.Error(), true)
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return AuthResult{}, err
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return AuthResult{Authenticated: false}, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return AuthResult{}, classifyHTTP(t.ID(), "auth", response.StatusCode, string(raw))
	}
	var decoded struct {
		Data struct {
			Media struct {
				ID    any    `json:"id"`
				IDStr string `json:"id_str"`
			} `json:"media"`
			User struct {
				ID         any    `json:"id"`
				ScreenName string `json:"screen_name"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return AuthResult{}, platformError(ErrUpstream, t.ID(), "auth", response.StatusCode, "invalid JSON response", false)
	}
	t.userID = valueString(decoded.Data.User.ID)
	t.mediaID = strings.TrimSpace(decoded.Data.Media.IDStr)
	if t.mediaID == "" {
		t.mediaID = valueString(decoded.Data.Media.ID)
	}
	if t.userID == "" {
		return AuthResult{Authenticated: false}, nil
	}
	return AuthResult{Authenticated: true, UserID: t.userID, Username: decoded.Data.User.ScreenName}, nil
}

func isToutiaoImage(source string) bool {
	parsed, err := url.Parse(source)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "image-tt-private.toutiao.com" ||
		strings.HasSuffix(host, ".toutiaoimg.com") ||
		strings.HasSuffix(host, ".toutiaostatic.com") ||
		strings.HasSuffix(host, ".bytescm.com")
}

type toutiaoImageResponse struct {
	Code    *int   `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ImageURL string `json:"image_url"`
		ImageURI string `json:"image_uri"`
		CoverURL string `json:"cover_url"`
	} `json:"data"`
}

// uploadPicture uses the creator editor's captured /spice/image contract.
// Binary images and existing remote images are distinct multipart operations.
func (t *toutiaoAdapter) uploadPicture(ctx context.Context, source string, image *RehostImage) (string, error) {
	var body io.Reader
	var bodyType string
	var err error
	query := url.Values{"aid": {"1231"}, "device_platform": {"web"}}
	if image == nil {
		query.Set("upload_source", "20020003")
		query.Set("need_cover_url", "1")
		buffer, multipartType, multipartErr := multipartBody(map[string]string{"imageUrl": source}, "", "", "", nil)
		body, bodyType, err = buffer, multipartType, multipartErr
	} else {
		query.Set("upload_source", "20020002")
		buffer, multipartType, multipartErr := multipartBody(nil, "image", inferImageFilename(image.Source, image.ContentType), image.ContentType, image.Payload)
		body, bodyType, err = buffer, multipartType, multipartErr
	}
	if err != nil {
		return "", err
	}
	rawURL := toutiaoOrigin + "/spice/image?" + query.Encode()
	req, err := t.request(ctx, http.MethodPost, rawURL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", bodyType)
	var decoded toutiaoImageResponse
	if err := doJSON(t.client, req, t.ID(), "image-upload", &decoded); err != nil {
		return "", err
	}
	if decoded.Code == nil || *decoded.Code != 0 || !isRemoteHTTPImage(decoded.Data.ImageURL) {
		return "", platformError(ErrUpload, t.ID(), "image-upload", 0, "Toutiao did not return a successful image_url", false)
	}
	return decoded.Data.ImageURL, nil
}

func (t *toutiaoAdapter) uploadByURL(ctx context.Context, source string) (string, error) {
	return t.uploadPicture(ctx, source, nil)
}

func (t *toutiaoAdapter) uploadBinary(ctx context.Context, image RehostImage) (string, error) {
	return t.uploadPicture(ctx, "", &image)
}

func (t *toutiaoAdapter) prepareHTML(ctx context.Context, input DraftInput) (string, error) {
	return rehostHTMLImages(ctx, t.client, input, normalizeToutiaoSectionReferences(normalizeToutiaoTOC(htmlFor(input))), ImageRehostOptions{
		Platform:       t.ID(),
		FailOpenRemote: true,
		AlreadyHosted:  isToutiaoImage,
	}, func(ctx context.Context, image RehostImage) (string, error) {
		if isRemoteHTTPImage(image.Source) {
			if target, err := t.uploadByURL(ctx, image.Source); err == nil {
				return target, nil
			}
		}
		return t.uploadBinary(ctx, image)
	})
}

func truncateToutiaoTitle(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= 30 {
		return string(runes)
	}
	return string(runes[:30])
}

// toutiaoArticleValues mirrors the creator editor's form for an ordinary
// graphic article. Both draft operations use save=0, as captured on Oct 9.
// Publishing is a separate explicit action (save=1); it must never be used
// as the draft update path.
func toutiaoArticleValues(input DraftInput, html, refID, covers string, publish bool) url.Values {
	values := url.Values{}
	values.Set("source", "29")
	values.Set("title", truncateToutiaoTitle(input.Title))
	values.Set("content", html)
	values.Set("save", "0")
	if publish {
		values.Set("save", "1")
	}
	if refID != "" && refID != "0" {
		values.Set("pgc_id", refID)
	}
	values.Set("extra", `{"content_source":100000000402,"content_word_cnt":`+strconv.Itoa(utf8.RuneCountInString(input.Markdown))+`,"is_multi_title":0,"sub_titles":[],"gd_ext":{"entrance":"","from_page":"publisher_mp","enter_from":"PC","device_platform":"mp","is_message":0},"tuwen_wtt_transfer_switch":"1"}`)
	values.Set("search_creation_info", `{"searchTopOne":0,"abstract":"","clue_id":""}`)
	values.Set("mp_editor_stat", "{}")
	values.Set("is_refute_rumor", "0")
	values.Set("entrance", "")
	values.Set("timer_status", "0")
	values.Set("timer_time", "")
	values.Set("educluecard", "")
	values.Set("draft_form_data", `{"coverType":2}`)
	if covers == "" {
		covers = "[]"
	}
	values.Set("pgc_feed_covers", covers)
	values.Set("article_ad_type", "2")
	values.Set("is_fans_article", "0")
	values.Set("govern_forward", "0")
	values.Set("praise", "1")
	values.Set("disable_praise", "0")
	values.Set("tree_plan_article", "0")
	values.Set("star_order_id", "")
	values.Set("star_order_name", "")
	values.Set("activity_tag", "0")
	values.Set("trends_writing_tag", "0")
	values.Set("claim_exclusive", "0")
	return values
}

func (t *toutiaoAdapter) mutate(ctx context.Context, refID string, input DraftInput, publish bool, covers string) (string, error) {
	// Bootstrap before image uploads. Both /spice/image and the final
	// article write use the same short-lived creator CSRF credential.
	csrfToken := ""
	if t.mediaID != "" {
		credential, csrfErr := fetchToutiaoCSRF(ctx, t.client, t.userAgent, time.Now())
		if csrfErr != nil {
			return "", platformError(ErrUpstream, t.ID(), "csrf-preflight", 0, csrfErr.Error(), false)
		}
		csrfToken = credential.Token
		previousHeaders := t.session.RequestHeaders
		updatedHeaders := make(map[string]string, len(previousHeaders)+1)
		for key, value := range previousHeaders {
			updatedHeaders[key] = value
		}
		updatedHeaders["x-secsdk-csrf-token"] = csrfToken
		t.session.RequestHeaders = updatedHeaders
		defer func() { t.session.RequestHeaders = previousHeaders }()
	}
	html, err := t.prepareHTML(ctx, input)
	if err != nil {
		return "", err
	}
	values := toutiaoArticleValues(input, html, strings.TrimSpace(refID), covers, publish)
	if !publish {
		values.Set("article_ad_type", "3")
		if strings.TrimSpace(refID) == "" || strings.TrimSpace(refID) == "0" {
			// First editor save: title_id ties creation to the media identity.
			if t.mediaID != "" {
				values.Set("title_id", strconv.FormatInt(time.Now().UnixMilli(), 10)+"_"+t.mediaID)
			}
			values.Set("customer_nick_name", "")
		} else {
			// Reopening an existing draft (published3.har): title_id is empty
			// and article_type=0, while article_ad_type stays 3.
			values.Set("title_id", "")
			values.Set("article_type", "0")
		}
	}
	if publish {
		// Captured editor republish form (save=1) for an already-published article.
		values.Set("article_type", "0")
		values.Set("entrance", "main")
		values.Set("praise", "0")
		values.Set("draft_form_data", `{"coverType":1}`)
		if strings.TrimSpace(covers) == "" || strings.TrimSpace(covers) == "[]" {
			values.Set("draft_form_data", `{"coverType":2}`)
		}
		values.Set("extra", `{"content_source":100000000402,"content_word_cnt":`+strconv.Itoa(utf8.RuneCountInString(input.Markdown))+`,"is_multi_title":0,"sub_titles":[],"gd_ext":{"entrance":"","from_page":"publisher_mp","enter_from":"PC","device_platform":"mp","is_message":0},"tuwen_wtt_trans_flag":"0","info_source":{"source_type":-1}}`)
		for _, name := range []string{"ic_uri_list", "appid_list", "stock_ids", "concern_list", "title_id"} {
			values.Set(name, "")
		}
	}
	req, err := t.request(ctx, http.MethodPost,
		toutiaoOrigin+"/mp/agw/article/publish?source=mp&type=article&aid=1231&mp_publish_ab_val=0",
		strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded;charset=UTF-8")
	if csrfToken != "" {
		req.Header.Set("x-secsdk-csrf-token", csrfToken)
	}
	if csrf := cookieValue(t.session, "passport_csrf_token"); csrf != "" {
		req.Header.Set("x-csrftoken", csrf)
	}
	var decoded struct {
		Code    *int   `json:"code"`
		ErrNo   *int   `json:"err_no"`
		Message string `json:"message"`
		Data    struct {
			PGCID any `json:"pgc_id"`
		} `json:"data"`
	}
	operation := "save-draft"
	if publish {
		operation = "publish-draft"
	}
	if err := doJSON(t.client, req, t.ID(), operation, &decoded); err != nil {
		return "", err
	}
	codeOK := decoded.Code != nil && *decoded.Code == 0 && (decoded.ErrNo == nil || *decoded.ErrNo == 0)
	id := valueString(decoded.Data.PGCID)
	if !codeOK || id == "" || id == "0" {
		message := responseMessage(decoded.Message)
		// Preserve upstream business status without exposing session headers,
		// tokens, request bodies or any other account credentials.
		if decoded.Code != nil {
			message += " (code=" + strconv.Itoa(*decoded.Code)
			if decoded.ErrNo != nil {
				message += ", err_no=" + strconv.Itoa(*decoded.ErrNo)
			}
			message += ")"
		} else if decoded.ErrNo != nil {
			message += " (err_no=" + strconv.Itoa(*decoded.ErrNo) + ")"
		}
		if !publish && refID != "" && refID != "0" && (strings.Contains(message, "不存在") || strings.Contains(strings.ToLower(message), "not found")) {
			return "", platformError(ErrValidation, t.ID(), operation, 0, "the draft no longer exists; refusing to recreate it without explicit confirmation", false)
		}
		return "", platformError(ErrUpstream, t.ID(), operation, 0, message, false)
	}
	if refID != "" && refID != "0" && id != refID {
		return "", platformError(ErrUpstream, t.ID(), operation, 0, "Toutiao returned a different article ID; refusing to overwrite the existing binding", false)
	}
	return id, nil
}

func (t *toutiaoAdapter) CreateDraft(ctx context.Context, input DraftInput) (DraftResult, error) {
	id, err := t.mutate(ctx, "0", input, false, "")
	if err != nil {
		return DraftResult{}, err
	}
	return DraftResult{ID: id, URL: toutiaoOrigin + "/profile_v4/graphic/publish?pgc_id=" + url.QueryEscape(id), Created: true}, nil
}

func (t *toutiaoAdapter) UpdateDraft(ctx context.Context, ref DraftRef, input DraftInput) (DraftResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return DraftResult{}, platformError(ErrValidation, t.ID(), "update-draft", 0, "draft id is required", false)
	}
	// Verify the draft is still present in this creator's draft inventory.
	// A stale ID may belong to an already published article; never rewrite it.
	drafts, err := t.listDrafts(ctx)
	if err != nil {
		return DraftResult{}, err
	}
	var selected *ToutiaoPost
	for index := range drafts {
		if drafts[index].ID == strings.TrimSpace(ref.ID) {
			selected = &drafts[index]
			break
		}
	}
	if selected == nil {
		return DraftResult{}, platformError(ErrValidation, t.ID(), "update-draft", 0,
			"draft ID is not in the authenticated creator's draft list; refusing to update or recreate it", false)
	}
	id, err := t.mutate(ctx, ref.ID, input, false, selected.FeedCovers)
	if err != nil {
		return DraftResult{}, err
	}
	return DraftResult{ID: id, URL: toutiaoOrigin + "/profile_v4/graphic/publish?pgc_id=" + url.QueryEscape(id), Updated: true}, nil
}

func (t *toutiaoAdapter) PublishDraft(ctx context.Context, ref DraftRef, input DraftInput) (PublishResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return PublishResult{}, platformError(ErrValidation, t.ID(), "publish-draft", 0, "draft id is required", false)
	}
	drafts, err := t.listDrafts(ctx)
	if err != nil {
		return PublishResult{}, err
	}
	var selected *ToutiaoPost
	for i := range drafts {
		if drafts[i].ID == ref.ID {
			selected = &drafts[i]
			break
		}
	}
	if selected == nil {
		return PublishResult{}, platformError(ErrValidation, t.ID(), "publish-draft", 0, "selected draft no longer exists in this account; detect again before publishing", false)
	}
	id, err := t.mutate(ctx, ref.ID, input, true, selected.FeedCovers)
	if err != nil {
		return PublishResult{}, err
	}
	return PublishResult{ID: id, URL: toutiaoPublic + "/article/" + url.PathEscape(id) + "/"}, nil
}

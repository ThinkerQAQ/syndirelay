package publisher

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	juejinOrigin         = "https://juejin.cn"
	juejinDefaultAPIBase = "https://api.juejin.cn"
	juejinImageXBase     = "https://imagex.bytedanceapi.com"
	juejinImageXAID      = "2608"
	juejinImageXService  = "73owjymdk6"
)

type imageXToken struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	ExpiresAt       time.Time
}

type juejinAdapter struct {
	client       *http.Client
	userAgent    string
	apiBase      string
	imageXBase   string
	uploadScheme string
	now          func() time.Time
	uuid         string

	mu               sync.Mutex
	csrfToken        string
	cachedImageToken *imageXToken
	imageTokenUntil  time.Time
}

func NewJuejinAdapter(base *http.Client, session Session) (Adapter, error) {
	client, err := HTTPClientForSession(base, session)
	if err != nil {
		return nil, err
	}
	return &juejinAdapter{
		client: client, userAgent: session.UserAgent,
		apiBase: juejinDefaultAPIBase, imageXBase: juejinImageXBase, uploadScheme: "https",
		now: time.Now, uuid: randomJuejinUUID(),
	}, nil
}

func randomJuejinUUID() string {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err == nil {
		return hex.EncodeToString(value)
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func (j *juejinAdapter) ID() string { return "juejin" }

func (j *juejinAdapter) request(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("origin", juejinOrigin)
	req.Header.Set("referer", juejinOrigin+"/")
	if strings.TrimSpace(j.userAgent) != "" {
		req.Header.Set("user-agent", j.userAgent)
	}
	return req, nil
}

func readBounded(response *http.Response, limit int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > limit {
		return nil, errors.New("upstream response is too large")
	}
	return payload, nil
}

func classifyHTTP(platform, operation string, status int, payload string) error {
	safe := strings.TrimSpace(payload)
	if len(safe) > 800 {
		safe = safe[:800]
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return platformError(ErrAuthExpired, platform, operation, status, safe, false)
	case http.StatusNotFound, http.StatusGone:
		return platformError(ErrRemoteDraftMissing, platform, operation, status, safe, false)
	case http.StatusTooManyRequests:
		return platformError(ErrRateLimited, platform, operation, status, safe, true)
	default:
		return platformError(ErrUpstream, platform, operation, status, safe, status >= 500)
	}
}

func (j *juejinAdapter) CheckAuth(ctx context.Context) (AuthResult, error) {
	req, err := j.request(ctx, http.MethodGet, j.apiBase+"/user_api/v1/user/get", nil)
	if err != nil {
		return AuthResult{}, err
	}
	response, err := j.client.Do(req)
	if err != nil {
		return AuthResult{}, err
	}
	defer response.Body.Close()
	payload, err := readBounded(response, 1<<20)
	if err != nil {
		return AuthResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return AuthResult{Authenticated: false}, nil
		}
		return AuthResult{}, classifyHTTP("juejin", "auth", response.StatusCode, string(payload))
	}
	var decoded struct {
		Data struct {
			UserID   string `json:"user_id"`
			UserName string `json:"user_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return AuthResult{}, platformError(ErrUpstream, "juejin", "auth", response.StatusCode, "invalid JSON response", false)
	}
	return AuthResult{
		Authenticated: decoded.Data.UserID != "",
		UserID:        decoded.Data.UserID,
		Username:      decoded.Data.UserName,
	}, nil
}

func (j *juejinAdapter) csrf(ctx context.Context) (string, error) {
	j.mu.Lock()
	cached := j.csrfToken
	j.mu.Unlock()
	if cached != "" {
		return cached, nil
	}

	req, err := j.request(ctx, http.MethodHead, j.apiBase+"/user_api/v1/sys/token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("x-secsdk-csrf-request", "1")
	req.Header.Set("x-secsdk-csrf-version", "1.2.10")
	response, err := j.client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return "", classifyHTTP("juejin", "csrf", response.StatusCode, "")
	}
	raw := response.Header.Get("x-ware-csrf-token")
	parts := strings.Split(raw, ",")
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return "", platformError(ErrCSRF, "juejin", "csrf", response.StatusCode, "x-ware-csrf-token missing or invalid", false)
	}
	token := strings.TrimSpace(parts[1])
	j.mu.Lock()
	j.csrfToken = token
	j.mu.Unlock()
	return token, nil
}

func truncateRunes(value string, maximum int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maximum {
		return string(runes)
	}
	return string(runes[:maximum])
}

type juejinID string

func (id *juejinID) UnmarshalJSON(raw []byte) error {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		*id = ""
		return nil
	}
	if strings.HasPrefix(value, "\"") {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
		*id = juejinID(text)
		return nil
	}
	*id = juejinID(value)
	return nil
}

func juejinStringIDs(values []juejinID) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		id := strings.TrimSpace(string(value))
		if id != "" && id != "0" {
			result = append(result, id)
		}
	}
	return result
}

func (j *juejinAdapter) draftPayload(ctx context.Context, input DraftInput, id string, detail *juejinDraftDetail) (map[string]any, error) {
	markdown, err := j.prepareMarkdown(ctx, input)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"brief_content": truncateRunes(input.Description, 100),
		"category_id":   "0",
		"cover_image":   "",
		"edit_type":     10,
		"html_content":  "deprecated",
		"link_url":      "",
		"mark_content":  markdown,
		"tag_ids":       []string{},
		"title":         input.Title,
	}
	if detail != nil {
		base := detail.Data.ArticleDraft
		payload["id"] = id
		payload["category_id"] = base.CategoryID
		payload["tag_ids"] = juejinStringIDs(base.TagIDs)
		payload["link_url"] = base.LinkURL
		payload["cover_image"] = base.CoverImage
		payload["is_gfw"] = base.IsGFW
		payload["is_english"] = base.IsEnglish
		payload["is_original"] = base.IsOriginal
		if base.EditType != 0 {
			payload["edit_type"] = base.EditType
		}
		payload["theme_ids"] = juejinStringIDs(base.ThemeIDs)
		payload["pics"] = base.Pics
		return payload, nil
	}
	if id != "" {
		payload["id"] = id
	}
	return payload, nil
}

func (j *juejinAdapter) mutateDraft(ctx context.Context, path, operation string, input DraftInput, id string, detail *juejinDraftDetail) (DraftResult, error) {
	payload, err := j.draftPayload(ctx, input, id, detail)
	if err != nil {
		return DraftResult{}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return DraftResult{}, err
	}
	csrf, err := j.csrf(ctx)
	if err != nil {
		return DraftResult{}, err
	}
	req, err := j.request(ctx, http.MethodPost, j.apiBase+path, bytes.NewReader(encoded))
	if err != nil {
		return DraftResult{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-secsdk-csrf-token", csrf)
	response, err := j.client.Do(req)
	if err != nil {
		return DraftResult{}, err
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return DraftResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return DraftResult{}, classifyHTTP("juejin", operation, response.StatusCode, string(raw))
	}
	var decoded struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
		ErrNo  int    `json:"err_no"`
		ErrMsg string `json:"err_msg"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return DraftResult{}, platformError(ErrUpstream, "juejin", operation, response.StatusCode, "invalid JSON response", false)
	}
	if decoded.ErrNo != 0 {
		message := strings.TrimSpace(decoded.ErrMsg)
		lower := strings.ToLower(message)
		if operation == "update-draft" &&
			(strings.Contains(message, "不存在") || strings.Contains(lower, "not found") || strings.Contains(lower, "not exist")) {
			return DraftResult{}, platformError(ErrRemoteDraftMissing, "juejin", operation, response.StatusCode, message, false)
		}
		return DraftResult{}, platformError(ErrUpstream, "juejin", operation, response.StatusCode, message, false)
	}
	draftID := decoded.Data.ID
	if draftID == "" {
		draftID = id
	}
	if draftID == "" {
		return DraftResult{}, platformError(ErrUpstream, "juejin", operation, response.StatusCode, "response did not contain a draft id", false)
	}
	return DraftResult{
		ID:      draftID,
		URL:     juejinOrigin + "/editor/drafts/" + url.PathEscape(draftID),
		Created: operation == "create-draft",
		Updated: operation == "update-draft",
	}, nil
}

func (j *juejinAdapter) CreateDraft(ctx context.Context, input DraftInput) (DraftResult, error) {
	return j.mutateDraft(ctx, "/content_api/v1/article_draft/create", "create-draft", input, "", nil)
}

func (j *juejinAdapter) UpdateDraft(ctx context.Context, ref DraftRef, input DraftInput) (DraftResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return DraftResult{}, platformError(ErrValidation, "juejin", "update-draft", 0, "draft id is required", false)
	}
	detail, err := j.draftDetail(ctx, ref.ID)
	if err != nil {
		return DraftResult{}, err
	}
	return j.mutateDraft(ctx, "/content_api/v1/article_draft/update", "update-draft", input, ref.ID, &detail)
}

type juejinDraftArticle struct {
	ID         string           `json:"id"`
	ArticleID  string           `json:"article_id"`
	Title      string           `json:"title"`
	CategoryID string           `json:"category_id"`
	TagIDs     []juejinID       `json:"tag_ids"`
	LinkURL    string           `json:"link_url"`
	CoverImage string           `json:"cover_image"`
	IsGFW      int              `json:"is_gfw"`
	IsEnglish  int              `json:"is_english"`
	IsOriginal int              `json:"is_original"`
	EditType   int              `json:"edit_type"`
	ThemeIDs   []juejinID       `json:"theme_ids"`
	Pics       []map[string]any `json:"pics"`
}

type juejinDraftDetail struct {
	Data struct {
		DraftID      string             `json:"draft_id"`
		ArticleDraft juejinDraftArticle `json:"article_draft"`
		Columns      []struct {
			Column struct {
				ColumnID string `json:"column_id"`
			} `json:"column"`
		} `json:"columns"`
		ThemeList []struct {
			Theme struct {
				ThemeID string `json:"theme_id"`
			} `json:"theme"`
		} `json:"theme_list"`
	} `json:"data"`
	ErrNo  int    `json:"err_no"`
	ErrMsg string `json:"err_msg"`
}

func (j *juejinAdapter) draftDetail(ctx context.Context, draftID string) (juejinDraftDetail, error) {
	encoded, _ := json.Marshal(map[string]string{"draft_id": draftID})
	req, err := j.request(ctx, http.MethodPost, j.apiBase+"/content_api/v1/article_draft/detail", bytes.NewReader(encoded))
	if err != nil {
		return juejinDraftDetail{}, err
	}
	req.Header.Set("content-type", "application/json")
	csrf, err := j.csrf(ctx)
	if err != nil {
		return juejinDraftDetail{}, err
	}
	req.Header.Set("x-secsdk-csrf-token", csrf)
	response, err := j.client.Do(req)
	if err != nil {
		return juejinDraftDetail{}, err
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return juejinDraftDetail{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return juejinDraftDetail{}, classifyHTTP("juejin", "draft-detail", response.StatusCode, string(raw))
	}
	var decoded juejinDraftDetail
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return juejinDraftDetail{}, platformError(ErrUpstream, "juejin", "draft-detail", response.StatusCode, "invalid JSON response", false)
	}
	if decoded.ErrNo != 0 {
		message := strings.TrimSpace(decoded.ErrMsg)
		if strings.Contains(message, "不存在") || strings.Contains(strings.ToLower(message), "not found") {
			return juejinDraftDetail{}, platformError(ErrRemoteDraftMissing, "juejin", "draft-detail", response.StatusCode, message, false)
		}
		return juejinDraftDetail{}, platformError(ErrUpstream, "juejin", "draft-detail", response.StatusCode, message, false)
	}
	return decoded, nil
}

type juejinNamedID struct {
	ID   string
	Name string
}

func juejinTextID(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	case float64:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.0f", typed), ".0"), ".")
	default:
		return ""
	}
}

func collectJuejinNamedIDs(value any, idKey, nameKey string, result *[]juejinNamedID) {
	switch typed := value.(type) {
	case map[string]any:
		id := juejinTextID(typed[idKey])
		name, _ := typed[nameKey].(string)
		name = strings.TrimSpace(name)
		if id != "" && id != "0" && name != "" {
			*result = append(*result, juejinNamedID{ID: id, Name: name})
		}
		for _, child := range typed {
			collectJuejinNamedIDs(child, idKey, nameKey, result)
		}
	case []any:
		for _, child := range typed {
			collectJuejinNamedIDs(child, idKey, nameKey, result)
		}
	}
}

func dedupeJuejinNamedIDs(values []juejinNamedID) []juejinNamedID {
	result := make([]juejinNamedID, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if value.ID == "" {
			continue
		}
		if _, ok := seen[value.ID]; ok {
			continue
		}
		seen[value.ID] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (j *juejinAdapter) queryNamedIDs(
	ctx context.Context,
	path, operation string,
	payload map[string]any,
	idKey, nameKey string,
) ([]juejinNamedID, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := j.request(ctx, http.MethodPost, j.apiBase+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	response, err := j.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, classifyHTTP("juejin", operation, response.StatusCode, string(raw))
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, platformError(ErrUpstream, "juejin", operation, response.StatusCode, "invalid JSON response", false)
	}
	if errNo := juejinTextID(decoded["err_no"]); errNo != "" && errNo != "0" {
		message, _ := decoded["err_msg"].(string)
		return nil, platformError(ErrUpstream, "juejin", operation, response.StatusCode, strings.TrimSpace(message), false)
	}
	values := []juejinNamedID{}
	collectJuejinNamedIDs(decoded["data"], idKey, nameKey, &values)
	return dedupeJuejinNamedIDs(values), nil
}

func juejinCategoryPreference(input DraftInput) []string {
	corpus := strings.ToLower(input.Title + " " + strings.Join(input.Tags, " ") + " " + input.Description)
	switch {
	case strings.Contains(corpus, "人工智能") || strings.Contains(corpus, " ai ") ||
		strings.Contains(corpus, "llm") || strings.Contains(corpus, "agent"):
		return []string{"人工智能", "后端"}
	case strings.Contains(corpus, "前端") || strings.Contains(corpus, "javascript") ||
		strings.Contains(corpus, "typescript") || strings.Contains(corpus, "react") || strings.Contains(corpus, "vue"):
		return []string{"前端", "后端"}
	case strings.Contains(corpus, "android"):
		return []string{"Android", "后端"}
	case strings.Contains(corpus, "ios"):
		return []string{"iOS", "后端"}
	default:
		return []string{"后端", "开发工具"}
	}
}

func (j *juejinAdapter) resolvePublishMetadata(ctx context.Context, input DraftInput) (string, []string, error) {
	categories, err := j.queryNamedIDs(
		ctx, "/tag_api/v1/query_category_list", "category-list", map[string]any{},
		"category_id", "category_name",
	)
	if err != nil {
		return "", nil, err
	}
	categoryID := ""
	for _, preferred := range juejinCategoryPreference(input) {
		for _, category := range categories {
			if strings.EqualFold(strings.TrimSpace(category.Name), preferred) {
				categoryID = category.ID
				break
			}
		}
		if categoryID != "" {
			break
		}
	}
	if categoryID == "" && len(categories) > 0 {
		categoryID = categories[0].ID
	}
	if categoryID == "" {
		return "", nil, platformError(ErrValidation, "juejin", "publish-metadata", 0, "Juejin returned no usable article category", false)
	}

	keywords := make([]string, 0, len(input.Tags)+4)
	seenKeywords := map[string]struct{}{}
	addKeyword := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seenKeywords[key]; ok {
			return
		}
		seenKeywords[key] = struct{}{}
		keywords = append(keywords, value)
	}
	for _, tag := range input.Tags {
		addKeyword(tag)
	}
	for _, candidate := range []string{"Go", "Java", "Python", "Redis", "Linux", "Docker", "Kubernetes", "并发", "后端"} {
		if strings.Contains(strings.ToLower(input.Title+" "+input.Description), strings.ToLower(candidate)) {
			addKeyword(candidate)
		}
	}
	addKeyword("后端")

	tagIDs := []string{}
	seenTagIDs := map[string]struct{}{}
	for _, keyword := range keywords {
		tags, searchErr := j.queryNamedIDs(
			ctx, "/recommend_api/v1/tag/recommend/search", "tag-search",
			map[string]any{"page_no": 1, "page_size": 20, "key_word": keyword},
			"tag_id", "tag_name",
		)
		if searchErr != nil {
			tags, searchErr = j.queryNamedIDs(
				ctx, "/tag_api/v1/query_tag_list", "tag-list",
				map[string]any{"page_no": 1, "page_size": 20, "key_word": keyword},
				"tag_id", "tag_name",
			)
		}
		if searchErr != nil {
			continue
		}
		var chosen *juejinNamedID
		for index := range tags {
			if strings.EqualFold(tags[index].Name, keyword) {
				chosen = &tags[index]
				break
			}
		}
		if chosen == nil && len(tags) > 0 {
			chosen = &tags[0]
		}
		if chosen == nil {
			continue
		}
		if _, ok := seenTagIDs[chosen.ID]; ok {
			continue
		}
		seenTagIDs[chosen.ID] = struct{}{}
		tagIDs = append(tagIDs, chosen.ID)
		if len(tagIDs) >= 3 {
			break
		}
	}
	if len(tagIDs) == 0 {
		return "", nil, platformError(ErrValidation, "juejin", "publish-metadata", 0, "Juejin returned no usable article tag", false)
	}
	return categoryID, tagIDs, nil
}

func (j *juejinAdapter) repairPublishMetadata(
	ctx context.Context,
	ref DraftRef,
	input DraftInput,
	detail juejinDraftDetail,
) (juejinDraftDetail, error) {
	categoryID, tagIDs, err := j.resolvePublishMetadata(ctx, input)
	if err != nil {
		return juejinDraftDetail{}, err
	}
	repaired := detail
	repaired.Data.ArticleDraft.CategoryID = categoryID
	repaired.Data.ArticleDraft.TagIDs = make([]juejinID, 0, len(tagIDs))
	for _, id := range tagIDs {
		repaired.Data.ArticleDraft.TagIDs = append(repaired.Data.ArticleDraft.TagIDs, juejinID(id))
	}
	if _, err := j.mutateDraft(
		ctx, "/content_api/v1/article_draft/update", "update-draft", input, ref.ID, &repaired,
	); err != nil {
		return juejinDraftDetail{}, err
	}
	return j.draftDetail(ctx, ref.ID)
}

func (j *juejinAdapter) PublishDraft(ctx context.Context, ref DraftRef, input DraftInput) (PublishResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return PublishResult{}, platformError(ErrValidation, "juejin", "publish-draft", 0, "draft id is required", false)
	}
	detail, err := j.draftDetail(ctx, ref.ID)
	if err != nil {
		return PublishResult{}, err
	}
	article := detail.Data.ArticleDraft
	articleID := strings.TrimSpace(article.ArticleID)
	if articleID == "0" {
		articleID = ""
	}
	categoryID := strings.TrimSpace(article.CategoryID)
	if categoryID == "" || categoryID == "0" || len(juejinStringIDs(article.TagIDs)) == 0 {
		detail, err = j.repairPublishMetadata(ctx, ref, input, detail)
		if err != nil {
			return PublishResult{}, err
		}
		article = detail.Data.ArticleDraft
		categoryID = strings.TrimSpace(article.CategoryID)
		if categoryID == "" || categoryID == "0" || len(juejinStringIDs(article.TagIDs)) == 0 {
			return PublishResult{}, platformError(
				ErrValidation, "juejin", "publish-draft", 0,
				"Juejin draft still has no category or tag after metadata repair",
				false,
			)
		}
	}

	columnIDs := make([]string, 0, len(detail.Data.Columns))
	for _, item := range detail.Data.Columns {
		if id := strings.TrimSpace(item.Column.ColumnID); id != "" && id != "0" {
			columnIDs = append(columnIDs, id)
		}
	}
	themeIDs := juejinStringIDs(article.ThemeIDs)
	if len(themeIDs) == 0 {
		for _, item := range detail.Data.ThemeList {
			if id := strings.TrimSpace(item.Theme.ThemeID); id != "" && id != "0" {
				themeIDs = append(themeIDs, id)
			}
		}
	}
	body, _ := json.Marshal(map[string]any{
		"draft_id":    ref.ID,
		"sync_to_org": false,
		"column_ids":  columnIDs,
		"theme_ids":   themeIDs,
	})
	req, err := j.request(ctx, http.MethodPost, j.apiBase+"/content_api/v1/article/publish", bytes.NewReader(body))
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("content-type", "application/json")
	csrf, err := j.csrf(ctx)
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("x-secsdk-csrf-token", csrf)
	response, err := j.client.Do(req)
	if err != nil {
		return PublishResult{}, err
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return PublishResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return PublishResult{}, classifyHTTP("juejin", "publish-draft", response.StatusCode, string(raw))
	}
	var decoded struct {
		Data struct {
			ArticleID string `json:"article_id"`
		} `json:"data"`
		ErrNo  int    `json:"err_no"`
		ErrMsg string `json:"err_msg"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return PublishResult{}, platformError(ErrUpstream, "juejin", "publish-draft", response.StatusCode, "invalid JSON response", false)
	}
	if decoded.ErrNo != 0 {
		return PublishResult{}, platformError(ErrUpstream, "juejin", "publish-draft", response.StatusCode, decoded.ErrMsg, false)
	}
	articleID = decoded.Data.ArticleID
	if articleID == "" {
		verified, verifyErr := j.draftDetail(ctx, ref.ID)
		if verifyErr == nil {
			articleID = strings.TrimSpace(verified.Data.ArticleDraft.ArticleID)
			if articleID == "0" {
				articleID = ""
			}
		}
	}
	if articleID == "" {
		return PublishResult{}, platformError(ErrUpstream, "juejin", "publish-draft", response.StatusCode, "publish response did not contain an article id", false)
	}
	_ = input
	return PublishResult{ID: articleID, URL: juejinOrigin + "/post/" + url.PathEscape(articleID)}, nil
}

func shouldKeepJuejinImage(source string) bool {
	parsed, err := url.Parse(source)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	for _, fragment := range []string{"juejin.cn", "p1-juejin", "p3-juejin", "p6-juejin", "p9-juejin", "byteimg.com"} {
		if strings.Contains(host, fragment) {
			return true
		}
	}
	return false
}

func (j *juejinAdapter) prepareMarkdown(ctx context.Context, input DraftInput) (string, error) {
	return rehostMarkdownImages(ctx, j.client, input, ImageRehostOptions{
		Platform:       j.ID(),
		FailOpenRemote: true,
		AlreadyHosted:  shouldKeepJuejinImage,
	}, func(ctx context.Context, image RehostImage) (string, error) {
		return j.uploadImage(ctx, image.Payload, image.ContentType)
	})
}

func (j *juejinAdapter) imageToken(ctx context.Context) (imageXToken, error) {
	j.mu.Lock()
	if j.cachedImageToken != nil && j.now().Before(j.imageTokenUntil.Add(-time.Minute)) {
		value := *j.cachedImageToken
		j.mu.Unlock()
		return value, nil
	}
	j.mu.Unlock()

	query := url.Values{}
	query.Set("aid", juejinImageXAID)
	query.Set("uuid", j.uuid)
	query.Set("client", "web")
	rawURL := j.apiBase + "/imagex/v2/gen_token?" + query.Encode()
	req, err := j.request(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return imageXToken{}, err
	}
	req.Header.Set("content-type", "application/json")
	response, err := j.client.Do(req)
	if err != nil {
		return imageXToken{}, err
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return imageXToken{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return imageXToken{}, classifyHTTP("juejin", "image-token", response.StatusCode, string(raw))
	}
	var decoded struct {
		Data struct {
			Token struct {
				AccessKeyID     string `json:"AccessKeyId"`
				SecretAccessKey string `json:"SecretAccessKey"`
				SessionToken    string `json:"SessionToken"`
				ExpiredTime     string `json:"ExpiredTime"`
			} `json:"token"`
		} `json:"data"`
		ErrNo  int    `json:"err_no"`
		ErrMsg string `json:"err_msg"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return imageXToken{}, platformError(ErrUpload, "juejin", "image-token", response.StatusCode, "invalid JSON response", false)
	}
	if decoded.ErrNo != 0 || decoded.Data.Token.AccessKeyID == "" || decoded.Data.Token.SecretAccessKey == "" {
		return imageXToken{}, platformError(ErrUpload, "juejin", "image-token", response.StatusCode, decoded.ErrMsg, false)
	}
	expires, err := time.Parse(time.RFC3339, decoded.Data.Token.ExpiredTime)
	if err != nil {
		expires = j.now().Add(5 * time.Minute)
	}
	token := imageXToken{
		AccessKeyID:     decoded.Data.Token.AccessKeyID,
		SecretAccessKey: decoded.Data.Token.SecretAccessKey,
		SessionToken:    decoded.Data.Token.SessionToken,
		ExpiresAt:       expires,
	}
	j.mu.Lock()
	j.cachedImageToken = &token
	j.imageTokenUntil = expires
	j.mu.Unlock()
	return token, nil
}

type imageXUploadAddress struct {
	StoreInfos []struct {
		StoreURI string `json:"StoreUri"`
		Auth     string `json:"Auth"`
		UploadID string `json:"UploadID"`
	} `json:"StoreInfos"`
	UploadHosts []string `json:"UploadHosts"`
	SessionKey  string   `json:"SessionKey"`
}

func (j *juejinAdapter) signedImageXRequest(ctx context.Context, method, rawURL string, token imageXToken) (*http.Request, error) {
	headers, err := SignAWS4(method, rawURL, AWS4Credentials{
		AccessKeyID:     token.AccessKeyID,
		SecretAccessKey: token.SecretAccessKey,
		SecurityToken:   token.SessionToken,
		Region:          "cn-north-1",
		Service:         "imagex",
	}, j.now())
	if err != nil {
		return nil, err
	}
	req, err := j.request(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	return req, nil
}

func (j *juejinAdapter) applyImageUpload(ctx context.Context, token imageXToken) (imageXUploadAddress, error) {
	query := url.Values{}
	query.Set("Action", "ApplyImageUpload")
	query.Set("Version", "2018-08-01")
	query.Set("ServiceId", juejinImageXService)
	rawURL := j.imageXBase + "/?" + query.Encode()
	req, err := j.signedImageXRequest(ctx, http.MethodGet, rawURL, token)
	if err != nil {
		return imageXUploadAddress{}, err
	}
	response, err := j.client.Do(req)
	if err != nil {
		return imageXUploadAddress{}, err
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return imageXUploadAddress{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return imageXUploadAddress{}, classifyHTTP("juejin", "image-apply", response.StatusCode, string(raw))
	}
	var decoded struct {
		Result struct {
			UploadAddress imageXUploadAddress `json:"UploadAddress"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return imageXUploadAddress{}, platformError(ErrUpload, "juejin", "image-apply", response.StatusCode, "invalid JSON response", false)
	}
	if decoded.Result.UploadAddress.SessionKey == "" || len(decoded.Result.UploadAddress.StoreInfos) == 0 || len(decoded.Result.UploadAddress.UploadHosts) == 0 {
		return imageXUploadAddress{}, platformError(ErrUpload, "juejin", "image-apply", response.StatusCode, "invalid upload address", false)
	}
	return decoded.Result.UploadAddress, nil
}

func (j *juejinAdapter) uploadTOS(ctx context.Context, address imageXUploadAddress, payload []byte, contentType string) error {
	store := address.StoreInfos[0]
	host := address.UploadHosts[0]
	scheme := j.uploadScheme
	if scheme == "" {
		scheme = "https"
	}
	rawURL := scheme + "://" + host + "/" + strings.TrimPrefix(store.StoreURI, "/")
	req, err := j.request(ctx, http.MethodPut, rawURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("authorization", store.Auth)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("content-type", contentType)
	req.Header.Set("content-crc32", CRC32Hex(payload))
	response, err := j.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 1<<20)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return platformError(ErrUpload, "juejin", "image-upload", response.StatusCode, string(raw), true)
	}
	return nil
}

func (j *juejinAdapter) commitImageUpload(ctx context.Context, token imageXToken, sessionKey string) error {
	query := url.Values{}
	query.Set("Action", "CommitImageUpload")
	query.Set("Version", "2018-08-01")
	query.Set("SessionKey", sessionKey)
	query.Set("ServiceId", juejinImageXService)
	rawURL := j.imageXBase + "/?" + query.Encode()
	req, err := j.signedImageXRequest(ctx, http.MethodPost, rawURL, token)
	if err != nil {
		return err
	}
	req.Header.Set("content-length", "0")
	response, err := j.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return platformError(ErrUpload, "juejin", "image-commit", response.StatusCode, string(raw), true)
	}
	var decoded struct {
		Result json.RawMessage `json:"Result"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded.Result) == 0 || string(decoded.Result) == "null" {
		return platformError(ErrUpload, "juejin", "image-commit", response.StatusCode, "invalid commit response", false)
	}
	return nil
}

func (j *juejinAdapter) imageURL(ctx context.Context, storeURI string) (string, error) {
	query := url.Values{}
	query.Set("aid", juejinImageXAID)
	query.Set("uuid", j.uuid)
	query.Set("uri", storeURI)
	query.Set("img_type", "private")
	rawURL := j.apiBase + "/imagex/v2/get_img_url?" + query.Encode()
	req, err := j.request(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	response, err := j.client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", classifyHTTP("juejin", "image-url", response.StatusCode, string(raw))
	}
	var decoded struct {
		Data struct {
			MainURL   string `json:"main_url"`
			BackupURL string `json:"backup_url"`
		} `json:"data"`
		ErrNo  int    `json:"err_no"`
		ErrMsg string `json:"err_msg"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", platformError(ErrUpload, "juejin", "image-url", response.StatusCode, "invalid JSON response", false)
	}
	if decoded.ErrNo != 0 {
		return "", platformError(ErrUpload, "juejin", "image-url", response.StatusCode, decoded.ErrMsg, false)
	}
	if decoded.Data.MainURL != "" {
		return decoded.Data.MainURL, nil
	}
	if decoded.Data.BackupURL != "" {
		return decoded.Data.BackupURL, nil
	}
	return "", platformError(ErrUpload, "juejin", "image-url", response.StatusCode, "image URL missing", false)
}

func (j *juejinAdapter) uploadImage(ctx context.Context, payload []byte, contentType string) (string, error) {
	token, err := j.imageToken(ctx)
	if err != nil {
		return "", err
	}
	address, err := j.applyImageUpload(ctx, token)
	if err != nil {
		return "", err
	}
	if err := j.uploadTOS(ctx, address, payload, contentType); err != nil {
		return "", err
	}
	if err := j.commitImageUpload(ctx, token, address.SessionKey); err != nil {
		return "", err
	}
	storeURI := address.StoreInfos[0].StoreURI
	return j.imageURL(ctx, storeURI)
}

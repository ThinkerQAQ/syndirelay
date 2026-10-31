package publisher

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const cto51Origin = "https://blog.51cto.com"

var (
	cto51CSRFPattern = regexp.MustCompile(`(?i)<meta\s+name=["']csrf-token["']\s+content=["']([^"']+)["']`)
	cto51UserPattern = regexp.MustCompile(`https?://blog\.51cto\.com/([A-Za-z0-9_-]+)/?["']`)
)

type cto51Adapter struct {
	client    *http.Client
	userAgent string
	username  string
	csrf      string
}

func New51CTOAdapter(base *http.Client, session Session) (Adapter, error) {
	client, err := HTTPClientForSession(base, session)
	if err != nil {
		return nil, err
	}
	return &cto51Adapter{client: client, userAgent: session.UserAgent}, nil
}

func (c *cto51Adapter) ID() string { return "51cto" }

func (c *cto51Adapter) request(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	return browserRequest(ctx, method, rawURL, cto51Origin, cto51Origin+"/blogger/publish", c.userAgent, body)
}

func (c *cto51Adapter) CheckAuth(ctx context.Context) (AuthResult, error) {
	req, err := c.request(ctx, http.MethodGet, cto51Origin+"/blogger/publish", nil)
	if err != nil {
		return AuthResult{}, err
	}
	response, err := c.client.Do(req)
	if err != nil {
		return AuthResult{}, platformError(ErrUpstream, c.ID(), "auth", 0, err.Error(), true)
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 4<<20)
	if err != nil {
		return AuthResult{}, err
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return AuthResult{Authenticated: false}, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return AuthResult{}, classifyHTTP(c.ID(), "auth", response.StatusCode, string(raw))
	}
	html := string(raw)
	if match := cto51CSRFPattern.FindStringSubmatch(html); len(match) == 2 {
		c.csrf = strings.TrimSpace(match[1])
	}
	for _, match := range cto51UserPattern.FindAllStringSubmatch(html, -1) {
		if len(match) != 2 {
			continue
		}
		candidate := strings.TrimSpace(match[1])
		switch candidate {
		case "", "blogger", "creative-center", "index", "home", "search", "topic", "column":
			continue
		default:
			c.username = candidate
			break
		}
		if c.username != "" {
			break
		}
	}
	if c.username == "" {
		return AuthResult{Authenticated: false}, nil
	}
	return AuthResult{Authenticated: true, UserID: c.username, Username: c.username}, nil
}

func (c *cto51Adapter) ensureAuth(ctx context.Context) error {
	if c.username != "" && c.csrf != "" {
		return nil
	}
	auth, err := c.CheckAuth(ctx)
	if err != nil {
		return err
	}
	if !auth.Authenticated {
		return platformError(ErrAuthExpired, c.ID(), "auth", http.StatusUnauthorized, "browser session is not authenticated", false)
	}
	return nil
}

func (c *cto51Adapter) uploadSign(ctx context.Context) (string, error) {
	values := url.Values{}
	values.Set("upload_type", "image")
	req, err := c.request(ctx, http.MethodPost, cto51Origin+"/getUploadSign", strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("x-requested-with", "XMLHttpRequest")
	var decoded struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Sign string `json:"sign"`
		} `json:"data"`
	}
	if err := doJSON(c.client, req, c.ID(), "image-sign", &decoded); err != nil {
		return "", err
	}
	if decoded.Code != 0 || decoded.Data.Sign == "" {
		return "", platformError(ErrUpload, c.ID(), "image-sign", decoded.Code, responseMessage(decoded.Msg), false)
	}
	return decoded.Data.Sign, nil
}

type cto51UploadConfig struct {
	URL    string `json:"url"`
	Fields struct {
		Key        string `json:"key"`
		Policy     string `json:"policy"`
		Algorithm  string `json:"x-amz-algorithm"`
		Signature  string `json:"x-amz-signature"`
		Credential string `json:"x-amz-credential"`
		Date       string `json:"X-Amz-Date"`
	} `json:"fields"`
}

func (c *cto51Adapter) uploadConfig(ctx context.Context, sign, contentType, filename string) (cto51UploadConfig, error) {
	values := url.Values{}
	values.Set("upload_type", "image")
	values.Set("upload_sign", sign)
	values.Set("ext", contentType)
	values.Set("name", filename)
	req, err := c.request(ctx, http.MethodPost, cto51Origin+"/getUploadConfig", strings.NewReader(values.Encode()))
	if err != nil {
		return cto51UploadConfig{}, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("x-requested-with", "XMLHttpRequest")
	var decoded struct {
		Code int               `json:"code"`
		Msg  string            `json:"msg"`
		Data cto51UploadConfig `json:"data"`
	}
	if err := doJSON(c.client, req, c.ID(), "image-config", &decoded); err != nil {
		return cto51UploadConfig{}, err
	}
	if decoded.Code != 0 || decoded.Data.URL == "" || decoded.Data.Fields.Key == "" {
		return cto51UploadConfig{}, platformError(ErrUpload, c.ID(), "image-config", decoded.Code, responseMessage(decoded.Msg), false)
	}
	return decoded.Data, nil
}

func (c *cto51Adapter) uploadImage(ctx context.Context, image RehostImage) (string, error) {
	filename := inferImageFilename(image.Source, image.ContentType)
	sign, err := c.uploadSign(ctx)
	if err != nil {
		return "", err
	}
	config, err := c.uploadConfig(ctx, sign, image.ContentType, filename)
	if err != nil {
		return "", err
	}
	fields := map[string]string{
		"key":              config.Fields.Key,
		"policy":           config.Fields.Policy,
		"x-amz-algorithm":  config.Fields.Algorithm,
		"x-amz-signature":  config.Fields.Signature,
		"x-amz-credential": config.Fields.Credential,
		"X-Amz-Date":       config.Fields.Date,
		"Content-Type":     image.ContentType,
	}
	body, bodyType, err := multipartBody(fields, "file", filename, image.ContentType, image.Payload)
	if err != nil {
		return "", err
	}
	req, err := c.request(ctx, http.MethodPost, config.URL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", bodyType)
	response, err := c.client.Do(req)
	if err != nil {
		return "", platformError(ErrUpload, c.ID(), "image-upload", 0, err.Error(), true)
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 1<<20)
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", platformError(ErrUpload, c.ID(), "image-upload", response.StatusCode, string(raw), true)
	}
	return "https://s2.51cto.com/" + strings.TrimPrefix(config.Fields.Key, "/"), nil
}

func (c *cto51Adapter) prepareMarkdown(ctx context.Context, input DraftInput) (string, error) {
	return rehostMarkdownImages(ctx, c.client, input, ImageRehostOptions{
		Platform:       c.ID(),
		FailOpenRemote: true,
		AlreadyHosted: func(source string) bool {
			return strings.Contains(strings.ToLower(source), "51cto.com")
		},
	}, c.uploadImage)
}

func cto51AppendImageURLs(values url.Values, content string) {
	for _, source := range imageSources(content) {
		parsed, err := url.Parse(strings.TrimSpace(source))
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			continue
		}
		values.Add("img_urls[]", source)
	}
}

const cto51MaxTags = 5

func normalizeCTO51Tags(tags []string) []string {
	result := make([]string, 0, min(len(tags), cto51MaxTags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		value := strings.TrimSpace(tag)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) == cto51MaxTags {
			break
		}
	}
	return result
}

type cto51Category struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Item []cto51Category `json:"item"`
}

type cto51UserCategory struct {
	CustomID string `json:"custom_id"`
	Name     string `json:"name"`
}

func normalizeCTO51CategoryName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func cto51CategoryMatches(tag, category string) bool {
	tag = normalizeCTO51CategoryName(tag)
	category = normalizeCTO51CategoryName(category)
	if tag == "" || category == "" {
		return false
	}
	return tag == category || strings.TrimSuffix(category, "语言") == tag
}

func selectCTO51Category(tags []string, categories []cto51Category) (string, string, string, error) {
	for _, tag := range normalizeCTO51Tags(tags) {
		for _, parent := range categories {
			for _, child := range parent.Item {
				if cto51CategoryMatches(tag, child.Name) {
					return parent.ID, child.ID, child.Name, nil
				}
			}
		}
	}
	// 51CTO's live taxonomy uses 后端开发 as a *primary* category. Its
	// secondary categories include 架构, Java, Go语言 and Python; there is
	// no secondary category literally named 后端. Resolve a real child ID
	// from the catalog instead of guessing any numeric IDs.
	for _, parent := range categories {
		switch normalizeCTO51CategoryName(parent.Name) {
		case "后端", "后端开发", "backend":
		default:
			continue
		}
		for _, fallback := range []string{"架构", "java", "go语言", "python"} {
			for _, child := range parent.Item {
				if normalizeCTO51CategoryName(child.Name) == fallback &&
					strings.TrimSpace(parent.ID) != "" && strings.TrimSpace(child.ID) != "" {
					return parent.ID, child.ID, child.Name, nil
				}
			}
		}
	}
	return "", "", "", fmt.Errorf("51CTO category catalog has no usable 后端开发 secondary category (架构/Java/Go语言/Python)")
}

// When no existing article tag matches the selected 51CTO secondary category,
// keep the user's requested generic 后端 tag. A secondary *category* ID is
// supplied independently in cate_id and must come from the live catalog.
func cto51ResolvedTags(tags []string, secondaryCategory string) []string {
	normalized := normalizeCTO51Tags(tags)
	for _, tag := range normalized {
		if cto51CategoryMatches(tag, secondaryCategory) {
			return normalized
		}
	}
	return []string{"后端"}
}

func selectCTO51UserCategory(title string, tags []string, categories []cto51UserCategory) (string, string) {
	if len(categories) == 1 {
		return strings.TrimSpace(categories[0].CustomID), strings.TrimSpace(categories[0].Name)
	}
	normalizedTitle := normalizeCTO51CategoryName(title)
	for _, category := range categories {
		name := normalizeCTO51CategoryName(category.Name)
		if name != "" && strings.Contains(normalizedTitle, name) {
			return strings.TrimSpace(category.CustomID), strings.TrimSpace(category.Name)
		}
		for _, tag := range tags {
			if normalizeCTO51CategoryName(tag) == name {
				return strings.TrimSpace(category.CustomID), strings.TrimSpace(category.Name)
			}
		}
	}
	return "", ""
}

func (c *cto51Adapter) publishingClassification(ctx context.Context, input DraftInput) (string, string, string, string, error) {
	started := time.Now()
	categoryReq, err := c.request(ctx, http.MethodGet, cto51Origin+"/category/get-child", nil)
	if err != nil {
		return "", "", "", "", err
	}
	categoryReq.Header.Set("accept", "application/json, text/plain, */*")
	categoryReq.Header.Set("x-requested-with", "XMLHttpRequest")
	var categoryResponse struct {
		Status int             `json:"status"`
		Msg    string          `json:"msg"`
		Data   []cto51Category `json:"data"`
	}
	if err := doJSON(c.client, categoryReq, c.ID(), "category-list", &categoryResponse); err != nil {
		return "", "", "", "", err
	}
	if categoryResponse.Status != 1 {
		return "", "", "", "", platformError(ErrUpstream, c.ID(), "category-list", 0, responseMessage(categoryResponse.Msg), false)
	}
	parentID, categoryID, categoryName, err := selectCTO51Category(input.Tags, categoryResponse.Data)
	if err != nil {
		slog.WarnContext(ctx, "51CTO category resolution found no match",
			"node", "51cto-adapter", "operation", "resolve-classification", "result", "no-category-match",
			"tagCount", len(input.Tags), "durationMs", time.Since(started).Milliseconds())
		return "", "", "", "", platformError(ErrValidation, c.ID(), "resolve-classification", 0, err.Error(), false)
	}

	userCategoryReq, err := c.request(ctx, http.MethodGet, cto51Origin+"/blogger-ajax/get-user-cate", nil)
	if err != nil {
		return "", "", "", "", err
	}
	userCategoryReq.Header.Set("accept", "application/json, text/plain, */*")
	userCategoryReq.Header.Set("x-requested-with", "XMLHttpRequest")
	var userCategoryResponse struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   struct {
			Custom []cto51UserCategory `json:"custom"`
		} `json:"data"`
	}
	if err := doJSON(c.client, userCategoryReq, c.ID(), "user-category-list", &userCategoryResponse); err != nil {
		return "", "", "", "", err
	}
	if userCategoryResponse.Status != 1 {
		return "", "", "", "", platformError(ErrUpstream, c.ID(), "user-category-list", 0, responseMessage(userCategoryResponse.Msg), false)
	}
	customID, customName := selectCTO51UserCategory(input.Title, input.Tags, userCategoryResponse.Data.Custom)
	slog.InfoContext(ctx, "51CTO publishing classification resolved",
		"node", "51cto-adapter", "operation", "resolve-classification", "result", "success",
		"parentCategoryId", parentID, "secondaryCategoryId", categoryID, "secondaryCategory", categoryName,
		"customCategoryId", customID, "customCategory", customName, "durationMs", time.Since(started).Milliseconds())
	return parentID, categoryID, customID, categoryName, nil
}

func (c *cto51Adapter) draftFields(ctx context.Context, refID string, input DraftInput) (url.Values, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, err
	}
	parentID, categoryID, customID, selectedCategory, err := c.publishingClassification(ctx, input)
	if err != nil {
		return nil, err
	}
	content, err := c.prepareMarkdown(ctx, input)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("title", input.Title)
	values.Set("content", content)
	values.Set("pid", parentID)
	values.Set("cate_id", categoryID)
	values.Set("custom_id", customID)
	tags := cto51ResolvedTags(input.Tags, selectedCategory)
	values.Set("tag", strings.Join(tags, ","))
	operation := "create-draft"
	if refID != "" {
		operation = "update-draft"
	}
	slog.InfoContext(ctx, "51CTO draft tags prepared",
		"node", "51cto-adapter", "operation", operation, "result", "success",
		"inputTagCount", len(input.Tags), "tagCount", len(tags), "truncated", len(tags) < len(input.Tags))
	values.Set("abstract", truncateRunes(input.Description, 200))
	values.Set("banner_type", "0")
	values.Set("blog_type", "1")
	values.Set("copy_code", "1")
	values.Set("is_hide", "0")
	values.Set("top_time", "0")
	values.Set("is_comment", "0")
	values.Set("is_old", "0")
	values.Set("blog_id", "")
	values.Set("did", refID)
	values.Set("work_id", "")
	values.Set("class_id", "")
	values.Set("subjectId", "")
	values.Set("import_type", "-1")
	values.Set("invite_code", "")
	values.Set("raffle", "")
	values.Set("orig", "")
	values.Set("_csrf", c.csrf)
	cto51AppendImageURLs(values, content)
	return values, nil
}

func (c *cto51Adapter) saveDraft(ctx context.Context, refID string, input DraftInput) (DraftResult, error) {
	values, err := c.draftFields(ctx, refID, input)
	if err != nil {
		return DraftResult{}, err
	}
	req, err := c.request(ctx, http.MethodPost, cto51Origin+"/blogger/draft", strings.NewReader(values.Encode()))
	if err != nil {
		return DraftResult{}, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("x-requested-with", "XMLHttpRequest")
	var decoded struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   struct {
			DID any `json:"did"`
		} `json:"data"`
	}
	if err := doJSON(c.client, req, c.ID(), "save-draft", &decoded); err != nil {
		return DraftResult{}, err
	}
	if decoded.Status != 1 {
		message := responseMessage(decoded.Msg)
		if refID != "" && (strings.Contains(message, "不存在") || strings.Contains(strings.ToLower(message), "not found")) {
			return DraftResult{}, platformError(ErrRemoteDraftMissing, c.ID(), "update-draft", 0, message, false)
		}
		return DraftResult{}, platformError(ErrUpstream, c.ID(), "save-draft", 0, message, false)
	}
	id := valueString(decoded.Data.DID)
	if id == "" {
		id = refID
	}
	if id == "" {
		return DraftResult{}, platformError(ErrUpstream, c.ID(), "save-draft", 0, "response did not contain a draft id", false)
	}
	return DraftResult{
		ID:      id,
		URL:     cto51Origin + "/blogger/draft/" + url.PathEscape(id),
		Created: refID == "",
		Updated: refID != "",
	}, nil
}

func (c *cto51Adapter) CreateDraft(ctx context.Context, input DraftInput) (DraftResult, error) {
	return c.saveDraft(ctx, "", input)
}

func (c *cto51Adapter) UpdateDraft(ctx context.Context, ref DraftRef, input DraftInput) (DraftResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return DraftResult{}, platformError(ErrValidation, c.ID(), "update-draft", 0, "draft id is required", false)
	}
	return c.saveDraft(ctx, ref.ID, input)
}

func (c *cto51Adapter) PublishDraft(ctx context.Context, ref DraftRef, input DraftInput) (PublishResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return PublishResult{}, platformError(ErrValidation, c.ID(), "publish-draft", 0, "draft id is required", false)
	}
	if err := c.ensureAuth(ctx); err != nil {
		return PublishResult{}, err
	}
	parentID, categoryID, customID, selectedCategory, err := c.publishingClassification(ctx, input)
	if err != nil {
		return PublishResult{}, err
	}
	content, err := c.prepareMarkdown(ctx, input)
	if err != nil {
		return PublishResult{}, err
	}
	values := url.Values{}
	values.Set("title", input.Title)
	values.Set("content", content)
	values.Set("pid", parentID)
	values.Set("cate_id", categoryID)
	values.Set("custom_id", customID)
	tags := cto51ResolvedTags(input.Tags, selectedCategory)
	values.Set("tag", strings.Join(tags, ","))
	slog.InfoContext(ctx, "51CTO publish tags prepared",
		"node", "51cto-adapter", "operation", "publish-draft", "result", "success",
		"inputTagCount", len(input.Tags), "tagCount", len(tags), "truncated", len(tags) < len(input.Tags))
	values.Set("abstract", truncateRunes(input.Description, 200))
	values.Set("banner_type", "0")
	values.Set("blog_type", "1")
	values.Set("copy_code", "1")
	values.Set("is_hide", "0")
	values.Set("top_time", "0")
	values.Set("is_comment", "0")
	values.Set("is_old", "0")
	values.Set("blog_id", "")
	values.Set("did", ref.ID)
	values.Set("work_id", "")
	values.Set("class_id", "")
	values.Set("subjectId", "")
	values.Set("import_type", "")
	values.Set("invite_code", "")
	values.Set("raffle", "")
	values.Set("orig", "")
	values.Set("_csrf", c.csrf)
	values.Set("check", "1")
	cto51AppendImageURLs(values, content)

	req, err := c.request(ctx, http.MethodPost, cto51Origin+"/blogger/publish", strings.NewReader(values.Encode()))
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("x-requested-with", "XMLHttpRequest")
	var decoded struct {
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Data   struct {
			BlogID  any    `json:"blog_id"`
			Request string `json:"request"`
		} `json:"data"`
	}
	if err := doJSON(c.client, req, c.ID(), "publish-draft", &decoded); err != nil {
		return PublishResult{}, err
	}
	if decoded.Status != 1 {
		return PublishResult{}, platformError(ErrUpstream, c.ID(), "publish-draft", 0, responseMessage(decoded.Msg), false)
	}
	blogID := valueString(decoded.Data.BlogID)
	if blogID == "" {
		return PublishResult{}, platformError(ErrUpstream, c.ID(), "publish-draft", 0, "response did not contain a public article id", false)
	}
	return PublishResult{ID: blogID, URL: cto51Origin + "/" + url.PathEscape(c.username) + "/" + url.PathEscape(blogID)}, nil
}

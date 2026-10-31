package publisher

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const cnBlogsOrigin = "https://i.cnblogs.com"

type cnBlogsAdapter struct {
	client    *http.Client
	session   Session
	userAgent string
	xsrf      string
	username  string
}

func NewCNBlogsAdapter(base *http.Client, session Session) (Adapter, error) {
	client, err := HTTPClientForSession(base, session)
	if err != nil {
		return nil, err
	}
	return &cnBlogsAdapter{client: client, session: session, userAgent: session.UserAgent}, nil
}

func (c *cnBlogsAdapter) ID() string { return "cnblogs" }

func cnBlogsEditorSessionID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	hexed := hex.EncodeToString(raw)
	return hexed[:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" + hexed[16:20] + "-" + hexed[20:32]
}

func (c *cnBlogsAdapter) request(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	req, err := browserRequest(ctx, method, rawURL, cnBlogsOrigin, cnBlogsOrigin+"/", c.userAgent, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json, text/plain, */*")
	if req.URL.Hostname() == "i.cnblogs.com" && c.session.RequestCookieHeader != "" {
		req.Header.Set("Cookie", c.session.RequestCookieHeader)
	}
	if header := strings.TrimSpace(c.session.RequestCookieHeaders[req.URL.Hostname()]); header != "" {
		req.Header.Set("Cookie", header)
	}
	return req, nil
}

func (c *cnBlogsAdapter) CheckAuth(ctx context.Context) (AuthResult, error) {
	req, err := c.request(ctx, http.MethodGet, cnBlogsOrigin+"/api/user", nil)
	if err != nil {
		return AuthResult{}, err
	}
	cookieNames := []string{}
	for _, cookie := range req.Cookies() {
		cookieNames = append(cookieNames, cookie.Name)
	}
	if c.client.Jar != nil {
		for _, cookie := range c.client.Jar.Cookies(req.URL) {
			cookieNames = append(cookieNames, cookie.Name)
		}
	}
	started := time.Now()
	response, err := c.client.Do(req)
	if err != nil {
		slog.Warn("cnblogs auth request failed", "operation", "auth", "endpoint", "/api/user", "cookieNames", cookieNames, "durationMs", time.Since(started).Milliseconds(), "error", err)
		return AuthResult{}, platformError(ErrUpstream, c.ID(), "auth", 0, err.Error(), true)
	}
	slog.Info("cnblogs auth response", "operation", "auth", "endpoint", "/api/user", "cookieNames", cookieNames, "status", response.StatusCode, "durationMs", time.Since(started).Milliseconds())
	defer response.Body.Close()
	raw, err := readBounded(response, 2<<20)
	if err != nil {
		return AuthResult{}, err
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return AuthResult{}, platformError(ErrAuthExpired, c.ID(), "auth", response.StatusCode,
			"unauthenticated /api/user (status "+strconv.Itoa(response.StatusCode)+")", false)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return AuthResult{}, classifyHTTP(c.ID(), "auth", response.StatusCode, string(raw))
	}
	var decoded struct {
		LoginName   string `json:"loginName"`
		DisplayName string `json:"displayName"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return AuthResult{}, platformError(ErrUpstream, c.ID(), "auth", response.StatusCode, "invalid JSON response", false)
	}
	if strings.TrimSpace(decoded.LoginName) == "" {
		return AuthResult{}, platformError(ErrAuthExpired, c.ID(), "auth", response.StatusCode,
			"no loginName in /api/user response", false)
	}
	c.username = decoded.LoginName
	username := strings.TrimSpace(decoded.DisplayName)
	if username == "" {
		username = decoded.LoginName
	}
	return AuthResult{Authenticated: true, UserID: decoded.LoginName, Username: username}, nil
}

func cnBlogsCookieHeaderValue(header, name string) string {
	for _, pair := range strings.Split(header, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func decodeCNBlogsXSRF(token string) string {
	token = strings.TrimSpace(token)
	if decoded, err := url.QueryUnescape(token); err == nil && decoded != "" {
		return strings.TrimSpace(decoded)
	}
	return token
}

func (c *cnBlogsAdapter) xsrfToken(ctx context.Context) (string, error) {
	if c.xsrf != "" {
		return c.xsrf, nil
	}
	token := cookieValue(c.session, "XSRF-TOKEN")
	if token == "" {
		token = cnBlogsCookieHeaderValue(c.session.RequestCookieHeader, "XSRF-TOKEN")
	}
	if token = decodeCNBlogsXSRF(token); token != "" {
		c.xsrf = token
		return token, nil
	}

	req, err := c.request(ctx, http.MethodGet, cnBlogsOrigin+"/posts/edit", nil)
	if err != nil {
		return "", err
	}
	response, err := c.client.Do(req)
	if err != nil {
		return "", platformError(ErrUpstream, c.ID(), "xsrf", 0, err.Error(), true)
	}
	defer response.Body.Close()
	_, _ = readBounded(response, 1<<20)
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return "", classifyHTTP(c.ID(), "xsrf", response.StatusCode, "")
	}
	for _, cookie := range response.Cookies() {
		if cookie.Name == "XSRF-TOKEN" {
			token = decodeCNBlogsXSRF(cookie.Value)
			break
		}
	}
	if token == "" {
		return "", platformError(ErrCSRF, c.ID(), "xsrf", response.StatusCode, "XSRF-TOKEN cookie is missing", false)
	}
	c.xsrf = token
	return token, nil
}

func cnBlogsUploadedImageURL(value any) string {
	switch typed := value.(type) {
	case string:
		candidate := strings.TrimSpace(typed)
		parsed, err := url.Parse(candidate)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return ""
		}
		host := strings.ToLower(parsed.Hostname())
		if host == "cnblogs.com" || strings.HasSuffix(host, ".cnblogs.com") {
			return candidate
		}
	case map[string]any:
		for _, key := range []string{"url", "imageUrl", "imgUrl", "src", "message", "data", "result"} {
			if target := cnBlogsUploadedImageURL(typed[key]); target != "" {
				return target
			}
		}
	case []any:
		for _, item := range typed {
			if target := cnBlogsUploadedImageURL(item); target != "" {
				return target
			}
		}
	}
	return ""
}

func (c *cnBlogsAdapter) uploadImageRequest(ctx context.Context, endpoint string, fields map[string]string, fileField string, payload []byte, filename, contentType, token string) (string, error) {
	body, bodyType, err := multipartBody(fields, fileField, filename, contentType, payload)
	if err != nil {
		return "", err
	}
	req, err := c.request(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", bodyType)
	req.Header.Set("x-xsrf-token", token)
	var decoded any
	if err := doJSON(c.client, req, c.ID(), "image-upload", &decoded); err != nil {
		return "", err
	}
	if target := cnBlogsUploadedImageURL(decoded); target != "" {
		return target, nil
	}
	return "", platformError(ErrUpload, c.ID(), "image-upload", 0, "image URL missing", false)
}

func (c *cnBlogsAdapter) uploadImage(ctx context.Context, image RehostImage) (string, error) {
	token, err := c.xsrfToken(ctx)
	if err != nil {
		return "", err
	}
	filename := inferImageFilename(image.Source, image.ContentType)

	return c.uploadImageRequest(
		ctx,
		"https://upload.cnblogs.com/v2/images/cors-upload",
		map[string]string{
			"app":        "blog",
			"uploadType": "Select",
		},
		"image",
		image.Payload,
		filename,
		image.ContentType,
		token,
	)
}

func (c *cnBlogsAdapter) prepareMarkdown(ctx context.Context, input DraftInput) (string, error) {
	return rehostMarkdownImages(ctx, c.client, input, ImageRehostOptions{
		Platform:       c.ID(),
		FailOpenRemote: true,
		AlreadyHosted: func(source string) bool {
			return strings.Contains(strings.ToLower(source), "cnblogs.com")
		},
	}, c.uploadImage)
}

func cnBlogsPayload(id string, input DraftInput, body string, publish bool) map[string]any {
	var idValue any
	if strings.TrimSpace(id) != "" {
		idValue = id
	}
	var description any
	if strings.TrimSpace(input.Description) != "" {
		description = input.Description
	}
	return map[string]any{
		"id":                                  idValue,
		"postType":                            1,
		"accessPermission":                    0,
		"title":                               input.Title,
		"url":                                 nil,
		"postBody":                            body,
		"categoryIds":                         nil,
		"categories":                          nil,
		"collectionIds":                       []any{},
		"inSiteCandidate":                     false,
		"inSiteHome":                          true,
		"siteCategoryId":                      nil,
		"blogTeamIds":                         nil,
		"isPublished":                         publish,
		"displayOnHomePage":                   true,
		"isAllowComments":                     true,
		"includeInMainSyndication":            true,
		"isPinned":                            false,
		"showBodyWhenPinned":                  false,
		"isOnlyForRegisterUser":               false,
		"isUpdateDateAdded":                   false,
		"entryName":                           nil,
		"description":                         description,
		"featuredImage":                       nil,
		"tags":                                nil,
		"password":                            nil,
		"publishAt":                           nil,
		"datePublished":                       time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"dateUpdated":                         nil,
		"isMarkdown":                          true,
		"isDraft":                             !publish,
		"isAigc":                              false,
		"autoDesc":                            nil,
		"changePostType":                      false,
		"blogId":                              0,
		"author":                              nil,
		"removeScript":                        false,
		"clientInfo":                          nil,
		"changeCreatedTime":                   false,
		"canChangeCreatedTime":                false,
		"isContributeToImpressiveBugActivity": false,
		"usingEditorId":                       5,
		"sourceUrl":                           nil,
	}
}

// fetchPost 读取 CNBlogs 已有草稿的服务端字段。更新草稿时必须先以此结果为基础，
// 以保留 url、datePublished、dateUpdated、blogId、author、autoDesc 等浏览器保存时沿用的字段。
// 草稿不存在时 doJSON 会把 404 归类为 ErrRemoteDraftMissing，调用方直接向上抛出。
func (c *cnBlogsAdapter) fetchPost(ctx context.Context, id string) (map[string]any, error) {
	req, err := c.request(ctx, http.MethodGet, cnBlogsOrigin+"/api/posts/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	var decoded struct {
		BlogPost map[string]any `json:"blogPost"`
	}
	if err := doJSON(c.client, req, c.ID(), "get-post", &decoded); err != nil {
		return nil, err
	}
	if len(decoded.BlogPost) == 0 {
		return nil, platformError(ErrRemoteDraftMissing, c.ID(), "get-post", 0, "draft not found", false)
	}
	return decoded.BlogPost, nil
}

// cnBlogsUpdatePayload 以服务端 blogPost 为基础构造更新 payload：保留服务端 url、
// datePublished、dateUpdated、blogId、author、autoDesc、displayOnHomePage 等原值，
// 仅覆盖本次明确要编辑的标题、正文、描述与草稿／发布状态，并把 usingEditorId 补为编辑器 id。
func cnBlogsUpdatePayload(id string, input DraftInput, body string, publish bool, base map[string]any) map[string]any {
	payload := make(map[string]any, len(base)+6)
	for key, value := range base {
		payload[key] = value
	}
	if payload["id"] == nil {
		payload["id"] = id
	}
	payload["title"] = input.Title
	payload["postBody"] = body
	payload["description"] = input.Description
	payload["isPublished"] = publish
	payload["isDraft"] = !publish
	payload["usingEditorId"] = 5
	return payload
}

func (c *cnBlogsAdapter) save(ctx context.Context, refID string, input DraftInput, publish, prepareImages bool) (map[string]any, error) {
	token, err := c.xsrfToken(ctx)
	if err != nil {
		return nil, err
	}
	bodyText := input.Markdown
	if prepareImages {
		bodyText, err = c.prepareMarkdown(ctx, input)
		if err != nil {
			return nil, err
		}
	}
	payloadValue := cnBlogsPayload(refID, input, bodyText, publish)
	if strings.TrimSpace(refID) != "" {
		base, err := c.fetchPost(ctx, refID)
		if err != nil {
			return nil, err
		}
		payloadValue = cnBlogsUpdatePayload(refID, input, bodyText, publish, base)
	}
	payload, _ := json.Marshal(payloadValue)
	req, err := c.request(ctx, http.MethodPost, cnBlogsOrigin+"/api/posts", strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-xsrf-token", token)
	req.Header.Set("sessionId", cnBlogsEditorSessionID())
	var decoded map[string]any
	if err := doJSON(c.client, req, c.ID(), map[bool]string{true: "publish-draft", false: "save-draft"}[publish], &decoded); err != nil {
		return nil, err
	}
	if valueString(decoded["id"]) == "" && refID == "" {
		return nil, platformError(ErrUpstream, c.ID(), "save-draft", 0, responseMessage(decoded["error"], decoded["message"]), false)
	}
	return decoded, nil
}

func (c *cnBlogsAdapter) CreateDraft(ctx context.Context, input DraftInput) (DraftResult, error) {
	decoded, err := c.save(ctx, "", input, false, true)
	if err != nil {
		return DraftResult{}, err
	}
	id := valueString(decoded["id"])
	return DraftResult{ID: id, URL: CNBlogsDraftEditorURL(id), Created: true}, nil
}

func (c *cnBlogsAdapter) UpdateDraft(ctx context.Context, ref DraftRef, input DraftInput) (DraftResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return DraftResult{}, platformError(ErrValidation, c.ID(), "update-draft", 0, "draft id is required", false)
	}
	decoded, err := c.save(ctx, ref.ID, input, false, true)
	if err != nil {
		if IsKind(err, ErrRemoteDraftMissing) {
			return DraftResult{}, err
		}
		return DraftResult{}, err
	}
	id := valueString(decoded["id"])
	if id == "" {
		id = ref.ID
	}
	return DraftResult{ID: id, URL: CNBlogsDraftEditorURL(id), Updated: true}, nil
}

func (c *cnBlogsAdapter) PublishDraft(ctx context.Context, ref DraftRef, input DraftInput) (PublishResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return PublishResult{}, platformError(ErrValidation, c.ID(), "publish-draft", 0, "draft id is required", false)
	}
	decoded, err := c.save(ctx, ref.ID, input, true, true)
	if err != nil {
		return PublishResult{}, err
	}
	for _, key := range []string{"url", "postUrl", "post_url"} {
		if target := valueString(decoded[key]); target != "" {
			if strings.HasPrefix(target, "//") {
				target = "https:" + target
			}
			if strings.HasPrefix(target, "/") {
				target = "https://www.cnblogs.com" + target
			}
			return PublishResult{ID: ref.ID, URL: target}, nil
		}
	}
	if c.username == "" {
		_, _ = c.CheckAuth(ctx)
	}
	if c.username == "" {
		return PublishResult{}, platformError(ErrUpstream, c.ID(), "publish-draft", 0, "published response did not include a public URL", false)
	}
	return PublishResult{ID: ref.ID, URL: "https://www.cnblogs.com/" + url.PathEscape(c.username) + "/p/" + url.PathEscape(ref.ID) + ".html"}, nil
}

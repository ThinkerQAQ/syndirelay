package publisher

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

const (
	csdnOrigin = "https://editor.csdn.net"
	csdnAPI    = "https://bizapi.csdn.net"
	csdnKey    = "203803574"
	csdnSecret = "9znpamsyl2c7cdrr9sas0le9vbc3r6ba"
)

type csdnAdapter struct {
	client    *http.Client
	session   Session
	userAgent string
	userID    string
}

func NewCSDNAdapter(base *http.Client, session Session) (Adapter, error) {
	client, err := HTTPClientForSession(base, session)
	if err != nil {
		return nil, err
	}
	return &csdnAdapter{client: client, session: session, userAgent: session.UserAgent}, nil
}

func (c *csdnAdapter) ID() string { return "csdn" }

func (c *csdnAdapter) request(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	return browserRequest(ctx, method, rawURL, csdnOrigin, csdnOrigin+"/", c.userAgent, body)
}

func csdnNonce() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err == nil {
		hexed := hex.EncodeToString(raw)
		return hexed[:8] + "-" + hexed[8:12] + "-4" + hexed[13:16] + "-a" + hexed[17:20] + "-" + hexed[20:32]
	}
	return "00000000-0000-4000-a000-000000000000"
}

// csdnSignaturePath mirrors the editor-side canonical URI used for X-Ca-Signature.
// Empty query values are signed as a bare key ("model_type"), while the actual
// request URL still keeps the normal "model_type=" form.
func csdnSignaturePath(rawPath string) string {
	base, query, ok := strings.Cut(rawPath, "?")
	if !ok || query == "" {
		return rawPath
	}
	parts := strings.Split(query, "&")
	for index, part := range parts {
		if strings.HasSuffix(part, "=") {
			parts[index] = strings.TrimSuffix(part, "=")
		}
	}
	return base + "?" + strings.Join(parts, "&")
}

func (c *csdnAdapter) signedHeaders(path, method string) http.Header {
	nonce := csdnNonce()
	contentType := ""
	if method != http.MethodGet {
		contentType = "application/json"
	}
	signString := method + "\n*/*\n\n" + contentType + "\n\n" +
		"x-ca-key:" + csdnKey + "\n" +
		"x-ca-nonce:" + nonce + "\n" +
		csdnSignaturePath(path)
	headers := make(http.Header)
	headers.Set("accept", "*/*")
	headers.Set("x-ca-key", csdnKey)
	headers.Set("x-ca-nonce", nonce)
	headers.Set("x-ca-signature", hmacSHA256Base64(csdnSecret, signString))
	headers.Set("x-ca-signature-headers", "x-ca-key,x-ca-nonce")
	if contentType != "" {
		headers.Set("content-type", contentType)
	}
	return headers
}

func (c *csdnAdapter) apiRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := c.request(ctx, method, csdnAPI+path, body)
	if err != nil {
		return nil, err
	}
	for name, values := range c.signedHeaders(path, method) {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	return req, nil
}

func (c *csdnAdapter) CheckAuth(ctx context.Context) (AuthResult, error) {
	req, err := c.apiRequest(ctx, http.MethodGet, "/blog-console-api/v3/editor/getBaseInfo", nil)
	if err != nil {
		return AuthResult{}, err
	}
	var decoded struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Name     string `json:"name"`
			Nickname string `json:"nickname"`
		} `json:"data"`
	}
	if err := doJSON(c.client, req, c.ID(), "auth", &decoded); err != nil {
		if IsKind(err, ErrAuthExpired) {
			return AuthResult{Authenticated: false}, nil
		}
		return AuthResult{}, err
	}
	if decoded.Code != 200 || strings.TrimSpace(decoded.Data.Name) == "" {
		return AuthResult{Authenticated: false}, nil
	}
	c.userID = strings.TrimSpace(decoded.Data.Name)
	name := strings.TrimSpace(decoded.Data.Nickname)
	if name == "" {
		name = c.userID
	}
	return AuthResult{Authenticated: true, UserID: c.userID, Username: name}, nil
}

func csdnImageSuffix(source, contentType string) string {
	name := inferImageFilename(source, contentType)
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	switch extension {
	case "jpg", "jpeg", "png", "gif", "webp":
		return extension
	default:
		return "jpg"
	}
}

func (c *csdnAdapter) uploadImage(ctx context.Context, image RehostImage) (string, error) {
	suffix := csdnImageSuffix(image.Source, image.ContentType)
	path := "/resource-api/v1/image/direct/upload/signature"
	body, _ := json.Marshal(map[string]any{
		"imageTemplate": "",
		"appName":       "direct_blog_markdown",
		"imageSuffix":   suffix,
	})
	req, err := c.apiRequest(ctx, http.MethodPost, path, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	var signed struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			FilePath         string         `json:"filePath"`
			Host             string         `json:"host"`
			AccessID         string         `json:"accessId"`
			Policy           string         `json:"policy"`
			Signature        string         `json:"signature"`
			CallbackURL      string         `json:"callbackUrl"`
			CallbackBody     string         `json:"callbackBody"`
			CallbackBodyType string         `json:"callbackBodyType"`
			CustomParam      map[string]any `json:"customParam"`
		} `json:"data"`
	}
	if err := doJSON(c.client, req, c.ID(), "image-signature", &signed); err != nil {
		return "", err
	}
	if signed.Code != 200 || signed.Data.Host == "" || signed.Data.FilePath == "" {
		return "", platformError(ErrUpload, c.ID(), "image-signature", signed.Code, responseMessage(signed.Message), false)
	}
	fields := map[string]string{
		"key":              signed.Data.FilePath,
		"policy":           signed.Data.Policy,
		"signature":        signed.Data.Signature,
		"callbackBody":     signed.Data.CallbackBody,
		"callbackBodyType": signed.Data.CallbackBodyType,
		"callbackUrl":      signed.Data.CallbackURL,
		"AccessKeyId":      signed.Data.AccessID,
	}
	for key, value := range signed.Data.CustomParam {
		fields["x:"+key] = valueString(value)
	}
	multipart, multipartType, err := multipartBody(fields, "file", "image."+suffix, image.ContentType, image.Payload)
	if err != nil {
		return "", err
	}
	uploadReq, err := c.request(ctx, http.MethodPost, signed.Data.Host, multipart)
	if err != nil {
		return "", err
	}
	uploadReq.Header.Set("content-type", multipartType)
	var uploaded struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			ImageURL string `json:"imageUrl"`
		} `json:"data"`
	}
	if err := doJSON(c.client, uploadReq, c.ID(), "image-upload", &uploaded); err != nil {
		return "", err
	}
	if uploaded.Code != 200 || uploaded.Data.ImageURL == "" {
		return "", platformError(ErrUpload, c.ID(), "image-upload", uploaded.Code, responseMessage(uploaded.Message), false)
	}
	return uploaded.Data.ImageURL, nil
}

func (c *csdnAdapter) prepareContent(ctx context.Context, input DraftInput) (string, string, error) {
	options := ImageRehostOptions{
		Platform:       c.ID(),
		FailOpenRemote: true,
		AlreadyHosted: func(source string) bool {
			lower := strings.ToLower(source)
			return strings.Contains(lower, "csdnimg.cn") || strings.Contains(lower, "csdn.net")
		},
	}
	replacements, err := rehostImageReplacements(ctx, c.client, input, input.Markdown, options, c.uploadImage)
	if err != nil {
		return "", "", err
	}
	return replaceImages(input.Markdown, replacements), replaceImages(htmlFor(input), replacements), nil
}

func (c *csdnAdapter) save(ctx context.Context, refID string, input DraftInput, publish bool) (map[string]any, error) {
	markdown, html, err := c.prepareContent(ctx, input)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"title":             input.Title,
		"markdowncontent":   markdown,
		"content":           html,
		"readType":          "public",
		"level":             0,
		"tags":              strings.Join(input.Tags, ","),
		"status":            2,
		"categories":        "",
		"type":              "original",
		"original_link":     "",
		"authorized_status": false,
		"not_auto_saved":    "1",
		"source":            "pc_mdeditor",
		"cover_images":      []string{},
		"cover_type":        1,
		"is_new":            1,
		"vote_id":           0,
		"resource_id":       "",
		"pubStatus":         "draft",
	}
	if refID != "" {
		payload["id"] = refID
	}
	if publish {
		payload["status"] = 0
		payload["pubStatus"] = "publish"
	}
	body, _ := json.Marshal(payload)
	path := "/blog-console-api/v3/mdeditor/saveArticle"
	req, err := c.apiRequest(ctx, http.MethodPost, path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := doJSON(c.client, req, c.ID(), map[bool]string{true: "publish-draft", false: "save-draft"}[publish], &decoded); err != nil {
		return nil, err
	}
	if int(csdnNumberValue(decoded["code"])) != 200 {
		return nil, platformError(ErrUpstream, c.ID(), "save-article", int(csdnNumberValue(decoded["code"])), responseMessage(decoded["msg"], decoded["message"]), false)
	}
	return decoded, nil
}

func csdnNumberValue(value any) float64 {
	number, _ := value.(float64)
	return number
}

func csdnSavedID(decoded map[string]any, fallback string) string {
	if data, ok := decoded["data"].(map[string]any); ok {
		if id := valueString(data["id"]); id != "" {
			return id
		}
	}
	return fallback
}

func (c *csdnAdapter) CreateDraft(ctx context.Context, input DraftInput) (DraftResult, error) {
	decoded, err := c.save(ctx, "", input, false)
	if err != nil {
		return DraftResult{}, err
	}
	id := csdnSavedID(decoded, "")
	if id == "" {
		return DraftResult{}, platformError(ErrUpstream, c.ID(), "create-draft", 0, "response did not contain an article id", false)
	}
	return DraftResult{ID: id, URL: csdnOrigin + "/md?articleId=" + url.QueryEscape(id), Created: true}, nil
}

func (c *csdnAdapter) UpdateDraft(ctx context.Context, ref DraftRef, input DraftInput) (DraftResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return DraftResult{}, platformError(ErrValidation, c.ID(), "update-draft", 0, "draft id is required", false)
	}
	decoded, err := c.save(ctx, ref.ID, input, false)
	if err != nil {
		return DraftResult{}, err
	}
	id := csdnSavedID(decoded, ref.ID)
	return DraftResult{ID: id, URL: csdnOrigin + "/md?articleId=" + url.QueryEscape(id), Updated: true}, nil
}

func (c *csdnAdapter) PublishDraft(ctx context.Context, ref DraftRef, input DraftInput) (PublishResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return PublishResult{}, platformError(ErrValidation, c.ID(), "publish-draft", 0, "draft id is required", false)
	}
	decoded, err := c.save(ctx, ref.ID, input, true)
	if err != nil {
		return PublishResult{}, err
	}
	id := csdnSavedID(decoded, ref.ID)
	if data, ok := decoded["data"].(map[string]any); ok {
		if target := valueString(data["url"]); target != "" {
			return PublishResult{ID: id, URL: target}, nil
		}
	}
	if c.userID == "" {
		_, _ = c.CheckAuth(ctx)
	}
	if c.userID == "" {
		return PublishResult{}, platformError(ErrUpstream, c.ID(), "publish-draft", 0, "published response did not include a public URL", false)
	}
	return PublishResult{ID: id, URL: "https://blog.csdn.net/" + url.PathEscape(c.userID) + "/article/details/" + url.PathEscape(id)}, nil
}

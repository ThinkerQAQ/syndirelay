package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	osChinaAPIOrigin = "https://apiv1.oschina.net"
	osChinaOrigin    = "https://my.oschina.net"
)

type osChinaAdapter struct {
	client    *http.Client
	session   Session
	userAgent string
	userID    string
	username  string
}

func NewOSChinaAdapter(base *http.Client, session Session) (Adapter, error) {
	client, err := HTTPClientForSession(base, session)
	if err != nil {
		return nil, err
	}
	return &osChinaAdapter{client: client, session: session, userAgent: session.UserAgent}, nil
}

func (o *osChinaAdapter) ID() string { return "oschina" }

func (o *osChinaAdapter) request(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	return browserRequest(ctx, method, rawURL, osChinaOrigin, osChinaOrigin+"/", o.userAgent, body)
}

func (o *osChinaAdapter) CheckAuth(ctx context.Context) (AuthResult, error) {
	req, err := o.request(ctx, http.MethodGet, osChinaAPIOrigin+"/oschinapi/user/myDetails", nil)
	if err != nil {
		return AuthResult{}, err
	}
	var decoded struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Result  struct {
			UserID any `json:"userId"`
			UserVo struct {
				Name string `json:"name"`
			} `json:"userVo"`
		} `json:"result"`
	}
	if err := doJSON(o.client, req, o.ID(), "auth", &decoded); err != nil {
		if IsKind(err, ErrAuthExpired) {
			return AuthResult{Authenticated: false}, nil
		}
		return AuthResult{}, err
	}
	o.userID = valueString(decoded.Result.UserID)
	o.username = strings.TrimSpace(decoded.Result.UserVo.Name)
	if !decoded.Success || o.userID == "" {
		return AuthResult{Authenticated: false}, nil
	}
	if o.username == "" {
		o.username = o.userID
	}
	return AuthResult{Authenticated: true, UserID: o.userID, Username: o.username}, nil
}

func (o *osChinaAdapter) ensureUser(ctx context.Context) error {
	if o.userID != "" {
		return nil
	}
	auth, err := o.CheckAuth(ctx)
	if err != nil {
		return err
	}
	if !auth.Authenticated {
		return platformError(ErrAuthExpired, o.ID(), "auth", http.StatusUnauthorized, "browser session is not authenticated", false)
	}
	return nil
}

func (o *osChinaAdapter) uploadImage(ctx context.Context, image RehostImage) (string, error) {
	body, bodyType, err := multipartBody(nil, "file", inferImageFilename(image.Source, image.ContentType), image.ContentType, image.Payload)
	if err != nil {
		return "", err
	}
	req, err := o.request(ctx, http.MethodPost, osChinaAPIOrigin+"/oschinapi/ai/creation/project/uploadDetail", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", bodyType)
	var decoded struct {
		Success bool   `json:"success"`
		Result  string `json:"result"`
		Message string `json:"message"`
	}
	if err := doJSON(o.client, req, o.ID(), "image-upload", &decoded); err != nil {
		return "", err
	}
	if !decoded.Success || strings.TrimSpace(decoded.Result) == "" {
		return "", platformError(ErrUpload, o.ID(), "image-upload", 0, responseMessage(decoded.Message), false)
	}
	return strings.TrimSpace(decoded.Result), nil
}

func (o *osChinaAdapter) prepareMarkdown(ctx context.Context, input DraftInput) (string, error) {
	return rehostMarkdownImages(ctx, o.client, input, ImageRehostOptions{
		Platform:       o.ID(),
		FailOpenRemote: true,
		AlreadyHosted: func(source string) bool {
			host := strings.ToLower(source)
			return strings.Contains(host, "oschina.net") || strings.Contains(host, "oscimg")
		},
	}, o.uploadImage)
}

func (o *osChinaAdapter) saveDraft(ctx context.Context, refID string, input DraftInput) (DraftResult, error) {
	if err := o.ensureUser(ctx); err != nil {
		return DraftResult{}, err
	}
	content, err := o.prepareMarkdown(ctx, input)
	if err != nil {
		return DraftResult{}, err
	}
	payload := map[string]any{
		"title":          input.Title,
		"user":           o.userID,
		"content":        content,
		"contentType":    1,
		"catalog":        0,
		"originUrl":      "",
		"privacy":        true,
		"disableComment": false,
	}
	if refID != "" {
		payload["draft"] = refID
	}
	body, _ := json.Marshal(payload)
	req, err := o.request(ctx, http.MethodPost, osChinaAPIOrigin+"/oschinapi/api/draft/save_draft", strings.NewReader(string(body)))
	if err != nil {
		return DraftResult{}, err
	}
	req.Header.Set("content-type", "application/json")
	var decoded struct {
		Success bool   `json:"success"`
		Code    int    `json:"code"`
		Message string `json:"message"`
		Result  struct {
			ID any `json:"id"`
		} `json:"result"`
	}
	if err := doJSON(o.client, req, o.ID(), "save-draft", &decoded); err != nil {
		return DraftResult{}, err
	}
	if !decoded.Success {
		message := responseMessage(decoded.Message)
		if refID != "" && (strings.Contains(message, "不存在") || strings.Contains(strings.ToLower(message), "not found")) {
			return DraftResult{}, platformError(ErrRemoteDraftMissing, o.ID(), "update-draft", decoded.Code, message, false)
		}
		return DraftResult{}, platformError(ErrUpstream, o.ID(), "save-draft", decoded.Code, message, false)
	}
	id := valueString(decoded.Result.ID)
	if id == "" {
		id = refID
	}
	if id == "" {
		return DraftResult{}, platformError(ErrUpstream, o.ID(), "save-draft", decoded.Code, "response did not contain a draft id", false)
	}
	return DraftResult{
		ID:      id,
		URL:     OSChinaDraftEditorURL(o.userID, id),
		Created: refID == "",
		Updated: refID != "",
	}, nil
}

func (o *osChinaAdapter) CreateDraft(ctx context.Context, input DraftInput) (DraftResult, error) {
	return o.saveDraft(ctx, "", input)
}

func (o *osChinaAdapter) UpdateDraft(ctx context.Context, ref DraftRef, input DraftInput) (DraftResult, error) {
	if strings.TrimSpace(ref.ID) == "" {
		return DraftResult{}, platformError(ErrValidation, o.ID(), "update-draft", 0, "draft id is required", false)
	}
	return o.saveDraft(ctx, ref.ID, input)
}

func (o *osChinaAdapter) catalogID(ctx context.Context) (string, error) {
	req, err := o.request(ctx, http.MethodGet, osChinaAPIOrigin+"/oschinapi/blog_catalog/list_by_user", nil)
	if err != nil {
		return "", err
	}
	var decoded struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Result  []struct {
			ID        any `json:"id"`
			BlogCount int `json:"blogCount"`
		} `json:"result"`
	}
	if err := doJSON(o.client, req, o.ID(), "catalogs", &decoded); err != nil {
		return "", err
	}
	if !decoded.Success || len(decoded.Result) == 0 {
		return "", platformError(ErrValidation, o.ID(), "catalogs", 0, responseMessage(decoded.Message, "no blog catalog is available"), false)
	}
	best := decoded.Result[0]
	for _, item := range decoded.Result[1:] {
		if item.BlogCount > best.BlogCount {
			best = item
		}
	}
	id := valueString(best.ID)
	if id == "" {
		return "", platformError(ErrValidation, o.ID(), "catalogs", 0, "blog catalog id is missing", false)
	}
	return id, nil
}

func (o *osChinaAdapter) PublishDraft(ctx context.Context, ref DraftRef, input DraftInput) (PublishResult, error) {
	if err := o.ensureUser(ctx); err != nil {
		return PublishResult{}, err
	}
	content, err := o.prepareMarkdown(ctx, input)
	if err != nil {
		return PublishResult{}, err
	}
	catalog, err := o.catalogID(ctx)
	if err != nil {
		return PublishResult{}, err
	}
	payload := map[string]any{
		"title":          input.Title,
		"content":        content,
		"contentType":    1,
		"type":           "1",
		"originUrl":      "",
		"catalog":        catalog,
		"privacy":        true,
		"disableComment": false,
		"user":           o.userID,
	}
	body, _ := json.Marshal(payload)
	req, err := o.request(ctx, http.MethodPost, osChinaAPIOrigin+"/oschinapi/blog/web/add", strings.NewReader(string(body)))
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("content-type", "application/json")
	var decoded struct {
		Code    int    `json:"code"`
		Success bool   `json:"success"`
		Message string `json:"message"`
		Result  any    `json:"result"`
	}
	if err := doJSON(o.client, req, o.ID(), "publish-draft", &decoded); err != nil {
		return PublishResult{}, err
	}
	if decoded.Code != 200 && !decoded.Success {
		return PublishResult{}, platformError(ErrUpstream, o.ID(), "publish-draft", decoded.Code, responseMessage(decoded.Message), false)
	}
	id := valueString(decoded.Result)
	if object, ok := decoded.Result.(map[string]any); ok {
		if candidate := valueString(object["id"]); candidate != "" {
			id = candidate
		}
	}
	if id == "" {
		return PublishResult{}, platformError(ErrUpstream, o.ID(), "publish-draft", decoded.Code, "response did not contain a public blog id", false)
	}
	_ = ref
	return PublishResult{ID: id, URL: fmt.Sprintf("%s/u/%s/blog/%s", osChinaOrigin, url.PathEscape(o.userID), url.PathEscape(id))}, nil
}

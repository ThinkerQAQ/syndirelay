package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	devtoOrigin = "https://dev.to"
	devtoAccept = "application/vnd.forem.api-v1+json"
)

type devtoAdapter struct {
	client           *http.Client
	apiKey           string
	origin           string
	browserCookies   []BrowserCookie
	browserUserAgent string
	browserCSRFToken string
}

// DEV.to returns tag_list / tags as either an array (API v1) or a
// comma-separated string (some browser/API responses). Accept both.
type devtoTagList []string

func (tags *devtoTagList) UnmarshalJSON(raw []byte) error {
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		*tags = values
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		*tags = nil
		return nil
	}
	for _, tag := range strings.Split(value, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			*tags = append(*tags, tag)
		}
	}
	return nil
}

type devtoArticle struct {
	ID                 int64        `json:"id"`
	Title              string       `json:"title"`
	Description        string       `json:"description"`
	CanonicalURL       string       `json:"canonical_url"`
	BodyMarkdown       string       `json:"body_markdown"`
	TagList            devtoTagList `json:"tag_list"`
	Tags               devtoTagList `json:"tags"`
	Published          bool         `json:"published"`
	PublishedAt        string       `json:"published_at"`
	PublishedTimestamp string       `json:"published_timestamp"`
	URL                string       `json:"url"`
}

type devtoPayload struct {
	Title        string `json:"title"`
	BodyMarkdown string `json:"body_markdown"`
	Published    bool   `json:"published"`
	CanonicalURL string `json:"canonical_url,omitempty"`
	Description  string `json:"description"`
	Tags         string `json:"tags"`
	MainImage    string `json:"main_image,omitempty"`
}

func NewDEVToAdapter(base *http.Client, session Session) (Adapter, error) {
	return newDEVToAdapter(base, session, devtoOrigin)
}

func newDEVToAdapter(base *http.Client, session Session, origin string) (*devtoAdapter, error) {
	if base == nil {
		base = http.DefaultClient
	}
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	if origin == "" {
		origin = devtoOrigin
	}
	if _, err := url.ParseRequestURI(origin); err != nil {
		return nil, err
	}
	return &devtoAdapter{
		client: base, apiKey: strings.TrimSpace(session.APIKey), origin: origin,
		browserCookies:   append([]BrowserCookie{}, session.Cookies...),
		browserUserAgent: strings.TrimSpace(session.UserAgent),
	}, nil
}

func (a *devtoAdapter) ID() string { return "devto" }

func (a *devtoAdapter) CheckAuth(_ context.Context) (AuthResult, error) {
	return AuthResult{Authenticated: a.apiKey != ""}, nil
}

func devtoCanonicalEqual(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	a, err := url.Parse(left)
	if err != nil {
		return false
	}
	b, err := url.Parse(right)
	if err != nil {
		return false
	}
	a.RawQuery, a.Fragment = "", ""
	b.RawQuery, b.Fragment = "", ""
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Host, b.Host) &&
		strings.TrimRight(a.Path, "/") == strings.TrimRight(b.Path, "/")
}

func normalizeDEVToTags(tags []string) []string {
	result := []string{}
	seen := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tag), " ", ""))
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
		if len(result) == 4 {
			break
		}
	}
	return result
}

func devtoArticlePublished(article devtoArticle) bool {
	return article.Published || strings.TrimSpace(article.PublishedAt) != "" || strings.TrimSpace(article.PublishedTimestamp) != ""
}

func devtoRemoteTags(article devtoArticle) []string {
	if len(article.TagList) > 0 {
		return normalizeDEVToTags(article.TagList)
	}
	if len(article.Tags) > 0 {
		return normalizeDEVToTags(article.Tags)
	}
	return nil
}

func devtoDesired(input DraftInput) devtoPayload {
	return devtoPayload{
		Title:        input.Title,
		BodyMarkdown: strings.TrimSpace(input.Markdown) + "\n",
		Published:    input.Published,
		CanonicalURL: strings.TrimSpace(input.NativeCanonicalURL),
		Description:  input.Description,
		Tags:         strings.Join(normalizeDEVToTags(input.Tags), ","),
		MainImage:    strings.TrimSpace(input.CoverImageURL),
	}
}

func devtoMatches(remote devtoArticle, desired devtoPayload) bool {
	remotePublished := devtoArticlePublished(remote)
	canonicalMatches := strings.TrimSpace(desired.CanonicalURL) == ""
	if desired.CanonicalURL != "" {
		canonicalMatches = devtoCanonicalEqual(remote.CanonicalURL, desired.CanonicalURL)
	} else if strings.TrimSpace(remote.CanonicalURL) != "" {
		canonicalMatches = false
	}
	return remote.Title == desired.Title &&
		remote.Description == desired.Description &&
		canonicalMatches &&
		strings.TrimSpace(remote.BodyMarkdown) == strings.TrimSpace(desired.BodyMarkdown) &&
		strings.Join(devtoRemoteTags(remote), ",") == strings.Join(normalizeDEVToTags(strings.Split(desired.Tags, ",")), ",") &&
		remotePublished == desired.Published
}

func (a *devtoAdapter) endpoint(suffix string) string {
	return a.origin + "/api/" + strings.TrimLeft(suffix, "/")
}

func devtoError(operation string, status int, detail string) error {
	kind := ErrUpstream
	retryable := false
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		kind = ErrAuthExpired
	case http.StatusNotFound:
		kind = ErrRemoteDraftMissing
	case http.StatusTooManyRequests:
		kind, retryable = ErrRateLimited, true
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		kind = ErrValidation
	}
	return platformError(kind, "devto", operation, status, detail, retryable)
}

func (a *devtoAdapter) request(ctx context.Context, method, suffix string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, a.endpoint(suffix), reader)
	if err != nil {
		return err
	}
	request.Header.Set("accept", devtoAccept)
	request.Header.Set("api-key", a.apiKey)
	request.Header.Set("content-type", "application/json")
	request.Header.Set("user-agent", "ThinkerQAQ-BlogCTL/1.0")
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return devtoError(suffix, response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if target != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, target); err != nil {
			return fmt.Errorf("devto %s: invalid JSON response: %w", suffix, err)
		}
	}
	return nil
}

func (a *devtoAdapter) listArticles(ctx context.Context) ([]devtoArticle, error) {
	result := []devtoArticle{}
	for page := 1; ; page++ {
		var batch []devtoArticle
		suffix := "articles/me/all?page=" + strconv.Itoa(page) + "&per_page=100"
		if err := a.request(ctx, http.MethodGet, suffix, nil, &batch); err != nil {
			return nil, err
		}
		result = append(result, batch...)
		if len(batch) < 100 {
			return result, nil
		}
	}
}

func (a *devtoAdapter) getArticle(ctx context.Context, id string) (devtoArticle, error) {
	var article devtoArticle
	err := a.request(ctx, http.MethodGet, "articles/"+url.PathEscape(id), nil, &article)
	return article, err
}

func (a *devtoAdapter) findExisting(ctx context.Context, desired devtoPayload) (*devtoArticle, error) {
	articles, err := a.listArticles(ctx)
	if err != nil {
		return nil, err
	}
	for index := range articles {
		article := &articles[index]
		if desired.CanonicalURL != "" && devtoCanonicalEqual(article.CanonicalURL, desired.CanonicalURL) {
			return article, nil
		}
		if desired.CanonicalURL == "" && strings.TrimSpace(article.CanonicalURL) == "" && article.Title == desired.Title {
			return article, nil
		}
	}
	return nil, nil
}

func (a *devtoAdapter) upsertExisting(ctx context.Context, existing devtoArticle, input DraftInput) (DraftResult, error) {
	desired := devtoDesired(input)
	full := existing
	if strings.TrimSpace(full.BodyMarkdown) == "" {
		var err error
		full, err = a.getArticle(ctx, strconv.FormatInt(existing.ID, 10))
		if err != nil {
			return DraftResult{}, err
		}
	}
	// "Save" is a draft operation in BlogCTL. Updating a public DEV.to article
	// would make the new body live immediately, bypassing preview + explicit
	// publish. Fail closed until a separate published-update workflow exists.
	if !input.Published && devtoArticlePublished(full) {
		return DraftResult{}, platformError(
			ErrValidation, "devto", "save-draft", 0,
			"the matching DEV.to article is already published; safe published-article updates are not supported yet",
			false,
		)
	}
	if (input.Published || input.ChangedOnly) && devtoMatches(full, desired) {
		return DraftResult{ID: strconv.FormatInt(full.ID, 10), URL: full.URL, Skipped: true}, nil
	}
	var updated devtoArticle
	if err := a.request(ctx, http.MethodPut, "articles/"+strconv.FormatInt(existing.ID, 10), map[string]any{"article": desired}, &updated); err != nil {
		return DraftResult{}, err
	}
	return DraftResult{ID: strconv.FormatInt(updated.ID, 10), URL: updated.URL, Updated: true}, nil
}

func (a *devtoAdapter) CreateDraft(ctx context.Context, input DraftInput) (DraftResult, error) {
	prepared, err := a.prepareImages(ctx, input)
	if err != nil {
		return DraftResult{}, err
	}
	input = prepared
	desired := devtoDesired(input)
	existing, err := a.findExisting(ctx, desired)
	if err != nil {
		return DraftResult{}, err
	}
	if existing != nil {
		return a.upsertExisting(ctx, *existing, input)
	}
	var created devtoArticle
	if err := a.request(ctx, http.MethodPost, "articles", map[string]any{"article": desired}, &created); err != nil {
		return DraftResult{}, err
	}
	return DraftResult{ID: strconv.FormatInt(created.ID, 10), URL: created.URL, Created: true}, nil
}

func (a *devtoAdapter) UpdateDraft(ctx context.Context, ref DraftRef, input DraftInput) (DraftResult, error) {
	prepared, err := a.prepareImages(ctx, input)
	if err != nil {
		return DraftResult{}, err
	}
	existing, err := a.getArticle(ctx, ref.ID)
	if err != nil {
		return DraftResult{}, err
	}
	return a.upsertExisting(ctx, existing, prepared)
}

func (a *devtoAdapter) PublishDraft(ctx context.Context, ref DraftRef, _ DraftInput) (PublishResult, error) {
	var updated devtoArticle
	if err := a.request(ctx, http.MethodPut, "articles/"+url.PathEscape(ref.ID), map[string]any{
		"article": map[string]any{"published": true},
	}, &updated); err != nil {
		return PublishResult{}, err
	}
	if updated.ID == 0 {
		return PublishResult{}, platformError(ErrUpstream, "devto", "publish-draft", 0, "DEV.to publish response is missing article id", false)
	}
	if strings.TrimSpace(updated.URL) == "" {
		return PublishResult{}, platformError(ErrUpstream, "devto", "publish-draft", 0, "DEV.to publish response is missing article URL", false)
	}
	return PublishResult{ID: strconv.FormatInt(updated.ID, 10), URL: updated.URL}, nil
}

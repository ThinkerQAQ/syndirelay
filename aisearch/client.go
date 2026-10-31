package aisearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SyncConfig struct {
	AccountID          string
	APIToken           string
	InstanceName       string
	BlogOrigin         string
	ContentRoot        string
	ChangeManifestPath string
	ForceFull          bool
	RequestTimeout     time.Duration
	MaxRetries         int
	RetryBase          time.Duration
	RetryMax           time.Duration
}

type SyncResult struct {
	InstanceState     string `json:"instanceState"`
	PublicDocuments   int    `json:"publicDocuments"`
	Uploaded          int    `json:"uploaded"`
	MetadataRefreshes int    `json:"metadataRefreshes"`
	Deleted           int    `json:"deleted"`
	Incremental       bool   `json:"incremental"`
	FastPath          bool   `json:"fastPath"`
	ChangedPaths      *int   `json:"changedPaths,omitempty"`
	SchemaVersion     int    `json:"schemaVersion"`
}

type cloudflareAPIError struct {
	Status int
	Body   string
}

func (e *cloudflareAPIError) Error() string {
	return fmt.Sprintf("Cloudflare API %d: %s", e.Status, e.Body)
}

type cloudflareClient struct {
	httpClient *http.Client
	config     SyncConfig
	apiBase    string
}

type cloudflareEnvelope struct {
	Success *bool           `json:"success"`
	Result  json.RawMessage `json:"result"`
	Errors  []struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

type remoteItem struct {
	ID       string         `json:"id"`
	Key      string         `json:"key"`
	Metadata map[string]any `json:"metadata"`
}

func normalizeSyncConfig(config SyncConfig) (SyncConfig, error) {
	config.AccountID = strings.TrimSpace(config.AccountID)
	config.APIToken = strings.TrimSpace(config.APIToken)
	config.InstanceName = strings.TrimSpace(config.InstanceName)
	config.BlogOrigin = strings.TrimRight(strings.TrimSpace(config.BlogOrigin), "/")
	config.ContentRoot = strings.TrimSpace(config.ContentRoot)
	config.ChangeManifestPath = strings.TrimSpace(config.ChangeManifestPath)
	if config.AccountID == "" || config.APIToken == "" {
		return config, errors.New("CLOUDFLARE_ACCOUNT_ID and CLOUDFLARE_AI_SEARCH_TOKEN are required")
	}
	if config.InstanceName == "" {
		config.InstanceName = "thinkerqaq-blog"
	}
	if config.BlogOrigin == "" {
		config.BlogOrigin = DefaultBlogOrigin
	}
	if config.ContentRoot == "" {
		return config, errors.New("AI Search content root is required")
	}
	if config.ChangeManifestPath == "" {
		config.ChangeManifestPath = filepath.Join(config.ContentRoot, "ai-search-changed-paths.txt")
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 8
	}
	if config.RetryBase <= 0 {
		config.RetryBase = 2 * time.Second
	}
	if config.RetryMax <= 0 {
		config.RetryMax = 15 * time.Second
	}
	return config, nil
}

func newCloudflareClient(client *http.Client, config SyncConfig) *cloudflareClient {
	if client == nil {
		client = &http.Client{}
	}
	return &cloudflareClient{
		httpClient: client,
		config:     config,
		apiBase: "https://api.cloudflare.com/client/v4/accounts/" +
			url.PathEscape(config.AccountID) + "/ai-search/instances",
	}
}

func acceptedStatus(status int, accepted []int) bool {
	for _, value := range accepted {
		if status == value {
			return true
		}
	}
	return false
}

func transientStatus(status int) bool {
	switch status {
	case 408, 425, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func mutating7017(status int, body []byte) bool {
	if status != http.StatusServiceUnavailable {
		return false
	}
	if strings.Contains(string(body), "unable_to_connect_to_ai_search") {
		return true
	}
	var envelope struct {
		Errors []struct {
			Code any `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return false
	}
	for _, entry := range envelope.Errors {
		if fmt.Sprint(entry.Code) == "7017" {
			return true
		}
	}
	return false
}

func retryAfter(response *http.Response) time.Duration {
	value := strings.TrimSpace(response.Header.Get("retry-after"))
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if timestamp, err := http.ParseTime(value); err == nil {
		delay := time.Until(timestamp)
		if delay > 0 {
			return delay
		}
	}
	return 0
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func (c *cloudflareClient) do(
	ctx context.Context,
	method, endpoint, contentType string,
	bodyFactory func() (io.Reader, error),
	accepted []int,
) ([]byte, int, error) {
	var lastErr error
	maxAttempts := c.config.MaxRetries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var body io.Reader
		if bodyFactory != nil {
			var err error
			body, err = bodyFactory()
			if err != nil {
				return nil, 0, err
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
		request, err := http.NewRequestWithContext(attemptCtx, method, endpoint, body)
		if err != nil {
			cancel()
			return nil, 0, err
		}
		request.Header.Set("authorization", "Bearer "+c.config.APIToken)
		if contentType != "" {
			request.Header.Set("content-type", contentType)
		}
		response, err := c.httpClient.Do(request)
		if err != nil {
			cancel()
			lastErr = err
			if attempt >= maxAttempts || (method != http.MethodGet && method != http.MethodHead) {
				return nil, 0, err
			}
			delay := minDuration(c.config.RetryBase*time.Duration(1<<minInt(attempt-1, 6)), c.config.RetryMax)
			if err := sleepContext(ctx, delay); err != nil {
				return nil, 0, err
			}
			continue
		}
		payload, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<20))
		response.Body.Close()
		retryHeader := retryAfter(response)
		cancel()
		if readErr != nil {
			return nil, response.StatusCode, readErr
		}
		if acceptedStatus(response.StatusCode, accepted) {
			return payload, response.StatusCode, nil
		}

		retryable := false
		if method == http.MethodGet || method == http.MethodHead {
			retryable = transientStatus(response.StatusCode)
		} else {
			retryable = mutating7017(response.StatusCode, payload)
		}
		if !retryable || attempt >= maxAttempts {
			return payload, response.StatusCode, &cloudflareAPIError{
				Status: response.StatusCode,
				Body:   strings.TrimSpace(string(payload)),
			}
		}

		delay := retryHeader
		if delay <= 0 {
			delay = minDuration(c.config.RetryBase*time.Duration(1<<minInt(attempt-1, 6)), c.config.RetryMax)
		}
		if err := sleepContext(ctx, minDuration(delay, c.config.RetryMax)); err != nil {
			return nil, 0, err
		}
	}
	return nil, 0, lastErr
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *cloudflareClient) jsonRequest(
	ctx context.Context,
	method, endpoint string,
	input any,
	accepted []int,
) (json.RawMessage, int, error) {
	var data []byte
	var err error
	if input != nil {
		data, err = json.Marshal(input)
		if err != nil {
			return nil, 0, err
		}
	}
	payload, status, err := c.do(ctx, method, endpoint, func() string {
		if input != nil {
			return "application/json"
		}
		return ""
	}(), func() (io.Reader, error) {
		if data == nil {
			return nil, nil
		}
		return bytes.NewReader(data), nil
	}, accepted)
	if err != nil {
		return nil, status, err
	}
	if status == http.StatusNoContent || len(payload) == 0 {
		return nil, status, nil
	}
	var envelope cloudflareEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, status, err
	}
	if envelope.Success != nil && !*envelope.Success {
		messages := []string{}
		for _, entry := range envelope.Errors {
			if strings.TrimSpace(entry.Message) != "" {
				messages = append(messages, entry.Message)
			} else if entry.Code != nil {
				messages = append(messages, fmt.Sprint(entry.Code))
			}
		}
		if len(messages) == 0 {
			messages = append(messages, "unknown Cloudflare API error")
		}
		return nil, status, errors.New("Cloudflare API returned success=false: " + strings.Join(messages, "; "))
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return json.RawMessage("null"), status, nil
	}
	return envelope.Result, status, nil
}

func desiredInstanceConfiguration() map[string]any {
	return map[string]any{
		"index_method":     map[string]bool{"vector": true, "keyword": true},
		"fusion_method":    "rrf",
		"indexing_options": map[string]string{"keyword_tokenizer": "trigram"},
		"retrieval_options": map[string]any{
			"keyword_match_mode": "or",
			"boost_by":           []map[string]string{{"field": "priority", "direction": "desc"}},
		},
		"custom_metadata": []map[string]string{
			{"field_name": "source_url", "data_type": "text"},
			{"field_name": "title", "data_type": "text"},
			{"field_name": "language", "data_type": "text"},
			{"field_name": "priority", "data_type": "number"},
			{"field_name": "schema_version", "data_type": "number"},
		},
		"reranking":       true,
		"reranking_model": "@cf/baai/bge-reranker-base",
		"rewrite_query":   false,
		"chunk_size":      512,
		"chunk_overlap":   15,
		"max_num_results": 20,
	}
}

func hasDesiredInstanceConfiguration(raw json.RawMessage) bool {
	var info struct {
		IndexMethod struct {
			Vector  bool `json:"vector"`
			Keyword bool `json:"keyword"`
		} `json:"index_method"`
		FusionMethod    string `json:"fusion_method"`
		IndexingOptions struct {
			KeywordTokenizer string `json:"keyword_tokenizer"`
		} `json:"indexing_options"`
		RetrievalOptions struct {
			KeywordMatchMode string `json:"keyword_match_mode"`
			BoostBy          []struct {
				Field     string `json:"field"`
				Direction string `json:"direction"`
			} `json:"boost_by"`
		} `json:"retrieval_options"`
		CustomMetadata []struct {
			FieldName string `json:"field_name"`
			DataType  string `json:"data_type"`
		} `json:"custom_metadata"`
		Reranking     bool `json:"reranking"`
		RewriteQuery  bool `json:"rewrite_query"`
		MaxNumResults int  `json:"max_num_results"`
	}
	if json.Unmarshal(raw, &info) != nil {
		return false
	}
	if !info.IndexMethod.Vector || !info.IndexMethod.Keyword ||
		info.FusionMethod != "rrf" ||
		info.IndexingOptions.KeywordTokenizer != "trigram" ||
		info.RetrievalOptions.KeywordMatchMode != "or" ||
		!info.Reranking || info.RewriteQuery || info.MaxNumResults != 20 {
		return false
	}
	hasBoost := false
	for _, boost := range info.RetrievalOptions.BoostBy {
		if boost.Field == "priority" && boost.Direction == "desc" {
			hasBoost = true
			break
		}
	}
	if !hasBoost || len(info.CustomMetadata) != 5 {
		return false
	}
	expected := map[string]string{
		"source_url":     "text",
		"title":          "text",
		"language":       "text",
		"priority":       "number",
		"schema_version": "number",
	}
	for _, field := range info.CustomMetadata {
		if expected[strings.ToLower(field.FieldName)] != field.DataType {
			return false
		}
		delete(expected, strings.ToLower(field.FieldName))
	}
	return len(expected) == 0
}

func (c *cloudflareClient) ensureInstance(ctx context.Context) (string, error) {
	instanceURL := c.apiBase + "/" + url.PathEscape(c.config.InstanceName)
	existing, _, err := c.jsonRequest(ctx, http.MethodGet, instanceURL, nil, []int{200})
	if err != nil {
		var apiErr *cloudflareAPIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
			return "", err
		}
		existing = nil
	}
	desired := desiredInstanceConfiguration()
	if existing == nil {
		input := map[string]any{"id": c.config.InstanceName}
		for key, value := range desired {
			input[key] = value
		}
		if _, _, err := c.jsonRequest(ctx, http.MethodPost, c.apiBase, input, []int{200, 201}); err != nil {
			return "", err
		}
		return "created", nil
	}
	if hasDesiredInstanceConfiguration(existing) {
		return "unchanged", nil
	}
	if _, _, err := c.jsonRequest(ctx, http.MethodPut, instanceURL, desired, []int{200}); err != nil {
		return "", err
	}
	return "updated", nil
}

func (c *cloudflareClient) listItems(ctx context.Context) ([]remoteItem, error) {
	items := []remoteItem{}
	for page := 1; ; page++ {
		endpoint := fmt.Sprintf(
			"%s/%s/items?page=%d&per_page=50&source=builtin",
			c.apiBase, url.PathEscape(c.config.InstanceName), page,
		)
		result, _, err := c.jsonRequest(ctx, http.MethodGet, endpoint, nil, []int{200})
		if err != nil {
			return nil, err
		}
		var pageItems []remoteItem
		if err := json.Unmarshal(result, &pageItems); err != nil {
			return nil, err
		}
		items = append(items, pageItems...)
		if len(pageItems) < 50 {
			return items, nil
		}
	}
}

func (c *cloudflareClient) findItemByKey(ctx context.Context, key string) (*remoteItem, error) {
	endpoint := fmt.Sprintf(
		"%s/%s/items?key=%s&source=builtin",
		c.apiBase, url.PathEscape(c.config.InstanceName), url.QueryEscape(key),
	)
	result, _, err := c.jsonRequest(ctx, http.MethodGet, endpoint, nil, []int{200})
	if err != nil {
		return nil, err
	}
	var items []remoteItem
	if json.Unmarshal(result, &items) == nil {
		for index := range items {
			if items[index].Key == key {
				return &items[index], nil
			}
		}
		return nil, nil
	}
	var item remoteItem
	if json.Unmarshal(result, &item) == nil && item.Key == key {
		return &item, nil
	}
	return nil, nil
}

func (c *cloudflareClient) deleteItem(ctx context.Context, item remoteItem) error {
	endpoint := c.apiBase + "/" + url.PathEscape(c.config.InstanceName) +
		"/items/" + url.PathEscape(item.ID)
	_, _, err := c.jsonRequest(ctx, http.MethodDelete, endpoint, nil, []int{200, 204})
	return err
}

func (c *cloudflareClient) uploadDocument(ctx context.Context, document Document) error {
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, strings.ReplaceAll(document.Key, `"`, "")))
	header.Set("Content-Type", "text/markdown; charset=utf-8")
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(part, document.Content); err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{
		"source_url":     document.URL,
		"title":          document.Title,
		"language":       document.Language,
		"priority":       strconv.Itoa(document.Priority),
		"schema_version": strconv.Itoa(document.SchemaVersion),
	})
	if err := writer.WriteField("metadata", string(metadata)); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	endpoint := c.apiBase + "/" + url.PathEscape(c.config.InstanceName) + "/items"
	responseBody, _, err := c.do(
		ctx,
		http.MethodPost,
		endpoint,
		writer.FormDataContentType(),
		func() (io.Reader, error) { return bytes.NewReader(payload.Bytes()), nil },
		[]int{200, 201, 202},
	)
	if err != nil {
		return err
	}
	if len(responseBody) == 0 {
		return nil
	}
	var envelope cloudflareEnvelope
	if json.Unmarshal(responseBody, &envelope) == nil && envelope.Success != nil && !*envelope.Success {
		messages := []string{}
		for _, entry := range envelope.Errors {
			if strings.TrimSpace(entry.Message) != "" {
				messages = append(messages, entry.Message)
			} else if entry.Code != nil {
				messages = append(messages, fmt.Sprint(entry.Code))
			}
		}
		return errors.New("Cloudflare AI Search upload returned success=false: " + strings.Join(messages, "; "))
	}
	return nil
}

func metadataString(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func itemMetadataNeedsRefresh(item remoteItem, document Document) bool {
	priority, _ := strconv.Atoi(metadataString(item.Metadata, "priority"))
	schemaVersion, _ := strconv.Atoi(metadataString(item.Metadata, "schema_version"))
	return metadataString(item.Metadata, "source_url") != document.URL ||
		metadataString(item.Metadata, "title") != document.Title ||
		metadataString(item.Metadata, "language") != document.Language ||
		priority != document.Priority ||
		schemaVersion != document.SchemaVersion
}

func loadChangedPaths(path string) (map[string]struct{}, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	result := map[string]struct{}{}
	for _, line := range strings.Split(string(data), "\n") {
		value := filepath.ToSlash(strings.TrimSpace(line))
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result, true, nil
}

func incrementalCandidates(documents map[string]Document, changedPaths map[string]struct{}) ([]Document, bool) {
	bySource := make(map[string]Document, len(documents))
	for _, document := range documents {
		bySource[document.SourcePath] = document
	}
	selected := map[string]Document{}
	for changedPath := range changedPaths {
		document, ok := bySource[changedPath]
		if !ok {
			return nil, false
		}
		selected[document.Key] = document
		if document.Collection == "notes" && strings.HasPrefix(document.SourcePath, "src/content/notes/") {
			for _, dependent := range documents {
				if dependent.Collection == "notes" &&
					dependent.ID == document.ID &&
					strings.HasPrefix(dependent.SourcePath, "src/content/note-translations/") {
					selected[dependent.Key] = dependent
				}
			}
		}
	}
	result := make([]Document, 0, len(selected))
	for _, document := range selected {
		result = append(result, document)
	}
	return result, true
}

func runDocuments(items []Document, concurrency int, handler func(Document) error) error {
	if len(items) == 0 {
		return nil
	}
	if concurrency < 1 {
		concurrency = 1
	}
	work := make(chan Document)
	errCh := make(chan error, concurrency)
	var workers sync.WaitGroup
	for index := 0; index < concurrency; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range work {
				if err := handler(item); err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
			}
		}()
	}
	for _, item := range items {
		select {
		case err := <-errCh:
			close(work)
			workers.Wait()
			return err
		case work <- item:
		}
	}
	close(work)
	workers.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func runRemoteItems(items []remoteItem, concurrency int, handler func(remoteItem) error) error {
	if len(items) == 0 {
		return nil
	}
	documents := make([]Document, len(items))
	lookup := make(map[string]remoteItem, len(items))
	for index, item := range items {
		key := "__remote__" + strconv.Itoa(index)
		documents[index] = Document{Key: key}
		lookup[key] = item
	}
	return runDocuments(documents, concurrency, func(document Document) error {
		return handler(lookup[document.Key])
	})
}

func Sync(ctx context.Context, client *http.Client, config SyncConfig) (SyncResult, error) {
	config, err := normalizeSyncConfig(config)
	if err != nil {
		return SyncResult{}, err
	}
	api := newCloudflareClient(client, config)
	documents, err := LoadDocuments(config.ContentRoot, config.BlogOrigin)
	if err != nil {
		return SyncResult{}, err
	}
	instanceState, err := api.ensureInstance(ctx)
	if err != nil {
		return SyncResult{}, err
	}

	var changedPaths map[string]struct{}
	manifestAvailable := false
	if instanceState == "unchanged" && !config.ForceFull {
		changedPaths, manifestAvailable, err = loadChangedPaths(config.ChangeManifestPath)
		if err != nil {
			return SyncResult{}, err
		}
	}
	result := SyncResult{
		InstanceState:   instanceState,
		PublicDocuments: len(documents),
		Incremental:     manifestAvailable,
		SchemaVersion:   IndexSchemaVersion,
	}
	if manifestAvailable {
		count := len(changedPaths)
		result.ChangedPaths = &count
	}

	candidates := []Document{}
	fastPath := false
	if manifestAvailable {
		candidates, fastPath = incrementalCandidates(documents, changedPaths)
	}
	result.FastPath = fastPath

	type uploadCandidate struct {
		Document Document
		Existing *remoteItem
	}
	uploads := []uploadCandidate{}
	staleItems := []remoteItem{}
	var uploadMu sync.Mutex

	if fastPath {
		if err := runDocuments(candidates, 6, func(document Document) error {
			existing, err := api.findItemByKey(ctx, document.Key)
			if err != nil {
				return err
			}
			uploadMu.Lock()
			if existing != nil && itemMetadataNeedsRefresh(*existing, document) {
				result.MetadataRefreshes++
			}
			uploads = append(uploads, uploadCandidate{Document: document, Existing: existing})
			uploadMu.Unlock()
			return nil
		}); err != nil {
			return result, err
		}
	} else {
		existingItems, err := api.listItems(ctx)
		if err != nil {
			return result, err
		}
		existingByKey := make(map[string]remoteItem, len(existingItems))
		for _, item := range existingItems {
			existingByKey[item.Key] = item
			if strings.HasPrefix(item.Key, "blog--") {
				if _, ok := documents[item.Key]; !ok {
					staleItems = append(staleItems, item)
				}
			}
		}
		if err := runRemoteItems(staleItems, 4, func(item remoteItem) error {
			return api.deleteItem(ctx, item)
		}); err != nil {
			return result, err
		}
		for _, document := range documents {
			existing, ok := existingByKey[document.Key]
			_, contentChanged := changedPaths[document.SourcePath]
			if !manifestAvailable {
				contentChanged = true
			}
			metadataChanged := !ok || itemMetadataNeedsRefresh(existing, document)
			if !ok || contentChanged || metadataChanged {
				var existingPointer *remoteItem
				if ok {
					copyItem := existing
					existingPointer = &copyItem
				}
				uploads = append(uploads, uploadCandidate{Document: document, Existing: existingPointer})
				if metadataChanged {
					result.MetadataRefreshes++
				}
			}
		}
	}

	if len(uploads) > 0 {
		uploadDocs := make([]Document, len(uploads))
		uploadByKey := make(map[string]uploadCandidate, len(uploads))
		for index, candidate := range uploads {
			uploadDocs[index] = candidate.Document
			uploadByKey[candidate.Document.Key] = candidate
		}
		if err := runDocuments(uploadDocs, 4, func(document Document) error {
			candidate := uploadByKey[document.Key]
			if candidate.Existing != nil {
				if err := api.deleteItem(ctx, *candidate.Existing); err != nil {
					return err
				}
			}
			if err := api.uploadDocument(ctx, document); err != nil {
				return fmt.Errorf("failed to upload %s as %s: %w", document.SourcePath, document.Key, err)
			}
			return nil
		}); err != nil {
			return result, err
		}
	}

	result.Uploaded = len(uploads)
	result.Deleted = len(staleItems)
	return result, nil
}

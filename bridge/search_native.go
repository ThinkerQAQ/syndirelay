package bridge

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"

	blogsearch "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/search"
)

type searchDiffPayload struct {
	Mode           string `json:"mode"`
	SelectedCount  int    `json:"selectedCount"`
	AddedCount     int    `json:"addedCount"`
	ChangedCount   int    `json:"changedCount"`
	DeletedCount   int    `json:"deletedCount"`
	UnchangedCount int    `json:"unchangedCount"`
}

type searchHTTPResult struct {
	HTTPStatus int `json:"httpStatus"`
}

func (s *Server) searchSite() blogsearch.SiteConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.config.Search
}

type indexNowSubmissionPayload struct {
	Inventory searchInventoryState `json:"inventory"`
	Diff      searchDiffPayload    `json:"diff"`
	Result    struct {
		URLCount   int                `json:"urlCount"`
		BatchCount int                `json:"batchCount"`
		Results    []searchHTTPResult `json:"results"`
	} `json:"result"`
}

type baiduSubmissionPayload struct {
	Inventory searchInventoryState `json:"inventory"`
	Diff      searchDiffPayload    `json:"diff"`
	Result    struct {
		URLCount     int                `json:"urlCount"`
		SuccessCount int                `json:"successCount"`
		Remain       int                `json:"remain"`
		BatchCount   int                `json:"batchCount"`
		Complete     bool               `json:"complete"`
		Results      []searchHTTPResult `json:"results"`
	} `json:"result"`
}

func searchInventoryFromCore(value blogsearch.Inventory) searchInventoryState {
	return searchInventoryState{
		Source:              value.Source,
		FingerprintSource:   value.FingerprintSource,
		FingerprintCoverage: value.FingerprintCoverage,
		Origin:              value.Origin,
		FetchedAt:           value.FetchedAt,
		Total:               value.Total,
		URLs:                append([]string{}, value.URLs...),
		Fingerprints:        cloneStringMap(value.Fingerprints),
	}
}

func searchInventoryToCore(value searchInventoryState) blogsearch.Inventory {
	return blogsearch.Inventory{
		Source:              value.Source,
		FingerprintSource:   value.FingerprintSource,
		FingerprintCoverage: value.FingerprintCoverage,
		Origin:              value.Origin,
		FetchedAt:           value.FetchedAt,
		Total:               value.Total,
		URLs:                append([]string{}, value.URLs...),
		Fingerprints:        cloneStringMap(value.Fingerprints),
	}
}

func diffPayload(value blogsearch.Diff) searchDiffPayload {
	return searchDiffPayload{
		Mode:           value.Mode,
		SelectedCount:  value.SelectedCount,
		AddedCount:     value.AddedCount,
		ChangedCount:   value.ChangedCount,
		DeletedCount:   value.DeletedCount,
		UnchangedCount: value.UnchangedCount,
	}
}

func (s *Server) fetchSearchInventory(ctx context.Context) (searchInventoryState, error) {
	site := s.searchSite()
	slog.Info("search inventory fetch", "operation", "inventory-fetch", "origin", site.Origin, "source", site.URLListURL)
	inventory, err := blogsearch.FetchRemoteInventory(ctx, s.httpClient, site.Origin, site)
	if err != nil {
		return searchInventoryState{}, err
	}
	return searchInventoryFromCore(inventory), nil
}

func (s *Server) submitIndexNow(ctx context.Context, mode string, previous searchInventoryState) (indexNowSubmissionPayload, error) {
	inventory, err := s.fetchSearchInventory(ctx)
	if err != nil {
		return indexNowSubmissionPayload{}, err
	}
	diff, err := blogsearch.DiffInventories(searchInventoryToCore(previous), searchInventoryToCore(inventory), mode, true)
	if err != nil {
		return indexNowSubmissionPayload{}, err
	}
	payload := indexNowSubmissionPayload{
		Inventory: inventory,
		Diff:      diffPayload(diff),
	}
	if len(diff.Selected) == 0 {
		return payload, nil
	}
	engineRoot := strings.TrimSpace(s.config.EngineRoot)
	if engineRoot == "" {
		return indexNowSubmissionPayload{}, errors.New("Public Engine path is not configured")
	}
	config, err := blogsearch.ResolveIndexNowConfig(
		inventory.Origin,
		filepath.Join(engineRoot, "public"),
		indexNowEndpoint(s.config),
		indexNowKey(s.config),
		indexNowKeyLocation(s.config),
		true,
	)
	if err != nil {
		return indexNowSubmissionPayload{}, err
	}
	result, err := blogsearch.SubmitIndexNow(ctx, s.httpClient, diff.Selected, config)
	if err != nil {
		return indexNowSubmissionPayload{}, err
	}
	payload.Result.URLCount = result.URLCount
	payload.Result.BatchCount = result.BatchCount
	for _, batch := range result.Results {
		payload.Result.Results = append(payload.Result.Results, searchHTTPResult{HTTPStatus: batch.HTTPStatus})
	}
	return payload, nil
}

func (s *Server) submitBaidu(ctx context.Context, mode string, previous searchInventoryState) (baiduSubmissionPayload, error) {
	inventory, err := s.fetchSearchInventory(ctx)
	if err != nil {
		return baiduSubmissionPayload{}, err
	}
	diff, err := blogsearch.DiffInventories(searchInventoryToCore(previous), searchInventoryToCore(inventory), mode, false)
	if err != nil {
		return baiduSubmissionPayload{}, err
	}
	payload := baiduSubmissionPayload{
		Inventory: inventory,
		Diff:      diffPayload(diff),
	}
	if len(diff.Selected) == 0 {
		payload.Result.Complete = true
		return payload, nil
	}
	config, err := blogsearch.ResolveBaiduConfig(inventory.Origin, baiduSite(s.config), baiduToken(s.config))
	if err != nil {
		return baiduSubmissionPayload{}, err
	}
	result, err := blogsearch.SubmitBaidu(ctx, s.httpClient, diff.Selected, config)
	payload.Result.URLCount = result.URLCount
	payload.Result.SuccessCount = result.SuccessCount
	payload.Result.Remain = result.Remain
	payload.Result.BatchCount = result.BatchCount
	payload.Result.Complete = result.Complete
	for _, batch := range result.Results {
		payload.Result.Results = append(payload.Result.Results, searchHTTPResult{HTTPStatus: batch.HTTPStatus})
	}
	if err != nil {
		return payload, err
	}
	return payload, nil
}

type googleSitemapsPayload struct {
	Inventory searchInventoryState             `json:"inventory"`
	Result    []blogsearch.GoogleSitemapResult `json:"result"`
}

func (s *Server) googleAccessToken(ctx context.Context) (string, error) {
	raw := googleSearchConsoleServiceJSON(s.config)
	credentials, err := blogsearch.ParseGoogleServiceAccount(raw)
	if err != nil {
		return "", err
	}
	token, err := blogsearch.FetchGoogleAccessToken(ctx, s.httpClient, credentials)
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func (s *Server) submitGoogleSitemapsNative(ctx context.Context) (googleSitemapsPayload, error) {
	site := s.searchSite()
	inventory, err := s.fetchSearchInventory(ctx)
	if err != nil {
		return googleSitemapsPayload{}, err
	}
	accessToken, err := s.googleAccessToken(ctx)
	if err != nil {
		return googleSitemapsPayload{}, err
	}
	results, err := blogsearch.SubmitGoogleSitemaps(
		ctx,
		s.httpClient,
		site.GoogleProperty,
		inventory.Origin,
		accessToken,
		site.SitemapURL, site.URLListURL,
	)
	if err != nil {
		return googleSitemapsPayload{}, err
	}
	return googleSitemapsPayload{Inventory: inventory, Result: results}, nil
}

func coreInspectionResult(value blogsearch.GoogleInspectionResult) searchInspectionResult {
	return searchInspectionResult{
		URL:             value.URL,
		Verdict:         value.Verdict,
		CoverageState:   value.CoverageState,
		RobotsTxtState:  value.RobotsTxtState,
		IndexingState:   value.IndexingState,
		LastCrawlTime:   value.LastCrawlTime,
		PageFetchState:  value.PageFetchState,
		UserCanonical:   value.UserCanonical,
		GoogleCanonical: value.GoogleCanonical,
		CrawledAs:       value.CrawledAs,
		ReferringURLs:   append([]string{}, value.ReferringURLs...),
		Sitemap:         append([]string{}, value.Sitemap...),
	}
}

func (s *Server) checkIndexNowNative(ctx context.Context) (map[string]any, error) {
	engineRoot := strings.TrimSpace(s.config.EngineRoot)
	if engineRoot == "" {
		return nil, errors.New("Public Engine path is not configured")
	}
	config, err := blogsearch.ResolveIndexNowConfig(
		s.searchSite().Origin,
		filepath.Join(engineRoot, "public"),
		indexNowEndpoint(s.config),
		indexNowKey(s.config),
		indexNowKeyLocation(s.config),
		false,
	)
	if err != nil {
		return nil, err
	}
	if err := blogsearch.CheckIndexNowKey(ctx, s.httpClient, config); err != nil {
		return nil, err
	}
	return map[string]any{
		"endpoint":      config.Endpoint,
		"keyLocation":   config.KeyLocation,
		"keyFileStatus": 200,
	}, nil
}

func (s *Server) checkGoogleSearchConsoleNative(ctx context.Context) (map[string]any, error) {
	accessToken, err := s.googleAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	return blogsearch.CheckGoogleSearchConsoleSite(
		ctx,
		s.httpClient,
		s.searchSite().GoogleProperty,
		accessToken,
	)
}

func maxSearchHTTPStatus(results []searchHTTPResult) int {
	status := 0
	for _, result := range results {
		if result.HTTPStatus > status {
			status = result.HTTPStatus
		}
	}
	return status
}

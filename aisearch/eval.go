package aisearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type EvalCase struct {
	Language    string
	Query       string
	ExpectedAny []string
}

type EvalDocument struct {
	Key            string `json:"key"`
	Title          string `json:"title"`
	Collection     string `json:"collection"`
	Language       string `json:"language"`
	SourceURL      string `json:"sourceUrl"`
	SchemaVersion  int    `json:"schemaVersion"`
	Score          any    `json:"score,omitempty"`
	VectorScore    any    `json:"vectorScore,omitempty"`
	KeywordScore   any    `json:"keywordScore,omitempty"`
	RerankingScore any    `json:"rerankingScore,omitempty"`
}

type EvalDiagnostic struct {
	Language    string         `json:"language"`
	Query       string         `json:"query"`
	Status      string         `json:"status"`
	ExpectedAny []string       `json:"expectedAny"`
	MatchedRank *int           `json:"matchedRank"`
	TopFive     []EvalDocument `json:"topFive"`
}

var DefaultEvalCases = []EvalCase{
	{Language: "zh", Query: "Go CAS 为什么可以无锁？", ExpectedAny: []string{"/articles/concurrency-series-05-atomic-cas/", "/articles/concurrency-series-06-atomic-implementation/", "/notes/algorithm-concurrent/cas/", "/notes/algorithm-concurrent/lock-free-queue/"}},
	{Language: "zh", Query: "Go CAS 在 amd64 上如何实现？", ExpectedAny: []string{"/articles/concurrency-series-06-atomic-implementation/"}},
	{Language: "zh", Query: "AtomicInteger 底层如何实现？", ExpectedAny: []string{"/articles/concurrency-series-06-atomic-implementation/"}},
	{Language: "zh", Query: "Java synchronized 底层怎么实现？", ExpectedAny: []string{"/articles/concurrency-series-04-mutex-implementation/"}},
	{Language: "zh", Query: "volatile 能保证原子性吗？", ExpectedAny: []string{"/articles/concurrency-series-07-volatile/"}},
	{Language: "zh", Query: "Go Mutex 竞争失败以后发生什么？", ExpectedAny: []string{"/articles/concurrency-series-04-mutex-implementation/"}},
	{Language: "en", Query: "How is CAS implemented on amd64?", ExpectedAny: []string{"/en/articles/concurrency-series-06-atomic-implementation/"}},
	{Language: "en", Query: "What does volatile guarantee?", ExpectedAny: []string{"/en/articles/concurrency-series-07-volatile/"}},
}

type EvalConfig struct {
	AccountID    string
	APIToken     string
	InstanceName string
	Timeout      time.Duration
	RetryDelay   time.Duration
}

type evalSearchResponse struct {
	Success *bool `json:"success"`
	Errors  []struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result struct {
		Chunks []struct {
			Score          any `json:"score"`
			ScoringDetails struct {
				VectorScore    any `json:"vector_score"`
				KeywordScore   any `json:"keyword_score"`
				RerankingScore any `json:"reranking_score"`
			} `json:"scoring_details"`
			Item struct {
				Key      string `json:"key"`
				Metadata struct {
					Title         string      `json:"title"`
					Language      string      `json:"language"`
					SourceURL     string      `json:"source_url"`
					SchemaVersion json.Number `json:"schema_version"`
				} `json:"metadata"`
			} `json:"item"`
		} `json:"chunks"`
	} `json:"result"`
	Chunks []struct{} `json:"chunks"`
}

func collectionFromKey(key string) string {
	if strings.HasPrefix(key, "blog--articles--") {
		return "articles"
	}
	if strings.HasPrefix(key, "blog--notes--") {
		return "notes"
	}
	return ""
}

func evaluateSearch(ctx context.Context, client *http.Client, config EvalConfig, testCase EvalCase) (EvalDiagnostic, error) {
	body, _ := json.Marshal(map[string]any{
		"query": testCase.Query,
		"ai_search_options": map[string]any{
			"retrieval": map[string]any{
				"retrieval_type":     "hybrid",
				"fusion_method":      "rrf",
				"keyword_match_mode": "or",
				"boost_by":           []map[string]string{{"field": "priority", "direction": "desc"}},
				"filters":            map[string]string{"language": testCase.Language},
				"match_threshold":    0,
				"max_num_results":    20,
				"context_expansion":  1,
				"return_on_failure":  false,
			},
			"query_rewrite": map[string]any{"enabled": false},
			"reranking": map[string]any{
				"enabled":         true,
				"model":           "@cf/baai/bge-reranker-base",
				"match_threshold": 0.1,
			},
		},
	})
	endpoint := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/ai-search/instances/%s/search",
		config.AccountID,
		config.InstanceName,
	)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return EvalDiagnostic{}, err
	}
	request.Header.Set("authorization", "Bearer "+config.APIToken)
	request.Header.Set("content-type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return EvalDiagnostic{}, err
	}
	payload, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	response.Body.Close()
	if readErr != nil {
		return EvalDiagnostic{}, readErr
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return EvalDiagnostic{}, fmt.Errorf("AI Search %d for %s: %s", response.StatusCode, testCase.Query, strings.TrimSpace(string(payload)))
	}
	var decoded evalSearchResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return EvalDiagnostic{}, err
	}
	if decoded.Success != nil && !*decoded.Success {
		return EvalDiagnostic{}, fmt.Errorf("AI Search returned success=false for %s", testCase.Query)
	}

	seen := map[string]struct{}{}
	documents := []EvalDocument{}
	for _, chunk := range decoded.Result.Chunks {
		key := strings.TrimSpace(chunk.Item.Key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		schemaVersion := 0
		if value := string(chunk.Item.Metadata.SchemaVersion); value != "" {
			_, _ = fmt.Sscan(value, &schemaVersion)
		}
		documents = append(documents, EvalDocument{
			Key:            key,
			Title:          chunk.Item.Metadata.Title,
			Collection:     collectionFromKey(key),
			Language:       chunk.Item.Metadata.Language,
			SourceURL:      chunk.Item.Metadata.SourceURL,
			SchemaVersion:  schemaVersion,
			Score:          chunk.Score,
			VectorScore:    chunk.ScoringDetails.VectorScore,
			KeywordScore:   chunk.ScoringDetails.KeywordScore,
			RerankingScore: chunk.ScoringDetails.RerankingScore,
		})
	}
	if len(documents) > 5 {
		documents = documents[:5]
	}

	diagnostic := EvalDiagnostic{
		Language:    testCase.Language,
		Query:       testCase.Query,
		Status:      "fail",
		ExpectedAny: append([]string{}, testCase.ExpectedAny...),
		TopFive:     documents,
	}
	for index, document := range documents {
		if document.Language != testCase.Language {
			continue
		}
		for _, expected := range testCase.ExpectedAny {
			if strings.Contains(document.SourceURL, expected) {
				rank := index + 1
				diagnostic.MatchedRank = &rank
				diagnostic.Status = "pass"
				return diagnostic, nil
			}
		}
	}
	return diagnostic, nil
}

func Evaluate(ctx context.Context, client *http.Client, config EvalConfig, cases []EvalCase, onDiagnostic func(EvalDiagnostic)) error {
	if strings.TrimSpace(config.AccountID) == "" || strings.TrimSpace(config.APIToken) == "" {
		return errors.New("Cloudflare account ID and AI Search token are required")
	}
	if config.InstanceName == "" {
		config.InstanceName = "thinkerqaq-blog"
	}
	if config.Timeout <= 0 {
		config.Timeout = 3 * time.Minute
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 10 * time.Second
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if len(cases) == 0 {
		cases = DefaultEvalCases
	}

	deadline := time.Now().Add(config.Timeout)
	pending := append([]EvalCase{}, cases...)
	for len(pending) > 0 {
		failed := []EvalCase{}
		for _, testCase := range pending {
			diagnostic, err := evaluateSearch(ctx, client, config, testCase)
			if err != nil {
				return err
			}
			if onDiagnostic != nil {
				onDiagnostic(diagnostic)
			}
			if diagnostic.Status != "pass" {
				failed = append(failed, testCase)
			}
		}
		if len(failed) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%d AI Search retrieval regression case(s) failed after %s", len(failed), config.Timeout)
		}
		timer := time.NewTimer(config.RetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		pending = failed
	}
	return nil
}

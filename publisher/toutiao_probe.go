package publisher

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// ToutiaoProbeResult contains only non-secret verification outcomes.
type ToutiaoProbeResult struct {
	Authenticated  bool   `json:"authenticated"`
	CSRFReady      bool   `json:"csrfReady"`
	CreatedDraftID string `json:"createdDraftID,omitempty"`
	DraftVerified  bool   `json:"draftVerified,omitempty"`
}

// ProbeToutiaoHTTP verifies the native HTTPS authentication and CSRF mechanism.
// If createDraft is explicitly true, it creates exactly one PRIVATE draft and
// reads back its ID. Never publishes or edits any existing article.
func ProbeToutiaoHTTP(ctx context.Context, base *http.Client, session Session, createDraft bool) (ToutiaoProbeResult, error) {
	result := ToutiaoProbeResult{}
	adapterValue, err := NewToutiaoAdapter(base, session)
	if err != nil {
		return result, err
	}
	adapter := adapterValue.(*toutiaoAdapter)
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return result, err
	}
	if !auth.Authenticated {
		return result, errors.New("Toutiao authenticated session not available")
	}
	result.Authenticated = true
	if !createDraft {
		_, err = fetchToutiaoCSRF(ctx, adapter.client, adapter.userAgent, time.Now())
		if err != nil {
			return result, err
		}
		result.CSRFReady = true
		return result, nil
	}
	draft, err := adapter.CreateDraft(ctx, DraftInput{
		Title:    "BlogCTL 纯接口测试草稿",
		Markdown: "这是 BlogCTL 创建的临时测试草稿，仅用于验证今日头条接口。请勿发布。",
	})
	if err != nil {
		return result, err
	}
	result.CSRFReady = true
	result.CreatedDraftID = draft.ID
	drafts, err := adapter.listDrafts(ctx)
	if err != nil {
		return result, err
	}
	for _, candidate := range drafts {
		if candidate.ID == draft.ID && candidate.Title == "BlogCTL 纯接口测试草稿" {
			result.DraftVerified = true
			return result, nil
		}
	}
	return result, errors.New("draft POST succeeded but the newly created draft was not confirmed in creator inventory")
}

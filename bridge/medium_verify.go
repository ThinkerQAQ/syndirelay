package bridge

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

// Medium's editor sends numeric paragraph types, while its read-only
// GraphQL response uses named enum types. Decode both into one model.
type mediumParagraphType int

func (value *mediumParagraphType) UnmarshalJSON(raw []byte) error {
	var numeric int
	if err := json.Unmarshal(raw, &numeric); err == nil {
		*value = mediumParagraphType(numeric)
		return nil
	}
	var name string
	if err := json.Unmarshal(raw, &name); err != nil {
		return fmt.Errorf("Medium paragraph type must be numeric or an enum: %w", err)
	}
	types := map[string]int{"P": 1, "H3": 3, "IMG": 4, "BQ": 6, "PRE": 8, "ULI": 9, "OLI": 10}
	numeric, ok := types[name]
	if !ok {
		return fmt.Errorf("unknown Medium saved paragraph type %q", name)
	}
	*value = mediumParagraphType(numeric)
	return nil
}

// GraphQL returns enum strings for inline marks; the editor's own deltas
// use numeric markup IDs. Normalize both before comparing spans.
type mediumSavedMarkupType int

func (value *mediumSavedMarkupType) UnmarshalJSON(raw []byte) error {
	var numeric int
	if err := json.Unmarshal(raw, &numeric); err == nil {
		*value = mediumSavedMarkupType(numeric)
		return nil
	}
	var name string
	if err := json.Unmarshal(raw, &name); err != nil {
		return fmt.Errorf("Medium markup type must be numeric or an enum: %w", err)
	}
	types := map[string]int{"STRONG": 1, "EM": 2, "A": 3, "CODE": 10, "DEL": 11}
	numeric, ok := types[name]
	if !ok {
		return fmt.Errorf("unknown Medium saved markup type %q", name)
	}
	*value = mediumSavedMarkupType(numeric)
	return nil
}

type mediumSavedMarkup struct {
	Type  mediumSavedMarkupType `json:"type"`
	Start int                   `json:"start"`
	End   int                   `json:"end"`
}

func verifyMediumSavedMarkups(expected any, actual []mediumSavedMarkup) error {
	if expected == nil && len(actual) == 0 {
		return nil
	}
	raw, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	var wanted []mediumSavedMarkup
	if err := json.Unmarshal(raw, &wanted); err != nil {
		return err
	}
	if len(wanted) != len(actual) {
		return fmt.Errorf("Medium inline styles differ: expected %d, got %d", len(wanted), len(actual))
	}
	seen := make(map[mediumSavedMarkup]int, len(wanted))
	for _, item := range wanted {
		seen[item]++
	}
	for _, item := range actual {
		seen[item]--
	}
	for _, count := range seen {
		if count != 0 {
			return errors.New("Medium inline style type or UTF-16 offset differs")
		}
	}
	return nil
}

// Only the fields required to verify native text, PRE and IMAGE paragraphs.
type mediumSavedParagraph struct {
	Type     mediumParagraphType `json:"type"`
	Text     string              `json:"text"`
	Markups  []mediumSavedMarkup `json:"markups"`
	Metadata struct {
		ID             string `json:"id"`
		OriginalWidth  int    `json:"originalWidth"`
		OriginalHeight int    `json:"originalHeight"`
	} `json:"metadata"`
}

const mediumDraftBodyQuery = `query BlogCTLMediumDraftVerification($postId: ID!) {
  postResult(id: $postId) {
    __typename
    ... on Post {
      id
      content {
        bodyModel {
          paragraphs {
            type
            text
            markups { type start end }
            metadata { id originalWidth originalHeight }
          }
        }
      }
    }
  }
}`

func (c mediumClient) readDraftParagraphs(ctx context.Context, session platformSession, postID string) ([]mediumSavedParagraph, error) {
	payload, err := json.Marshal([]any{map[string]any{
		"operationName": "BlogCTLMediumDraftVerification",
		"variables":     map[string]any{"postId": postID},
		"query":         mediumDraftBodyQuery,
	}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediumOrigin+"/_/graphql", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	setMediumGraphQLHeaders(req, session, mediumOrigin+"/p/"+postID+"/edit", "BlogCTLMediumDraftVerification")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Medium draft readback HTTP %d", resp.StatusCode)
	}
	var results []struct {
		Data struct {
			PostResult struct {
				ID      string `json:"id"`
				Content *struct {
					BodyModel *struct {
						Paragraphs []mediumSavedParagraph `json:"paragraphs"`
					} `json:"bodyModel"`
				} `json:"content"`
			} `json:"postResult"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	normalized := strings.TrimSpace(string(raw))
	if delimiter := strings.Index(normalized, "</x>"); delimiter >= 0 {
		normalized = strings.TrimSpace(normalized[delimiter+len("</x>"):])
	}
	if strings.HasPrefix(normalized, "])}") {
		if marker := strings.Index(normalized, "[{"); marker >= 0 {
			normalized = normalized[marker:]
		}
	}
	if err := json.Unmarshal([]byte(normalized), &results); err != nil || len(results) != 1 {
		return nil, errors.New("Medium draft readback returned invalid GraphQL JSON")
	}
	result := results[0]
	if len(result.Errors) > 0 {
		return nil, errors.New("Medium draft readback GraphQL query was rejected")
	}
	post := result.Data.PostResult
	if post.ID != postID || post.Content == nil || post.Content.BodyModel == nil || post.Content.BodyModel.Paragraphs == nil {
		return nil, errors.New("Medium draft readback did not return expected post and paragraphs")
	}
	return post.Content.BodyModel.Paragraphs, nil
}

// Compiled deltas are JSON-decoded into map[string]any; numbers may be float64.
func mediumDeltaNumber(value any) (int, bool) {
	switch n := value.(type) {
	case int:
		return n, true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	}
	return 0, false
}

func verifyMediumParagraphs(deltas []map[string]any, actual []mediumSavedParagraph) error {
	if len(actual) != len(deltas) {
		return fmt.Errorf("Medium draft has %d paragraphs, expected %d", len(actual), len(deltas))
	}
	for index, delta := range deltas {
		expected, ok := delta["paragraph"].(map[string]any)
		if !ok {
			return fmt.Errorf("expected paragraph %d is invalid", index)
		}
		typ, ok := mediumDeltaNumber(expected["type"])
		if !ok {
			return fmt.Errorf("expected paragraph %d type is invalid", index)
		}
		got := actual[index]
		if int(got.Type) != typ {
			return fmt.Errorf("Medium paragraph %d type %d, expected %d", index, got.Type, typ)
		}
		text, _ := expected["text"].(string)
		if got.Text != text {
			return fmt.Errorf("Medium paragraph %d text differs (type %d)", index, typ)
		}
		if err := verifyMediumSavedMarkups(expected["markups"], got.Markups); err != nil {
			return fmt.Errorf("Medium paragraph %d formatting: %w", index, err)
		}
		if typ != 4 {
			continue
		}
		meta, ok := expected["metadata"].(map[string]any)
		if !ok {
			return fmt.Errorf("Medium image %d missing expected metadata", index)
		}
		id, _ := meta["id"].(string)
		width, _ := mediumDeltaNumber(meta["originalWidth"])
		height, _ := mediumDeltaNumber(meta["originalHeight"])
		if !strings.HasPrefix(id, "1*") || got.Metadata.ID != id || got.Metadata.OriginalWidth != width || got.Metadata.OriginalHeight != height {
			return fmt.Errorf("Medium image %d missing or incorrect metadata", index)
		}
	}
	return nil
}

// Readback is mandatory before reporting a complete draft. Medium GraphQL may
// lag behind the save response; bounded retries cannot hide a failed write.
func (c mediumClient) verifyDraft(ctx context.Context, session platformSession, postID string, deltas []map[string]any) error {
	var last error
	for i := 0; i < 4; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(750 * time.Millisecond):
			}
		}
		paragraphs, err := c.readDraftParagraphs(ctx, session, postID)
		if err == nil {
			err = verifyMediumParagraphs(deltas, paragraphs)
		}
		if err == nil {
			return nil
		}
		last = err
	}
	return fmt.Errorf("remote draft integrity check failed: %w", last)
}

// verifyMediumBrowserSavedParagraphs uses the server's persisted body as
// authoritative evidence when the editor's DraftSaved badge is delayed.
// Medium inserts an IMG block and clears each placeholder P in-place. Thus
// filtering IMG blocks leaves exactly the compiler's original text model.
func verifyMediumBrowserSavedParagraphs(draft mediumDraft, saved []mediumSavedParagraph) error {
	plan := draft.BrowserPlan
	if plan == nil {
		return errors.New("missing Medium browser compiler plan")
	}
	wantText := len(plan.Deltas) + 1 // title and original body paragraphs
	wantImages := len(plan.Images)
	if len(saved) != wantText+wantImages {
		return fmt.Errorf("remote Medium draft has %d saved paragraphs; expected %d (text %d + images %d)",
			len(saved), wantText+wantImages, wantText, wantImages)
	}
	images := 0
	index := 0
	verifyText := func(typ int, text string, markups any) error {
		for index < len(saved) && saved[index].Type == 4 {
			image := saved[index]
			if !strings.HasPrefix(image.Metadata.ID, "1*") || image.Metadata.OriginalWidth <= 0 || image.Metadata.OriginalHeight <= 0 {
				return fmt.Errorf("Medium remote image %d is missing native CDN metadata", images+1)
			}
			images++
			index++
		}
		if index >= len(saved) {
			return errors.New("Medium remote text paragraphs are missing")
		}
		if int(saved[index].Type) != typ || saved[index].Text != text {
			return fmt.Errorf("Medium remote text paragraph %d differs from the compiler", index)
		}
		if err := verifyMediumSavedMarkups(markups, saved[index].Markups); err != nil {
			return fmt.Errorf("Medium remote paragraph %d formatting: %w", index, err)
		}
		index++
		return nil
	}
	if err := verifyText(3, draft.Title, nil); err != nil {
		return err
	}
	markers := map[string]bool{}
	for _, image := range plan.Images {
		markers[image.Marker] = true
	}
	for _, delta := range plan.Deltas {
		paragraph, ok := delta["paragraph"].(map[string]any)
		if !ok {
			return errors.New("Medium compiler paragraph data is missing")
		}
		typ, ok := mediumDeltaNumber(paragraph["type"])
		if !ok {
			return errors.New("Medium compiler paragraph type is invalid")
		}
		text, _ := paragraph["text"].(string)
		if markers[text] {
			if typ != 1 {
				return errors.New("Medium image marker is not a plain paragraph")
			}
			text = "" // the editor clears the marker after native paste
		}
		if err := verifyText(typ, text, paragraph["markups"]); err != nil {
			return err
		}
	}
	for index < len(saved) && saved[index].Type == 4 {
		image := saved[index]
		if !strings.HasPrefix(image.Metadata.ID, "1*") || image.Metadata.OriginalWidth <= 0 || image.Metadata.OriginalHeight <= 0 {
			return fmt.Errorf("Medium remote image %d is missing native CDN metadata", images+1)
		}
		images++
		index++
	}
	if index != len(saved) || images != wantImages {
		return fmt.Errorf("Medium remote image integrity mismatch: %d saved, expected %d", images, wantImages)
	}
	return nil
}

// The editor save indicator is UI state, not a persistence proof. Confirm
// actual server-side paragraphs and images before accepting a delayed badge.
func (c mediumClient) verifyMediumBrowserPersisted(ctx context.Context, session platformSession, postID string, draft mediumDraft) error {
	var last error
	for i := 0; i < 4; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(900 * time.Millisecond):
			}
		}
		paragraphs, err := c.readDraftParagraphs(ctx, session, postID)
		if err == nil {
			err = verifyMediumBrowserSavedParagraphs(draft, paragraphs)
		}
		if err == nil {
			return nil
		}
		last = err
	}
	return fmt.Errorf("Medium remote persistence was not confirmed: %w", last)
}

package bridge

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

var mediumBrowserMarker = regexp.MustCompile("^\\[BLOGCTL_IMAGE_[0-9]{3}\\]$")

// Save the compiler's native text/PRE/list/heading paragraphs first.
// The editor, not a private image Delta request, inserts PNGs at markers.
func (c mediumClient) createDraftUsingBrowserImages(ctx context.Context, session platformSession,
	draft mediumDraft, input publisher.DraftInput, postID, mediumURL string) (map[string]any, error) {
	return c.completeDraftBrowserImages(ctx, session, draft, input, postID, mediumURL, true)
}

func (c mediumClient) resumeDraftUsingBrowserImages(ctx context.Context, session platformSession,
	draft mediumDraft, input publisher.DraftInput, postID string) (map[string]any, error) {
	// A delayed autosave can complete after the previous task has failed.
	// Verify that exact saved revision first; a complete document needs no
	// image-marker replay, downloads or second text replacement.
	if draft.BrowserPlan != nil && c.browserVerify != nil && c.browserDocument != nil {
		if err := c.browserVerify(ctx, postID, len(draft.BrowserPlan.Images)); err == nil {
			if err := c.browserDocument(ctx, postID, draft); err != nil {
				return mediumDraftResult(postID, "", draft, 0, true), fmt.Errorf("saved Medium editor differs from retry source: %w", err)
			}
			return mediumDraftResult(postID, "", draft, 0, true), nil
		}
	}
	return c.completeDraftBrowserImages(ctx, session, draft, input, postID, "", false)
}

func (c mediumClient) completeDraftBrowserImages(ctx context.Context, session platformSession,
	draft mediumDraft, input publisher.DraftInput, postID, mediumURL string, writeText bool) (map[string]any, error) {
	partial := func(err error) (map[string]any, error) {
		return mediumDraftResult(postID, mediumURL, draft, 0, true),
			fmt.Errorf("Medium draft %s exists but browser creation is incomplete: %w", postID, err)
	}
	plan := draft.BrowserPlan
	if plan == nil || len(plan.Deltas) == 0 {
		return partial(fmt.Errorf("compiler browser plan is empty"))
	}
	if len(plan.Deltas) != len(draft.Deltas) {
		return partial(fmt.Errorf("compiler browser plan paragraph mismatch"))
	}
	if len(plan.Images) > 0 && (c.browserInsert == nil || c.browserVerify == nil) {
		return partial(fmt.Errorf("Medium browser editor insertion is unavailable"))
	}
	for index, item := range plan.Images {
		if !mediumBrowserMarker.MatchString(item.Marker) || item.Marker != fmt.Sprintf("[BLOGCTL_IMAGE_%03d]", index+1) {
			return partial(fmt.Errorf("invalid image marker at index %d", index))
		}
		count := 0
		for _, delta := range plan.Deltas {
			para, _ := delta["paragraph"].(map[string]any)
			if para != nil && para["text"] == item.Marker {
				count++
			}
		}
		if count != 1 {
			return partial(fmt.Errorf("image marker %s is not unique", item.Marker))
		}
	}
	deltas := make([]map[string]any, 0, len(plan.Deltas)+1)
	deltas = append(deltas, map[string]any{
		"type": 1, "index": 0,
		"paragraph": map[string]any{"type": 3, "text": draft.Title, "markups": []any{}},
	})
	for _, original := range plan.Deltas {
		delta := cloneMediumDelta(original)
		delta["index"] = len(deltas)
		if _, unexpected := delta["image"]; unexpected {
			return partial(fmt.Errorf("browser text draft contains an image Delta"))
		}
		deltas = append(deltas, delta)
	}
	if writeText {
		if _, err := c.writeDeltas(ctx, session, postID, -1, deltas); err != nil {
			return partial(fmt.Errorf("native text save: %w", err))
		}
	}
	for index, item := range plan.Images {
		image, err := c.loadImage(ctx, input, item.URL)
		if err != nil {
			return partial(fmt.Errorf("image %d load: %w", index+1, err))
		}
		if err := c.browserInsert(ctx, postID, item.Marker, image); err != nil {
			return partial(fmt.Errorf("image %d browser insert: %w", index+1, err))
		}
	}
	if c.browserVerify != nil {
		if err := c.browserVerify(ctx, postID, len(plan.Images)); err != nil {
			// Only a late DraftSaved badge may fall back to authoritative remote
			// GraphQL readback. Missing images, stale editor content and conflicts
			// remain hard failures and must never be silently accepted.
			if !strings.Contains(err.Error(), "Medium editor has not confirmed autosave") {
				return partial(fmt.Errorf("browser autosave verification: %w", err))
			}
			var persistedErr error
			if c.browserDocument != nil {
				// Published GraphQL is the old public body. Wait for and
				// verify the saved editor revision that this job wrote.
				persistedErr = c.browserDocument(ctx, postID, draft)
			} else {
				persistedErr = c.verifyMediumBrowserPersisted(ctx, session, postID, draft)
			}
			if persistedErr != nil {
				return partial(fmt.Errorf("browser autosave verification: %w; %v", err, persistedErr))
			}
		}
	}
	if c.browserDocument != nil {
		if err := c.browserDocument(ctx, postID, draft); err != nil {
			return partial(fmt.Errorf("complete document verification: %w", err))
		}
	}
	return mediumDraftResult(postID, mediumURL, draft, 0, true), nil
}

// mediumFullReplacementDeltas is the same title+body insert sequence as
// creation, preceded by reverse-order deletion of the old document.
// All deltas go to the same revision-checked request and post ID.
// Never mutate compiler-owned deltas: retries need them intact.
func mediumFullReplacementDeltas(existingCount int, title string, body []map[string]any) ([]map[string]any, error) {
	if existingCount < 1 || strings.TrimSpace(title) == "" {
		return nil, errors.New("Medium complete replacement requires an existing draft and title")
	}
	deltas := make([]map[string]any, 0, existingCount+len(body)+1)
	for index := existingCount - 1; index >= 0; index-- {
		deltas = append(deltas, map[string]any{"type": 2, "index": index})
	}
	deltas = append(deltas, map[string]any{
		"type": 1, "index": 0,
		"paragraph": map[string]any{"type": 3, "text": title, "markups": []any{}},
	})
	for i, original := range body {
		delta := cloneMediumDelta(original)
		if _, ok := delta["paragraph"].(map[string]any); !ok {
			return nil, fmt.Errorf("Medium replacement paragraph %d is missing", i)
		}
		delta["index"] = i + 1
		deltas = append(deltas, delta)
	}
	return deltas, nil
}

// updateDraftUsingBrowserImages replaces ALL native paragraphs of the same
// draft or pending edit of a published story, including its title.
// Browser inserts PNGs after the full text has been persisted and checked.
// The editor must not be concurrently open during the revisioned replacement.
func (c mediumClient) updateDraftUsingBrowserImages(ctx context.Context,
	session platformSession, draft mediumDraft, input publisher.DraftInput,
	postID string) (map[string]any, error) {
	if draft.BrowserPlan == nil || c.browserInsert == nil || c.browserVerify == nil {
		return nil, errors.New("Medium browser image update flow unavailable")
	}
	session = c.primeXSRF(ctx, session)
	meta, err := c.postMeta(ctx, session, postID)
	if err != nil {
		return nil, err
	}
	// For a published story, Medium stores edits separately from the public
	// body until its Save and publish action. Keep that action manual.
	publishedEdit := meta.FirstPublishedAt > 0
	var count int
	if publishedEdit && c.browserParagraphCount != nil {
		count, err = c.browserParagraphCount(ctx, postID)
		if err == nil {
			meta, err = c.postMeta(ctx, session, postID)
		}
	} else {
		count, err = c.paragraphCount(ctx, session, postID)
	}
	if err != nil {
		return nil, err
	}
	if count < 1 {
		return nil, errors.New("Existing Medium draft has no title")
	}
	// A Medium update is a fresh compiler document on the same draft ID.
	// Replace the title as well as the body, avoiding style inherited from
	// the existing title/paragraph boundary.
	deltas, err := mediumFullReplacementDeltas(count, draft.Title, draft.BrowserPlan.Deltas)
	if err != nil {
		return nil, err
	}
	for _, paragraph := range draft.BrowserPlan.Deltas {
		if _, bad := paragraph["image"]; bad {
			return nil, errors.New("Browser draft text unexpectedly contains image delta")
		}
	}
	if _, err := c.writeDeltas(ctx, session, postID, meta.LatestRev, deltas); err != nil {
		return nil, fmt.Errorf("Medium browser draft full text replacement failed: %w", err)
	}
	// Read back the completely replaced source text before beginning browser
	// image insertions. No image upload should touch a partially saved model.
	if !publishedEdit {
		if err := c.verifyDraft(ctx, session, postID, deltas[count:]); err != nil {
			return nil, fmt.Errorf("Medium browser draft full text verification failed: %w", err)
		}
	}
	if c.browserDocument != nil {
		if err := c.browserDocument(ctx, postID, draft); err != nil {
			return nil, fmt.Errorf("Medium browser draft full text verification failed: %w", err)
		}
	}
	result, err := c.completeDraftBrowserImages(ctx, session, draft, input, postID, meta.MediumURL, false)
	if err != nil {
		return result, err
	}
	// A published post's public GraphQL body is the OLD published version,
	// not the editor's new saved-but-unpublished revision. Browser Verify
	// already proves image/marker/CDN state and the Medium Saved indicator;
	// reading public GraphQL here would yield a false mismatch.
	if !publishedEdit {
		if err := c.verifyMediumBrowserPersisted(ctx, session, postID, draft); err != nil {
			return result, fmt.Errorf("Medium full document replacement integrity check failed: %w", err)
		}
	}
	return result, nil
}

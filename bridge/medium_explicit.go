package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	blogcompiler "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/compiler"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

// Medium has an existing, verified browser-native transport in the bridge.
// Do not route it through publisher.newAdapter: no separate Medium adapter
// exists there. Explicit create/update targets are selected per task, without
// implicitly reading or changing the user's stored publication binding.
func runMediumExplicit(ctx context.Context, session publisher.Session, httpClient *http.Client,
	compiled blogcompiler.CompiledArticle, input publisher.DraftInput, distributionRoot,
	operation string, target explicitTaskTarget, publishAfter bool,
	browserUploaders ...func(context.Context, string, publisher.RehostImage) (mediumUploadResult, error),
) (publisher.ExplicitResult, error) {
	return runMediumExplicitWithProgress(ctx, session, httpClient, compiled, input, distributionRoot,
		operation, target, publishAfter, nil, browserUploaders...)
}

// On real jobs, record the allocated ID before image preparation and writing
// the body. A failed draft can then be inspected instead of disappearing from
// the job history.
func runMediumExplicitWithProgress(ctx context.Context, session publisher.Session, httpClient *http.Client,
	compiled blogcompiler.CompiledArticle, input publisher.DraftInput, distributionRoot,
	operation string, target explicitTaskTarget, publishAfter bool, onCreated func(string),
	browserUploaders ...func(context.Context, string, publisher.RehostImage) (mediumUploadResult, error),
) (publisher.ExplicitResult, error) {
	return runMediumExplicitConfigured(ctx, session, httpClient, compiled, input, distributionRoot,
		operation, target, publishAfter, onCreated, nil, browserUploaders...)
}

type mediumBrowserFlow struct {
	insert         func(context.Context, string, string, publisher.RehostImage) error
	verify         func(context.Context, string, int) error
	canonical      func(context.Context, string, string) error
	paragraphCount func(context.Context, string) (int, error)
	document       func(context.Context, string, mediumDraft) error
}

func runMediumExplicitWithBrowser(ctx context.Context, session publisher.Session, httpClient *http.Client,
	compiled blogcompiler.CompiledArticle, input publisher.DraftInput, distributionRoot,
	operation string, target explicitTaskTarget, publishAfter bool, onCreated func(string),
	flow mediumBrowserFlow, browserUploaders ...func(context.Context, string, publisher.RehostImage) (mediumUploadResult, error),
) (publisher.ExplicitResult, error) {
	return runMediumExplicitConfigured(ctx, session, httpClient, compiled, input, distributionRoot,
		operation, target, publishAfter, onCreated, &flow, browserUploaders...)
}

func runMediumExplicitConfigured(ctx context.Context, session publisher.Session, httpClient *http.Client,
	compiled blogcompiler.CompiledArticle, input publisher.DraftInput, distributionRoot,
	operation string, target explicitTaskTarget, publishAfter bool, onCreated func(string),
	flow *mediumBrowserFlow, browserUploaders ...func(context.Context, string, publisher.RehostImage) (mediumUploadResult, error),
) (publisher.ExplicitResult, error) {
	medium := mediumClient{httpClient: httpClient, verifyAfterCreate: true, onStoryCreated: onCreated}
	if flow != nil {
		medium.browserInsert, medium.browserVerify = flow.insert, flow.verify
		medium.browserParagraphCount, medium.browserDocument = flow.paragraphCount, flow.document
	}
	if len(browserUploaders) > 0 {
		medium.browserUpload = browserUploaders[0]
	}
	auth := mediumPlatformSession(session)
	publish := func(postID string) (publisher.ExplicitResult, error) {
		remote, err := medium.publishDraft(ctx, auth, postID, compiled.Title, compiled.Description)
		if err != nil {
			return publisher.ExplicitResult{ID: postID, URL: mediumOrigin + "/p/" + url.PathEscape(postID) + "/edit", Kind: "draft-updated"}, err
		}
		id, _ := remote["postId"].(string)
		publicURL, _ := remote["url"].(string)
		if id != postID || !validMediumPublishedURL(publicURL) {
			return publisher.ExplicitResult{}, errors.New("Medium publish returned an unexpected post ID or URL")
		}
		return publisher.ExplicitResult{ID: id, URL: publicURL, Kind: "published"}, nil
	}
	if operation == "publish-draft" {
		if target.State != "draft" || target.ID == "" {
			return publisher.ExplicitResult{}, errors.New("Medium publish requires a selected draft")
		}
		return publish(target.ID)
	}
	if operation != "create" && operation != "update" && operation != "resume-medium" {
		return publisher.ExplicitResult{}, fmt.Errorf("unsupported Medium operation %q", operation)
	}
	if operation == "create" && publishAfter {
		return publisher.ExplicitResult{}, errors.New("Medium create only saves a draft; publish it manually in Medium")
	}
	if operation == "update" && target.State != "draft" && target.State != "published" {
		return publisher.ExplicitResult{}, errors.New("Medium update requires a selected draft or published article")
	}
	if operation == "update" && target.State == "published" && publishAfter {
		return publisher.ExplicitResult{}, errors.New("Medium published edits must be saved for manual Save and publish")
	}
	if len(compiled.Payload) == 0 {
		return publisher.ExplicitResult{}, errors.New("compiled Medium article is missing transport payload")
	}
	var draft mediumDraft
	if err := json.Unmarshal(compiled.Payload, &draft); err != nil {
		return publisher.ExplicitResult{}, fmt.Errorf("invalid compiled Medium payload: %w", err)
	}
	if _, err := writeMediumFallback(distributionRoot, compiled); err != nil {
		return publisher.ExplicitResult{}, err
	}
	var result map[string]any
	var err error
	if operation == "resume-medium" {
		if !mediumEditorPostID.MatchString(target.ID) || draft.BrowserPlan == nil || medium.browserInsert == nil {
			return publisher.ExplicitResult{}, errors.New("Medium resume requires a known draft ID and browser driver")
		}
		result, err = medium.resumeDraftUsingBrowserImages(ctx, auth, draft, input, target.ID)
	} else if operation == "create" {
		if target.ID != "" {
			return publisher.ExplicitResult{}, errors.New("Medium create must not specify an existing remote post")
		}
		result, err = medium.createDraft(ctx, auth, draft, input)
	} else {
		if target.ID == "" {
			return publisher.ExplicitResult{}, errors.New("Medium update requires a selected draft ID")
		}
		if draft.BrowserPlan != nil && medium.browserInsert != nil {
			result, err = medium.updateDraftUsingBrowserImages(ctx, auth, draft, input, target.ID)
		} else {
			result, err = medium.updateDraft(ctx, auth, target.ID, draft, input)
		}
	}
	if err != nil {
		// Preserve a newly created but incomplete remote draft reference.
		// Do not mark it as saved or published.
		if result != nil {
			if id, ok := result["postId"].(string); ok && id != "" {
				return publisher.ExplicitResult{ID: id, URL: mediumOrigin + "/p/" + url.PathEscape(id) + "/edit", Kind: "draft-created"}, err
			}
		}
		return publisher.ExplicitResult{}, err
	}
	id, _ := result["postId"].(string)
	draftURL, _ := result["draftUrl"].(string)
	if id == "" || (operation == "update" && id != target.ID) || draftURL != mediumOrigin+"/p/"+url.PathEscape(id)+"/edit" {
		return publisher.ExplicitResult{}, errors.New("Medium returned an incomplete or inconsistent draft reference")
	}
	if flow != nil && flow.canonical != nil && draft.CanonicalURL != "" {
		if err := flow.canonical(ctx, id, draft.CanonicalURL); err != nil {
			return publisher.ExplicitResult{ID: id, URL: mediumOrigin + "/p/" + url.PathEscape(id) + "/edit", Kind: "draft-updated"}, fmt.Errorf("Medium content saved but canonical settings incomplete: %w", err)
		}
	}
	kind := "draft-created"
	if operation == "update" || operation == "resume-medium" {
		kind = "draft-updated"
		if target.State == "published" {
			kind = "published-edit-saved"
		}
	}
	saved := publisher.ExplicitResult{ID: id, URL: draftURL, Kind: kind}
	if !publishAfter {
		return saved, nil
	}
	published, err := publish(id)
	if err != nil {
		return saved, fmt.Errorf("Medium draft %s was saved, but publish failed: %w", id, err)
	}
	return published, nil
}

func validMediumPublishedURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() == "medium.com" &&
		parsed.Port() == "" && parsed.User == nil && parsed.Path != ""
}

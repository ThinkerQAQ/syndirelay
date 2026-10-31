package publisher

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Service struct {
	HTTPClient      *http.Client
	Now             func() time.Time
	PublicationPath string
}

func (s Service) publicationPath() string {
	return strings.TrimSpace(s.PublicationPath)
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Service) authenticatedAdapter(ctx context.Context, platform string, session Session) (Adapter, error) {
	adapter, err := newAdapter(platform, s.HTTPClient, session)
	if err != nil {
		return nil, err
	}
	auth, err := adapter.CheckAuth(ctx)
	if err != nil {
		return nil, err
	}
	if !auth.Authenticated {
		return nil, platformError(ErrAuthExpired, platform, "auth", http.StatusUnauthorized, "browser session is not authenticated", false)
	}
	return adapter, nil
}

func retryableAuthError(err error) bool {
	return IsKind(err, ErrAuthExpired) || IsKind(err, ErrCSRF)
}

func publicationHasPublishedState(state PublicationState) bool {
	return strings.TrimSpace(state.PublishedRemoteID) != "" || strings.TrimSpace(state.PublishedURL) != ""
}

func mayRecreateMissingDraft(platform string, state PublicationState) bool {
	if (platform == "toutiao" || platform == "devto" || platform == "hashnode") && publicationHasPublishedState(state) {
		return false
	}
	return !publicationHasPublishedState(state) || PlatformCapabilitiesFor(platform).PublishedUpdate
}

func (s Service) CreateOrUpdateDraftInput(
	ctx context.Context,
	platform string,
	session Session,
	contentRoot string,
	input DraftInput,
	changedOnly bool,
) (DraftResult, error) {
	slug := input.Slug
	input.ChangedOnly = changedOnly
	state, _, err := LoadPublicationState(s.publicationPath(), slug, platform)
	if err != nil {
		return DraftResult{}, err
	}
	input.RemoteDraftID = state.RemoteDraftID
	input.DraftURL = state.DraftURL
	input.DraftHash = state.DraftHash
	capabilities := PlatformCapabilitiesFor(platform)
	if input.RemoteDraftID == "" && state.PublishedRemoteID != "" && capabilities.PublishedDraftEdit && platform != "juejin" {
		input.RemoteDraftID = state.PublishedRemoteID
		input.DraftURL = state.PublishedURL
		slog.Info("reopening published article draft", "operation", "published-draft-edit", "platform", platform, "slug", slug, "remoteId", input.RemoteDraftID)
	}
	if input.RemoteDraftID == "" && (state.PublishedRemoteID != "" || state.PublishedURL != "") &&
		((platform == "toutiao" || platform == "devto" || platform == "hashnode") || (!capabilities.PublishedUpdate && !capabilities.PublishedDraftEdit)) {
		return DraftResult{}, platformError(
			ErrValidation, platform, "save-draft", 0,
			"the article is already published; safe published-article updates are not supported for this platform yet",
			false,
		)
	}
	if platform != "devto" && changedOnly && input.ContentHash == input.DraftHash && input.RemoteDraftID != "" {
		return DraftResult{
			ID: input.RemoteDraftID, URL: input.DraftURL, Skipped: true,
		}, nil
	}

	var result DraftResult
	run := func(adapter Adapter) error {
		var operationErr error
		if input.RemoteDraftID != "" {
			result, operationErr = adapter.UpdateDraft(ctx, DraftRef{ID: input.RemoteDraftID, URL: input.DraftURL}, input)
			if operationErr != nil && IsKind(operationErr, ErrRemoteDraftMissing) {
				if !mayRecreateMissingDraft(platform, state) {
					return platformError(
						ErrValidation, platform, "save-draft", 0,
						"the recorded draft no longer exists and a published article is already bound; refusing to create a duplicate draft",
						false,
					)
				}
				result, operationErr = adapter.CreateDraft(ctx, input)
			}
		} else {
			result, operationErr = adapter.CreateDraft(ctx, input)
			if operationErr == nil && platform == "hashnode" {
				// Persist the empty remote draft immediately. If image upload or
				// editing later fails, a retry must reuse it, not create another.
				if operationErr = SavePublicationDraftResult(s.publicationPath(), slug, platform, "", result, s.now()); operationErr == nil {
					input.RemoteDraftID = result.ID
					result, operationErr = adapter.UpdateDraft(ctx, DraftRef{ID: input.RemoteDraftID, URL: result.URL}, input)
				}
			}
		}
		return operationErr
	}

	adapter, err := s.authenticatedAdapter(ctx, platform, session)
	if err != nil {
		return DraftResult{}, err
	}
	if platform == "cnblogs" {
		cnblogs := adapter.(*cnBlogsAdapter)
		if state.Account != "" && !strings.EqualFold(state.Account, cnblogs.username) {
			return DraftResult{}, platformError(ErrValidation, platform, "binding", 0, "publication belongs to a different CNBlogs account", false)
		}
	}
	if platform == "csdn" {
		csdn := adapter.(*csdnAdapter)
		if state.Account != "" && !strings.EqualFold(state.Account, csdn.userID) {
			return DraftResult{}, platformError(ErrValidation, platform, "binding", 0, "publication belongs to a different CSDN account", false)
		}
	}
	if platform == "toutiao" {
		toutiao := adapter.(*toutiaoAdapter)
		if state.Account != "" && state.Account != toutiao.userID {
			return DraftResult{}, platformError(ErrValidation, platform, "binding", 0,
				"publication belongs to a different Toutiao account", false)
		}
	}
	if platform == "juejin" && input.RemoteDraftID == "" && state.PublishedRemoteID != "" && capabilities.PublishedDraftEdit {
		juejin := adapter.(*juejinAdapter)
		auth, lookupErr := juejin.CheckAuth(ctx)
		if lookupErr != nil {
			return DraftResult{}, lookupErr
		}
		if !auth.Authenticated || strings.TrimSpace(auth.UserID) == "" {
			return DraftResult{}, platformError(ErrAuthExpired, platform, "published-draft-edit", http.StatusUnauthorized, "browser session is not authenticated", false)
		}
		posts, lookupErr := juejin.listPublished(ctx, auth.UserID)
		if lookupErr != nil {
			return DraftResult{}, lookupErr
		}
		for _, post := range posts {
			if post.ID == state.PublishedRemoteID && strings.TrimSpace(post.DraftID) != "" {
				input.RemoteDraftID = strings.TrimSpace(post.DraftID)
				input.DraftURL = juejinOrigin + "/editor/drafts/" + input.RemoteDraftID
				break
			}
		}
		if input.RemoteDraftID == "" {
			return DraftResult{}, platformError(ErrValidation, platform, "published-draft-edit", 0, "published article has no editable draft id", false)
		}
		slog.Info("resolved published article draft", "operation", "published-draft-edit", "platform", platform, "slug", slug, "articleId", state.PublishedRemoteID, "draftId", input.RemoteDraftID)
	}
	err = run(adapter)
	if err != nil && retryableAuthError(err) && platform != "hashnode" {
		adapter, refreshErr := s.authenticatedAdapter(ctx, platform, session)
		if refreshErr != nil {
			return DraftResult{}, refreshErr
		}
		err = run(adapter)
	}
	if err != nil {
		return DraftResult{}, err
	}
	if result.ID == "" || result.URL == "" {
		return DraftResult{}, fmt.Errorf("%s adapter returned an incomplete draft result", platform)
	}
	if err := SavePublicationDraftResult(s.publicationPath(), slug, platform, input.ContentHash, result, s.now()); err != nil {
		return DraftResult{}, err
	}
	if platform == "devto" && input.Published {
		if err := SavePublicationPublishResult(s.publicationPath(), slug, platform, input.ContentHash, PublishResult{ID: result.ID, URL: result.URL}, s.now()); err != nil {
			return DraftResult{}, err
		}
	}
	if platform == "cnblogs" {
		binding, found, loadErr := LoadPublicationBinding(s.publicationPath(), slug, platform)
		if loadErr != nil {
			return DraftResult{}, loadErr
		}
		if found {
			binding.Account = adapter.(*cnBlogsAdapter).username
			if binding.Source == "" {
				binding.Source = "blogctl"
			}
			binding.VerifiedAt = verifiedAt(s.now())
			if err := SavePublicationBinding(s.publicationPath(), binding); err != nil {
				return DraftResult{}, err
			}
		}
	}
	if platform == "csdn" {
		binding, found, loadErr := LoadPublicationBinding(s.publicationPath(), slug, platform)
		if loadErr != nil {
			return DraftResult{}, loadErr
		}
		if found {
			binding.Account = adapter.(*csdnAdapter).userID
			if binding.Source == "" {
				binding.Source = "blogctl"
			}
			binding.VerifiedAt = verifiedAt(s.now())
			if err := SavePublicationBinding(s.publicationPath(), binding); err != nil {
				return DraftResult{}, err
			}
		}
	}
	return result, nil
}

func (s Service) PublishDraftInput(
	ctx context.Context,
	platform string,
	session Session,
	contentRoot string,
	input DraftInput,
) (PublishResult, error) {
	slug := input.Slug
	state, _, err := LoadPublicationState(s.publicationPath(), slug, platform)
	if err != nil {
		return PublishResult{}, err
	}
	input.RemoteDraftID = state.RemoteDraftID
	input.DraftURL = state.DraftURL
	input.DraftHash = state.DraftHash
	if input.RemoteDraftID == "" {
		return PublishResult{}, platformError(ErrValidation, platform, "publish-draft", 0, "remote draft id is missing; create or update the draft first", false)
	}
	if input.DraftHash == "" || input.DraftHash != input.ContentHash {
		return PublishResult{}, platformError(ErrValidation, platform, "publish-draft", 0, "source changed after the remote draft was prepared; update and preview the draft again", false)
	}

	adapter, err := s.authenticatedAdapter(ctx, platform, session)
	if err != nil {
		return PublishResult{}, err
	}
	if platform == "cnblogs" && state.Account != "" && !strings.EqualFold(state.Account, adapter.(*cnBlogsAdapter).username) {
		return PublishResult{}, platformError(ErrValidation, platform, "binding", 0, "publication belongs to a different CNBlogs account", false)
	}
	if platform == "csdn" && state.Account != "" && !strings.EqualFold(state.Account, adapter.(*csdnAdapter).userID) {
		return PublishResult{}, platformError(ErrValidation, platform, "binding", 0, "publication belongs to a different CSDN account", false)
	}
	result, err := adapter.PublishDraft(ctx, DraftRef{ID: input.RemoteDraftID, URL: input.DraftURL}, input)
	if err != nil && retryableAuthError(err) && platform != "hashnode" {
		adapter, refreshErr := s.authenticatedAdapter(ctx, platform, session)
		if refreshErr != nil {
			return PublishResult{}, refreshErr
		}
		result, err = adapter.PublishDraft(ctx, DraftRef{ID: input.RemoteDraftID, URL: input.DraftURL}, input)
	}
	if err != nil {
		return PublishResult{}, err
	}
	if strings.TrimSpace(result.ID) == "" || strings.TrimSpace(result.URL) == "" {
		return PublishResult{}, fmt.Errorf("%s adapter returned an incomplete publish result", platform)
	}
	if err := SavePublicationPublishResult(s.publicationPath(), slug, platform, input.ContentHash, result, s.now()); err != nil {
		return PublishResult{}, err
	}
	if platform == "cnblogs" {
		cnblogs := adapter.(*cnBlogsAdapter)
		binding, found, loadErr := LoadPublicationBinding(s.publicationPath(), slug, platform)
		if loadErr != nil {
			return PublishResult{}, loadErr
		}
		if found {
			binding.PublishedRemoteID = input.RemoteDraftID
			binding.Account = cnblogs.username
			if binding.Source == "" {
				binding.Source = "blogctl"
			}
			if post, lookupErr := cnblogs.fetchPost(ctx, input.RemoteDraftID); lookupErr == nil {
				binding.RemoteUpdatedAt = valueString(post["dateUpdated"])
			}
			binding.VerifiedAt = verifiedAt(s.now())
			// CNBlogs has no independent save-only draft after publication.
			binding.RemoteDraftID = ""
			binding.DraftURL = ""
			binding.DraftHash = ""
			binding.DraftSyncedAt = ""
			if err := SavePublicationBinding(s.publicationPath(), binding); err != nil {
				return PublishResult{}, err
			}
		}
	}
	return result, nil
}

// UpdateCNBlogsPublished is an explicit operation; the draft path never changes a public post.
func (s Service) UpdateCNBlogsPublishedInput(ctx context.Context, session Session, contentRoot string, input DraftInput) (PublishResult, bool, error) {
	slug := input.Slug
	binding, found, err := LoadPublicationBinding(s.publicationPath(), slug, "cnblogs")
	if err != nil {
		return PublishResult{}, false, err
	}
	if !found || binding.PublishedRemoteID == "" || binding.PublishedURL == "" {
		return PublishResult{}, false, platformError(ErrValidation, "cnblogs", "update-published", 0, "a verified published publication is required", false)
	}
	adapterValue, err := s.authenticatedAdapter(ctx, "cnblogs", session)
	if err != nil {
		return PublishResult{}, false, err
	}
	adapter := adapterValue.(*cnBlogsAdapter)
	if binding.Account != "" && !strings.EqualFold(binding.Account, adapter.username) {
		return PublishResult{}, false, platformError(ErrValidation, "cnblogs", "binding", 0, "publication belongs to a different CNBlogs account", false)
	}
	base, err := adapter.fetchPost(ctx, binding.PublishedRemoteID)
	if err != nil {
		return PublishResult{}, false, err
	}
	if author := valueString(base["author"]); author != "" && !strings.EqualFold(author, adapter.username) {
		return PublishResult{}, false, platformError(ErrValidation, "cnblogs", "binding", 0, "post belongs to a different CNBlogs account", false)
	}
	if published, _ := base["isPublished"].(bool); !published {
		return PublishResult{}, false, platformError(ErrValidation, "cnblogs", "update-published", 0, "remote post is no longer published", false)
	}
	remoteUpdatedAt := valueString(base["dateUpdated"])
	if binding.RemoteUpdatedAt == "" || remoteUpdatedAt == "" || binding.RemoteUpdatedAt != remoteUpdatedAt {
		return PublishResult{}, false, platformError(ErrValidation, "cnblogs", "update-published", 0, "remote post changed or has no verified baseline; verify the publication again", false)
	}
	if binding.PublishedHash != "" && binding.PublishedHash == input.ContentHash {
		return PublishResult{URL: binding.PublishedURL}, true, nil
	}
	decoded, err := adapter.save(ctx, binding.PublishedRemoteID, input, true, true)
	if err != nil {
		return PublishResult{}, false, err
	}
	if returned := valueString(decoded["id"]); returned != "" && returned != binding.PublishedRemoteID {
		return PublishResult{}, false, platformError(ErrUpstream, "cnblogs", "update-published", 0, "CNBlogs returned a different post ID", false)
	}
	updated, err := adapter.fetchPost(ctx, binding.PublishedRemoteID)
	if err != nil {
		return PublishResult{}, false, err
	}
	if published, _ := updated["isPublished"].(bool); !published {
		return PublishResult{}, false, platformError(ErrUpstream, "cnblogs", "update-published", 0, "post update was not published", false)
	}
	binding.Account = adapter.username
	if target := valueString(updated["url"]); target != "" {
		binding.PublishedURL = target
	}
	binding.PublishedHash = input.ContentHash
	binding.PublishedSyncedAt = verifiedAt(s.now())
	binding.RemoteUpdatedAt = valueString(updated["dateUpdated"])
	binding.VerifiedAt = verifiedAt(s.now())
	if binding.Source == "" {
		binding.Source = "blogctl"
	}
	if err := SavePublicationBinding(s.publicationPath(), binding); err != nil {
		return PublishResult{}, false, err
	}
	return PublishResult{URL: binding.PublishedURL}, false, nil
}

// UpdateToutiaoPublishedInput edits the verified remote published article directly,
// using the existing pgc_id and the editor's captured save=1 republish contract.
// Unlike saving a draft, this operation may change the public article.
func (s Service) UpdateToutiaoPublishedInput(ctx context.Context, session Session, contentRoot string, input DraftInput) (PublishResult, bool, error) {
	const platform = "toutiao"
	binding, found, err := LoadPublicationBinding(s.publicationPath(), input.Slug, platform)
	if err != nil {
		return PublishResult{}, false, err
	}
	if !found || binding.PublishedRemoteID == "" || binding.PublishedURL == "" {
		return PublishResult{}, false, platformError(ErrValidation, platform, "update-published", 0, "verify and bind an existing published Toutiao article first", false)
	}
	adapterValue, err := s.authenticatedAdapter(ctx, platform, session)
	if err != nil {
		return PublishResult{}, false, err
	}
	adapter := adapterValue.(*toutiaoAdapter)
	if binding.Account != "" && binding.Account != adapter.userID {
		return PublishResult{}, false, platformError(ErrValidation, platform, "update-published", 0, "bound article belongs to a different Toutiao account", false)
	}
	posts, err := adapter.listPublished(ctx)
	if err != nil {
		return PublishResult{}, false, err
	}
	var original *ToutiaoPost
	for index := range posts {
		if posts[index].ID == binding.PublishedRemoteID {
			original = &posts[index]
			break
		}
	}
	if original == nil {
		return PublishResult{}, false, platformError(ErrValidation, platform, "update-published", 0, "published article not found in the current creator account", false)
	}
	if strings.TrimSpace(binding.RemoteUpdatedAt) == "" || original.ModifiedAt == "" ||
		binding.RemoteUpdatedAt == "0" || original.ModifiedAt == "0" || binding.RemoteUpdatedAt != original.ModifiedAt {
		return PublishResult{}, false, platformError(ErrValidation, platform, "update-published", 0,
			"remote article changed or no revision baseline is recorded; verify and bind the published article again", false)
	}
	if binding.PublishedHash != "" && binding.PublishedHash == input.ContentHash {
		return PublishResult{ID: original.ID, URL: original.URL}, true, nil
	}
	id, err := adapter.mutate(ctx, original.ID, input, true, "")
	if err != nil {
		return PublishResult{}, false, err
	}
	if id != original.ID {
		return PublishResult{}, false, platformError(ErrUpstream, platform, "update-published", 0, "Toutiao returned a different published article ID", false)
	}
	// Submission may enter review; a successful response does not guarantee the
	// public page already reflects the new content. Invalidate the old revision
	// baseline until the user explicitly re-verifies the remote article.
	binding.RemoteUpdatedAt = ""
	binding.PublishedHash = input.ContentHash
	binding.PublishedSyncedAt = verifiedAt(s.now())
	binding.VerifiedAt = ""
	if err := SavePublicationBinding(s.publicationPath(), binding); err != nil {
		return PublishResult{}, false, err
	}
	return PublishResult{ID: original.ID, URL: original.URL}, false, nil
}

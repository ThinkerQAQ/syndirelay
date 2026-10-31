package publisher

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var publicationBindingsMu sync.Mutex

type PublicationBinding struct {
	Slug              string   `json:"slug"`
	Platform          string   `json:"platform"`
	RemoteDraftID     string   `json:"remoteDraftId,omitempty"`
	DraftURL          string   `json:"draftUrl,omitempty"`
	DraftHash         string   `json:"draftHash,omitempty"`
	DraftSyncedAt     string   `json:"draftSyncedAt,omitempty"`
	PublishedRemoteID string   `json:"publishedRemoteId,omitempty"`
	PublishedURL      string   `json:"publishedUrl,omitempty"`
	PublishedHash     string   `json:"publishedHash,omitempty"`
	PublishedAt       string   `json:"publishedAt,omitempty"`
	PublishedSyncedAt string   `json:"publishedSyncedAt,omitempty"`
	Account           string   `json:"account,omitempty"`
	Source            string   `json:"source,omitempty"`
	RemoteUpdatedAt   string   `json:"remoteUpdatedAt,omitempty"`
	VerifiedAt        string   `json:"verifiedAt,omitempty"`
	PendingFields     []string `json:"pendingFields,omitempty"`
}

const bindingFileVersion = 2

type bindingFile struct {
	Version      int                  `json:"version"`
	Publications []PublicationBinding `json:"publications"`
}

func publicationBindingState(binding PublicationBinding) PublicationState {
	return PublicationState{
		RemoteDraftID:     binding.RemoteDraftID,
		DraftURL:          binding.DraftURL,
		DraftHash:         binding.DraftHash,
		PublishedRemoteID: binding.PublishedRemoteID,
		PublishedURL:      binding.PublishedURL,
		PublishedHash:     binding.PublishedHash,
		Account:           binding.Account,
		Source:            binding.Source,
		RemoteUpdatedAt:   binding.RemoteUpdatedAt,
		VerifiedAt:        binding.VerifiedAt,
	}
}

func publicationBindingFromFile(bindings bindingFile, slug, platform string) (PublicationBinding, bool) {
	for _, binding := range bindings.Publications {
		if binding.Slug == slug && binding.Platform == platform {
			return binding, true
		}
	}
	return PublicationBinding{}, false
}

func LoadPublicationBinding(storePath, slug, platform string) (PublicationBinding, bool, error) {
	bindings, err := readBindings(storePath)
	if err != nil {
		return PublicationBinding{}, false, err
	}
	binding, found := publicationBindingFromFile(bindings, slug, platform)
	return binding, found, nil
}

func upsertPublicationBinding(bindings *bindingFile, binding PublicationBinding) {
	for index := range bindings.Publications {
		if bindings.Publications[index].Slug == binding.Slug && bindings.Publications[index].Platform == binding.Platform {
			bindings.Publications[index] = binding
			return
		}
	}
	bindings.Publications = append(bindings.Publications, binding)
}

func validatePublicationBindingUniqueness(bindings bindingFile, binding PublicationBinding) error {
	for _, existing := range bindings.Publications {
		if existing.Platform != binding.Platform || existing.Slug == binding.Slug {
			continue
		}
		for _, remoteID := range []string{binding.RemoteDraftID, binding.PublishedRemoteID} {
			remoteID = strings.TrimSpace(remoteID)
			if remoteID == "" {
				continue
			}
			if remoteID == strings.TrimSpace(existing.RemoteDraftID) || remoteID == strings.TrimSpace(existing.PublishedRemoteID) {
				return fmt.Errorf("%s remote id %s is already bound to %s", binding.Platform, remoteID, existing.Slug)
			}
		}
	}
	return nil
}

func SavePublicationBinding(storePath string, binding PublicationBinding) error {
	publicationBindingsMu.Lock()
	defer publicationBindingsMu.Unlock()
	binding.Slug = strings.TrimSpace(binding.Slug)
	binding.Platform = strings.TrimSpace(strings.ToLower(binding.Platform))
	if binding.Slug == "" || binding.Platform == "" {
		return errors.New("invalid publication binding")
	}
	bindings, err := readBindings(storePath)
	if err != nil {
		return err
	}
	if err := validatePublicationBindingUniqueness(bindings, binding); err != nil {
		return err
	}
	upsertPublicationBinding(&bindings, binding)
	return writeBindings(storePath, bindings)
}

func DeletePublicationBindingState(storePath, slug, platform, state, remoteID string) error {
	publicationBindingsMu.Lock()
	defer publicationBindingsMu.Unlock()
	bindings, err := readBindings(storePath)
	if err != nil {
		return err
	}
	binding, found := publicationBindingFromFile(bindings, slug, platform)
	if !found {
		return errors.New("binding not found or changed")
	}
	switch state {
	case "draft":
		if remoteID != "" && binding.RemoteDraftID != remoteID {
			return errors.New("binding not found or changed")
		}
		binding.RemoteDraftID = ""
		binding.DraftURL = ""
		binding.DraftHash = ""
		binding.DraftSyncedAt = ""
	case "published":
		if remoteID != "" && binding.PublishedRemoteID != remoteID {
			return errors.New("binding not found or changed")
		}
		binding.PublishedRemoteID = ""
		binding.PublishedURL = ""
		binding.PublishedHash = ""
		binding.PublishedAt = ""
		binding.PublishedSyncedAt = ""
		binding.RemoteUpdatedAt = ""
		binding.VerifiedAt = ""
	default:
		return errors.New("invalid publication binding state")
	}
	if binding.RemoteDraftID == "" && binding.PublishedRemoteID == "" && binding.DraftURL == "" && binding.PublishedURL == "" && len(binding.PendingFields) == 0 {
		filtered := bindings.Publications[:0]
		for _, existing := range bindings.Publications {
			if existing.Slug == slug && existing.Platform == platform {
				continue
			}
			filtered = append(filtered, existing)
		}
		bindings.Publications = filtered
		return writeBindings(storePath, bindings)
	}
	upsertPublicationBinding(&bindings, binding)
	return writeBindings(storePath, bindings)
}

func normalizePendingFields(fields []string) []string {
	result := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, exists := seen[field]; exists {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	return result
}

func SavePublicationPendingFields(storePath, slug, platform string, fields []string) error {
	publicationBindingsMu.Lock()
	defer publicationBindingsMu.Unlock()
	bindings, err := readBindings(storePath)
	if err != nil {
		return err
	}
	binding, found := publicationBindingFromFile(bindings, slug, platform)
	if !found {
		return errors.New("publication binding not found")
	}
	binding.PendingFields = normalizePendingFields(fields)
	upsertPublicationBinding(&bindings, binding)
	return writeBindings(storePath, bindings)
}

func ResolvePublicationPendingFields(storePath, slug, platform string, resolved []string) ([]string, error) {
	publicationBindingsMu.Lock()
	defer publicationBindingsMu.Unlock()
	bindings, err := readBindings(storePath)
	if err != nil {
		return nil, err
	}
	binding, found := publicationBindingFromFile(bindings, slug, platform)
	if !found {
		return nil, errors.New("publication binding not found")
	}
	resolved = normalizePendingFields(resolved)
	if len(resolved) == 0 {
		binding.PendingFields = nil
	} else {
		resolvedSet := map[string]struct{}{}
		for _, field := range resolved {
			resolvedSet[field] = struct{}{}
		}
		remaining := make([]string, 0, len(binding.PendingFields))
		for _, field := range binding.PendingFields {
			if _, ok := resolvedSet[field]; !ok {
				remaining = append(remaining, field)
			}
		}
		binding.PendingFields = remaining
	}
	upsertPublicationBinding(&bindings, binding)
	if err := writeBindings(storePath, bindings); err != nil {
		return nil, err
	}
	return append([]string{}, binding.PendingFields...), nil
}

func SavePublicationDraftResult(storePath, slug, platform, contentHash string, result DraftResult, now time.Time) error {
	publicationBindingsMu.Lock()
	defer publicationBindingsMu.Unlock()
	bindings, err := readBindings(storePath)
	if err != nil {
		return err
	}
	binding, found := publicationBindingFromFile(bindings, slug, platform)
	if !found {
		binding = PublicationBinding{Slug: slug, Platform: platform}
	}
	binding.RemoteDraftID = result.ID
	binding.DraftURL = result.URL
	binding.DraftHash = contentHash
	binding.DraftSyncedAt = now.UTC().Format(time.RFC3339)
	binding.PendingFields = nil
	if err := validatePublicationBindingUniqueness(bindings, binding); err != nil {
		return err
	}
	upsertPublicationBinding(&bindings, binding)
	return writeBindings(storePath, bindings)
}

func SavePublicationPublishResult(storePath, slug, platform, contentHash string, result PublishResult, now time.Time) error {
	publicationBindingsMu.Lock()
	defer publicationBindingsMu.Unlock()
	result.ID = strings.TrimSpace(result.ID)
	result.URL = strings.TrimSpace(result.URL)
	if result.ID == "" || result.URL == "" {
		return errors.New("refusing to record publication without remote id and URL")
	}
	bindings, err := readBindings(storePath)
	if err != nil {
		return err
	}
	binding, found := publicationBindingFromFile(bindings, slug, platform)
	if found {
		if binding.DraftHash != contentHash {
			return errors.New("refusing to record publication for a stale draft")
		}
	} else {
		state, _, loadErr := LoadPublicationState(storePath, slug, platform)
		if loadErr != nil {
			return loadErr
		}
		if state.DraftHash != contentHash {
			return errors.New("refusing to record publication for a stale draft")
		}
		binding = PublicationBinding{
			Slug: slug, Platform: platform,
			RemoteDraftID: state.RemoteDraftID, DraftURL: state.DraftURL, DraftHash: state.DraftHash,
		}
	}
	binding.PublishedRemoteID = result.ID
	binding.PublishedURL = result.URL
	binding.PublishedHash = contentHash
	binding.PublishedAt = now.UTC().Format(time.RFC3339)
	// A successful publish ends the prepared-draft lifecycle. Keeping the old
	// draft slot makes the UI offer a second publish and can cause a later
	// "Save" to mutate the public object or create a duplicate publication.
	binding.RemoteDraftID = ""
	binding.DraftURL = ""
	binding.DraftHash = ""
	binding.DraftSyncedAt = ""
	if err := validatePublicationBindingUniqueness(bindings, binding); err != nil {
		return err
	}
	upsertPublicationBinding(&bindings, binding)
	return writeBindings(storePath, bindings)
}

func SavePublicationPublishedUpdateResult(storePath, slug, platform, contentHash string, now time.Time) error {
	publicationBindingsMu.Lock()
	defer publicationBindingsMu.Unlock()
	bindings, err := readBindings(storePath)
	if err != nil {
		return err
	}
	binding, found := publicationBindingFromFile(bindings, slug, platform)
	if !found {
		state, _, loadErr := LoadPublicationState(storePath, slug, platform)
		if loadErr != nil {
			return loadErr
		}
		binding = PublicationBinding{
			Slug: slug, Platform: platform,
			RemoteDraftID: state.RemoteDraftID, DraftURL: state.DraftURL, DraftHash: state.DraftHash,
			PublishedURL: state.PublishedURL,
		}
	}
	binding.PublishedHash = contentHash
	binding.PublishedSyncedAt = now.UTC().Format(time.RFC3339)
	upsertPublicationBinding(&bindings, binding)
	return writeBindings(storePath, bindings)
}

func readBindings(storePath string) (bindingFile, error) {
	result := bindingFile{Version: bindingFileVersion, Publications: []PublicationBinding{}}
	storePath = strings.TrimSpace(storePath)
	if storePath == "" {
		return result, errors.New("publication bindings path is not configured")
	}
	raw, err := os.ReadFile(storePath)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, fmt.Errorf("decode publication bindings: %w", err)
	}
	if result.Version != bindingFileVersion {
		return result, fmt.Errorf("unsupported bindings version: %d", result.Version)
	}
	if result.Publications == nil {
		result.Publications = []PublicationBinding{}
	}
	return result, nil
}

func writeBindings(storePath string, bindings bindingFile) error {
	storePath = strings.TrimSpace(storePath)
	if storePath == "" {
		return errors.New("publication bindings path is not configured")
	}
	bindings.Version = bindingFileVersion
	if bindings.Publications == nil {
		bindings.Publications = []PublicationBinding{}
	}
	path := storePath
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(bindings, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func verifiedAt(now time.Time) string { return now.UTC().Format(time.RFC3339) }

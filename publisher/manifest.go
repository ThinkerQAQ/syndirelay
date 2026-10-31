package publisher

import (
	"net/url"
	"sort"
	"strings"
)

type PublicationState struct {
	RemoteDraftID     string
	DraftURL          string
	DraftHash         string
	PublishedRemoteID string
	PublishedURL      string
	PublishedHash     string
	Account           string
	Source            string
	RemoteUpdatedAt   string
	VerifiedAt        string
}

func LoadPublicationState(storePath, slug, platform string) (PublicationState, string, error) {
	storePath = strings.TrimSpace(storePath)
	binding, found, err := LoadPublicationBinding(storePath, slug, platform)
	if err != nil {
		return PublicationState{}, "", err
	}
	if !found {
		return PublicationState{}, storePath, nil
	}
	return publicationBindingState(binding), storePath, nil
}

type ArticleLink struct {
	Platform          string `json:"platform"`
	RemoteID          string `json:"remoteId,omitempty"`
	PublishedRemoteID string `json:"publishedRemoteId,omitempty"`
	DraftURL          string `json:"draftUrl,omitempty"`
	PublishedURL      string `json:"publishedUrl,omitempty"`
}

type PublicationRecord struct {
	Article           string   `json:"article"`
	Platform          string   `json:"platform"`
	RemoteID          string   `json:"remoteId,omitempty"`
	PublishedRemoteID string   `json:"publishedRemoteId,omitempty"`
	DraftURL          string   `json:"draftUrl,omitempty"`
	PublishedURL      string   `json:"publishedUrl,omitempty"`
	DraftSyncedAt     string   `json:"draftSyncedAt,omitempty"`
	PublishedAt       string   `json:"publishedAt,omitempty"`
	PublishedSyncedAt string   `json:"publishedSyncedAt,omitempty"`
	PendingFields     []string `json:"pendingFields,omitempty"`
	UpdatedAt         string   `json:"updatedAt,omitempty"`
}

func publicationRecordFromBinding(binding PublicationBinding) PublicationRecord {
	record := PublicationRecord{
		Article: binding.Slug, Platform: binding.Platform, RemoteID: binding.RemoteDraftID,
		PublishedRemoteID: binding.PublishedRemoteID,
		DraftURL:          binding.DraftURL, PublishedURL: binding.PublishedURL,
		DraftSyncedAt: binding.DraftSyncedAt, PublishedAt: binding.PublishedAt,
		PublishedSyncedAt: binding.PublishedSyncedAt,
		PendingFields:     append([]string{}, binding.PendingFields...),
	}
	if binding.Platform == "cnblogs" {
		record.DraftURL = cnBlogsDraftURL(record.RemoteID, record.DraftURL)
	}
	if binding.Platform == "oschina" {
		record.DraftURL = OSChinaDraftURL(binding.Account, record.RemoteID, record.DraftURL)
	}
	for _, candidate := range []string{record.DraftSyncedAt, record.PublishedAt, record.PublishedSyncedAt} {
		if candidate > record.UpdatedAt {
			record.UpdatedAt = candidate
		}
	}
	return record
}

func ListPublicationRecords(storePath string) ([]PublicationRecord, error) {
	records := []PublicationRecord{}
	bindings, err := readBindings(storePath)
	if err != nil {
		return nil, err
	}
	for _, binding := range bindings.Publications {
		record := publicationRecordFromBinding(binding)
		if record.RemoteID == "" && record.PublishedRemoteID == "" && record.DraftURL == "" && record.PublishedURL == "" {
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].UpdatedAt != records[j].UpdatedAt {
			return records[i].UpdatedAt > records[j].UpdatedAt
		}
		if records[i].Article != records[j].Article {
			return records[i].Article < records[j].Article
		}
		return records[i].Platform < records[j].Platform
	})
	return records, nil
}

// LoadArticleLinks reads locally recorded remote references without contacting a platform.
func LoadArticleLinks(storePath, slug string) (map[string]ArticleLink, error) {
	result := map[string]ArticleLink{}
	bindings, err := readBindings(storePath)
	if err != nil {
		return nil, err
	}
	for _, binding := range bindings.Publications {
		if binding.Slug != slug {
			continue
		}
		link := ArticleLink{
			Platform: binding.Platform, RemoteID: binding.RemoteDraftID,
			PublishedRemoteID: binding.PublishedRemoteID,
			DraftURL:          binding.DraftURL, PublishedURL: binding.PublishedURL,
		}
		if link.RemoteID == "" {
			link.RemoteID = draftIDFromURL(binding.Platform, link.DraftURL)
		}
		if binding.Platform == "cnblogs" {
			link.DraftURL = cnBlogsDraftURL(link.RemoteID, link.DraftURL)
		}
		if binding.Platform == "oschina" {
			link.DraftURL = OSChinaDraftURL(binding.Account, link.RemoteID, link.DraftURL)
		}
		if link.RemoteID != "" || link.PublishedRemoteID != "" || link.DraftURL != "" || link.PublishedURL != "" {
			result[binding.Platform] = link
		}
	}
	return result, nil
}

func draftIDFromURL(platform, rawURL string) string {
	text := strings.TrimSpace(rawURL)
	if text == "" {
		return ""
	}
	parsed, err := url.Parse(text)
	if err != nil {
		return ""
	}
	pathID := func() string {
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(segments) == 0 {
			return ""
		}
		return strings.TrimSpace(segments[len(segments)-1])
	}
	queryID := func(name string) string {
		return strings.TrimSpace(parsed.Query().Get(name))
	}
	switch platform {
	case "juejin", "51cto", "oschina":
		return pathID()
	case "zhihu":
		// 草稿/编辑链接为 /p/<articleID>/edit，公开链接为 /p/<articleID>；
		// 兼容更早的 /write/<articleID> 形态。
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(segments) >= 2 && segments[0] == "p" && strings.TrimSpace(segments[1]) != "" {
			return strings.TrimSpace(segments[1])
		}
		return pathID()
	case "csdn":
		return queryID("articleId")
	case "segmentfault":
		return queryID("draftId")
	case "toutiao":
		return queryID("pgc_id")
	case "cnblogs":
		// Draft editor URLs use a matrix-style parameter: /posts/edit;postId=<id>.
		const marker = "postId="
		if index := strings.Index(text, marker); index >= 0 {
			rest := text[index+len(marker):]
			if end := strings.IndexAny(rest, "&?#"); end >= 0 {
				rest = rest[:end]
			}
			return strings.TrimSpace(rest)
		}
		return ""
	default:
		return ""
	}
}

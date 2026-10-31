package social

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var safeID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,99}$`)

type Source struct {
	Kind     string `json:"kind"`
	Slug     string `json:"slug"`
	Language string `json:"language,omitempty"`
	URL      string `json:"url,omitempty"`
}

type DraftInput struct {
	ID     string          `json:"id"`
	Source Source          `json:"source"`
	Posts  map[string]Post `json:"posts"`
}
type Post struct {
	Text     string `json:"text"`
	MediaURL string `json:"mediaUrl,omitempty"`
	MediaAlt string `json:"mediaAlt,omitempty"`
	LinkCard bool   `json:"linkCard,omitempty"`
}
type Remote struct {
	ID        string `json:"id,omitempty"`
	ChannelID string `json:"channelId,omitempty"`
	State     string `json:"state,omitempty"`
	Error     string `json:"error,omitempty"`
	URL       string `json:"url,omitempty"`
}
type Draft struct {
	DraftInput
	Revision         string            `json:"revision"`
	ApprovedRevision string            `json:"approvedRevision,omitempty"`
	Remote           map[string]Remote `json:"remote,omitempty"`
}

func (d Draft) Ready() bool { return d.ApprovedRevision != "" && d.ApprovedRevision == d.Revision }
func (d Draft) Validate() error {
	if !safeID.MatchString(d.ID) {
		return errors.New("invalid social draft id")
	}
	if d.Source.Kind != "article" && d.Source.Kind != "series" && d.Source.Kind != "project" {
		return errors.New("unsupported source kind")
	}
	if !safeID.MatchString(d.Source.Slug) {
		return errors.New("invalid source slug")
	}
	if d.Source.Language != "en" {
		return errors.New("social source language must be en")
	}
	parsed, err := url.Parse(d.Source.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return errors.New("invalid source URL")
	}
	if len(d.Posts) == 0 {
		return errors.New("social draft has no platform posts")
	}
	for platform, post := range d.Posts {
		if platform != "linkedin" && platform != "twitter" && platform != "bluesky" {
			return fmt.Errorf("unsupported social platform %q", platform)
		}
		if strings.TrimSpace(post.Text) == "" {
			return fmt.Errorf("%s post text is empty", platform)
		}
		if post.MediaURL != "" {
			media, err := url.Parse(post.MediaURL)
			if err != nil || media.Scheme != "https" || media.Host == "" {
				return fmt.Errorf("%s media must be HTTPS", platform)
			}
			if post.LinkCard {
				return fmt.Errorf("%s link card and image are mutually exclusive", platform)
			}
		}
		rendered := RenderText(d.Source.URL, platform, post)
		limit := 0
		if platform == "twitter" {
			limit = 280
		}
		if platform == "bluesky" {
			limit = 300
		}
		if limit > 0 && utf8.RuneCountInString(rendered) > limit {
			return fmt.Errorf("%s post exceeds %d characters including URL", platform, limit)
		}
		if utf8.RuneCountInString(rendered) > 2900 {
			return fmt.Errorf("%s post is too long", platform)
		}
	}
	return nil
}
func RenderText(sourceURL, platform string, post Post) string {
	u, _ := url.Parse(sourceURL)
	q := u.Query()
	q.Set("utm_source", platform)
	q.Set("utm_medium", "social")
	q.Set("utm_campaign", "article_syndication")
	u.RawQuery = q.Encode()
	return strings.TrimSpace(post.Text) + "\n\n" + u.String()
}
func (d Draft) Hash() string {
	canonical := struct {
		Source Source
		Posts  map[string]Post
	}{d.Source, d.Posts}
	raw, _ := json.Marshal(canonical)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func Platforms(d Draft) []string {
	keys := make([]string, 0, len(d.Posts))
	for key := range d.Posts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func SafeFile(root, id string) (string, error) {
	if !safeID.MatchString(id) {
		return "", errors.New("invalid id")
	}
	return filepath.Join(root, "social", id+".json"), nil
}

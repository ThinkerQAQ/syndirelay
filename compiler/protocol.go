package compiler

import (
	"encoding/json"
	"errors"
	"strings"
)

const ProtocolVersion = 1

type Asset struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Renderer  string `json:"renderer,omitempty"`
	ObjectKey string `json:"objectKey"`
	PublicURL string `json:"publicUrl"`
	Source    string `json:"source,omitempty"`
	Content   string `json:"content,omitempty"`
	Alt       string `json:"alt,omitempty"`
}

type CompiledArticle struct {
	Version            int             `json:"version"`
	Slug               string          `json:"slug"`
	Platform           string          `json:"platform"`
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	Series             string          `json:"series,omitempty"`
	Markdown           string          `json:"markdown"`
	HTML               string          `json:"html"`
	Language           string          `json:"language"`
	CanonicalURL       string          `json:"canonicalUrl"`
	ContentHash        string          `json:"contentHash"`
	SourceDir          string          `json:"sourceDir"`
	Tags               []string        `json:"tags,omitempty"`
	CoverImageURL      string          `json:"coverImageUrl,omitempty"`
	NativeCanonicalURL string          `json:"nativeCanonicalUrl,omitempty"`
	Published          bool            `json:"published"`
	Payload            json.RawMessage `json:"payload,omitempty"`
	FallbackHTML       string          `json:"fallbackHtml,omitempty"`
	RequiresFallback   bool            `json:"requiresFallback,omitempty"`
	Warnings           []string        `json:"warnings,omitempty"`
	Assets             []Asset         `json:"assets,omitempty"`
}

func (article CompiledArticle) Validate() error {
	if article.Version != ProtocolVersion {
		return errors.New("unsupported compiled article protocol version")
	}
	for name, value := range map[string]string{
		"slug": article.Slug, "platform": article.Platform, "title": article.Title,
		"markdown": article.Markdown, "language": article.Language,
		"contentHash": article.ContentHash, "sourceDir": article.SourceDir,
	} {
		if strings.TrimSpace(value) == "" {
			return errors.New("compiled article is missing " + name)
		}
	}
	return nil
}

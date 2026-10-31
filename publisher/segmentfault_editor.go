package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var segmentFaultNextDataPattern = regexp.MustCompile(`(?s)<script[^>]*id=["']__NEXT_DATA__["'][^>]*>(.*?)</script>`)

type segmentFaultEditorBlog struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type segmentFaultEditorDraft struct {
	ID    int64             `json:"id"`
	Title string            `json:"title"`
	Text  string            `json:"text"`
	Cover string            `json:"cover"`
	Tags  []segmentFaultTag `json:"tags"`
}

type segmentFaultEditorContext struct {
	Token string
	Blogs []segmentFaultEditorBlog
	Draft segmentFaultEditorDraft
}

func segmentFaultEditorURL(draftID string) string {
	if strings.TrimSpace(draftID) == "" {
		return segmentFaultOrigin + "/write"
	}
	return segmentFaultOrigin + "/write?draftId=" + url.QueryEscape(strings.TrimSpace(draftID))
}

func parseSegmentFaultEditorContext(raw []byte) (segmentFaultEditorContext, error) {
	result := segmentFaultEditorContext{}
	match := segmentFaultTokenPattern.FindSubmatch(raw)
	if len(match) >= 2 {
		for _, candidate := range match[1:] {
			if value := strings.TrimSpace(string(candidate)); value != "" {
				result.Token = value
				break
			}
		}
	}
	nextData := segmentFaultNextDataPattern.FindSubmatch(raw)
	if len(nextData) != 2 {
		return result, fmt.Errorf("SegmentFault editor __NEXT_DATA__ was not found")
	}
	var page struct {
		Props struct {
			PageProps struct {
				InitialState struct {
					Editor struct {
						Detail struct {
							Blogs []segmentFaultEditorBlog `json:"blogs"`
							Draft segmentFaultEditorDraft  `json:"draft"`
						} `json:"detail"`
					} `json:"editor"`
				} `json:"initialState"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal(nextData[1], &page); err != nil {
		return result, fmt.Errorf("decode SegmentFault editor state: %w", err)
	}
	result.Blogs = page.Props.PageProps.InitialState.Editor.Detail.Blogs
	result.Draft = page.Props.PageProps.InitialState.Editor.Detail.Draft
	if result.Token == "" {
		return result, fmt.Errorf("SegmentFault editor session token was not found")
	}
	return result, nil
}

func (s *segmentFaultAdapter) loadEditorContext(ctx context.Context, draftID string) (segmentFaultEditorContext, error) {
	rawURL := segmentFaultEditorURL(draftID)
	req, err := s.request(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return segmentFaultEditorContext{}, err
	}
	req.Header.Del("origin")
	response, err := s.client.Do(req)
	if err != nil {
		return segmentFaultEditorContext{}, platformError(ErrUpstream, s.ID(), "editor-context", 0, err.Error(), true)
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 8<<20)
	if err != nil {
		return segmentFaultEditorContext{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return segmentFaultEditorContext{}, classifyHTTP(s.ID(), "editor-context", response.StatusCode, string(raw))
	}
	result, err := parseSegmentFaultEditorContext(raw)
	if err != nil {
		return segmentFaultEditorContext{}, platformError(ErrUpstream, s.ID(), "editor-context", response.StatusCode, err.Error(), false)
	}
	s.token = result.Token
	if strings.TrimSpace(draftID) != "" {
		expected, parseErr := strconv.ParseInt(strings.TrimSpace(draftID), 10, 64)
		if parseErr != nil {
			return segmentFaultEditorContext{}, platformError(ErrValidation, s.ID(), "editor-context", 0, "draft id must be numeric", false)
		}
		if result.Draft.ID != 0 && result.Draft.ID != expected {
			return segmentFaultEditorContext{}, platformError(ErrUpstream, s.ID(), "editor-context", response.StatusCode, "editor returned a different draft id", false)
		}
	}
	return result, nil
}

func selectSegmentFaultBlogID(blogs []segmentFaultEditorBlog) int64 {
	if len(blogs) == 1 {
		return blogs[0].ID
	}
	return 0
}

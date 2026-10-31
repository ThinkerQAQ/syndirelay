package buffer

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

const Endpoint = "https://api.buffer.com"

type Client struct {
	APIKey     string
	HTTPClient *http.Client
	Endpoint   string
}

func (c Client) endpoint() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	return Endpoint
}

type graphResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c Client) query(ctx context.Context, query string, variables any, target any) error {
	if c.APIKey == "" {
		return errors.New("Buffer API Key is not configured")
	}
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Buffer request: %w (remote outcome unknown for writes)", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read Buffer response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Buffer HTTP status %d", resp.StatusCode)
	}
	var envelope graphResponse
	if err = json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("invalid Buffer JSON: %w", err)
	}
	if len(envelope.Errors) > 0 {
		messages := make([]string, 0, len(envelope.Errors))
		for _, e := range envelope.Errors {
			messages = append(messages, e.Message)
		}
		return fmt.Errorf("Buffer GraphQL: %s", strings.Join(messages, "; "))
	}
	if len(envelope.Data) == 0 {
		return errors.New("Buffer GraphQL response missing data")
	}
	return json.Unmarshal(envelope.Data, target)
}

type Channel struct {
	ID             string `json:"id"`
	Service        string `json:"service"`
	DisplayName    string `json:"displayName"`
	Name           string `json:"name"`
	IsQueuePaused  bool   `json:"isQueuePaused"`
	IsLocked       bool   `json:"isLocked"`
	IsDisconnected bool   `json:"isDisconnected"`
}

func (c Client) Channels(ctx context.Context) ([]Channel, error) {
	var account struct {
		Account struct {
			Organizations []struct {
				ID string `json:"id"`
			} `json:"organizations"`
		} `json:"account"`
	}
	if err := c.query(ctx, `query { account { organizations { id } } }`, nil, &account); err != nil {
		return nil, err
	}
	result := []Channel{}
	for _, org := range account.Account.Organizations {
		var channels struct {
			Channels []Channel `json:"channels"`
		}
		if err := c.query(ctx, `query($id:OrganizationId!){channels(input:{organizationId:$id}){id service name displayName isQueuePaused isLocked isDisconnected}}`, map[string]any{"id": org.ID}, &channels); err != nil {
			return nil, err
		}
		result = append(result, channels.Channels...)
	}
	return result, nil
}

type RemotePost struct {
	ID           string `json:"id"`
	Text         string `json:"text"`
	Status       string `json:"status"`
	ChannelID    string `json:"channelId"`
	ExternalLink string `json:"externalLink,omitempty"`
}

func (c Client) Post(ctx context.Context, id string) (RemotePost, error) {
	var data struct {
		Post RemotePost `json:"post"`
	}
	err := c.query(ctx, `query($id:PostId!){post(input:{id:$id}){id text status channelId externalLink}}`, map[string]any{"id": id}, &data)
	if err != nil {
		return RemotePost{}, err
	}
	if data.Post.ID == "" {
		return RemotePost{}, errors.New("Buffer post not found")
	}
	return data.Post, nil
}

type PostPayload struct {
	ChannelID string
	Text      string
	ImageURL  string
	LinkCard  map[string]any
}

func (p PostPayload) input() map[string]any {
	result := map[string]any{"channelId": p.ChannelID, "text": p.Text, "schedulingType": "automatic", "mode": "addToQueue", "saveToDraft": true, "assets": []any{}, "needsApproval": false, "aiAssisted": true, "source": "blogctl"}
	if p.ImageURL != "" {
		result["assets"] = []any{map[string]any{"image": map[string]any{"url": p.ImageURL}}}
	}
	if len(p.LinkCard) > 0 {
		result["metadata"] = map[string]any{"linkedin": map[string]any{"linkAttachment": p.LinkCard}}
	}
	return result
}

const postFields = `{ __typename ... on PostActionSuccess { post { id text status channelId externalLink } } ... on MutationError { message } }`

type action struct {
	Type    string     `json:"__typename"`
	Post    RemotePost `json:"post"`
	Message string     `json:"message"`
}

func checkAction(a action) (RemotePost, error) {
	if a.Type != "PostActionSuccess" || a.Post.ID == "" {
		return RemotePost{}, fmt.Errorf("Buffer mutation failed: %s", a.Message)
	}
	return a.Post, nil
}
func (c Client) CreateDraft(ctx context.Context, p PostPayload) (RemotePost, error) {
	var data struct {
		CreatePost action `json:"createPost"`
	}
	err := c.query(ctx, `mutation($input:CreatePostInput!){createPost(input:$input)`+postFields+`}`, map[string]any{"input": p.input()}, &data)
	if err != nil {
		return RemotePost{}, err
	}
	return checkAction(data.CreatePost)
}

// PublishDraft advances an existing Buffer draft without creating another post.
// Buffer validates content on editPost, so the verified text must be sent again.
func (c Client) PublishDraft(ctx context.Context, id, text string) (RemotePost, error) {
	if strings.TrimSpace(text) == "" {
		return RemotePost{}, errors.New("Buffer publication requires non-empty reviewed text")
	}
	var data struct {
		EditPost action `json:"editPost"`
	}
	err := c.query(ctx, `mutation($input:EditPostInput!){editPost(input:$input)`+postFields+`}`, map[string]any{"input": map[string]any{"id": id, "text": text, "mode": "shareNow", "saveToDraft": false}}, &data)
	if err != nil {
		return RemotePost{}, err
	}
	return checkAction(data.EditPost)
}

package social

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/social/buffer"
)

type BufferAPI interface {
	Channels(context.Context) ([]buffer.Channel, error)
	Post(context.Context, string) (buffer.RemotePost, error)
	CreateDraft(context.Context, buffer.PostPayload) (buffer.RemotePost, error)
	PublishDraft(context.Context, string, string) (buffer.RemotePost, error)
}
type Service struct {
	DataRoot string
	Buffer   BufferAPI
}

func (s Service) approved(id, platform, revision string) (Draft, Post, buffer.Channel, error) {
	draft, err := Load(s.DataRoot, id)
	if err != nil {
		return Draft{}, Post{}, buffer.Channel{}, err
	}
	post, ok := draft.Posts[platform]
	if !ok {
		return Draft{}, Post{}, buffer.Channel{}, fmt.Errorf("no %s content", platform)
	}
	if !draft.Ready() || revision != draft.Revision {
		return Draft{}, Post{}, buffer.Channel{}, errors.New("draft is not approved for this exact content revision")
	}
	return draft, post, buffer.Channel{}, nil
}
func (s Service) channel(ctx context.Context, platform string) (buffer.Channel, error) {
	channels, err := s.Buffer.Channels(ctx)
	if err != nil {
		return buffer.Channel{}, err
	}
	var result buffer.Channel
	for _, ch := range channels {
		if ch.Service != platform {
			continue
		}
		if result.ID != "" {
			return buffer.Channel{}, errors.New("multiple channels; explicit selection is required")
		}
		result = ch
	}
	if result.ID == "" {
		return buffer.Channel{}, fmt.Errorf("%s not connected", platform)
	}
	if result.IsLocked || result.IsDisconnected {
		return buffer.Channel{}, fmt.Errorf("%s unavailable", platform)
	}
	return result, nil
}
func (s Service) Stage(ctx context.Context, id, platform, revision string) (Draft, error) {
	draft, err := Load(s.DataRoot, id)
	if err != nil {
		return Draft{}, err
	}
	if revision == "" || draft.Revision != revision {
		return Draft{}, errors.New("draft revision mismatch; reload and review before staging")
	}
	post, ok := draft.Posts[platform]
	if !ok {
		return Draft{}, fmt.Errorf("no %s content", platform)
	}
	channel, err := s.channel(ctx, platform)
	if err != nil {
		return Draft{}, err
	}
	state := draft.Remote[platform]
	if state.ID != "" || state.State != "" {
		return Draft{}, errors.New("remote state already exists or is uncertain; inspect Buffer before retry")
	}
	// An explicit stage request confirms the current content revision.
	// Record approval and the pending remote write in one atomic local save.
	draft.ApprovedRevision = draft.Revision
	state = Remote{ChannelID: channel.ID, State: "creating"}
	if draft.Remote == nil {
		draft.Remote = map[string]Remote{}
	}
	draft.Remote[platform] = state
	if err = Save(s.DataRoot, draft); err != nil {
		return Draft{}, err
	}
	// Writes are never retried: a timeout can mean the remote draft was created.
	payload := buffer.PostPayload{ChannelID: channel.ID, Text: RenderText(draft.Source.URL, platform, post), ImageURL: post.MediaURL}
	if post.LinkCard && platform == "linkedin" {
		payload.LinkCard = map[string]any{"url": draft.Source.URL}
	}
	remote, err := s.Buffer.CreateDraft(ctx, payload)
	if err != nil {
		return Draft{}, fmt.Errorf("remote create outcome uncertain; inspect Buffer: %w", err)
	}
	state.ID = remote.ID
	state.State = "draft"
	if remote.Status != "draft" || remote.ChannelID != channel.ID {
		state.State = "uncertain"
		state.Error = "unexpected remote state"
	}
	draft.Remote[platform] = state
	if err = Save(s.DataRoot, draft); err != nil {
		return Draft{}, err
	}
	if state.State != "draft" {
		return Draft{}, errors.New("remote draft created but state unexpected")
	}
	return draft, nil
}

// Reconcile reads the actual Buffer post before changing any local state.
// It cannot send or create a post. A recent in-flight publish is never reset
// to draft, even if the Buffer read still reports draft.
func (s Service) Reconcile(ctx context.Context, id, platform, revision string) (Draft, error) {
	draft, err := Load(s.DataRoot, id)
	if err != nil {
		return Draft{}, err
	}
	if revision == "" || revision != draft.Revision {
		return Draft{}, errors.New("revision mismatch; reload before reconciling")
	}
	post, ok := draft.Posts[platform]
	if !ok {
		return Draft{}, fmt.Errorf("no %s content", platform)
	}
	state, ok := draft.Remote[platform]
	if !ok || state.ID == "" || state.ChannelID == "" {
		return Draft{}, errors.New("no remote Buffer post for this platform")
	}
	remote, err := s.Buffer.Post(ctx, state.ID)
	if err != nil {
		return Draft{}, fmt.Errorf("unable to inspect Buffer post: %w", err)
	}
	if remote.ID != state.ID || remote.ChannelID != state.ChannelID {
		return Draft{}, errors.New("Buffer post identity mismatch; local state unchanged")
	}
	if remote.Text != RenderText(draft.Source.URL, platform, post) {
		return Draft{}, errors.New("Buffer post differs from reviewed content; local state unchanged")
	}
	if remote.Status == "draft" && (state.State == "publishing" || state.State == "creating") {
		path, err := SafeFile(s.DataRoot, id)
		if err != nil {
			return Draft{}, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return Draft{}, err
		}
		if time.Since(info.ModTime()) < 2*time.Minute {
			return Draft{}, errors.New("publish attempt is recent; wait two minutes before recovery")
		}
	}
	switch remote.Status {
	case "draft", "scheduled", "sending", "sent", "error":
		state.State = remote.Status
	default:
		state.State = "uncertain"
		state.Error = "Buffer returned unknown state: " + remote.Status
	}
	if remote.Status == "draft" && state.Error == "" {
		state.Error = "Buffer still has a private draft; no public post was confirmed"
	}
	if remote.Status == "sent" || remote.Status == "sending" || remote.Status == "scheduled" {
		state.Error = ""
	}
	state.URL = remote.ExternalLink
	draft.Remote[platform] = state
	if err := Save(s.DataRoot, draft); err != nil {
		return Draft{}, err
	}
	return draft, nil
}

func (s Service) Publish(ctx context.Context, id, platform, revision string) (Draft, error) {
	draft, post, _, err := s.approved(id, platform, revision)
	if err != nil {
		return Draft{}, err
	}
	channel, err := s.channel(ctx, platform)
	if err != nil {
		return Draft{}, err
	}
	state := draft.Remote[platform]
	if state.State != "draft" || state.ID == "" || state.ChannelID != channel.ID {
		return Draft{}, errors.New("no verified Buffer draft available to publish; check remote state first")
	}
	remote, err := s.Buffer.Post(ctx, state.ID)
	if err != nil {
		return Draft{}, err
	}
	if remote.ID != state.ID || remote.ChannelID != channel.ID || remote.Status != "draft" ||
		remote.Text != RenderText(draft.Source.URL, platform, post) {
		return Draft{}, errors.New("Buffer remote draft differs from reviewed content")
	}
	state.State = "publishing"
	state.Error = ""
	draft.Remote[platform] = state
	if err := Save(s.DataRoot, draft); err != nil {
		return Draft{}, err
	}
	result, err := s.Buffer.PublishDraft(ctx, state.ID, remote.Text)
	if err != nil {
		state.Error = strings.TrimSpace(err.Error())
		if len(state.Error) > 400 {
			state.Error = state.Error[:400]
		}
		draft.Remote[platform] = state
		// A request can be accepted by Buffer before the network fails.
		// Leave an explicit unresolved state until a read-only reconciliation.
		if saveErr := Save(s.DataRoot, draft); saveErr != nil {
			return Draft{}, fmt.Errorf("publish failed: %w (also failed to store result: %v)", err, saveErr)
		}
		return Draft{}, fmt.Errorf("publish not confirmed; check Buffer status before retrying: %w", err)
	}
	if result.ID != state.ID || result.ChannelID != state.ChannelID {
		state.State = "uncertain"
		state.Error = "Buffer response identity mismatch"
	} else {
		switch result.Status {
		case "sent", "sending", "scheduled":
			state.State = result.Status
		case "draft":
			state.State = "draft"
			state.Error = "Buffer returned a private draft instead of publishing"
		case "error":
			state.State = "error"
			state.Error = "Buffer reported a publishing failure"
		default:
			state.State = "uncertain"
			state.Error = "Buffer returned unexpected post status: " + result.Status
		}
	}
	state.URL = result.ExternalLink
	draft.Remote[platform] = state
	if err := Save(s.DataRoot, draft); err != nil {
		return Draft{}, err
	}
	if state.Error != "" {
		return Draft{}, fmt.Errorf("publish not confirmed: %s", state.Error)
	}
	return draft, nil
}

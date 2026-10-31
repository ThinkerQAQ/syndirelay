package social

import (
	"context"
	"errors"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/social/buffer"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeBuffer struct {
	posts              map[string]buffer.RemotePost
	fail               bool
	publishError       error
	publishStatus      string
	creates, publishes int
}

func (f *fakeBuffer) Channels(context.Context) ([]buffer.Channel, error) {
	return []buffer.Channel{{ID: "channel", Service: "linkedin", DisplayName: "scott zhang"}}, nil
}
func (f *fakeBuffer) Post(_ context.Context, id string) (buffer.RemotePost, error) {
	return f.posts[id], nil
}
func (f *fakeBuffer) CreateDraft(_ context.Context, p buffer.PostPayload) (buffer.RemotePost, error) {
	f.creates++
	if f.fail {
		return buffer.RemotePost{}, errors.New("simulated network timeout")
	}
	post := buffer.RemotePost{ID: "post1", ChannelID: p.ChannelID, Text: p.Text, Status: "draft"}
	f.posts[post.ID] = post
	return post, nil
}
func (f *fakeBuffer) PublishDraft(_ context.Context, id, text string) (buffer.RemotePost, error) {
	if text != f.posts[id].Text {
		return buffer.RemotePost{}, errors.New("publish text differs from reviewed Buffer draft")
	}
	f.publishes++
	if f.publishError != nil {
		return buffer.RemotePost{}, f.publishError
	}
	result := f.posts[id]
	result.Status = "sent"
	if f.publishStatus != "" {
		result.Status = f.publishStatus
	}
	f.posts[id] = result
	return result, nil
}
func sampleDraft(t *testing.T, dir string) Draft {
	t.Helper()
	draft, err := Create(dir, DraftInput{ID: "mutex-demo", Source: Source{Kind: "article", Slug: "mutex", Language: "en", URL: "https://thinkerqaq.com/en/articles/mutex/"}, Posts: map[string]Post{"linkedin": {Text: "A lock is more than mutual exclusion."}}})
	if err != nil {
		t.Fatal(err)
	}
	return draft
}
func TestStageImplicitlyApprovesAndPublishRequiresRemoteDraft(t *testing.T) {
	dir := t.TempDir()
	draft := sampleDraft(t, dir)
	mock := &fakeBuffer{posts: map[string]buffer.RemotePost{}}
	service := Service{DataRoot: dir, Buffer: mock}
	if _, err := service.Publish(context.Background(), draft.ID, "linkedin", draft.Revision); err == nil {
		t.Fatal("published before staging")
	}
	if _, err := service.Stage(context.Background(), draft.ID, "linkedin", "outdated-revision"); err == nil {
		t.Fatal("staged with wrong revision")
	}
	if mock.creates != 0 {
		t.Fatal("incorrect revision triggered remote write")
	}
	staged, err := service.Stage(context.Background(), draft.ID, "linkedin", draft.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if staged.Remote["linkedin"].State != "draft" || mock.creates != 1 || staged.ApprovedRevision != draft.Revision {
		t.Fatalf("%+v creates=%d approved=%q", staged.Remote, mock.creates, staged.ApprovedRevision)
	}
	if _, err := service.Stage(context.Background(), draft.ID, "linkedin", draft.Revision); err == nil {
		t.Fatal("duplicate stage allowed")
	}
	mock.posts["post1"] = buffer.RemotePost{ID: "post1", Status: "draft", ChannelID: "channel", Text: "different"}
	if _, err := service.Publish(context.Background(), draft.ID, "linkedin", draft.Revision); err == nil {
		t.Fatal("published tampered content")
	}
	mock.posts["post1"] = buffer.RemotePost{ID: "post1", Status: "draft", ChannelID: "channel", Text: RenderText(draft.Source.URL, "linkedin", draft.Posts["linkedin"])}
	result, err := service.Publish(context.Background(), draft.ID, "linkedin", draft.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if result.Remote["linkedin"].State != "sent" || mock.publishes != 1 {
		t.Fatal("did not publish")
	}
	if _, err := service.Publish(context.Background(), draft.ID, "linkedin", draft.Revision); err == nil {
		t.Fatal("duplicate publish allowed")
	}
}
func TestAmbiguousCreateNeverRetried(t *testing.T) {
	dir := t.TempDir()
	d := sampleDraft(t, dir)
	mock := &fakeBuffer{posts: map[string]buffer.RemotePost{}, fail: true}
	svc := Service{DataRoot: dir, Buffer: mock}
	_, err := svc.Stage(context.Background(), d.ID, "linkedin", d.Revision)
	if err == nil || !strings.Contains(err.Error(), "uncertain") {
		t.Fatal("ambiguous error missing")
	}
	if _, err := svc.Stage(context.Background(), d.ID, "linkedin", d.Revision); err == nil {
		t.Fatal("ambiguous creation retried")
	}
	if mock.creates != 1 {
		t.Fatal("unexpected duplicate remote write")
	}
}
func TestSourceAndStore(t *testing.T) {
	content := t.TempDir()
	root := filepath.Join(content, "src/content/articles/en")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.md"), []byte("---\nstatus: published\n---\nHello"), 0600); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveSource(content, "https://thinkerqaq.com", Source{Kind: "article", Slug: "demo", Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.URL != "https://thinkerqaq.com/en/articles/demo/" {
		t.Fatal(resolved.URL)
	}
	if _, err := ResolveSource(content, "https://thinkerqaq.com", Source{Kind: "article", Slug: "../secret", Language: "en"}); err == nil {
		t.Fatal("path traversal accepted")
	}
	d := sampleDraft(t, t.TempDir())
	if _, err := SafeFile("/tmp", "../secret"); err == nil {
		t.Fatal("unsafe filename")
	}
	if d.Revision != d.Hash() {
		t.Fatal("revision mismatch")
	}
}

func TestPublishErrorPersistsAndRequiresReadOnlyRecovery(t *testing.T) {
	dir := t.TempDir()
	d := sampleDraft(t, dir)
	mock := &fakeBuffer{posts: map[string]buffer.RemotePost{}, publishError: errors.New("Buffer rejected the publish request")}
	svc := Service{DataRoot: dir, Buffer: mock}
	if _, err := svc.Stage(context.Background(), d.ID, "linkedin", d.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(context.Background(), d.ID, "linkedin", d.Revision); err == nil {
		t.Fatal("expected publish error")
	}
	pending, err := Load(dir, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Remote["linkedin"].State != "publishing" || !strings.Contains(pending.Remote["linkedin"].Error, "rejected") {
		t.Fatalf("unrecorded error: %+v", pending.Remote)
	}
	if _, err := svc.Publish(context.Background(), d.ID, "linkedin", d.Revision); err == nil {
		t.Fatal("duplicate publish during uncertainty")
	}
	if mock.publishes != 1 {
		t.Fatal("duplicate Buffer mutation")
	}
	if _, err := svc.Reconcile(context.Background(), d.ID, "linkedin", d.Revision); err == nil {
		t.Fatal("reconciled a fresh in-flight request")
	}
	path, _ := SafeFile(dir, d.ID)
	old := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	recovered, err := svc.Reconcile(context.Background(), d.ID, "linkedin", d.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Remote["linkedin"].State != "draft" {
		t.Fatalf("not recovered: %+v", recovered.Remote)
	}
	if mock.publishes != 1 {
		t.Fatal("reconciliation must not publish")
	}
	mock.publishError = nil
	sent, err := svc.Publish(context.Background(), d.ID, "linkedin", d.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if sent.Remote["linkedin"].State != "sent" || mock.publishes != 2 {
		t.Fatal("explicit retry failed")
	}
}
func TestPublishSuccessWithStillDraftDoesNotClaimSent(t *testing.T) {
	dir := t.TempDir()
	d := sampleDraft(t, dir)
	mock := &fakeBuffer{posts: map[string]buffer.RemotePost{}, publishStatus: "draft"}
	svc := Service{DataRoot: dir, Buffer: mock}
	if _, err := svc.Stage(context.Background(), d.ID, "linkedin", d.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(context.Background(), d.ID, "linkedin", d.Revision); err == nil {
		t.Fatal("false success: still draft")
	}
	local, err := Load(dir, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if local.Remote["linkedin"].State != "draft" || local.Remote["linkedin"].Error == "" {
		t.Fatalf("false successful publication: %+v", local.Remote)
	}
}
func TestReconcileActualRemoteSentDoesNotPublishAgain(t *testing.T) {
	dir := t.TempDir()
	d := sampleDraft(t, dir)
	mock := &fakeBuffer{posts: map[string]buffer.RemotePost{}}
	svc := Service{DataRoot: dir, Buffer: mock}
	if _, err := svc.Stage(context.Background(), d.ID, "linkedin", d.Revision); err != nil {
		t.Fatal(err)
	}
	post := mock.posts["post1"]
	post.Status = "sent"
	post.ExternalLink = "https://www.linkedin.com/feed/update/test/"
	mock.posts[post.ID] = post
	local, err := svc.Reconcile(context.Background(), d.ID, "linkedin", d.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if local.Remote["linkedin"].State != "sent" || local.Remote["linkedin"].URL != post.ExternalLink {
		t.Fatalf("not reconciled: %+v", local.Remote)
	}
	if mock.publishes != 0 {
		t.Fatal("reconcile must not publish")
	}
}

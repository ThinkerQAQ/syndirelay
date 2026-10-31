package publisher

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRehostMarkdownImagesDownloadsRemoteAssetBeforeUpload(t *testing.T) {
	var downloads atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		w.Header().Set("content-type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer source.Close()

	inputURL := source.URL + "/generated/mermaid/diagram.png"
	input := DraftInput{
		Markdown: "![one](" + inputURL + ")\n\n![two](" + inputURL + ")",
	}
	uploads := 0
	got, err := rehostMarkdownImages(context.Background(), source.Client(), input, ImageRehostOptions{
		Platform:       "test",
		FailOpenRemote: true,
	}, func(_ context.Context, image RehostImage) (string, error) {
		uploads++
		if image.Source != inputURL {
			t.Fatalf("source = %q", image.Source)
		}
		if string(image.Payload) != "png-bytes" {
			t.Fatalf("payload = %q", string(image.Payload))
		}
		if !strings.HasPrefix(image.ContentType, "image/png") {
			t.Fatalf("content type = %q", image.ContentType)
		}
		return "https://platform.example/cdn/diagram.png", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if downloads.Load() != 1 {
		t.Fatalf("downloads = %d, want 1", downloads.Load())
	}
	if uploads != 1 {
		t.Fatalf("uploads = %d, want 1", uploads)
	}
	if strings.Contains(got, inputURL) || strings.Count(got, "https://platform.example/cdn/diagram.png") != 2 {
		t.Fatalf("markdown was not fully rewritten:\n%s", got)
	}
}

func TestRehostMarkdownImagesKeepsRemoteURLWhenPlatformUploadFails(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	defer source.Close()

	inputURL := source.URL + "/asset.png"
	input := DraftInput{Markdown: "![asset](" + inputURL + ")"}
	got, err := rehostMarkdownImages(context.Background(), source.Client(), input, ImageRehostOptions{
		Platform:       "test",
		FailOpenRemote: true,
	}, func(_ context.Context, _ RehostImage) (string, error) {
		return "", errors.New("platform upload unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != input.Markdown {
		t.Fatalf("remote fallback changed markdown: %q", got)
	}
}

func TestRehostMarkdownImagesFailsClosedForLocalAssetUploadFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "diagram.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := DraftInput{
		Markdown:  "![asset](diagram.png)",
		SourceDir: dir,
	}
	_, err := rehostMarkdownImages(context.Background(), http.DefaultClient, input, ImageRehostOptions{
		Platform:       "test",
		FailOpenRemote: true,
	}, func(_ context.Context, image RehostImage) (string, error) {
		if string(image.Payload) != "png-bytes" {
			t.Fatalf("payload = %q", string(image.Payload))
		}
		return "", errors.New("platform upload unavailable")
	})
	if err == nil {
		t.Fatal("expected local image upload failure")
	}
}

func TestRehostMarkdownImagesSkipsPlatformHostedAsset(t *testing.T) {
	input := DraftInput{Markdown: "![asset](https://platform.example/cdn/asset.png)"}
	called := false
	got, err := rehostMarkdownImages(context.Background(), http.DefaultClient, input, ImageRehostOptions{
		Platform: "test",
		AlreadyHosted: func(source string) bool {
			return strings.Contains(source, "platform.example")
		},
	}, func(_ context.Context, _ RehostImage) (string, error) {
		called = true
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("platform-hosted image should not be downloaded or uploaded")
	}
	if got != input.Markdown {
		t.Fatalf("markdown changed: %q", got)
	}
}

func TestRehostMarkdownImagesPrefersPlatformUploadBeforeR2Fallback(t *testing.T) {
	var r2Uploads atomic.Int32
	r2 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2Uploads.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer r2.Close()

	distributionRoot := t.TempDir()
	cacheDir := filepath.Join(distributionRoot, "assets", "mermaid")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "abc.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	source := "blogctl-asset://mermaid/abc"
	input := DraftInput{
		Markdown:         "![asset](" + source + ")",
		DistributionRoot: distributionRoot,
		Assets: []PublishingAsset{{
			Kind: "mermaid", ID: "abc", ObjectKey: "generated/mermaid/abc.png",
			PublicURL: "https://assets.example/generated/mermaid/abc.png", Source: source,
		}},
		R2Fallback: R2FallbackConfig{
			AccessKeyID: "key", SecretAccessKey: "secret",
			Endpoint: r2.URL, Bucket: "bucket", PublicBaseURL: "https://assets.example/",
		},
	}
	nativeUploads := 0
	got, err := rehostMarkdownImages(context.Background(), r2.Client(), input, ImageRehostOptions{
		Platform: "test",
	}, func(_ context.Context, image RehostImage) (string, error) {
		nativeUploads++
		if string(image.Payload) != "png-bytes" {
			t.Fatalf("payload = %q", string(image.Payload))
		}
		return "https://platform.example/image.png", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if nativeUploads != 1 {
		t.Fatalf("native uploads = %d, want 1", nativeUploads)
	}
	if r2Uploads.Load() != 0 {
		t.Fatalf("R2 uploads = %d, want 0", r2Uploads.Load())
	}
	if !strings.Contains(got, "https://platform.example/image.png") {
		t.Fatalf("markdown did not use platform image: %s", got)
	}
}

func TestRehostMarkdownImagesUsesR2OnlyAfterPlatformUploadFails(t *testing.T) {
	var r2Uploads atomic.Int32
	r2 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2Uploads.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer r2.Close()

	distributionRoot := t.TempDir()
	cacheDir := filepath.Join(distributionRoot, "assets", "mermaid")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "abc.png"), []byte("png-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	source := "blogctl-asset://mermaid/abc"
	input := DraftInput{
		Markdown:         "![asset](" + source + ")",
		DistributionRoot: distributionRoot,
		Assets: []PublishingAsset{{
			Kind: "mermaid", ID: "abc", ObjectKey: "generated/mermaid/abc.png",
			PublicURL: "https://assets.example/generated/mermaid/abc.png", Source: source,
		}},
		R2Fallback: R2FallbackConfig{
			AccessKeyID: "key", SecretAccessKey: "secret",
			Endpoint: r2.URL, Bucket: "bucket", PublicBaseURL: "https://assets.example/",
		},
	}
	nativeUploads := 0
	got, err := rehostMarkdownImages(context.Background(), r2.Client(), input, ImageRehostOptions{
		Platform: "test",
	}, func(_ context.Context, _ RehostImage) (string, error) {
		nativeUploads++
		return "", errors.New("native upload unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	if nativeUploads != 1 {
		t.Fatalf("native uploads = %d, want 1", nativeUploads)
	}
	if r2Uploads.Load() != 1 {
		t.Fatalf("R2 uploads = %d, want 1", r2Uploads.Load())
	}
	if !strings.Contains(got, "https://assets.example/generated/mermaid/abc.png") {
		t.Fatalf("markdown did not use R2 fallback: %s", got)
	}
}

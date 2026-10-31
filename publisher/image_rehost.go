package publisher

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

type RehostImage struct {
	Source      string
	Payload     []byte
	ContentType string
}

type ImageRehostOptions struct {
	Platform       string
	SourceDir      string
	FailOpenRemote bool
	AlreadyHosted  func(string) bool
}

type ImageRehostUploader func(context.Context, RehostImage) (string, error)

func isRemoteHTTPImage(source string) bool {
	parsed, err := url.Parse(strings.TrimSpace(source))
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != ""
}

func publishingAssetForSource(input DraftInput, source string) (PublishingAsset, bool) {
	for _, asset := range input.Assets {
		if strings.TrimSpace(asset.Source) != "" && asset.Source == source {
			return asset, true
		}
	}
	return PublishingAsset{}, false
}

func loadRehostImage(client *http.Client, input DraftInput, source, sourceDir string) ([]byte, string, error) {
	if asset, ok := publishingAssetForSource(input, source); ok && strings.HasPrefix(source, "blogctl-asset://") {
		root := strings.TrimSpace(input.DistributionRoot)
		if root == "" {
			return nil, "", fmt.Errorf("distribution root is missing for generated asset %s", asset.ID)
		}
		extension := ".png"
		relative := filepath.ToSlash(filepath.Join("assets", asset.Kind, asset.ID+extension))
		return loadImage(client, relative, root)
	}
	return loadImage(client, source, sourceDir)
}

func tryR2Fallback(ctx context.Context, client *http.Client, input DraftInput, image RehostImage, nativeErr error) (string, error) {
	target, fallbackErr := uploadR2Fallback(ctx, client, input, image)
	if fallbackErr == nil {
		slog.Warn("platform image upload failed; using R2 fallback",
			"source", image.Source, "error", nativeErr)
		return strings.TrimSpace(target), nil
	}
	if nativeErr == nil {
		return "", fallbackErr
	}
	return "", fmt.Errorf("platform image upload failed: %v; R2 fallback failed: %w", nativeErr, fallbackErr)
}

func rehostImageReplacements(
	ctx context.Context,
	client *http.Client,
	input DraftInput,
	markdown string,
	options ImageRehostOptions,
	upload ImageRehostUploader,
) (map[string]string, error) {
	replacements := map[string]string{}
	for _, source := range imageSources(markdown) {
		if options.AlreadyHosted != nil && options.AlreadyHosted(source) {
			continue
		}
		payload, contentType, err := loadRehostImage(client, input, source, options.SourceDir)
		if err != nil {
			wrapped := platformError(ErrUpload, options.Platform, "download-image", 0, err.Error(), true)
			if options.FailOpenRemote && isRemoteHTTPImage(source) {
				slog.Warn("platform image download failed; keeping remote image URL",
					"platform", options.Platform, "source", source, "error", wrapped)
				continue
			}
			return nil, wrapped
		}

		image := RehostImage{
			Source:      source,
			Payload:     payload,
			ContentType: contentType,
		}
		target, uploadErr := upload(ctx, image)
		target = strings.TrimSpace(target)
		if uploadErr != nil || target == "" {
			if uploadErr == nil {
				uploadErr = platformError(ErrUpload, options.Platform, "image-upload", 0, "image URL missing", false)
			}
			fallbackTarget, fallbackErr := tryR2Fallback(ctx, client, input, image, uploadErr)
			if fallbackErr == nil && fallbackTarget != "" {
				replacements[source] = fallbackTarget
				continue
			}
			if options.FailOpenRemote && isRemoteHTTPImage(source) {
				slog.Warn("platform image rehost and R2 fallback failed; keeping remote image URL",
					"platform", options.Platform, "source", source, "error", fallbackErr)
				continue
			}
			return nil, fallbackErr
		}
		replacements[source] = target
	}
	return replacements, nil
}

func rehostMarkdownImages(
	ctx context.Context,
	client *http.Client,
	input DraftInput,
	options ImageRehostOptions,
	upload ImageRehostUploader,
) (string, error) {
	if options.SourceDir == "" {
		options.SourceDir = input.SourceDir
	}
	replacements, err := rehostImageReplacements(ctx, client, input, input.Markdown, options, upload)
	if err != nil {
		return "", err
	}
	return replaceImages(input.Markdown, replacements), nil
}

func rehostHTMLImages(
	ctx context.Context,
	client *http.Client,
	input DraftInput,
	html string,
	options ImageRehostOptions,
	upload ImageRehostUploader,
) (string, error) {
	if options.SourceDir == "" {
		options.SourceDir = input.SourceDir
	}
	replacements, err := rehostImageReplacements(ctx, client, input, input.Markdown, options, upload)
	if err != nil {
		return "", err
	}
	return replaceImages(html, replacements), nil
}

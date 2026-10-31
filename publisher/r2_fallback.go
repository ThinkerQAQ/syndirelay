package publisher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	blogr2 "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/storage/r2"
)

func r2FallbackObject(input DraftInput, image RehostImage) (string, string) {
	for _, asset := range input.Assets {
		if strings.TrimSpace(asset.Source) != "" && asset.Source == image.Source {
			return asset.ObjectKey, asset.PublicURL
		}
		if strings.TrimSpace(asset.PublicURL) != "" && asset.PublicURL == image.Source {
			return asset.ObjectKey, asset.PublicURL
		}
	}
	sum := sha256.Sum256(image.Payload)
	extension := strings.ToLower(filepath.Ext(inferImageFilename(image.Source, image.ContentType)))
	if extension == "" {
		extension = ".png"
	}
	return "fallback/images/" + hex.EncodeToString(sum[:16]) + extension, ""
}

// UploadR2Fallback exposes the configured R2 asset fallback to platform-specific
// transports that cannot reuse the generic Markdown rehost pipeline.
func UploadR2Fallback(ctx context.Context, client *http.Client, input DraftInput, image RehostImage) (string, error) {
	return uploadR2Fallback(ctx, client, input, image)
}

func uploadR2Fallback(ctx context.Context, client *http.Client, input DraftInput, image RehostImage) (string, error) {
	config := input.R2Fallback
	if !blogr2.Ready(config) {
		return "", errors.New("R2 fallback is not configured")
	}
	objectKey, expectedPublicURL := r2FallbackObject(input, image)
	objectKey = strings.TrimSpace(strings.TrimPrefix(objectKey, "/"))
	if objectKey == "" {
		return "", errors.New("R2 fallback object key is empty")
	}

	publicURL, err := blogr2.Upload(ctx, client, config, objectKey, image.Payload, image.ContentType)
	if err != nil {
		return "", err
	}
	if expectedPublicURL != "" && expectedPublicURL != publicURL {
		return "", fmt.Errorf("R2 fallback public URL mismatch: %s", objectKey)
	}
	return publicURL, nil
}

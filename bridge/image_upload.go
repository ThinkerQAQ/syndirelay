package bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	runtimeversion "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/internal/version"
	blogr2 "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/storage/r2"
)

const imagePipelineVersion = "v1"
const maxImageInput = 20 << 20
const maxImageOutput = 24 << 20

type imageUploadRequest struct {
	LocalPath string `json:"localPath"`
	Format    string `json:"format,omitempty"`
	Alt       string `json:"alt,omitempty"`
	DryRun    bool   `json:"dryRun,omitempty"`
}
type cleanedImage struct {
	Bytes          []byte
	Format         string `json:"format"`
	ContentType    string `json:"contentType"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	PrivacyCleaned bool   `json:"privacyCleaned"`
}

func allowedLocalImage(path string, roots []string) (string, error) {
	if !filepath.IsAbs(path) || len(roots) == 0 {
		return "", errors.New("absolute image path within a configured image root is required")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("image not accessible")
	}
	for _, root := range roots {
		if !filepath.IsAbs(root) {
			continue
		}
		trusted, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(trusted, resolved)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "." {
			return resolved, nil
		}
	}
	return "", errors.New("image is outside configured image roots")
}
func validInputImage(data []byte) bool {
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}) {
		return true
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return true
	}
	return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}
func (s *Server) imageConfig() (bridgeConfig, *http.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config, s.httpClient
}
func sanitizeImage(ctx context.Context, engineRoot string, input []byte, format string) (cleanedImage, error) {
	script := filepath.Join(engineRoot, "tools", "blogctl", "renderers", "node", "sanitize-image.mjs")
	if info, err := os.Stat(script); err != nil || info.IsDir() {
		return cleanedImage{}, errors.New("image sanitizer is unavailable; configure engine_root and install its Node dependencies")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return cleanedImage{}, errors.New("Node.js is required for image privacy processing")
	}
	temp, err := os.CreateTemp("", "blogctl-image-clean-*")
	if err != nil {
		return cleanedImage{}, errors.New("cannot allocate private image buffer")
	}
	path := temp.Name()
	_ = temp.Close()
	defer os.Remove(path)
	command := exec.CommandContext(ctx, node, script, format, path)
	command.Dir = engineRoot
	command.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		// Sanitizer stderr can include local paths; never expose it to MCP.
		return cleanedImage{}, fmt.Errorf("image privacy processing failed: %w", err)
	}
	var meta cleanedImage
	if json.Unmarshal(stdout.Bytes(), &meta) != nil || meta.Format != format || !meta.PrivacyCleaned || meta.Width <= 0 || meta.Height <= 0 {
		return cleanedImage{}, errors.New("image sanitizer did not verify a safe result")
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 || len(raw) > maxImageOutput || !validInputImage(raw) {
		return cleanedImage{}, errors.New("invalid sanitized image output")
	}
	meta.Bytes = raw
	return meta, nil
}
func validateImagePayload(req imageUploadRequest, config bridgeConfig) (string, []byte, string, error) {
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "webp"
	}
	if format != "webp" && format != "png" && format != "jpeg" {
		return "", nil, "", errors.New("format must be webp, png or jpeg")
	}
	if len(req.Alt) > 160 || strings.ContainsAny(req.Alt, "\r\n") {
		return "", nil, "", errors.New("alt must be a single line of at most 160 characters")
	}
	roots := append([]string{}, config.ImageUploadRoots...)
	if strings.TrimSpace(config.ContentRoot) != "" {
		roots = append(roots, config.ContentRoot)
	}
	filename, err := allowedLocalImage(strings.TrimSpace(req.LocalPath), roots)
	if err != nil {
		return "", nil, "", err
	}
	f, err := os.Open(filename)
	if err != nil {
		return "", nil, "", errors.New("image cannot be read")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() == 0 || st.Size() > maxImageInput {
		return "", nil, "", errors.New("image must be a regular file between 1 byte and 20 MiB")
	}
	payload, err := io.ReadAll(io.LimitReader(f, maxImageInput+1))
	if err != nil || len(payload) == 0 || len(payload) > maxImageInput || !validInputImage(payload) {
		return "", nil, "", errors.New("invalid or unsupported image bytes")
	}
	return format, payload, req.Alt, nil
}

func imageObjectKey(payload []byte, format string) (string, string) {
	hash := sha256.Sum256(payload)
	digest := hex.EncodeToString(hash[:])
	return "images/" + imagePipelineVersion + "/" + digest[:32] + "." + format, digest
}
func (s *Server) handleMCPImageUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body imageUploadRequest
	if err := readJSON(r, 8192, &body); err != nil {
		writeError(w, err)
		return
	}
	config, client := s.imageConfig()
	format, input, alt, err := validateImagePayload(body, config)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_image", err.Error(), nil)
		return
	}
	result, err := sanitizeImage(r.Context(), config.EngineRoot, input, format)
	if err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, "privacy_processing_failed", err.Error(), nil)
		return
	}
	key, digest := imageObjectKey(result.Bytes, format)
	response := map[string]any{
		"success": true, "dry_run": body.DryRun, "version": imagePipelineVersion,
		"processor_version": imagePipelineVersion,
		"blogctl_version":   runtimeversion.Current, "sha256": digest, "object_key": key,
		"format": format, "content_type": result.ContentType, "width": result.Width, "height": result.Height,
		"size_bytes": len(result.Bytes), "privacy_cleaned": true,
	}
	if body.DryRun {
		writeJSON(w, http.StatusOK, response)
		return
	}
	settings := config.Publishing.Assets.R2
	r2config := blogr2.Config{
		AccessKeyID: settings.AccessKeyID, SecretAccessKey: settings.SecretAccessKey,
		AccountID: settings.AccountID, Endpoint: settings.Endpoint,
		Bucket: settings.Bucket, PublicBaseURL: settings.PublicBaseURL,
	}
	if !blogr2.Ready(r2config) {
		writeAPIError(w, http.StatusPreconditionFailed, "r2_unavailable", "R2 is not configured", nil)
		return
	}
	url, err := blogr2.Upload(r.Context(), client, r2config, key, result.Bytes, result.ContentType)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "r2_upload_failed", "R2 upload failed; remote object state may need verification", nil)
		return
	}
	safeAlt := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]").Replace(alt)
	if safeAlt == "" {
		safeAlt = "Image"
	}
	response["url"] = url
	response["markdown"] = "![" + safeAlt + "](" + url + ")"
	writeJSON(w, http.StatusOK, response)
}

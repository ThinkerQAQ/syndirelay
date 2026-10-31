package bridge

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func smallPNG(t *testing.T) []byte {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.RGBA{R: 250, G: 20, B: 40, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func fakeImageEngine(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "tools", "blogctl", "renderers", "node")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Fake worker validates the Go orchestration. Real Sharp privacy tests run in Node.
	script := `import fs from "node:fs";
 const chunks=[];for await(const b of process.stdin)chunks.push(b);
 fs.writeFileSync(process.argv[3],Buffer.concat(chunks));
 process.stdout.write(JSON.stringify({format:process.argv[2],contentType:"image/png",width:2,height:2,privacyCleaned:true}));`
	if err := os.WriteFile(filepath.Join(dir, "sanitize-image.mjs"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}
func TestImageSourceBoundaryRejectsSymlinksAndOutside(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	source := filepath.Join(outside, "private.png")
	if err := os.WriteFile(source, smallPNG(t), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := allowedLocalImage(source, []string{allowed}); err == nil {
		t.Fatal("outside path allowed")
	}
	if _, err := allowedLocalImage("relative.png", []string{allowed}); err == nil {
		t.Fatal("relative path allowed")
	}
	link := filepath.Join(allowed, "escape.png")
	if err := os.Symlink(source, link); err == nil {
		if _, err := allowedLocalImage(link, []string{allowed}); err == nil {
			t.Fatal("escaping symlink allowed")
		}
	}
	inside := filepath.Join(allowed, "valid.png")
	if err := os.WriteFile(inside, smallPNG(t), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := allowedLocalImage(inside, []string{allowed}); err != nil || got != inside {
		t.Fatalf("valid image rejected %s %v", got, err)
	}
}
func TestImageInputValidationFailClosed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "image.png")
	for _, input := range [][]byte{[]byte("plain text"), {0xff, 0xd8, 0xff}} {
		if err := os.WriteFile(p, input, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := validateImagePayload(imageUploadRequest{LocalPath: p, Format: "invalid"}, bridgeConfig{ContentRoot: dir}); err == nil {
			t.Fatal("invalid format accepted")
		}
	}
	if err := os.WriteFile(p, []byte("wrong-image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := validateImagePayload(imageUploadRequest{LocalPath: p, Format: "png"}, bridgeConfig{ContentRoot: dir}); err == nil {
		t.Fatal("unknown bytes accepted")
	}
	cfg := bridgeConfig{ContentRoot: dir, ImageUploadRoots: []string{dir}}
	if _, _, _, err := validateImagePayload(imageUploadRequest{LocalPath: p, Format: "png", Alt: "secret\nmore"}, cfg); err == nil {
		t.Fatal("newline alt accepted")
	}
}
func TestImageBridgeDryRunAndMockR2Upload(t *testing.T) {
	t.Setenv("BLOGCTL_DATA_DIR", t.TempDir())
	photos := t.TempDir()
	file := filepath.Join(photos, "photo.png")
	if err := os.WriteFile(file, smallPNG(t), 0600); err != nil {
		t.Fatal(err)
	}
	var uploads atomic.Int32
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploads.Add(1)
		if r.Method != "PUT" || !strings.HasPrefix(r.URL.Path, "/photos/images/v1/") {
			t.Errorf("unexpected route %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "image/png" {
			t.Errorf("content type %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer remote.Close()
	server, err := New("test-token")
	if err != nil {
		t.Fatal(err)
	}
	server.config.ContentRoot = photos
	server.config.EngineRoot = fakeImageEngine(t)
	server.config.Publishing.Assets.R2.Bucket = "photos"
	server.config.Publishing.Assets.R2.Endpoint = remote.URL
	server.config.Publishing.Assets.R2.PublicBaseURL = "https://cdn.example.com/"
	server.config.Publishing.Assets.R2.AccessKeyID = "test-access"
	server.config.Publishing.Assets.R2.SecretAccessKey = "test-secret"
	server.httpClient = remote.Client()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	body := map[string]any{"localPath": file, "format": "png", "alt": "sunset", "dryRun": true}
	r := mcpRequest(t, ts, "POST", "/v1/mcp/images/upload", body)
	var preview map[string]any
	if err := json.NewDecoder(r.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 200 || preview["privacy_cleaned"] != true || preview["version"] != "v1" {
		t.Fatalf("dry run status=%d payload=%v", r.StatusCode, preview)
	}
	if _, ok := preview["url"]; ok {
		t.Fatal("dry run produced public URL")
	}
	if uploads.Load() != 0 {
		t.Fatal("dry run uploaded to R2")
	}
	body["dryRun"] = false
	r = mcpRequest(t, ts, "POST", "/v1/mcp/images/upload", body)
	var actual map[string]any
	if err := json.NewDecoder(r.Body).Decode(&actual); err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 200 || uploads.Load() != 1 {
		t.Fatalf("upload status=%d uploads=%d %v", r.StatusCode, uploads.Load(), actual)
	}
	if url, ok := actual["url"].(string); !ok || !strings.HasPrefix(url, "https://cdn.example.com/images/v1/") {
		t.Fatalf("bad uploaded URL %v", actual)
	}
	if !strings.Contains(actual["markdown"].(string), "![sunset](") {
		t.Fatalf("bad Markdown %v", actual)
	}
	if strings.Contains(actual["markdown"].(string), file) {
		t.Fatal("local filename leaked into Markdown")
	}
	req, _ := http.NewRequest("POST", ts.URL+"/v1/mcp/images/upload", bytes.NewReader([]byte("{}")))
	r, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated upload status=%d", r.StatusCode)
	}
}

func TestImageUploadRootsConfigPersistsAndStaysPrivate(t *testing.T) {
	useIsolatedUserConfigDir(t)
	root := t.TempDir()
	normalized, err := normalizeBridgeConfig(bridgeConfig{ImageUploadRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveBridgeConfig(normalized); err != nil {
		t.Fatal(err)
	}
	reloaded := loadBridgeConfig()
	if len(reloaded.ImageUploadRoots) != 1 || reloaded.ImageUploadRoots[0] != root {
		t.Fatalf("image roots did not persist")
	}
	published, _ := json.Marshal(publicBridgeConfig(reloaded))
	if bytes.Contains(published, []byte(root)) || bytes.Contains(published, []byte("imageUploadRoots")) {
		t.Fatal("private file root leaked through public config")
	}
	if _, err := normalizeBridgeConfig(bridgeConfig{ImageUploadRoots: []string{"relative/images"}}); err == nil {
		t.Fatal("relative allowlist root accepted")
	}
}

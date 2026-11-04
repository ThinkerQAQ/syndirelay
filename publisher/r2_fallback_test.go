package publisher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testFallbackConfig(endpoint string) R2FallbackConfig {
	return R2FallbackConfig{
		AccessKeyID: "test-key", SecretAccessKey: "test-secret",
		Endpoint: endpoint, Bucket: "test-bucket",
		PublicBaseURL: "https://cdn.example.test/",
	}
}

func TestR2FallbackLegacyExplicitKeyRemainsMutable(t *testing.T) {
	for _, publicURL := range []string{"", "https://cdn.example.test/legacy/image.webp"} {
		t.Run("publicURL="+publicURL, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("If-None-Match") != "" {
					t.Error("explicit legacy object key unexpectedly became create-once")
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			image := RehostImage{Source: "https://source.example/image.webp", Payload: []byte("updated image"), ContentType: "image/webp"}
			input := DraftInput{
				R2Fallback: testFallbackConfig(server.URL),
				Assets:     []PublishingAsset{{Source: image.Source, ObjectKey: "legacy/image.webp", PublicURL: publicURL}},
			}
			got, err := uploadR2Fallback(context.Background(), server.Client(), input, image)
			if err != nil || got != "https://cdn.example.test/legacy/image.webp" || calls != 1 {
				t.Fatalf("mutable fallback: url=%q calls=%d error=%v", got, calls, err)
			}
		})
	}
}

func TestR2FallbackGeneratedDigestKeyIsImmutable(t *testing.T) {
	image := RehostImage{Source: "https://source.example/generated.png", Payload: []byte("synthetic image"), ContentType: "image/png"}
	sum := sha256.Sum256(image.Payload)
	digest := hex.EncodeToString(sum[:16])
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("If-None-Match") != "*" || !strings.Contains(r.URL.Path, digest) {
			t.Error("generated content-addressed fallback did not use conditional PUT")
		}
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer server.Close()
	got, err := uploadR2Fallback(context.Background(), server.Client(), DraftInput{R2Fallback: testFallbackConfig(server.URL)}, image)
	if err != nil || got != "https://cdn.example.test/fallback/images/"+digest+".png" || calls != 1 {
		t.Fatalf("immutable fallback: url=%q calls=%d error=%v", got, calls, err)
	}
}

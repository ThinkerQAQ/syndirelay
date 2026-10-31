package publisher

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func browserRequest(ctx context.Context, method, rawURL, origin, referer, userAgent string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	if origin != "" {
		req.Header.Set("origin", origin)
	}
	if referer != "" {
		req.Header.Set("referer", referer)
	}
	if strings.TrimSpace(userAgent) != "" {
		req.Header.Set("user-agent", userAgent)
	}
	return req, nil
}

func doJSON(client *http.Client, req *http.Request, platform, operation string, output any) error {
	started := time.Now()
	response, err := client.Do(req)
	if err != nil {
		slog.Warn("platform request failed", "operation", operation, "platform", platform, "durationMs", time.Since(started).Milliseconds(), "errorType", fmt.Sprintf("%T", err))
		return platformError(ErrUpstream, platform, operation, 0, err.Error(), true)
	}
	defer response.Body.Close()
	raw, err := readBounded(response, 4<<20)
	if err != nil {
		return platformError(ErrUpstream, platform, operation, response.StatusCode, err.Error(), true)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return classifyHTTP(platform, operation, response.StatusCode, string(raw))
	}
	if output == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, output); err != nil {
		bodyKind := "other"
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) > 0 && trimmed[0] == '<' {
			bodyKind = "html"
		}
		slog.Warn("platform returned invalid JSON", "operation", operation, "platform", platform, "status", response.StatusCode, "contentType", response.Header.Get("content-type"), "bodyKind", bodyKind, "byteSize", len(raw), "durationMs", time.Since(started).Milliseconds())
		return platformError(ErrUpstream, platform, operation, response.StatusCode, "invalid JSON response", false)
	}
	return nil
}

func encodeJSONBody(value any) (io.Reader, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(payload), nil
}

func cookieValue(session Session, names ...string) string {
	for _, name := range names {
		for _, cookie := range session.Cookies {
			if cookie.Name == name && cookie.Value != "" {
				return cookie.Value
			}
		}
	}
	return ""
}

func hmacSHA256Base64(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func hmacSHA1Base64(secret, message string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func md5Hex(payload []byte) string {
	sum := md5.Sum(payload)
	return hex.EncodeToString(sum[:])
}

func inferImageFilename(source, contentType string) string {
	extension := ""
	if parsed, err := url.Parse(source); err == nil {
		extension = strings.ToLower(filepath.Ext(parsed.Path))
	}
	if extension == "" {
		switch strings.ToLower(strings.Split(contentType, ";")[0]) {
		case "image/png":
			extension = ".png"
		case "image/gif":
			extension = ".gif"
		case "image/webp":
			extension = ".webp"
		default:
			extension = ".jpg"
		}
	}
	return "image" + extension
}

func multipartBody(fields map[string]string, fileField, filename, contentType string, payload []byte) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}
	if fileField != "" {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fileField, filename))
		if contentType != "" {
			header.Set("Content-Type", contentType)
		}
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(payload); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return &body, writer.FormDataContentType(), nil
}

func formBody(values url.Values) io.Reader {
	return strings.NewReader(values.Encode())
}

func htmlFor(input DraftInput) string {
	if strings.TrimSpace(input.HTML) != "" {
		return input.HTML
	}
	return "<p>" + strings.ReplaceAll(strings.ReplaceAll(input.Markdown, "&", "&amp;"), "\n\n", "</p><p>") + "</p>"
}

func valueString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func responseMessage(values ...any) string {
	for _, value := range values {
		if text := valueString(value); text != "" {
			return text
		}
	}
	return "upstream rejected the request"
}

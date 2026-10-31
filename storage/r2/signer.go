package r2

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func SignPut(rawURL string, payload []byte, contentType string, config Config, now time.Time) (map[string]string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("R2 endpoint must use HTTPS")
	}
	if strings.TrimSpace(config.AccessKeyID) == "" || strings.TrimSpace(config.SecretAccessKey) == "" {
		return nil, errors.New("R2 credentials are required")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	utc := now.UTC()
	amzDate := utc.Format("20060102T150405Z")
	dateStamp := utc.Format("20060102")
	payloadHashBytes := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(payloadHashBytes[:])
	canonicalHeaders := strings.Join([]string{
		"content-type:" + contentType,
		"host:" + parsed.Host,
		"x-amz-content-sha256:" + payloadHash,
		"x-amz-date:" + amzDate,
		"",
	}, "\n")
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	canonicalRequest := strings.Join([]string{
		http.MethodPut,
		path,
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	scope := dateStamp + "/auto/s3/aws4_request"
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(requestHash[:]),
	}, "\n")
	dateKey := hmacSHA256([]byte("AWS4"+config.SecretAccessKey), dateStamp)
	regionKey := hmacSHA256(dateKey, "auto")
	serviceKey := hmacSHA256(regionKey, "s3")
	signingKey := hmacSHA256(serviceKey, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	return map[string]string{
		"authorization": "AWS4-HMAC-SHA256 Credential=" + config.AccessKeyID + "/" + scope +
			", SignedHeaders=" + signedHeaders + ", Signature=" + signature,
		"content-type":         contentType,
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           amzDate,
		"cache-control":        "public, max-age=31536000, immutable",
	}, nil
}

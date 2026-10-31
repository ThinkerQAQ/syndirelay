package publisher

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"hash/crc32"
	"net/url"
	"sort"
	"strings"
	"time"
)

type AWS4Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SecurityToken   string
	Region          string
	Service         string
}

func hmacSHA256(key []byte, message string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(message))
	return mac.Sum(nil)
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func awsEscape(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func canonicalQuery(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, key := range keys {
		items := append([]string{}, values[key]...)
		sort.Strings(items)
		for _, value := range items {
			parts = append(parts, awsEscape(key)+"="+awsEscape(value))
		}
	}
	return strings.Join(parts, "&")
}

func SignAWS4(method, rawURL string, credentials AWS4Credentials, now time.Time) (map[string]string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	region := credentials.Region
	if region == "" {
		region = "cn-north-1"
	}
	service := credentials.Service
	if service == "" {
		service = "imagex"
	}
	utc := now.UTC()
	amzDate := utc.Format("20060102T150405Z")
	dateStamp := utc.Format("20060102")

	signedHeaders := []string{"x-amz-date"}
	headers := map[string]string{"x-amz-date": amzDate}
	if credentials.SecurityToken != "" {
		headers["x-amz-security-token"] = credentials.SecurityToken
		signedHeaders = append(signedHeaders, "x-amz-security-token")
	}
	sort.Strings(signedHeaders)

	var canonicalHeaders strings.Builder
	for _, name := range signedHeaders {
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(strings.TrimSpace(headers[name]))
		canonicalHeaders.WriteByte('\n')
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	canonicalRequest := strings.Join([]string{
		strings.ToUpper(method),
		path,
		canonicalQuery(parsed.Query()),
		canonicalHeaders.String(),
		strings.Join(signedHeaders, ";"),
		sha256Hex(""),
	}, "\n")

	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex(canonicalRequest),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+credentials.SecretAccessKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	headers["authorization"] = "AWS4-HMAC-SHA256 Credential=" + credentials.AccessKeyID + "/" + scope +
		", SignedHeaders=" + strings.Join(signedHeaders, ";") + ", Signature=" + signature
	return headers, nil
}

func CRC32Hex(data []byte) string {
	value := crc32.ChecksumIEEE(data)
	const hexDigits = "0123456789abcdef"
	out := make([]byte, 8)
	for index := 7; index >= 0; index-- {
		out[index] = hexDigits[value&0xf]
		value >>= 4
	}
	return string(out)
}

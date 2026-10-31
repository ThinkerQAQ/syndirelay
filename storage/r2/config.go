package r2

import (
	"errors"
	"net/url"
	"strings"
)

type Config struct {
	AccessKeyID     string
	SecretAccessKey string
	AccountID       string
	Endpoint        string
	Bucket          string
	PublicBaseURL   string
}

func Endpoint(config Config) string {
	if value := strings.TrimRight(strings.TrimSpace(config.Endpoint), "/"); value != "" {
		return value
	}
	if accountID := strings.TrimSpace(config.AccountID); accountID != "" {
		return "https://" + accountID + ".r2.cloudflarestorage.com"
	}
	return ""
}

func Ready(config Config) bool {
	return strings.TrimSpace(config.AccessKeyID) != "" &&
		strings.TrimSpace(config.SecretAccessKey) != "" &&
		strings.TrimSpace(config.Bucket) != "" &&
		Endpoint(config) != "" &&
		strings.TrimSpace(config.PublicBaseURL) != ""
}

func Validate(config Config) error {
	if !Ready(config) {
		return errors.New("R2 is not configured")
	}
	endpoint, err := url.Parse(Endpoint(config))
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
		return errors.New("R2 endpoint must use HTTPS")
	}
	publicBase, err := url.Parse(strings.TrimSpace(config.PublicBaseURL))
	if err != nil || publicBase.Scheme != "https" || publicBase.Host == "" {
		return errors.New("R2 public base URL is invalid")
	}
	return nil
}

func encodePath(value string) string {
	parts := strings.Split(strings.ReplaceAll(value, "\\", "/"), "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func PublicURL(config Config, objectKey string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(config.PublicBaseURL))
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return "", errors.New("R2 public base URL is invalid")
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	relative, err := url.Parse(encodePath(strings.TrimPrefix(strings.TrimSpace(objectKey), "/")))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(relative).String(), nil
}

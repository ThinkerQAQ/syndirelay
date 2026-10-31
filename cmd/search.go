package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	blogbridge "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/bridge"
	blogsearch "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/search"
)

func (a app) runSearch(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: blogctl search <build|inventory|submit|audit|notify> [options]")
	}
	switch args[0] {
	case "build":
		return a.runSearchBuild(args[1:])
	case "inventory":
		return a.runSearchInventory(args[1:])
	case "submit":
		return a.runSearchSubmit(args[1:])
	case "audit":
		return a.runSearchAudit(args[1:])
	case "notify":
		return a.runSearchNotify(args[1:])
	default:
		return errors.New("usage: blogctl search <build|inventory|submit|audit|notify> [options]")
	}
}

func searchOption(args []string, name, fallback string) (string, error) {
	for index := 0; index < len(args); index++ {
		if args[index] != name {
			continue
		}
		if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
			return "", fmt.Errorf("%s requires a value", name)
		}
		return args[index+1], nil
	}
	return fallback, nil
}

func searchFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func searchProviders(args []string) ([]string, error) {
	raw, err := searchOption(args, "--providers", "")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("--providers is required")
	}
	seen := map[string]struct{}{}
	result := []string{}
	for _, item := range strings.Split(raw, ",") {
		provider := strings.TrimSpace(item)
		if provider == "" {
			continue
		}
		switch provider {
		case "indexnow", "baidu", "google":
		default:
			return nil, fmt.Errorf("unsupported search provider: %s", provider)
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		result = append(result, provider)
	}
	if len(result) == 0 {
		return nil, errors.New("--providers is required")
	}
	return result, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (a app) runSearchBuild(args []string) error {
	distRoot, err := searchOption(args, "--dist", "dist")
	if err != nil {
		return err
	}
	output, err := searchOption(args, "--output", blogsearch.DefaultTextSitemap)
	if err != nil {
		return err
	}
	fingerprintOutput, err := searchOption(args, "--fingerprint-output", blogsearch.DefaultFingerprintManifest)
	if err != nil {
		return err
	}
	distRoot = filepath.Join(a.root, distRoot)
	inventory, err := blogsearch.WriteGeneratedInventory(distRoot, output, fingerprintOutput, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "[search] built %d URLs for %s\n", len(inventory.URLList), inventory.Origin)
	return nil
}

func (a app) runSearchInventory(args []string) error {
	distRoot, err := searchOption(args, "--dist", "dist")
	if err != nil {
		return err
	}
	expectedOrigin, err := searchOption(args, "--site-url", "")
	if err != nil {
		return err
	}
	if expectedOrigin != "" {
		if strings.HasPrefix(expectedOrigin, "sc-domain:") {
			expectedOrigin = "https://" + strings.TrimPrefix(expectedOrigin, "sc-domain:")
		}
	}
	inventory, err := blogsearch.LoadGeneratedInventory(filepath.Join(a.root, distRoot), blogsearch.DefaultSitemapIndex, expectedOrigin)
	if err != nil {
		return err
	}
	if searchFlag(args, "--json") {
		payload := map[string]any{
			"origin":          inventory.Origin,
			"sitemapIndexUrl": inventory.SitemapIndexURL,
			"childSitemaps":   len(inventory.SitemapURLs),
			"urlCount":        len(inventory.URLList),
			"urlList":         inventory.URLList,
		}
		data, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Fprintln(a.out, string(data))
		return nil
	}
	fmt.Fprintf(a.out, "[search] origin=%s childSitemaps=%d urls=%d\n", inventory.Origin, len(inventory.SitemapURLs), len(inventory.URLList))
	return nil
}

func searchSiteURL(args []string) (string, error) {
	config, err := blogbridge.ResolvedSearchRuntimeConfig()
	if err != nil {
		return "", err
	}
	value, err := searchOption(args, "--site-url", "")
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = config.Site.GoogleProperty
	}
	if value != config.Site.GoogleProperty {
		return "", errors.New("--site-url must match search.google_property in blogctl.toml")
	}
	return value, nil
}

func searchOrigin(siteURL string) (string, error) {
	config, err := blogbridge.ResolvedSearchRuntimeConfig()
	if err != nil {
		return "", err
	}
	if siteURL != config.Site.GoogleProperty {
		return "", errors.New("Search Console property must match search.google_property")
	}
	return config.Site.Origin, nil
}

func searchURLs(args []string, origin, distRoot string) ([]string, error) {
	full := searchFlag(args, "--all")
	urlsFile, err := searchOption(args, "--urls-file", "")
	if err != nil {
		return nil, err
	}
	if full == (strings.TrimSpace(urlsFile) != "") {
		return nil, errors.New("submission requires exactly one of --all or --urls-file <file>")
	}
	if urlsFile != "" {
		return blogsearch.ReadURLFile(urlsFile, origin)
	}
	inventory, err := blogsearch.LoadGeneratedInventory(distRoot, blogsearch.DefaultSitemapIndex, origin)
	if err != nil {
		return nil, err
	}
	return inventory.URLList, nil
}

func aggregateProviderStatus(result blogsearch.IndexNowResult) int {
	status := 0
	for _, item := range result.Results {
		if item.HTTPStatus > status {
			status = item.HTTPStatus
		}
	}
	return status
}

func (a app) runSearchSubmit(args []string) error {
	providers, err := searchProviders(args)
	if err != nil {
		return err
	}
	siteURL, err := searchSiteURL(args)
	if err != nil {
		return err
	}
	origin, err := searchOrigin(siteURL)
	if err != nil {
		return err
	}
	distRoot, err := searchOption(args, "--dist", "dist")
	if err != nil {
		return err
	}
	publicRoot, err := searchOption(args, "--public", "public")
	if err != nil {
		return err
	}
	distRoot = filepath.Join(a.root, distRoot)
	publicRoot = filepath.Join(a.root, publicRoot)

	needsURLs := containsString(providers, "indexnow") || containsString(providers, "baidu")
	var urls []string
	if needsURLs {
		urls, err = searchURLs(args, origin, distRoot)
		if err != nil {
			return err
		}
	}

	client := &http.Client{Timeout: 45 * time.Second}
	ctx := context.Background()
	localConfig, err := blogbridge.ResolvedSearchRuntimeConfig()
	if err != nil {
		return err
	}

	if containsString(providers, "indexnow") {
		config, err := blogsearch.ResolveIndexNowConfig(
			origin,
			publicRoot,
			strings.TrimSpace(localConfig.IndexNowEndpoint),
			strings.TrimSpace(localConfig.IndexNowKey),
			strings.TrimSpace(localConfig.IndexNowKeyLocation),
			true,
		)
		if err != nil {
			return err
		}
		result, err := blogsearch.SubmitIndexNow(ctx, client, urls, config)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "[search:indexnow] submitted=%d batches=%d http=%d\n", result.URLCount, result.BatchCount, aggregateProviderStatus(result))
	}

	if containsString(providers, "baidu") {
		token := strings.TrimSpace(localConfig.BaiduToken)
		if token == "" {
			return errors.New("Baidu token is required in blogctl.toml for Baidu submission")
		}
		site := localConfig.Site.Origin
		config, err := blogsearch.ResolveBaiduConfig(origin, site, token)
		if err != nil {
			return err
		}
		result, err := blogsearch.SubmitBaidu(ctx, client, urls, config)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "[search:baidu] submitted=%d success=%d remain=%d batches=%d\n", result.URLCount, result.SuccessCount, result.Remain, result.BatchCount)
	}

	if containsString(providers, "google") {
		serviceAccount, err := googleServiceAccountForCLI(args, localConfig.GoogleServiceAccountJSON)
		if err != nil {
			return err
		}
		accessToken, err := googleAccessTokenForCLI(ctx, client, serviceAccount, searchFlag(args, "--optional-google"))
		if err != nil {
			return err
		}
		if accessToken == "" {
			fmt.Fprintln(a.out, "[search:google] skipped: Google service account is not configured in blogctl.toml")
		} else {
			results, err := blogsearch.SubmitGoogleSitemaps(ctx, client, siteURL, origin, accessToken, localConfig.Site.SitemapURL, localConfig.Site.URLListURL)
			if err != nil {
				return err
			}
			fmt.Fprintf(a.out, "[search:google] submitted=%d sitemaps\n", len(results))
		}
	}
	return nil
}

func googleServiceAccountForCLI(args []string, configured string) (string, error) {
	path, err := searchOption(args, "--google-service-account-file", "")
	if err != nil {
		return "", err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return configured, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Google service account file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func googleAccessTokenForCLI(ctx context.Context, client *http.Client, configured string, optional bool) (string, error) {
	raw := strings.TrimSpace(configured)
	if raw == "" {
		if optional {
			return "", nil
		}
		return "", errors.New("Google service account is required in blogctl.toml")
	}
	credentials, err := blogsearch.ParseGoogleServiceAccount(raw)
	if err != nil {
		return "", err
	}
	token, err := blogsearch.FetchGoogleAccessToken(ctx, client, credentials)
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func nonNegativeIntOption(args []string, name string, fallback int) (int, error) {
	raw, err := searchOption(args, name, strconv.Itoa(fallback))
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return value, nil
}

func (a app) runSearchAudit(args []string) error {
	provider, err := searchOption(args, "--provider", "")
	if err != nil {
		return err
	}
	if provider != "google" {
		return errors.New("only --provider google is supported for search audit")
	}
	siteURL, err := searchSiteURL(args)
	if err != nil {
		return err
	}
	origin, err := searchOrigin(siteURL)
	if err != nil {
		return err
	}
	offset, err := nonNegativeIntOption(args, "--offset", 0)
	if err != nil {
		return err
	}
	limit, err := nonNegativeIntOption(args, "--limit", blogsearch.GoogleURLInspectionDailySiteLimit)
	if err != nil {
		return err
	}
	if limit < 1 || limit > blogsearch.GoogleURLInspectionDailySiteLimit {
		return fmt.Errorf("--limit must be between 1 and %d", blogsearch.GoogleURLInspectionDailySiteLimit)
	}
	delayMS, err := nonNegativeIntOption(args, "--request-delay-ms", int(blogsearch.GoogleURLInspectionDefaultDelay/time.Millisecond))
	if err != nil {
		return err
	}

	urlsFile, err := searchOption(args, "--urls-file", "")
	if err != nil {
		return err
	}
	var urls []string
	if urlsFile != "" {
		urls, err = blogsearch.ReadURLFile(urlsFile, origin)
	} else {
		distRoot, optionErr := searchOption(args, "--dist", "dist")
		if optionErr != nil {
			return optionErr
		}
		inventory, inventoryErr := blogsearch.LoadGeneratedInventory(
			filepath.Join(a.root, distRoot),
			blogsearch.DefaultSitemapIndex,
			origin,
		)
		if inventoryErr != nil {
			return inventoryErr
		}
		urls = inventory.URLList
	}
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 45 * time.Second}
	ctx := context.Background()
	localConfig, err := blogbridge.ResolvedSearchRuntimeConfig()
	if err != nil {
		return err
	}
	serviceAccount, err := googleServiceAccountForCLI(args, localConfig.GoogleServiceAccountJSON)
	if err != nil {
		return err
	}
	accessToken, err := googleAccessTokenForCLI(ctx, client, serviceAccount, false)
	if err != nil {
		return err
	}
	report, err := blogsearch.AuditGoogleURLs(
		ctx,
		client,
		urls,
		siteURL,
		origin,
		accessToken,
		offset,
		limit,
		time.Duration(delayMS)*time.Millisecond,
		nil,
	)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	output, err := searchOption(args, "--output", "")
	if err != nil {
		return err
	}
	if output == "" {
		fmt.Fprintln(a.out, string(data))
		return nil
	}
	outputPath, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "[search:google] inspection report written: %s\n", outputPath)
	return nil
}

func (a app) runSearchNotify(args []string) error {
	siteURL, err := searchSiteURL(args)
	if err != nil {
		return err
	}
	origin, err := searchOrigin(siteURL)
	if err != nil {
		return err
	}
	config, err := blogbridge.ResolvedSearchRuntimeConfig()
	if err != nil {
		return err
	}
	inventory, err := blogsearch.FetchRemoteInventory(context.Background(), &http.Client{Timeout: 45 * time.Second}, origin, config.Site)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp("", "blogctl-sitemap-*.txt")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.WriteString(strings.Join(inventory.URLs, "\n") + "\n"); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}

	// Deploy notification is intentionally stateless. Baidu incremental
	// submission requires durable provider state in the local Bridge, so CI does
	// not silently convert it into a full-site push.
	providers := "indexnow,google"
	fmt.Fprintf(a.out, "[search] live inventory: %s (%d URLs)\n", inventory.Source, inventory.Total)
	submitArgs := []string{
		"--providers", providers,
		"--urls-file", name,
		"--site-url", siteURL,
		"--optional-google",
		"--public", "public",
	}
	serviceAccountFile, err := searchOption(args, "--google-service-account-file", "")
	if err != nil {
		return err
	}
	if strings.TrimSpace(serviceAccountFile) != "" {
		submitArgs = append(submitArgs, "--google-service-account-file", serviceAccountFile)
	}
	return a.runSearchSubmit(submitArgs)
}

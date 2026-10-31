package bridge

import "strings"

type publishingAssetStatusView struct {
	Ready   bool     `json:"ready"`
	Missing []string `json:"missing,omitempty"`
}

func publishingAssetStatus(config bridgeConfig) publishingAssetStatusView {
	r2 := config.Publishing.Assets.R2
	missing := []string{}
	if strings.TrimSpace(r2.Bucket) == "" {
		missing = append(missing, "R2 Bucket")
	}
	if strings.TrimSpace(r2.AccessKeyID) == "" {
		missing = append(missing, "Access Key ID")
	}
	if strings.TrimSpace(r2.SecretAccessKey) == "" {
		missing = append(missing, "Secret Access Key")
	}
	if strings.TrimSpace(r2.Endpoint) == "" && strings.TrimSpace(r2.AccountID) == "" {
		missing = append(missing, "Account ID 或 Endpoint")
	}
	if strings.TrimSpace(r2.PublicBaseURL) == "" {
		missing = append(missing, "Public Base URL")
	}
	return publishingAssetStatusView{Ready: len(missing) == 0, Missing: missing}
}

type publishingR2View struct {
	Bucket                    string `json:"bucket"`
	PublicBaseURL             string `json:"publicBaseUrl"`
	AccessKeyID               string `json:"accessKeyId,omitempty"`
	AccountID                 string `json:"accountId,omitempty"`
	Endpoint                  string `json:"endpoint,omitempty"`
	SecretAccessKeyConfigured bool   `json:"secretAccessKeyConfigured"`
}

type publishingAssetsView struct {
	Store string           `json:"store"`
	R2    publishingR2View `json:"r2"`
}

func publishingAssetsPublicView(config bridgeConfig) publishingAssetsView {
	r2 := config.Publishing.Assets.R2
	return publishingAssetsView{
		Store: config.Publishing.Assets.Store,
		R2: publishingR2View{
			Bucket: r2.Bucket, PublicBaseURL: r2.PublicBaseURL, AccessKeyID: r2.AccessKeyID,
			AccountID: r2.AccountID, Endpoint: r2.Endpoint,
			SecretAccessKeyConfigured: strings.TrimSpace(r2.SecretAccessKey) != "",
		},
	}
}

func applyPublishingRuntimeConfig(config bridgeConfig, compiler *publishingCompilerConfig, assets *publishingAssetsConfig) (bridgeConfig, error) {
	if compiler != nil {
		config.Publishing.Compiler = *compiler
	}
	if assets != nil {
		if strings.TrimSpace(assets.R2.SecretAccessKey) == "" {
			assets.R2.SecretAccessKey = config.Publishing.Assets.R2.SecretAccessKey
		}
		config.Publishing.Assets = *assets
	}
	return normalizeBridgeConfig(config)
}

func publishingControlPayload(config bridgeConfig) map[string]any {
	return map[string]any{
		"platforms":   publishingViews(config),
		"compiler":    config.Publishing.Compiler,
		"assets":      publishingAssetsPublicView(config),
		"assetStatus": publishingAssetStatus(config),
	}
}

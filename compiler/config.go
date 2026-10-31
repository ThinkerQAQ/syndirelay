package compiler

import blogpublishing "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publishing"

const DefaultAssetBaseURL = blogpublishing.DefaultAssetBaseURL

type FooterConfig = blogpublishing.FooterConfig
type CanonicalConfig = blogpublishing.CanonicalConfig
type TrackingConfig = blogpublishing.TrackingConfig
type PlatformConfig = blogpublishing.PlatformConfig
type PublishingConfig = blogpublishing.Config

func defaultPlatformConfig(id string) PlatformConfig {
	return blogpublishing.DefaultPlatformConfig(id)
}

func normalizePublishingConfig(config PublishingConfig) (PublishingConfig, error) {
	return blogpublishing.Normalize(config)
}

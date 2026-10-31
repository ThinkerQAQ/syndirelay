package publisher

import blogplatform "github.com/ThinkerQAQ/syndirelay/platform"

type PlatformCapabilities = blogplatform.Capabilities

func PlatformCapabilitiesFor(platform string) PlatformCapabilities {
	return blogplatform.For(platform)
}

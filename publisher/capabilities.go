package publisher

import blogplatform "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/platform"

type PlatformCapabilities = blogplatform.Capabilities

func PlatformCapabilitiesFor(platform string) PlatformCapabilities {
	return blogplatform.For(platform)
}

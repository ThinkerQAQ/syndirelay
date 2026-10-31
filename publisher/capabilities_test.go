package publisher

import "testing"

func TestPlatformCapabilitiesMatchCurrentControlPlane(t *testing.T) {
	cases := []struct {
		platform string
		check    func(PlatformCapabilities) bool
	}{
		{"cnblogs", func(value PlatformCapabilities) bool {
			return value.BrowserSession && value.DraftCreate && value.DraftUpdate && value.ExplicitPublish &&
				value.PublishedUpdate && value.RemoteList && value.BodyImages && value.BodyImageRehost
		}},
		{"devto", func(value PlatformCapabilities) bool {
			return value.BrowserSession && value.APIKey && value.DraftCreate && value.DraftUpdate && value.ExplicitPublish &&
				value.PublishedUpdate && value.RemoteList && value.BodyImages && value.BodyImageRehost &&
				value.CoverImage && value.NativeCanonical && value.Tags
		}},
		{"medium", func(value PlatformCapabilities) bool {
			return value.BrowserSession && value.DraftCreate && value.DraftUpdate && value.ExplicitPublish &&
				!value.PublishedUpdate && value.RemoteList && value.BodyImages && value.BodyImageRehost &&
				!value.CoverImage && !value.NativeCanonical && !value.Tags
		}},
		{"toutiao", func(value PlatformCapabilities) bool {
			return value.BrowserSession && value.RemoteList &&
				value.DraftCreate && value.DraftUpdate && value.ExplicitPublish &&
				value.PublishedUpdate && value.BodyImages && value.BodyImageRehost
		}},
		{"csdn", func(value PlatformCapabilities) bool {
			return value.BrowserSession && value.DraftCreate && value.DraftUpdate && value.ExplicitPublish &&
				value.RemoteList && value.BodyImages && value.BodyImageRehost
		}},

		{"zhihu", func(value PlatformCapabilities) bool {
			return value.BrowserSession && value.DraftCreate && value.DraftUpdate && value.ExplicitPublish &&
				value.RemoteList && value.BodyImages && value.BodyImageRehost
		}},
		{"oschina", func(value PlatformCapabilities) bool {
			return value.BrowserSession && value.DraftCreate && value.DraftUpdate && !value.ExplicitPublish &&
				value.RemoteList && value.BodyImages && value.BodyImageRehost
		}},
	}
	for _, tc := range cases {
		t.Run(tc.platform, func(t *testing.T) {
			if value := PlatformCapabilitiesFor(tc.platform); !tc.check(value) {
				t.Fatalf("capabilities = %#v", value)
			}
		})
	}
	if value := PlatformCapabilitiesFor("unknown"); value != (PlatformCapabilities{}) {
		t.Fatalf("unknown capabilities = %#v", value)
	}
}

func TestMissingDraftRecreationPolicyFailsClosedForPublishedArticles(t *testing.T) {
	published := PublicationState{PublishedRemoteID: "published-1", PublishedURL: "https://example.com/published-1"}
	if mayRecreateMissingDraft("devto", published) {
		t.Fatal("DEV.to must not recreate a missing draft when a published article is already bound")
	}
	if mayRecreateMissingDraft("juejin", published) {
		t.Fatal("Juejin must not recreate a missing draft when published-update safety is not verified")
	}
	if mayRecreateMissingDraft("toutiao", published) {
		t.Fatal("Toutiao published articles may only change via explicit update-published operation")
	}
	if !mayRecreateMissingDraft("cnblogs", published) {
		t.Fatal("CNBlogs has a verified published-update workflow and may create a replacement draft")
	}
	if !mayRecreateMissingDraft("devto", PublicationState{}) {
		t.Fatal("a missing draft may be recreated when no published article exists")
	}
}

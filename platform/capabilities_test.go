package platform

import (
	"reflect"
	"testing"
)

func TestRegistryCoversSupportedPublishingPlatforms(t *testing.T) {
	want := []string{
		"cnblogs", "juejin", "csdn", "segmentfault", "zhihu",
		"51cto", "oschina", "toutiao", "devto", "hashnode", "medium",
	}
	if got := IDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs = %#v", got)
	}
	for _, id := range want {
		if id == "toutiao" {
			if !Supported(id) || !For(id).RemoteList || !For(id).DraftCreate || !For(id).DraftUpdate ||
				!For(id).PublishedUpdate || !For(id).ExplicitPublish {
				t.Fatalf("%s should support drafts, explicit publication and guarded published updates: %#v", id, For(id))
			}
			continue
		}
		if !Supported(id) || !For(id).DraftCreate {
			t.Fatalf("%s capabilities = %#v", id, For(id))
		}
	}
	if Supported("unknown") || For("unknown") != (Capabilities{}) {
		t.Fatalf("unknown platform unexpectedly supported")
	}
}

func TestVerifiedAdvancedCapabilitiesStayFailClosed(t *testing.T) {
	devto := For("devto")
	if !devto.BrowserSession || !devto.APIKey || !devto.DraftUpdate || !devto.ExplicitPublish ||
		!devto.PublishedUpdate || !devto.RemoteList || !devto.BodyImages || !devto.BodyImageRehost ||
		!devto.CoverImage || !devto.NativeCanonical || !devto.Tags {
		t.Fatalf("DEV.to capabilities = %#v", devto)
	}

	hashnode := For("hashnode")
	if !hashnode.BrowserSession || hashnode.ExplicitPublish || hashnode.PublishedUpdate || !hashnode.NativeCanonical || !hashnode.BodyImageRehost || !hashnode.CoverImage {
		t.Fatalf("Hashnode capabilities = %#v", hashnode)
	}

	medium := For("medium")
	if !medium.BrowserSession || !medium.DraftCreate || !medium.DraftUpdate || !medium.ExplicitPublish ||
		!medium.RemoteList || !medium.BodyImages || !medium.BodyImageRehost ||
		medium.CoverImage || medium.NativeCanonical || medium.Tags || medium.PublishedUpdate || !medium.PublishedDraftEdit {
		t.Fatalf("Medium capabilities = %#v", medium)
	}

	oschina := For("oschina")
	if oschina.ExplicitPublish {
		t.Fatal("OSChina must not claim draft publishing: current adapter adds a new article")
	}

	csdn := For("csdn")
	if !csdn.BrowserSession || !csdn.DraftCreate || !csdn.DraftUpdate || !csdn.ExplicitPublish ||
		!csdn.RemoteList || !csdn.BodyImages || !csdn.BodyImageRehost || csdn.PublishedUpdate {
		t.Fatalf("CSDN capabilities = %#v", csdn)
	}

	juejin := For("juejin")
	if !juejin.RemoteList || !juejin.BodyImages || !juejin.BodyImageRehost || !juejin.ExplicitPublish || juejin.PublishedUpdate || juejin.CoverImage || juejin.NativeCanonical || juejin.Tags {
		t.Fatalf("Juejin capabilities = %#v", juejin)
	}
}

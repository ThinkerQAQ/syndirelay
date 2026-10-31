package bridge

import (
	"testing"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

func TestOSChinaBindingViewUsesAuthenticatedNumericAccountForDraftEditor(t *testing.T) {
	binding := publisher.PublicationBinding{
		Platform: "oschina", RemoteDraftID: "3328466",
		DraftURL:          "https://my.oschina.net/shengkunz/blog/write/draft/3328466",
		PublishedRemoteID: "19763618",
		PublishedURL:      "https://my.oschina.net/u/2360403/blog/19763618",
	}
	views := osChinaBindingViews(binding, "2360403")
	if len(views) != 2 ||
		views[0].URL != "https://my.oschina.net/u/2360403/blog/ai-write/draft/3328466" ||
		views[1].URL != binding.PublishedURL {
		t.Fatalf("bound draft should open editor and public link should remain unchanged: %+v", views)
	}
}

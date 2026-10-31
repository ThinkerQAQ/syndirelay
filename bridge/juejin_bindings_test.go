package bridge

import (
	"testing"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

func TestJuejinBindingStateMatchesPublishedDraftID(t *testing.T) {
	binding := publisher.PublicationBinding{
		RemoteDraftID: "7688942552989876259",
	}
	post := publisher.JuejinPost{
		ID:        "7689415591787773978",
		DraftID:   "7688942552989876259",
		Published: true,
	}
	bound, state := juejinBindingState(binding, post)
	if !bound || state != "draft" {
		t.Fatalf("bound/state = %v/%q", bound, state)
	}
}

func TestJuejinBindingStateKeepsPublishedBinding(t *testing.T) {
	binding := publisher.PublicationBinding{
		PublishedRemoteID: "7689415591787773978",
	}
	post := publisher.JuejinPost{
		ID:        "7689415591787773978",
		DraftID:   "7688942552989876259",
		Published: true,
	}
	bound, state := juejinBindingState(binding, post)
	if !bound || state != "published" {
		t.Fatalf("bound/state = %v/%q", bound, state)
	}
}

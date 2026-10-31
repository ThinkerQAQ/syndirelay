package compiler

import (
	"strings"
	"testing"
)

func TestMediumOnlyASCIIArtworkConversion(t *testing.T) {
	wide := "Java                                               Go                                               Python\n" +
		" │                                                   │                                                   │\n" +
		" ▼                                                   ▼                                                   ▼\n" +
		"Java Memory Model                                   Go Memory Model                                     CPython Memory Model"
	source := "Before\n\n~~~text\n" + wide + "\n~~~\n\n~~~text\ncount++\n~~~\n"
	body, assets, err := CompileMediumPublishingMarkdown(source, DefaultAssetBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].Kind != "ascii" {
		t.Fatalf("wrong image conversion assets=%+v", assets)
	}
	if !strings.Contains(body, "![") || !strings.Contains(body, "~~~text\ncount++\n~~~") {
		t.Fatalf("converted code example or lost diagram: %q", body)
	}
	original, plainAssets, err := CompilePublishingMarkdown(source, DefaultAssetBaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(plainAssets) != 0 || original != source {
		t.Fatal("Medium workaround changed other platform")
	}
}
func TestMediumASCIIArtworkConservativeRecognition(t *testing.T) {
	cases := []struct {
		src      string
		expected bool
	}{
		{"count++\nready = true", false},
		{"Thread A                  Thread B\nread count                    read count", false},
		{"Java\n │\n ▼\nLanguage\n └── CPU", true},
		{"Java                                                Go                                                Python\n │                                                   │                                                   │\n ▼                                                   ▼                                                   ▼", true},
		{"state ──┐\n        │\n        ▼", true},
	}
	for _, c := range cases {
		if got := mediumASCIIDiagram(c.src); got != c.expected {
			t.Errorf("got %v expected %v for %q", got, c.expected, c.src)
		}
	}
}

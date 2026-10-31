package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/bridge"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/social"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/social/buffer"
)

func (a app) runSocial(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: blogctl social <channels|source|create|list|show|stage|reconcile|publish>")
	}
	sub := args[0]
	flags := flag.NewFlagSet("social "+sub, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	id := flags.String("id", "", "local social draft id")
	input := flags.String("input", "", "JSON draft input file")
	revision := flags.String("revision", "", "reviewed content revision")
	platform := flags.String("platform", "", "linkedin, twitter, or bluesky")
	kind := flags.String("kind", "article", "source type")
	slug := flags.String("slug", "", "content source slug")
	contentRoot := flags.String("content-root", "", "BlogContent checkout")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	dataRoot, err := bridge.ConfigDir()
	if err != nil {
		return err
	}
	config := bridge.ResolvedSocialConfig()
	httpClient, err := bridge.ResolvedSocialHTTPClient()
	if err != nil {
		return err
	}
	client := buffer.Client{APIKey: config.Buffer.APIKey, HTTPClient: httpClient}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	show := func(value any) error {
		encoder := json.NewEncoder(a.out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	resolve := func(src social.Source) (social.Source, error) {
		root := strings.TrimSpace(*contentRoot)
		if root == "" {
			root = os.Getenv("BLOGCTL_CONTENT_ROOT")
		}
		if root == "" {
			cwd, _ := os.Getwd()
			root = findAncestor(cwd, isContentRoot)
		}
		if root == "" {
			return social.Source{}, errors.New("content repository not found: specify --content-root or BLOGCTL_CONTENT_ROOT")
		}
		search, err := bridge.ResolvedSearchRuntimeConfig()
		if err != nil {
			return social.Source{}, err
		}
		return social.ResolveSource(root, search.Site.Origin, src)
	}
	switch sub {
	case "channels":
		channels, err := client.Channels(ctx)
		if err != nil {
			return err
		}
		return show(channels)
	case "source":
		source, err := resolve(social.Source{Kind: *kind, Slug: *slug, Language: "en"})
		if err != nil {
			return err
		}
		return show(source)
	case "create":
		if *input == "" {
			return errors.New("--input is required")
		}
		raw, err := os.ReadFile(*input)
		if err != nil {
			return err
		}
		var incoming social.DraftInput
		if err := json.Unmarshal(raw, &incoming); err != nil {
			return err
		}
		incoming.Source, err = resolve(incoming.Source)
		if err != nil {
			return err
		}
		draft, err := social.Create(dataRoot, incoming)
		if err != nil {
			return err
		}
		return show(draft)
	case "list":
		drafts, err := social.List(dataRoot)
		if err != nil {
			return err
		}
		return show(drafts)
	case "show":
		draft, err := social.Load(dataRoot, *id)
		if err != nil {
			return err
		}
		return show(draft)
	case "stage", "reconcile", "publish":
		if *platform == "" {
			return errors.New("--platform is required")
		}
		svc := social.Service{DataRoot: dataRoot, Buffer: client}
		var draft social.Draft
		var err error
		if sub == "stage" {
			draft, err = svc.Stage(ctx, *id, *platform, *revision)
		}
		if sub == "publish" {
			draft, err = svc.Publish(ctx, *id, *platform, *revision)
		}
		if sub == "reconcile" {
			draft, err = svc.Reconcile(ctx, *id, *platform, *revision)
		}
		if err != nil {
			return err
		}
		return show(draft)

	default:
		return fmt.Errorf("unknown social command %q", sub)
	}
}

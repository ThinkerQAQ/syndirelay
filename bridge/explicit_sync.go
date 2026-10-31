package bridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	blogapp "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/app"
	blogassets "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/assets"
	blogcompiler "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/compiler"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
)

func validExplicitRemoteID(id string) bool {
	if len(id) == 0 || len(id) > 100 {
		return false
	}
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' || r == '-') {
			return false
		}
	}
	return strings.TrimSpace(id) == id
}

type explicitExecution struct {
	platform string
	results  []string
	failed   []string
}

// A single explicitly selected target is one side-effect unit. Platforms
// execute in parallel; multiple targets on one platform run sequentially.
func (s *Server) runExplicitSync(ctx context.Context, config bridgeConfig,
	req syncRequest, onEvent func(blogapp.SyncEvent)) (string, error) {
	outcomes := make(chan explicitExecution, len(req.Platforms))
	var wg sync.WaitGroup
	for _, platform := range req.Platforms {
		platform := platform
		wg.Add(1)
		go func() {
			defer wg.Done()
			execution := explicitExecution{platform: platform}
			defer func() { outcomes <- execution }()
			report := func(state, result, url, message, targetID string) {
				onEvent(blogapp.SyncEvent{Platform: platform, State: state, Result: result, URL: url, Message: message, TargetID: targetID})
			}
			report("running", "", "", "Preparing source and images", "")
			articles, err := blogcompiler.CompilePlatform(ctx, blogcompiler.CompileOptions{
				EngineRoot: config.EngineRoot, ContentRoot: config.ContentRoot,
				Publishing: config.Publishing, Node: config.ToolPaths["node"],
				Platform: platform, Articles: []string{req.Article}, DryRun: req.DryRun,
			})
			if err != nil || len(articles) != 1 {
				if err == nil {
					err = errors.New("publishing compiler returned no article")
				}
				execution.failed = append(execution.failed, err.Error())
				report("failed", "", "", err.Error(), "")
				return
			}
			if req.DryRun {
				report("completed", "dry-run", "", "No remote writes performed", "")
				return
			}
			_, err = blogassets.Prepare(ctx, articles, blogassets.Config{
				EngineRoot: config.EngineRoot, DistributionRoot: config.DistributionRoot,
				Node: config.ToolPaths["node"], MermaidWidth: config.Publishing.Compiler.Mermaid.Width,
				MermaidScale: config.Publishing.Compiler.Mermaid.Scale,
			})
			if err != nil {
				execution.failed = append(execution.failed, err.Error())
				report("failed", "", "", "asset preparation: "+err.Error(), "")
				return
			}
			session, client, err := (bridgeNativePublisher{server: s}).publisherSession(platform)
			if err != nil {
				execution.failed = append(execution.failed, err.Error())
				report("failed", "", "", err.Error(), "")
				return
			}
			input := draftInputFromCompiled(articles[0], config.ContentRoot, config.DistributionRoot, config)
			service := publisher.Service{HTTPClient: client}
			if platform == "hashnode" {
				// Hashnode creates an empty remote draft before uploading images.
				// Persist its ID immediately so partial failures can resume safely.
				service.PublicationPath = s.publicationBindingsPath()
			}
			targets := []explicitTaskTarget{{Platform: platform, State: "draft"}}
			if req.Operation == "update" || req.Operation == "publish-draft" {
				targets = nil
				for _, item := range req.Targets {
					if item.Platform == platform {
						targets = append(targets, item)
					}
				}
			}
			if len(targets) == 0 {
				err = fmt.Errorf("no selected targets for platform %s", platform)
				execution.failed = append(execution.failed, err.Error())
				report("failed", "", "", err.Error(), "")
				return
			}
			for _, target := range targets {
				var result publisher.ExplicitResult
				var writeErr error
				if platform == "medium" {
					mediumOperation := req.Operation
					if (req.Operation == "create" || req.Operation == "update") && req.ResumeMediumDraftID != "" {
						mediumOperation = "resume-medium"
						target.ID = req.ResumeMediumDraftID
						report("running", "draft-resumed", mediumOrigin+"/p/"+target.ID+"/edit",
							"Resuming saved Medium editor content without a second text write", target.ID)
					}
					result, writeErr = runMediumExplicitWithBrowser(ctx, session, client, articles[0], input,
						config.DistributionRoot, mediumOperation, target, req.PublishAfter,
						func(postID string) {
							url := mediumOrigin + "/p/" + postID + "/edit"
							report("running", "draft-allocated", url,
								"Medium draft allocated; preparing images and content", postID)
						}, mediumBrowserFlow{insert: s.mediumUploads.insert, verify: s.mediumUploads.verify, canonical: s.mediumUploads.canonical, paragraphCount: s.mediumUploads.paragraphCount, document: s.mediumUploads.document}, s.mediumUploads.upload)
				} else if platform == "hashnode" && req.Operation == "create" && !req.PublishAfter {
					var draft publisher.DraftResult
					draft, writeErr = service.CreateOrUpdateDraftInput(ctx, platform, session, config.ContentRoot, input, true)
					if writeErr == nil {
						kind := "draft-created"
						if draft.Updated {
							kind = "draft-updated"
						}
						if draft.Skipped {
							kind = "skipped"
						}
						result = publisher.ExplicitResult{ID: draft.ID, URL: draft.URL, Kind: kind}
					}
				} else {
					result, writeErr = service.RunExplicitDraft(ctx, platform, session, input, req.Operation,
						publisher.ExplicitTarget{ID: target.ID, URL: target.URL, State: target.State, UpdatedAt: target.UpdatedAt},
						req.PublishAfter)
				}
				if writeErr != nil {
					execution.failed = append(execution.failed, fmt.Sprintf("%s: %v", target.ID, writeErr))
					failedID := target.ID
					if result.ID != "" {
						failedID = result.ID
					}
					report("running", "target-failed", result.URL, writeErr.Error(), failedID)
					continue
				}
				execution.results = append(execution.results, fmt.Sprintf("%s → %s", result.ID, result.Kind))
				report("running", result.Kind, result.URL, fmt.Sprintf("Remote ID %s: %s", result.ID, result.Kind), result.ID)
			}
			if len(execution.failed) > 0 {
				report("failed", "partial-failure", "", fmt.Sprintf("%d succeeded; %d failed: %s",
					len(execution.results), len(execution.failed), strings.Join(execution.failed, "; ")), "")
			} else {
				report("completed", "explicit-"+req.Operation, "",
					fmt.Sprintf("%d remote target(s) completed", len(execution.results)), "")
			}
		}()
	}
	wg.Wait()
	close(outcomes)
	failures := []string{}
	completed := []string{}
	for item := range outcomes {
		for _, detail := range item.results {
			completed = append(completed, item.platform+": "+detail)
		}
		for _, failure := range item.failed {
			failures = append(failures, item.platform+": "+failure)
		}
	}
	output := strings.Join(completed, "\n")
	if len(failures) > 0 {
		return output, fmt.Errorf("%d publishing operation(s) failed: %s", len(failures), strings.Join(failures, "; "))
	}
	return output, nil
}

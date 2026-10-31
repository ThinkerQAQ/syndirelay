# BlogCTL commands

Run `blogctl help` to inspect your installed executable. Source dispatch is in [`cmd/main.go`](../../cmd/main.go); the CLI and UI do not expose identical workflows.

## Working directory

| Commands | Where to run |
| --- | --- |
| `help`, `install`, `uninstall`, `-mcp-stdio` | No engine checkout required (`-mcp-stdio` uses the installed local Bridge) |
| `social ...` | Uses local configuration and explicit content inputs; see [Buffer guide](../guide/social-publishing.md) |
| `sync`, `toutiao` | Content checkout, or explicit workspace environment variables |
| `doctor`, `dev`, `preview`, `build`, `check`, `test`, `site`, `search`, `ai-search`, `diagrams` | Inside the Inkerio engine checkout |

For `sync`, `BLOG_CONTENT_ROOT` and `BLOGCTL_ENGINE_ROOT` override workspace discovery. Otherwise the CLI looks for the current content checkout and a sibling engine named `ThinkerQAQ.github.io`; from the engine it can find a sibling `blog-content`. Settings paths saved by the UI are not a substitute for these CLI discovery rules. `BLOGCTL_DATA_DIR` separately chooses persistent configuration/state and must be absolute.

## Site and maintenance commands

| Command | Operation |
| --- | --- |
| `blogctl doctor` | Inspect Node/npm/Git and OS |
| `blogctl dev` | Run the npm demo development flow; reassembles the public template |
| `blogctl preview` | Serve existing `dist/` |
| `blogctl check` / `test` | Run npm check / full test flow |
| `blogctl build` | Build currently assembled site content |
| `blogctl site assemble --content-root PATH` | Replace generated content from a selected checkout |
| `blogctl site build [--content-root PATH]` | Optionally assemble, build site and URL inventory, then Pagefind |
| `blogctl diagrams [plantuml\|drawio]` | Render all diagrams or the selected supported subcommand |
| `blogctl search build\|inventory\|submit\|audit\|notify` | Build/read inventories and perform provider tasks |
| `blogctl ai-search prepare\|sync\|verify` | Prepare and synchronize optional AI Search content |

Search submission requires explicit providers and URL scope (`--all` or `--urls-file` for IndexNow/Baidu). `search submit` has no dry-run mode and can send real requests. Check configuration and command-specific options in [`cmd/search.go`](../../cmd/search.go) before a live submission. `inventory --json` reads local built output; it does not submit URLs.

The engineering-only `go run ./tools/blogctl/cmd/maint verify|build|package` runs from the engine root. `verify` installs Node dependencies and runs the BlogCTL test set; `package` writes binaries, Extension ZIP and checksums without publishing a release. See [engine commands](../../../../docs/reference/commands.md).

## Article sync

```bash
blogctl sync --article sample-article --platforms devto --dry-run
```

| Flag | Meaning |
| --- | --- |
| `--article SLUG` | Root article slug; repeat for specific articles |
| `--all` | All published articles; mutually exclusive with `--article` |
| `--platforms p1,p2` | Required registered platform IDs |
| `--dry-run` | Compile locally; no remote article writes or publishing-asset upload |
| `--changed` | Use changed-content filtering in the sync workflow |
| `--draft` | Legacy sync draft option; not a remote target selector |

Dry-run does not need a browser connection, but may install Node dependencies and persist workspace roots locally. It returns a success/failure result and does not promise a rendered preview file. Live `sync` uses the Bridge's `draft` workflow; the explicit Create/Update target controls documented for the UI are not CLI `sync` flags. Use the UI to choose a specific remote update target.

Registered IDs are in [`platform/capabilities.go`](../../platform/capabilities.go). Registration alone does not make every action available: Toutiao supports account detection, drafts and explicit publication; OSChina/Hashnode explicit publishing is disabled.

## Social and experimental commands

`blogctl social channels|source|create|list|show|stage|reconcile|publish` manages local drafts and Buffer operations. [The guide](../guide/social-publishing.md) defines inputs and revision checks.

`toutiao probe --har FILE` is a developer diagnostic, not the recommended publication path. HAR files can contain account credentials; keep captures out of the repository and use the normal supported UI for everyday work.

## MCP / ChatGPT integration

`blogctl -mcp-stdio` starts a **STDIO-only** MCP server that delegates to the same local Bridge used by the Extension and Web Console. No repository checkout is needed when running the installed binary. Use the same Windows instance as the logged-in Edge extension; WSL/Linux processes have separate local Bridges. Available tools include article reads, publishing previews, explicitly targeted draft creation/update/publication, and task lookup. See [MCP integration](../MCP.md).

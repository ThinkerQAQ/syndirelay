# SyndiRelay CLI reference

The supported entry point is `syndirelay` (or `syndirelay.exe` on Windows), built from this repository's `./cmd`. Run `syndirelay help` to confirm your installed version. Do **not** invoke `go run ./tools/blogctl/cmd` in the retired website repository.

| Command | Description | Side effects |
| --- | --- | --- |
| `syndirelay help` | Print current command overview | Read-only |
| `syndirelay doctor` | Check local Node/npm/Git and OS | Read-only |
| `syndirelay compile --file FILE --platform ID` | Compile standalone Markdown for one platform | Local output only, no remote publishing |
| `syndirelay sync --article SLUG --platforms ID,... --dry-run` | Compile selected content-repository article | **No** remote write and no local publishing-state mutation |
| `syndirelay sync --article SLUG --platforms ID,...` | Execute supported publisher workflow | **Remote side effects; authorization required** |
| `syndirelay mcp-stdio` | Run limited local STDIO MCP server | Connects to local Bridge for authorized tools |
| `syndirelay install` / `uninstall` | Register/remove local Native Messaging Host | Local OS registration changes |
| `syndirelay social ...` | Local social metadata and optional explicitly selected Buffer workflows | Depends on subcommand; `publish` can write remotely |

Options can vary across adapters; `--platform` names refer to formatting, not guaranteed official publishing API support. Generic single-file `compile` needs no Inkerio/Astro installation. Optional `--canonical-url` and `--asset-base-url` must be set to your own content origin; no developer site identity is inferred.

## Local data and workspace discovery

- Set `SYNDIRELAY_HOME` to the installed tool/renderer root if the process is not running from a checkout.
- Set **absolute** `SYNDIRELAY_DATA_DIR` to isolate private records from old BlogCTL, or use the supported local default. The older `BLOGCTL_DATA_DIR` and `BLOGCTL_ENGINE_ROOT` identifiers are accepted only for migration compatibility; do not use them for new installations.
- Set `BLOG_CONTENT_ROOT` or explicit `--content-root` for repository-aware publishing. Saved UI paths and CLI environment may differ; check which process owns the current Bridge.
- The standalone publisher's `npm ci` installs only its own runtime renderers, not Inkerio site dependencies.

## Legacy site/search commands

`site`, `build`, `dev`, `preview`, `diagrams`, `search` and `ai-search` remain in some CLI dispatch paths for backward compatibility with site CI. **They are not the standalone publishing product's installation or website build instructions.** Use [Inkerio](https://github.com/ThinkerQAQ/inkerio) (`node scripts/assemble-content.mjs`, `npm run build`) to construct a blog and use a site-owned deployment workflow for optional indexing notifications.

For MCP operations and browser connection, see [MCP](../MCP.md). For configuration, see [workspace](../how-to/configure-workspace.md). For security and release limits, see [publication readiness](../compliance/PUBLICATION_READINESS.md).

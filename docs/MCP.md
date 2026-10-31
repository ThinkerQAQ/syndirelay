# BlogCTL MCP

BlogCTL exposes its existing local Bridge through a minimal Go MCP server. It does **not** implement a second publishing pipeline, store browser cookies in ChatGPT, create a separate database, or add a Skill.

## Start

Run the *installed* BlogCTL binary on the machine that owns your signed-in Edge/Chrome profile and the registered BlogCTL Extension:

```powershell
C:\software\Coding\blogctl\blogctl-windows-amd64.exe -mcp-stdio
```

For development, from the engine checkout:

```sh
go run ./tools/blogctl/cmd -mcp-stdio
```

The process communicates only over MCP **STDIO**; it does not open an MCP HTTP listener. Connect a dedicated **Secure MCP Tunnel** to that STDIO process, as with Photo Memory Local. Do not reuse the private Photo Memory or Wardrobe tunnel; each app has its own binding. Never commit tunnel IDs, API keys or the local `Data/` directory.

The MCP entrypoint reuses the local `bridge.json` state and starts the standard Bridge when necessary. The Bridge must run in the same OS/runtime as the browser extension. Running an unrelated Linux/WSL Bridge does not grant access to your Windows Edge cookies. The existing Browser Extension, not the MCP, collects authenticated sessions in the background.

## Tools

| Tool | What it does |
| --- | --- |
| `list_articles` | List local published articles and English-mirror status |
| `get_article` | Read local Markdown by slug/language |
| `list_platforms` | List registered platform capabilities |
| `get_publications` | Read saved distribution records |
| `get_article_links` | Read an article's saved platform links |
| `preview_publish` | Create a local-only dry-run task (no remote publish) |
| `create_draft` | Explicitly create new remote draft(s) |
| `create_and_publish` | Explicitly create and publish on supported platforms |
| `update_article` | Update selected remote target ID(s) |
| `update_and_publish` | Update and publish selected supported draft ID(s) |
| `publish_draft` | Publish existing selected draft ID(s) |
| `get_task` | Get publishing task result and remote links |
| `list_tasks` | List saved task history |

The publishing tools use the same `/v1/sync/jobs` contract and `operation=create|update|publish-draft` as the Extension, with the same platform and target checks. `create` deliberately creates a new draft; remote detection is **optional in the existing BlogCTL workflow**, not an automatic deduplication guarantee. For an existing post use an explicit `update_article` target, never create again blindly. `publish_draft` requires a remote draft ID; unsupported platform operations are rejected by the original Bridge.

### Session handling

The Bridge keeps browser sessions in memory for 10 minutes. MCP write tools first call the authenticated local `/v1/mcp/session-refresh` endpoint. The Bridge queues expired or missing platform sessions; the installed Extension's MV3 background alarm checks pending requests every 30 seconds and calls its **existing** `syncPlatformSession`. The MCP polls only the session's authenticated status, **never Cookie values**, before enqueueing the original publish task. The Extension does not need its side panel opened. Keep Edge/Chrome and the extension enabled and signed in; an absent extension causes a timeout without creating a task. DEV.to uses its existing API-key flow and does not require this Cookie refresh.

Session refresh is best-effort: a provider may still reject an expired remote login. Inspect the saved task and remote platform before taking further action. Background alarms are not a guarantee of immediate browser wakeup, especially when Edge is closed.

### Retries and task state

For side-effecting MCP tools provide a unique `request_id` (8–128 ASCII letters, digits, `_`, `.`, `-`). Retry **the same intended request** with exactly the same ID. Within retained Bridge job history, duplicates return the same task ID; different payloads with a reused ID are rejected. The request ID is persisted with the existing `jobs.json` task payload; no additional database is added. History is bounded by BlogCTL's existing retention/pruning, so do not assume an arbitrarily old request ID is forever deduplicated.

If a request times out after submission, first use `get_task` / `list_tasks` and check the remote platform. Never generate a *new* request ID merely to retry an ambiguous Create/Publish. Normal BlogCTL rules for failed writes and platform-specific recovery still apply.

### Security

- The MCP is **STDIO only**. Do not expose the full local Bridge or Console over a public tunnel.
- The MCP is bound to the local Windows BlogCTL process; the Bridge token stays in local requests and is never returned to a tool caller.
- Only named operations are exposed; there is no arbitrary file access, shell execution, raw HTTP proxy, Cookie export, or config dump.
- Reading article Markdown explicitly through MCP means that returned text is sent to the connected AI service; Cookie values and platform credentials are not.
- No new TOML write/publish permission switches or redundant Skill are introduced. The existing BlogCTL business checks continue to apply, and tool calls remain subject to the ChatGPT client's action-confirmation behavior.

## Local verification

```sh
cd tools/blogctl
go test ./cmd ./bridge
go test ./...
node --check extension/background.js
node --test extension/tests/*.test.mjs
```

The added unit tests use a simulated Bridge and Extension session. They cannot prove a live Edge login or remote publication; those require a separate, explicitly authorized end-to-end acceptance test.

## Private image upload to R2

The `upload_image_to_r2` MCP tool is available in BlogCTL v0.1.190. It opens only an explicitly selected local JPEG/PNG/WebP inside a configured image root, auto-orients pixels, strips EXIF/GPS/XMP/IPTC/ICC with Sharp, verifies clean output, and calls the **existing** R2 uploader. The original is not modified and is never sent to ChatGPT or R2. Successful output includes `url`, `markdown`, `sha256`, `object_key`, `version`, `blogctl_version`, size and dimensions. `dry_run=true` checks locally without uploading.

Example MCP input:

```json
{"local_path":"C:\\Pictures\\sunset.jpg","format":"webp","alt":"Sunset","dry_run":true}
```

Add only the required image folders to your **local** `Data/blogctl.toml` (top-level, not the R2 section):

```toml
image_upload_roots = ['C:\Pictures']
```

The existing `content_root` is also allowed. Paths must be absolute and resolve inside an authorized directory; symlink escapes, invalid formats, and oversized images fail closed. Source paths and credentials never appear in tool results. Continue using existing `[publishing.assets.r2]` for credentials. The sanitizer requires Node.js and the existing Inkerio `sharp` dependency installed under `engine_root` (e.g. `npm ci` from the engine checkout). The executable alone is insufficient. As with other MCP tools, the Windows Bridge must be updated to the same version as the MCP binary.

Versioned objects use `images/v1/<first-32-hex-of-cleaned-SHA256>.<format>`. The full SHA-256 and processor version are returned. Changing the privacy processing algorithm should increment `v1`. The cleaned image contains no application EXIF version tag. Removing embedded metadata **cannot** remove visible faces, addresses, license plates, or other private content visible in pixels. Existing publishing R2 fallback behavior is unchanged in this phase.

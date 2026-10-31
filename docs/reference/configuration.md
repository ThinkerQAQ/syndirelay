# BlogCTL configuration

Use Settings in the Extension or Console to edit `blogctl.toml`. Stop the Bridge before manual file edits so a UI save does not overwrite them.

## Data directories

| OS | Default directory |
| --- | --- |
| Windows | `Data/` beside the executable |
| Linux | Usually `~/.config/BlogCTL/`, following `os.UserConfigDir()` |
| macOS | Usually `~/Library/Application Support/BlogCTL/` |
| Override | Absolute `BLOGCTL_DATA_DIR`, set before process startup |

`jobs.json`, `publications.json`, `distribution/`, logs and `social/` are runtime state. They may contain private data. Back up the directory before moving an installation; an environment override does not migrate files.

Windows and WSL have separate defaults and path syntax. Shared social drafts need both processes to address the same Data directory, using Windows and `/mnt/...` paths respectively. Pass an explicit Linux `--content-root` to WSL social commands if saved paths are Windows paths.

## Common TOML settings

| Key | Meaning |
| --- | --- |
| `engine_root`, `content_root` | Checkout roots accessible to the executable |
| `ui_locale` | `auto` (default), `zh-CN`, or `en`; interface only |
| `log_level`, `log_directory` | Default level `info`, optional log output path |
| `proxy_enabled`, `proxy_host`, `proxy_port` | Enable and select an HTTP proxy |
| `tool_paths` | Map of executable path overrides |
| `distribution_root`, `publication_bindings_path` | Optional runtime output/history locations |
| `devto_api_key` | DEV.to credential |
| `indexnow_endpoint`, `indexnow_key`, `indexnow_key_location` | IndexNow endpoint and ownership key |
| `baidu_token` | Baidu submission credential |
| `google_search_console_service_json` | Google service-account JSON setting |

Partial example; save through Settings to preserve initialized defaults:

```toml
engine_root = "/home/user/blog/ThinkerQAQ.github.io"
content_root = "/home/user/blog/blog-content"
ui_locale = "en"
log_level = "info"
proxy_enabled = false

[publishing.platforms.devto]
language = "en"
changed_only = false
```

For Windows paths, TOML literal strings avoid backslash escaping: `engine_root = 'C:\code\ThinkerQAQ.github.io'`.

## The site to index

Edit **Settings → Search engines → Indexing site and URLs**:

```toml
[search]
site_origin = "https://example.com"
url_list_url = "https://example.com/sitemap-all.txt"
sitemap_url = "https://example.com/sitemap-index.xml"
fingerprint_url = "https://example.com/sitemap-inventory.json"
google_property = "sc-domain:example.com"
```

Replace the example origin/property with your deployed site. Resource URLs must belong to the site and actually exist. Set `fingerprint_url = ""` to disable fingerprint fetching; incremental submissions then conservatively treat existing URLs as changed.

Missing required fields, unknown search keys, foreign URLs and incompatible Google properties are rejected. CLI `--site-url` must match `google_property`. Older configurations without `[search]` are migrated to the previous site defaults; review them before submitting your own site. The obsolete `baidu_site` field is removed during migration.

Running indexing tasks block site edits. Queued tasks retain their site identity, and domain migrations reset active inventories rather than generating foreign-domain deletion requests. This configuration does not deploy the website or change DNS.

## Publishing

| Section | Fields / defaults |
| --- | --- |
| `[publishing.platforms.PLATFORM]` | `language`, `changed_only` |
| `.footer` | `enabled`, `template`; placeholders `{site}`, `{url}`, `{title}` |
| `.canonical` | `mode`: `native`, `footer`, or `none` |
| `.tracking` | `enabled`, `source`, `medium`, `campaign` |
| `[publishing.compiler.mermaid]` | `format` (`png`), `width` (1200), `scale` (2) |
| `[publishing.assets]` | `store` (`r2`) |
| `[publishing.assets.r2]` | `bucket`, `public_base_url`, `account_id`, `endpoint`, `access_key_id`, `secret_access_key` |

When a platform's `.footer.enabled` is true, BlogCTL preserves its existing `template` (including UTM source links) and **appends a localized copyright notice plus the untracked original article URL**. The notice and Inkerio's page notice read the same content-owned `src/content/site/copyright.json` under the configured `content_root`; editing it takes effect on the next compilation without rebuilding BlogCTL. Older content trees without this file fall back to the embedded legacy policy; invalid present files fail compilation. Older `blogctl.toml` files keep their existing footer template and gain the new notice automatically. Turning off `.footer.enabled` disables both the traditional footer and the copyright notice. Native Canonical URL settings are unchanged. Existing remote posts are not rewritten until an explicitly requested Create/Update.

Dotted rows are nested under the platform section, for example `[publishing.platforms.devto.footer]`. DEV.to, Medium and Hashnode default to English and native canonical mode; other registered platforms use their default language and footer mode. Article language selects an existing source translation; it does not translate text.

Configure your own R2 bucket and credentials before publishing assets. The author's built-in public asset base grants no write access to that bucket.

**Domain limitation:** the publishing compiler still uses `SiteOrigin` in [compiler/markdown.go](../../compiler/markdown.go) for canonical and root-relative links. Changing Astro `site` or `[search]` does not change it. Adapt and verify publishing links before using a fork to syndicate your own content.

## Buffer

```toml
[social.buffer]
api_key = "YOUR_PERSONAL_BUFFER_API_KEY"
```

The Bridge masks the saved key in API responses. `BUFFER_API_KEY` is a fallback when it is empty; Bash startup variables may not reach a Windows or noninteractive Bridge. See [Buffer publishing](../guide/social-publishing.md).

Implementation: [Bridge config](../../bridge/config.go), [publishing config](../../publishing/config.go). Keep real configuration, credentials and Data files out of Git.

## MCP and Secure Tunnel

MCP does not introduce a new `[mcp]` permission section or modify your publishing settings. Use `blogctl -mcp-stdio` on the machine running the signed-in browser extension. Configure the Secure MCP Tunnel in a **private** tunnel-client profile (tunnel ID, API-key reference, and command path); never commit the profile, credential or the installed `Data/` directory. The existing Bridge still reads `blogctl.toml`. For the full setup and session refresh behavior, see [MCP integration](../MCP.md).

### Local image upload authorization

`upload_image_to_r2` only accepts images under `content_root` or local top-level `image_upload_roots = ['C:\Pictures']`. Do not permit broad system roots. These roots are not returned through the Bridge configuration API. Node.js, the Inkerio `engine_root` checkout and its `sharp` dependency must be installed. R2 uses the existing `[publishing.assets.r2]` settings; no credentials are added to MCP.

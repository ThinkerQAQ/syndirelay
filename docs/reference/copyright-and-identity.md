# Site identity, copyright, and publishing defaults

**SyndiRelay does not own your content.** The application includes **no default developer domain, author, copyright text, or R2 public bucket**. The MIT LICENSE in the source repository identifies the copyright holder of SyndiRelay's source code; it does **not** license users' articles or set their content attribution.

## Recommended configuration

Edit `blogctl.toml` through **Settings → Platform configuration → Source & Copyright** or as a private local file. Preserve your existing other TOML sections when migrating:

```toml
[publishing.site]
origin = "https://your-site.example"
name_zh = "我的站点"
name_en = "My Writing Site"
articles_zh = "/articles/"
articles_en = "/en/articles/"
legacy_origins = [] # optional previously used domains for DEV.to matching

[publishing.copyright]
mode = "auto"

[publishing.copyright.zh]
syndication = "© 作者。本文原创文字按作者指定的许可协议分享。"
source = "原文"

[publishing.copyright.en]
syndication = "© The Author. Original text is shared under the stated license."
source = "Original article"

[publishing.assets]
store = "none" # set to "r2" and configure your own bucket when uploading assets
```

### Copyright precedence

| Mode | Result |
|---|---|
| `auto` (default) | SyndiRelay's explicit text for article locale → `content_root/src/content/site/copyright.json` → no copyright notice |
| `config` | Only SyndiRelay's explicit locale text. Both locale texts required for configuration. |
| `content` | Only content-owned `copyright.json`; missing or invalid content file is an error. |
| `none` | Never append a copyright notice. |

An invalid existing content file fails closed; a **missing** file does not silently load any old developer text. With no explicit copyright and no content policy, the attribution footer may still link to the explicitly configured source site, but no license or legal claim is invented. Disable the per-platform footer to remove even that attribution paragraph.

The configured `site.origin` is required for blog-content article compilation. For a generic standalone `syndirelay compile --file article.md --platform devto`, use optional `--canonical-url` instead: it does not require a site config or produce a copyright footer.

`{site}` in the footer template resolves to the configured localized site name. If no display name is supplied it uses the hostname from the **configured** site origin. `{url}` uses that origin with the configured article URL path, plus the platform's optional UTM parameters. A Canonical URL is always the untracked URL. Relative cover/media paths resolve against the configured origin, never the software author's website.

## Safe migration from BlogCTL

1. Back up `Data/blogctl.toml`, `jobs.json`, `publications.json` and `distribution/` first.
2. Set your own `[publishing.site]` and `[publishing.copyright]`; do not copy the original author's configuration into a public example or source repository.
3. If you formerly used a custom R2 CDN, keep your own `[publishing.assets.r2]` and `store = "r2"`. A new installation defaults to `store = "none"`; no built-in third-party public R2 endpoint exists.
4. Keep original platform session data, browser extension identity and token protocol for older migrated clients until migration has been explicitly completed. Changing protocol identities is a separate migration, not a global string-replacement task.
5. Use **Dry Run** to verify all 11 platform outputs. Configuration changes never update already created remote drafts or articles; remote updates require explicit action.

## Legacy subsystems

The old search and AI Search modules are temporarily retained for compatibility, but their new-install placeholder is the non-routable reserved `example.invalid` domain. Set a real authorized search-site origin and property before using indexing tools. They are not generic Markdown publishing prerequisites and will eventually be separated from SyndiRelay.

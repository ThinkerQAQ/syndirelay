# SyndiRelay

[简体中文](README_ZH.md) · [Source migration record](docs/migration/SOURCE_COMMIT.txt) · [MIT License](LICENSE)

SyndiRelay is a **standalone, cross-platform Markdown content syndication and publishing application**, extracted from BlogCTL. It preserves the complete 11-platform compilation, publication, draft, image pipeline, local browser Bridge/Extension, and MCP source code while removing the dependency on the Inkerio/Astro site engine.

**Status: private migration and compatibility audit, not yet a public release.** Native code and most platform-specific behavior have been imported; external platform publishing must still pass authorization, safety, runtime packaging and environment migration reviews before a public binary or extension is published.

## Independent Markdown compilation (preview; no network publishing)

Requires Go 1.24+ and Node.js 22+. There is **no Inkerio installation requirement**.

```bash
npm ci
go build -o syndirelay ./cmd
./syndirelay compile --file ./example.md --platform devto
./syndirelay compile --file ./example.md --platform medium --asset-base-url https://your-cdn.example/
```

The Markdown file may have YAML frontmatter or just `# Title` and body text. Optional `--canonical-url https://your-domain.example/your-article/` is never inferred from ThinkerQAQ. Metadata-free documents generate titles/descriptions locally. Media that requires rendering (Mermaid, PlantUML, Medium table images) requires your explicit `--asset-base-url`; a preview does not upload assets or publish.

For installation bundles, ship the Go binary alongside `renderers/`, `scripts/plantuml/`, `package.json` and `package-lock.json`; run `npm ci` in that directory and set `SYNDIRELAY_HOME` to its absolute path. The browser Extension uses an existing authorized browser session only when a platform operation is explicitly requested; this is separate from `compile`.

## Legacy-compatible compilation

The original `blog-content` adapter and platform compilers remain preserved. Migration validation compiled one fixture on **all 11 original platforms** and compared the complete JSON result **byte for byte** against the original BlogCTL commit. All 11 matched, including Medium's specialized payload, HTML, content hash and attribution rules. Existing user content and publishing database are not copied.

## Optional impact tracking

[Connect Reverfolio](docs/how-to/reverfolio.md) to register confirmed public publications through a local durable outbox. It is disabled by default, uses your own site/catalog settings and transfers no article body or credentials. Publishing and impact tracking remain independently deployable.

## Architecture

- Source readers: standalone Markdown file, original `blog-content` layout. Other adapters can be added without changing the platform compilers.
- Publishing core: `compiler/`, `platform/`, `publishing/`, `publisher/`, `assets/` and `renderers/`.
- Local automation: `bridge/`, `extension/`, `ui/`, MCP entry in `cmd/`.
- Search/AI Search and old site build compatibility: temporarily retained until the old production deployment wrapper no longer imports them; not part of the long-term distribution core.

## Security and copyright

Code migration does not authorize use of unofficial platform APIs, third-party photos or other people's text. Session/Cookie flows require an explicit action and platform-specific permissions. Old `tls-client`/`fhttp` dependencies are absent in the current Go module; old already-published binaries are unaffected. This private repo is intentionally **not** public until a detailed per-platform authorization review, third-party license collection and cross-platform install/release verification are complete.

Never commit users' content, cookies, tokens, local publication databases or release secrets.

# SyndiRelay

[简体中文](README_ZH.md) · [Documentation](docs/index.md) · [Quick start](docs/quick-start.md) · [MIT License](LICENSE) · [Third-party notices](THIRD_PARTY_NOTICES.md)

**SyndiRelay is an independent Markdown compilation and cross-platform syndication toolkit.** It started as BlogCTL, but no longer requires an Inkerio/Astro engine or a ThinkerQAQ website checkout. It supports standalone Markdown input, optional repository-aware publishing, a local browser Bridge and Extension, STDIO MCP tools, and optional publication-impact handoff to Reverfolio.

**Availability:** This repository is **private during the public-source audit**. There is no approved SyndiRelay binary/extension Release yet. Existing `ThinkerQAQ.github.io` BlogCTL downloads are *historical builds, not SyndiRelay installers*. Open-sourcing the source and distributing precompiled binaries are separate decisions; see [public-source and binary release gates](docs/compliance/PUBLICATION_READINESS.md).

## What it does

| Capability | Requirements | Safety boundary |
| --- | --- | --- |
| Compile Markdown for 11 platform formats | Go and Node.js; local file | Local preview, **no remote writes** |
| Preview publishing from a content repository | Explicit content root, site identity and copyright policy | `--dry-run` must not mutate remote or local publishing state |
| Create/update/publish external articles | Per-platform account authorization, supported operation, local Bridge/Extension where required | Explicit target and user action; support and policy vary by platform |
| Local Console and browser Extension | Matching native executable and authorized browser session | Local bridge on `127.0.0.1:32146`; keep credentials and cookies local |
| STDIO MCP | Local executable and Bridge | Named tools; no public unrestricted Bridge endpoint |
| Optional impact tracking | Separate Reverfolio configuration | Off by default; separate data and lifecycle |
| Search/AI Search integration | Optional site-owned CI and provider credentials | Private server-side credentials, not a publishing dependency |

The 11 **compiler formats** are `51cto`, `cnblogs`, `csdn`, `devto`, `hashnode`, `juejin`, `medium`, `oschina`, `segmentfault`, `toutiao` and `zhihu`. **A compiler format is not a guarantee of official API access or live publishing capability.** In particular, browser-authenticated or undocumented platform endpoints must not be used without appropriate authorization.

## Compile a standalone article

Prerequisites: Go **1.24+**, Node.js **22+**, npm and Git. Authorized source access is currently required.

```sh
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
go build -o syndirelay ./cmd
printf '# Example article\n\nMy original content.\n' > example.md
./syndirelay compile --file ./example.md --platform devto
```

The example compiles locally. It does not sign into a publishing service, upload media or create a draft. For rendered diagrams or external media, supply your own explicit `--asset-base-url https://your-cdn.example/`; for repository-aware syndication, configure your own canonical site origin and copyright policy. No developer site identity is inserted by default.

For Windows PowerShell, build with `go build -o syndirelay.exe ./cmd`, write a local `.md` file and run `./syndirelay.exe compile --file ./example.md --platform devto`.

## Architecture and distribution

- `compiler/`, `publisher/`, `publishing/`, `platform/`, `renderers/`: content compilation, operation validation and platform-specific adapters.
- `bridge/`, `extension/`, `ui/`, `cmd/`: local authenticated workflows, browser integration and MCP. Never expose the whole Bridge to the Internet.
- `search/`, `aisearch/`, legacy `site` commands: backward-compatible optional CI integrations. The **Inkerio engine** builds websites; a site-specific deployment wrapper orchestrates optional search tasks.
- `Data/` and `SYNDIRELAY_DATA_DIR`: private local state. Never commit or package cookies, credentials, databases or historical publication bindings.
- Optional [Reverfolio](docs/how-to/reverfolio.md): independently operated impact tracking; no default transfer of article bodies or tokens.

[Inkerio](https://github.com/ThinkerQAQ/inkerio) owns the reusable static blog engine; [inkerio-content-template](https://github.com/ThinkerQAQ/inkerio-content-template) provides public example content; the [ThinkerQAQ website wrapper](https://github.com/ThinkerQAQ/ThinkerQAQ.github.io) manages one site's public identity and deployment. None is a runtime prerequisite for standalone SyndiRelay compilation.

## Develop and validate

```sh
npm ci
go test ./...
npm run test:renderers
npm run test:extension
```

CI runs on **Windows, macOS and Linux**, including native builds and tests. Build success is not a guarantee that a real browser, account publishing, packaging license notices or every operating-system installation was validated. Binary/extension Releases remain gated by third-party license evidence, external platform authorization and installation acceptance tests. Review [compliance](docs/compliance/PUBLICATION_READINESS.md) before publishing anything.

**License:** Original SyndiRelay source is MIT; this does not relicense the runtime dependencies, upstream assets, users' writing/photos or platform services. SyndiRelay is not affiliated with or endorsed by third-party publishing platforms. Use it only with rights to the content and account operations involved.

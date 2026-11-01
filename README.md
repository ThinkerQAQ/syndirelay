# SyndiRelay

[简体中文](README_ZH.md) · [MIT License](LICENSE) · [Third-party notices](THIRD_PARTY_NOTICES.md)

**SyndiRelay is an independent Markdown compilation and cross-platform syndication toolkit.** It started as BlogCTL, but no longer requires an Inkerio/Astro engine or a ThinkerQAQ website checkout. It supports standalone Markdown input, optional repository-aware publishing, a local browser Bridge and Extension, STDIO MCP tools, and optional publication-impact handoff to Reverfolio.

**Availability:** The source is publicly available under MIT; no SyndiRelay binary or browser Extension Release has been approved. The historical BlogCTL website Releases are retired. Source publication and redistributable binaries have separate gates; see the public-source audit and binary-release checklist.

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

Prerequisites: Go **1.24+**, Node.js **22+**, npm and Git.

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
- Optional Reverfolio: independently operated impact tracking; no default transfer of article bodies or tokens.

[Inkerio](https://github.com/ThinkerQAQ/inkerio) owns the reusable static blog engine; [inkerio-content-template](https://github.com/ThinkerQAQ/inkerio-content-template) provides public example content; the [ThinkerQAQ website wrapper](https://github.com/ThinkerQAQ/ThinkerQAQ.github.io) manages one site's public identity and deployment. None is a runtime prerequisite for standalone SyndiRelay compilation.

For ChatGPT, use the **native Windows PowerShell or Linux/macOS Secure MCP Tunnel setup** explained below. The Tunnel client and SyndiRelay run natively on each platform; **WSL is optional, not a Windows requirement**. An existing authorized Tunnel credential can be reused without generating a new per-app key. A locally working MCP and a connected ChatGPT plugin are separate states.

## Optional Bridge, browser Extension and ChatGPT MCP

Build the executable on the **same operating system** as your signed-in browser (Windows: PowerShell with `go build -o syndirelay.exe ./cmd`; WSL is not required). Run `./syndirelay install` (Windows: `.\syndirelay.exe install`) to register Native Messaging. In Edge/Chrome, load the repository's `extension/` as an unpacked extension. Check the Bridge at `127.0.0.1:32146` (**not** the old BlogCTL port 32145). Runtime secrets, browser sessions and `Data/` stay local; the Bridge is not a public API.

Start a local STDIO MCP server:

```sh
./syndirelay mcp-stdio
# Windows: .\syndirelay.exe mcp-stdio
```

For ChatGPT, install the official [tunnel-client](https://github.com/openai/tunnel-client/releases/latest) for your operating system. Use your **existing authorized private runtime credential**; no new one-time API key is needed for each application. The Tunnel ID identifies the connection but is not itself a secret.

```powershell
# Windows PowerShell
.\scripts\connect-chatgpt-mcp.ps1 -TunnelId tunnel_YOUR_TUNNEL_ID
.\scripts\connect-chatgpt-mcp.ps1 -TunnelId tunnel_YOUR_TUNNEL_ID -Status
```

```sh
# Linux/macOS: native tools running on the same host
./scripts/connect-chatgpt-mcp.sh tunnel_YOUR_TUNNEL_ID
./scripts/connect-chatgpt-mcp.sh tunnel_YOUR_TUNNEL_ID --status
```

Add a custom MCP Tunnel server in ChatGPT and verify read-only operations first. **Native MCP running, Tunnel ready, and ChatGPT tools callable are three separate checks.** Never tunnel the full Bridge HTTP service or publish cookies, tokens or private runtime credentials.

For a local content workspace, configure `content_root` and `[publishing.site].origin` in your private `Data/blogctl.toml`. In automatic copyright mode, explicit SyndiRelay TOML copyright text takes priority over content-owned `src/content/site/copyright.json`. Preview / Dry Run before any explicitly authorized remote create, update or publish operation.

## Optional: use the same content repository as Inkerio

If your site was created from the [Inkerio Content Template](https://github.com/ThinkerQAQ/inkerio-content-template), you **already have** a content repository. Point SyndiRelay's local workspace `content_root` at that checkout and set `[publishing.site].origin` to your own public site URL. You do not need a second `blog-content` repository or an Inkerio runtime to use SyndiRelay.

For example, the same `alice.github.io` repository can power GitHub Pages through Inkerio and supply Markdown to SyndiRelay for explicitly authorized off-site publishing. Site identity and copyright remain user-owned: with `mode = "auto"`, an explicitly configured local copyright policy takes precedence; otherwise SyndiRelay can read `src/content/site/copyright.json`. The independent [Inkerio README](https://github.com/ThinkerQAQ/inkerio) explains the complete blog-creation workflow.

## Develop and validate

```sh
npm ci
go test ./...
npm run test:renderers
npm run test:extension
```

CI runs on **Windows, macOS and Linux**, including native builds and tests. Build success is not a guarantee that a real browser, account publishing, packaging license notices or every operating-system installation was validated. Binary/extension Releases remain gated by third-party license evidence, external platform authorization and installation acceptance tests. Before releasing binaries or the extension, audit third-party licenses, account permissions, native installation and real platform acceptance; CI compilation alone does not establish distribution readiness.

**License:** Original SyndiRelay source is MIT; this does not relicense the runtime dependencies, upstream assets, users' writing/photos or platform services. SyndiRelay is not affiliated with or endorsed by third-party publishing platforms. Use it only with rights to the content and account operations involved.

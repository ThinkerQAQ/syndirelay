# BlogCTL

[简体中文](README_ZH.md) · [Documentation](docs/index.md) · [Releases](https://github.com/ThinkerQAQ/ThinkerQAQ.github.io/releases/latest)

BlogCTL is Inkerio's companion Go application. It assembles content, compiles articles for publishing platforms, submits URLs to search engines and runs the local Bridge used by the browser Extension and Web Console.

The Extension accesses your logged-in browser sessions; the Bridge runs local jobs. Platform support varies: creating a draft, editing a draft and updating a published article are separate capabilities. **Browser sessions do not grant permission to automate unpublished platform APIs.** Medium has stopped issuing new integration tokens and restricts nonpublic API / scripted access. Use Medium’s official website import or manual publishing unless you have specific platform authorization; a successful HTTP request does not imply permission. Standard Go HTTP transport is used rather than TLS fingerprint spoofing.

## First use

From an engine checkout:

```bash
go run ./tools/blogctl/cmd help
go run ./tools/blogctl/cmd doctor
```

For a desktop installation, download the **OS/architecture-specific ZIP bundle** (executable + MIT LICENSE) and the separate Extension ZIP from Releases, and verify their hashes using `SHA256SUMS`. Standalone executables remain available for existing users. Extract them into persistent directories, run the executable's `install` command and load the unpacked Extension in Edge or Chrome. On Windows:

```powershell
& C:\software\Coding\blogctl\blogctl-windows-amd64.exe install
```

Keep the adjacent `Data/` directory during upgrades. See [Quick Start](docs/quick-start.md) for storage locations on other systems and [new-machine setup](docs/tutorial/first-run-on-new-machine.md) for screenshots.

## Publishing

Select a local article and platforms in the Console. Existing-article detection is optional and runs only when requested. A match removes that platform from batch creation; detection failure does not prevent creation. Review remote state before retrying a failed creation.

Updating requires an explicit remote article or draft ID. Medium published-article edits are saved in its editor and require a manual publish action. See [workflows](docs/guide/workflows.md) for the full operation and retry rules.

Short-form publishing through Buffer has a separate [review and publication workflow](docs/guide/social-publishing.md).

| Task | English | 简体中文 |
| --- | --- | --- |
| Install and connect | [Quick Start](docs/quick-start.md) | [快速开始](docs/zh-CN/quick-start.md) |
| Create the first draft | [Tutorial](docs/tutorial/first-publish.md) | [首次发布](docs/zh-CN/tutorial/first-publish.md) |
| Configure paths and storage | [Configuration](docs/reference/configuration.md) | [配置参考](docs/zh-CN/reference/configuration.md) |
| Diagnose a disconnected Console | [Bridge troubleshooting](docs/how-to/debug-bridge.md) | [连接排查](docs/zh-CN/how-to/debug-bridge.md) |
| Inspect CLI flags | [CLI](docs/reference/cli.md) | [命令行](docs/zh-CN/reference/cli.md) |

## Development

From the engine root:

```bash
go run ./tools/blogctl/cmd/maint verify
go run ./tools/blogctl/cmd/maint build
go run ./tools/blogctl/cmd/maint package
```

Build output goes to `.build/out/`. Packaging produces six platform executables, the Extension ZIP and checksums. Read the [root rules](../../AGENTS.md) and [BlogCTL rules](AGENTS.md) before modifying the code.

[MIT License](../../LICENSE)

## ChatGPT / MCP

Run `blogctl -mcp-stdio` from the installed machine to expose the existing Bridge as MCP over STDIO. The browser Extension continues to own browser sessions; no separate publisher or public HTTP endpoint is created. See [MCP integration](docs/MCP.md).

The MCP tool `upload_image_to_r2` sanitizes images locally before uploading with the existing R2 credentials, and returns content-versioned Markdown URLs. Configure `image_upload_roots` in private TOML; see [MCP image uploads](docs/MCP.md#private-image-upload-to-r2).

## Legal Use and Third-Party Services

BlogCTL is an open-source tool for content management and cross-platform publishing. It is not affiliated with or officially endorsed by any supported third-party platform.

Users are responsible for ensuring they have the necessary rights to access accounts and publish content, and for complying with applicable laws and third-party terms of service. The project must not be used for unauthorized access, infringement of third-party rights, or other unlawful activities.

BlogCTL is licensed under the [MIT License](../../LICENSE). This license does not grant authorization to access third-party services. The software is provided "as is," subject to the disclaimers in the LICENSE file.

The new dependency-license packaging check is currently blocked by fhttp v0.6.9; see [third-party notices](../../THIRD_PARTY_NOTICES.md). v0.1.193 includes project MIT files; it has not passed this new transitive-dependency review.

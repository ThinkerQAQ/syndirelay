# SyndiRelay documentation

[简体中文](zh-CN/index.md) · [Project README](../README.md) · [Inkerio engine](https://github.com/ThinkerQAQ/inkerio)

**SyndiRelay is the standalone Markdown syndication and publishing product formerly called BlogCTL.** Inkerio now builds the website; SyndiRelay optionally compiles and distributes articles. A standalone Markdown file does not need the site engine or the author's content repository.

| Goal | Guide |
| --- | --- |
| Build and locally compile an article | [Quick start](quick-start.md) |
| Set up the CLI and Extension on a machine | [New-machine setup](tutorial/first-run-on-new-machine.md) |
| Connect the STDIO MCP without a public Bridge | [MCP](MCP.md) |
| Configure a content directory and personal publishing identity | [Workspace](how-to/configure-workspace.md) and [copyright](reference/copyright-and-identity.md) |
| Understand per-platform draft and publication operations | [Publishing workflow](guide/workflows.md) |
| Inspect supported commands and optional legacy CI tasks | [CLI reference](reference/cli.md) |
| Understand system components and local data | [Architecture](architecture/index.md) |
| Track verified publications in Reverfolio | [Optional impact tracking](how-to/reverfolio.md) |
| Review source audit and remaining binary-release requirements | [Publication review](compliance/PUBLICATION_READINESS.md) |
| Understand history of the original BlogCTL extraction | [Migration archive](migration/README.md) |

The repository source is **public and MIT-licensed**; third-party platform operations and binary/Extension Releases have separate verification requirements. All remote publishing requires an explicitly authorized user/account action and platform-specific support. A successful local compilation does not imply remote publishing permission. Read [third-party notices](../THIRD_PARTY_NOTICES.md) before distributing artifacts.

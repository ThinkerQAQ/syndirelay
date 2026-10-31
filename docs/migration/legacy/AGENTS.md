# BlogCTL Agent Scope

Read [the root Agent Contract](../../AGENTS.md) first. This file adds BlogCTL-specific constraints only.

- Build/verify/package from the engine root via `go run ./tools/blogctl/cmd/maint <build|verify|package>`. This Go command is engineering-only; the product CLI remains `go run ./tools/blogctl/cmd <command>`.
- Prefer `gopls serve` through an LSP client for Go definitions, references, diagnostics and safe renames. For JavaScript or Markdown use an installed language server when actually available; otherwise use scoped reads, `rg`, `node --check` and existing test scripts.
- CLI: [cmd/main.go](cmd/main.go); config/Bridge: [bridge/config.go](bridge/config.go); publishing: [publishing/config.go](publishing/config.go) and [publisher](publisher/); browser: [extension](extension/).
- Read [CLI Reference](docs/reference/cli.md), [Configuration Reference](docs/reference/configuration.md) and [Workflows](docs/guide/workflows.md) before editing user-facing behavior.
- Read/detect/list never mutate remote content. Create/update/publish/index are explicit side effects; persist remote identities and do not blindly retry ambiguous writes.
- Browser credentials stay ephemeral. Go implements backend operations; browser-only behavior belongs in the Extension. Avoid duplicate Web Console business logic.
- Prefer upstream adapters, standards and SDKs; keep small commits and preserve installed `Data/`.

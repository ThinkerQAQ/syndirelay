# SyndiRelay MCP (local STDIO)

[简体中文说明](zh-CN/quick-start.md) · [Quick start](quick-start.md) · [Security and distribution review](compliance/PUBLICATION_READINESS.md)

SyndiRelay exposes named MCP tools through a **local STDIO** process, backed by its existing Bridge and (for authorized browser workflows) the matching installed Extension. It does not open a second public HTTP MCP listener, export cookies to the assistant or grant blanket permission to access external services.

## Start the correct executable

Install the native binary for the **same operating system as the signed-in browser**, and preserve its local data directory. Build from an authorized SyndiRelay checkout:

```sh
npm ci
go build -o syndirelay ./cmd
./syndirelay mcp-stdio
```

Windows PowerShell:

```powershell
go build -o syndirelay.exe ./cmd
.\syndirelay.exe mcp-stdio
```

Running from the checkout is suitable for local testing. For stable installation, keep the executable, `extension/`, `renderers/`, `package.json`, `package-lock.json` and any required diagram tooling in a durable directory, install runtime dependencies there, and set `SYNDIRELAY_HOME` to it. If needed, set the absolute `SYNDIRELAY_DATA_DIR` **privately**, never in a public repository.

To pair with Edge or Chrome, first register `./syndirelay install` (Windows: `.\syndirelay.exe install`) and load this repository's `extension/` as an unpacked extension. This is distinct from the retired `blogctl` Native Host, which used a different extension ID and port. SyndiRelay's Bridge listens on `127.0.0.1:32146`, **not 32145**. An unrelated WSL/Linux binary cannot read your Windows browser session.

When connecting to a ChatGPT or other MCP client, attach a *dedicated local STDIO transport*. If a remote tunnel is used, tunnel **only the limited MCP STDIO interface** with authentication, never the Bridge's entire HTTP service, console or browser session state. Never include tunnel IDs, local tokens or browser cookies in documentation or code.

## Available operations and authorization

MCP tools include local article lookup, available-platform inspection, publication record lookup, compilation preview, selected draft creation/update/publication, task history and optional local image preparation/upload when explicitly configured. Exact tools depend on the installed build: inspect the MCP tool schema before invoking actions.

- Local compilation and dry-run are nonpublishing operations; do not conflate them with remote create/update.
- Remote operations require explicit user intent, the correct platform account and where applicable a **specific** remote article/draft ID.
- An ambiguous timeout after submitting a create request is not permission to retry with a new ID. First inspect the existing task and remote draft to avoid duplication.
- Some platforms support only a subset of detection/create/update/publish operations; account cookies or page access do **not** imply access to undocumented service APIs.
- The Extension may temporarily relay authentication context to the local Bridge; tools must never return raw cookies, authorization headers, private R2 keys or Bridge tokens.
- Reading Markdown through an MCP tool sends its returned text to the MCP client; handle private content according to the user's own disclosure settings.

## Verify without publishing

```sh
go test ./cmd ./bridge
node --test extension/tests/*.test.mjs
./syndirelay doctor
```

For Windows browser integration check the Extension's connection state and local Bridge health, then run only an explicitly requested, authorized operation. Unit tests do not establish remote account authorization.

## Image processing

The optional R2 image tool requires its own local permitted directories, Sharp runtime and user-owned R2 settings. It can strip EXIF/GPS metadata from JPEG/PNG/WebP output; this does **not** remove visible addresses, faces, license plates or other details inside pixels. It does not make third-party photos yours to publish. Keep originals and local credentials out of MCP and release artifacts.

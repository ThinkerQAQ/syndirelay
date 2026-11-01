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

## Connect to ChatGPT through a dedicated Secure MCP Tunnel

**Default rule: run both SyndiRelay MCP and the Tunnel client natively on the user's OS.** The official [`tunnel-client` releases](https://github.com/openai/tunnel-client/releases/latest) include builds for Windows, macOS and Linux. Windows users never need Ubuntu or WSL. The local `syndirelay mcp-stdio` server exposes 14 MCP tools; a separately associated Tunnel ID and ChatGPT connection are additional steps.

A Secure MCP Tunnel uses **an existing authorized runtime credential** (Tunnels Read + Use). It does **not** need a fresh/one-time API key for each application. Reuse an authorized user/session environment reference; do not copy another application's credential profile, hard-code a secret in a script, or commit it to Git. The Tunnel ID is an identifier, not a credential.

### Windows (recommended default for Windows users)

1. Build/install `syndirelay-windows-amd64.exe` and its private `Data/`, Extension and renderer dependencies on Windows. Keep the installed browser Native Host on Windows.
2. Download the **Windows** `tunnel-client` ZIP from the [official latest release](https://github.com/openai/tunnel-client/releases/latest) and extract `tunnel-client.exe` next to SyndiRelay or into a directory on `PATH`. The script deliberately fails instead of silently launching WSL.
3. In a regular **Windows PowerShell** session that already has access to your authorized Tunnel runtime credential, set `CONTROL_PLANE_API_KEY` using your established secure method (do not place the value inside the repository or in a committed PowerShell profile). This is not a request to create a new key.
4. Run the standalone Windows setup:

```powershell
.\scripts\connect-chatgpt-mcp.ps1 -TunnelId tunnel_YOUR_TUNNEL_ID
.\scripts\connect-chatgpt-mcp.ps1 -TunnelId tunnel_YOUR_TUNNEL_ID -Status
```

The Windows launcher uses the real Windows `tunnel-client.exe`, the Windows `syndirelay-windows-amd64.exe mcp-stdio`, and a separate Windows profile under `%LOCALAPPDATA%\SyndiRelay\tunnel`. For nonstandard locations pass `-TunnelClientPath` and `-SyndiRelayPath` (absolute paths). For an existing named environment reference use `-RuntimeKeyRef env:VARIABLE`; a private file reference `file:C:\private\path` is also supported. It never edits the older BlogCTL Native Host or user data.

### Linux / macOS

Install the native `syndirelay` and native `tunnel-client` executable on the **same host**. Expose your existing authorized runtime credential in `CONTROL_PLANE_API_KEY` through your private process manager, or select `SYNDIRELAY_RUNTIME_KEY_REF=env:VARIABLE` / `file:/private/path`. Then:

```sh
./scripts/connect-chatgpt-mcp.sh tunnel_YOUR_TUNNEL_ID
./scripts/connect-chatgpt-mcp.sh tunnel_YOUR_TUNNEL_ID --status
```

Do not rely on a Windows `C:\...` path or WSL drive mounts. Both scripts refuse an absent runtime credential or legacy BlogCTL MCP executable. Linux/macOS Native Messaging installation, when used, is separate from Tunnel setup.

### Optional WSL bridge (advanced, not the Windows default)

WSL can carry the outbound Tunnel connection to an explicitly specified **Windows** `syndirelay.exe mcp-stdio` when the user already has WSL. This is optional and should never be required in setup docs for normal Windows users. Set `SYNDIRELAY_MCP_COMMAND` explicitly if you choose it; it is not autodetected.

### Connect the ChatGPT plugin

Once the Tunnel reports healthy and ready, go to **ChatGPT Plugins → Add custom MCP server → Tunnel**, select/paste the SyndiRelay-specific Tunnel ID and review the requested permissions. Test read-only `list_platforms` / `list_articles` first. The three states remain separate: **native MCP running; Tunnel ready; ChatGPT plugin registered and tools called**. Do not report success until the third state is verified.

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

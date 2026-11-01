# SyndiRelay

[简体中文](README_ZH.md)

- [1. Introduction](#1-introduction)
- [2. Quick Start](#2-quick-start)
- [3. Configuration](#3-configuration)
- [4. Optional Features](#4-optional-features)
- [5. Development](#5-development)
- [6. License](#6-license)

## 1. Introduction

SyndiRelay converts Markdown into platform-specific articles and helps authors distribute them from a local workspace. It provides a CLI, a browser extension with a local Bridge, and an MCP server for reading articles, previewing output, and managing publishing tasks.

The compiler supports 11 platform formats: `51cto`, `cnblogs`, `csdn`, `devto`, `hashnode`, `juejin`, `medium`, `oschina`, `segmentfault`, `toutiao`, and `zhihu`. Remote operations depend on each adapter and the connected account; format support alone does not guarantee publishing support.

SyndiRelay works with ordinary Markdown files. Authors using [Inkerio](https://github.com/ThinkerQAQ/inkerio) can also reuse their existing content repository; installing the blog engine is optional.

## 2. Quick Start

Compile one Markdown article into DEV-compatible output. No account, browser extension, or configuration file is required.

### 2.1. Prerequisites

- Windows, macOS, or Linux.
- Git, Go (see [go.mod](go.mod) for the required version), Node.js **22+**, and npm.
- Network access to download source and dependencies.

### 2.2. Get the source and build

Build from source.

```sh
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
```

Linux / macOS:

```sh
go build -o syndirelay ./cmd
```

Windows PowerShell:

```powershell
go build -o syndirelay.exe ./cmd
```

### 2.3. Compile your first article

In the same `syndirelay` checkout, create an example article and compile it for DEV.

Linux / macOS:

```sh
printf '# Hello SyndiRelay\n\nMy first article.\n' > example.md
./syndirelay compile --file ./example.md --platform devto
```

Windows PowerShell:

```powershell
Set-Content -Path example.md -Encoding utf8 -Value "# Hello SyndiRelay`n`nMy first article."
.\syndirelay.exe compile --file .\example.md --platform devto
```

### 2.4. Verify the output

The command must exit successfully and print JSON containing:

- `"platform": "devto"`
- `"title": "Hello SyndiRelay"`
- `markdown` containing `My first article.`
- A nonempty `contentHash`.

`compile` produces local output. Its `published` field describes source content status, not a remote publishing result.

### 2.5. Troubleshooting

- **Node module or renderer not found:** run `npm ci` and retry from the checkout. When running elsewhere, set `SYNDIRELAY_HOME` to the checkout's absolute path.
- **Unsupported platform:** use one of the platform IDs listed above.
- **Invalid input:** provide a nonempty UTF-8 Markdown file no larger than 5 MiB; YAML frontmatter, if present, must be valid.
- **Diagram/table media need an asset URL:** pass `--asset-base-url https://your-cdn.example/`. This supplies URLs for generated assets; compilation does not upload them.
- **Missing dependencies:** run `./syndirelay doctor` (PowerShell: `.\syndirelay.exe doctor`) to check Node.js, npm, and Git.

If the problem persists, [open an Issue](https://github.com/ThinkerQAQ/syndirelay/issues) with your OS, command, and redacted error output.

## 3. Configuration

Standalone `compile` only requires `--file` and `--platform`. Optional flags are `--language en|zh-CN`, `--canonical-url`, and `--asset-base-url`; both URL flags require HTTPS. Inspect the complete flags with `./syndirelay compile --help`.

Bridge settings are stored in `blogctl.toml`:

- Windows: `Data\blogctl.toml` beside the executable.
- Linux: `$XDG_CONFIG_HOME/SyndiRelay/blogctl.toml`, defaulting to `~/.config/SyndiRelay/blogctl.toml`.
- macOS: `~/Library/Application Support/SyndiRelay/blogctl.toml`.
- Set `SYNDIRELAY_DATA_DIR` to an **absolute directory** to override the data location.

For content-repository workflows, create or edit `blogctl.toml` at the location above. Create its parent directory if needed, then set your absolute paths and site URL:

```toml
content_root = "/absolute/path/to/your-content"
engine_root = "/absolute/path/to/syndirelay"
ui_locale = "auto"

[publishing.site]
origin = "https://your-site.example"
```

On Windows, use TOML literal strings such as `content_root = 'C:\sites\my-content'`. `ui_locale` defaults to `auto` and also accepts `en` or `zh-CN`; `BLOGCTL_LANG` overrides the CLI language.

Set `SYNDIRELAY_CONTENT_ROOT` and `SYNDIRELAY_HOME` explicitly for CLI repository selection, as shown below. In copyright auto mode, an explicit SyndiRelay copyright policy takes precedence over the content repository's `src/content/site/copyright.json`. Manage platform credentials, image storage, and other optional settings through the extension settings; additional fields are defined in [the configuration types](bridge/config.go) and [publishing settings](publishing/config.go).

## 4. Optional Features

### 4.1. Case1: Distribute articles from an Inkerio content repository

A repository created from [Inkerio Content Template](https://github.com/ThinkerQAQ/inkerio-content-template) can serve both your website and SyndiRelay. No second content repository is needed. CLI sync expects `src/content/articles/` and `src/data/content-manifest.json`.

Complete the build in section 2 and configure `publishing.site.origin` as shown in section 3. Return to the SyndiRelay checkout, select your content repository, and preview an existing article. Replace the sample paths and `your-article-slug` with your own values.

```sh
export SYNDIRELAY_HOME="$PWD"
export SYNDIRELAY_CONTENT_ROOT="/absolute/path/to/your-content"
./syndirelay sync --article your-article-slug --platforms devto --dry-run
```

```powershell
$env:SYNDIRELAY_HOME = (Get-Location).Path
$env:SYNDIRELAY_CONTENT_ROOT = 'C:\sites\my-content'
.\syndirelay.exe sync --article your-article-slug --platforms devto --dry-run
```

The command must exit successfully and print the selected article compiled for `devto`. `--dry-run` leaves publishing state unchanged. To create a remote draft after the preview, continue to the next section for the browser extension.

### 4.2. Case2: Create a draft with the browser extension

Use a native SyndiRelay executable on the same OS as your Edge or Chrome browser. If you already have a content repository, you can skip the optional Inkerio preview above. Complete [2.2](#22-get-the-source-and-build) and keep the terminal in the checkout. Windows does not require WSL.

1. Run `./syndirelay install` (PowerShell: `.\syndirelay.exe install`) to register the Native Messaging host.
2. After `Native Messaging Host registered` appears, open `edge://extensions` or `chrome://extensions`, enable developer mode, choose **Load unpacked**, and select the checkout's `extension/` directory.
3. Open SyndiRelay from the browser toolbar's extensions menu and confirm its Bridge connection. It listens on `127.0.0.1:32146`; if needed, run `./syndirelay --bridge` (PowerShell: `.\syndirelay.exe --bridge`) and keep that terminal running.
4. As shown in section 3, point `content_root` to an existing supported content repository, `engine_root` to the SyndiRelay checkout, and set your site URL. Inkerio Content Template repositories must contain `src/content/articles/` and `src/data/content-manifest.json`.
5. Sign in to the target platform in the same browser. Return to the extension, select an article and platform, preview it, then use the supported draft creation action.
6. Open the draft URL returned in the task result and check its title and body.

Keep the executable at its registered path; rerun `install` if you move it. If the extension cannot connect, verify that it uses the same OS installation and reload it after registration. Platform login and an available adapter operation are required for remote writes.

### 4.3. Case3: Connect an MCP client

Complete [2.2](#22-get-the-source-and-build). After adding the server, restart or reconnect your client before calling a tool.

For a client that supports STDIO MCP, add a server entry using the absolute executable path:

```json
{
  "mcpServers": {
    "syndirelay": {
      "command": "/absolute/path/to/syndirelay",
      "args": ["mcp-stdio"]
    }
  }
}
```

Windows clients should use the native executable, for example `"command": "C:\\tools\\syndirelay\\syndirelay.exe"`. Invoke `list_platforms` with `{}` to verify a real tool response. Article tools also need the configured content workspace; browser-backed publishing needs the extension.

### 4.4. Case4: Connect ChatGPT

Complete the build in section 2, then follow this order. A Tunnel ID and a runtime credential are separate values:

1. In OpenAI Platform [Tunnels management](https://platform.openai.com/settings/organization/tunnels), create or select a Tunnel for the relevant ChatGPT workspace and copy its full `tunnel_...` ID. Creating a Tunnel requires Tunnels Manage permission.
2. At [Runtime API Keys](https://platform.openai.com/settings/organization/api-keys), reuse an authorized runtime key with **Tunnels Read + Use** permissions or create a restricted key with those permissions. Supply it via `CONTROL_PLANE_API_KEY`, or set `SYNDIRELAY_RUNTIME_KEY_REF` to `env:NAME` or `file:ABSOLUTE_PATH`. On Linux/macOS, a credential file must have mode `0600`. **No new one-time or per-project API key is needed**, but a valid runtime credential is required; do not use an admin key instead.
3. Install the official [tunnel-client](https://github.com/openai/tunnel-client/releases/latest) for your operating system, ensure `tunnel-client` is on `PATH`, then run the commands below. Never commit the actual credential to Git.

Linux / macOS, from the checkout:

```sh
export SYNDIRELAY_MCP_COMMAND="$PWD/syndirelay mcp-stdio"
./scripts/connect-chatgpt-mcp.sh tunnel_YOUR_TUNNEL_ID
./scripts/connect-chatgpt-mcp.sh tunnel_YOUR_TUNNEL_ID --status
```

Windows PowerShell:

```powershell
.\scripts\connect-chatgpt-mcp.ps1 -TunnelId tunnel_YOUR_TUNNEL_ID
.\scripts\connect-chatgpt-mcp.ps1 -TunnelId tunnel_YOUR_TUNNEL_ID -Status
```

Use the status command shown above to confirm that the SyndiRelay runtime is running and healthy. In [ChatGPT connector settings](https://chatgpt.com/#settings/Connectors), choose **Connection: Tunnel** and select or paste the same Tunnel ID. Call `list_platforms` and confirm an actual tool response; a healthy local runtime alone does not verify ChatGPT connectivity. If the Tunnel is missing, check workspace scope and Tunnels Read + Use permissions. Keep the Bridge local.

[Reverfolio](https://github.com/ThinkerQAQ/reverfolio) impact tracking is a separate, optional integration and is disabled by default.

## 5. Development

Use the prerequisites above. From the checkout:

```sh
npm ci
go test ./...
npm run test:renderers
npm run test:extension
go build -o .build/syndirelay ./cmd
```

Linux/macOS Tunnel script checks: `bash scripts/connect-chatgpt-mcp.test.sh`. Windows PowerShell: `.\scripts\connect-chatgpt-mcp.windows.test.ps1`.

[GitHub Actions](.github/workflows/ci.yml) runs Go tests, renderer/extension tests, native builds, and Tunnel script checks on Linux, macOS, and Windows. Script checks use stubs; they do not contact a real Tunnel. CI does not verify browser installation or publishing with real platform accounts. Those require native manual testing.

## 6. License

Original source code is licensed under [MIT](LICENSE). Third-party components retain their own licenses; see [third-party notices](THIRD_PARTY_NOTICES.md) and [extension notices](extension/THIRD_PARTY_NOTICES.md). Public installation packages remain pending dependency-license and native-installation review.

The source license does not grant rights to user articles, example media, or third-party materials. Publish only content you may use, and follow each platform's terms and authorization requirements; some adapters use undocumented interfaces.

Keep account credentials, Cookies, local configuration, and publishing state out of Git and distribution packages. Remote publishing sends the selected content to the selected platform.

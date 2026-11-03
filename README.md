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

Install and open the SyndiRelay browser extension, then navigate to **Settings → MCP connection**:

1. Create a Tunnel in [OpenAI Tunnels](https://platform.openai.com/settings/organization/tunnels) and copy its full Tunnel ID.
2. Create or reuse a restricted Runtime API Key with **Tunnels Read + Use** permission in [OpenAI API Keys](https://platform.openai.com/api-keys). You do not need a new key for every setup.
3. Under **1. Local Tunnel service**, enter the Tunnel ID and Runtime API Key, then select **Save**. If necessary, SyndiRelay installs and verifies the official Tunnel Client. Saving does not start the service or enable launch-at-login.
4. Select **Start Service**. Use Stop or Restart when the service is running. The header status reports local Tunnel readiness.
5. Under **2. ChatGPT plugin**, open [ChatGPT plugin settings](https://chatgpt.com/settings/plugins-settings), create or enable the SyndiRelay connector, and call `list_platforms` to verify the integration.

The saved Tunnel ID remains visible. The Runtime API Key is masked; leave the field blank to reuse the saved credential. After changing either value, save and restart the service to apply it. Credentials are never written to the repository, logs, or process arguments. Windows uses DPAPI; Linux and macOS keep the key in a private user configuration directory.

**Note:** A healthy local Tunnel is not proof of ChatGPT plugin authorization. Standard STDIO MCP clients do not need a Tunnel; see section 4.3.

The existing `connect-chatgpt-mcp.ps1` and `connect-chatgpt-mcp.sh` scripts remain available for manual setup.

[Reverfolio](https://github.com/ThinkerQAQ/reverfolio) impact tracking is an independent, optional integration, disabled by default.

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

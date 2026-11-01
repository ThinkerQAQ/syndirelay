# SyndiRelay

[简体中文](README_ZH.md)

## Introduction

SyndiRelay converts Markdown into platform-specific articles and helps authors distribute them from a local workspace. It provides a CLI, a browser extension with a local Bridge, and an MCP server for reading articles, previewing output, and managing publishing tasks.

The compiler supports 11 platform formats: `51cto`, `cnblogs`, `csdn`, `devto`, `hashnode`, `juejin`, `medium`, `oschina`, `segmentfault`, `toutiao`, and `zhihu`. Remote operations depend on each adapter and the connected account; format support alone does not guarantee publishing support.

SyndiRelay works with ordinary Markdown files. Authors using [Inkerio](https://github.com/ThinkerQAQ/inkerio) can also reuse their existing content repository; installing the blog engine is optional.

## Quick Start

Compile one Markdown article into DEV-compatible output. No account, browser extension, or configuration file is required.

### Prerequisites

- Windows, macOS, or Linux.
- Git, Go **1.24.1+** (the module requests toolchain 1.24.5), Node.js **22+**, and npm.
- Network access to download source and dependencies.

### Install

Build from source; this repository currently has no published installation packages.

```sh
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
```

Linux / macOS:

```sh
go build -o syndirelay ./cmd
printf '# Hello SyndiRelay\n\nMy first article.\n' > example.md
./syndirelay compile --file ./example.md --platform devto
```

Windows PowerShell:

```powershell
go build -o syndirelay.exe ./cmd
Set-Content -Path example.md -Encoding utf8 -Value "# Hello SyndiRelay`n`nMy first article."
.\syndirelay.exe compile --file .\example.md --platform devto
```

Run these commands from the checkout so the compiler can find its Node renderers and installed dependencies.

### Verify

The command must exit successfully and print JSON containing:

- `"platform": "devto"`
- `"title": "Hello SyndiRelay"`
- `markdown` containing `My first article.`
- A nonempty `contentHash`.

This completes a local conversion. The command does not create a remote draft or publish an article. The JSON `published` field describes source content status, not a confirmed remote publication.

### Troubleshooting

- **Node module or renderer not found:** run `npm ci` and retry from the checkout. When running elsewhere, set `SYNDIRELAY_HOME` to the checkout's absolute path.
- **Unsupported platform:** use one of the platform IDs listed above.
- **Invalid input:** provide a nonempty UTF-8 Markdown file no larger than 5 MiB; YAML frontmatter, if present, must be valid.
- **Diagram/table media need an asset URL:** pass `--asset-base-url https://your-cdn.example/`. This supplies URLs for generated assets; compilation does not upload them.
- **Missing dependencies:** run `./syndirelay doctor` (PowerShell: `.\syndirelay.exe doctor`) to check Node.js, npm, and Git.

## Configuration

Standalone `compile` only requires `--file` and `--platform`. Optional flags are `--language en|zh-CN`, `--canonical-url`, and `--asset-base-url`; both URL flags require HTTPS. Inspect the complete flags with `./syndirelay compile --help`.

Bridge settings are stored in `blogctl.toml`:

- Windows: `Data\blogctl.toml` beside the executable.
- Linux: `$XDG_CONFIG_HOME/SyndiRelay/blogctl.toml`, defaulting to `~/.config/SyndiRelay/blogctl.toml`.
- macOS: `~/Library/Application Support/SyndiRelay/blogctl.toml`.
- Set `SYNDIRELAY_DATA_DIR` to an **absolute directory** to override the data location.

For content-repository workflows, configure your own paths and site identity:

```toml
content_root = "/absolute/path/to/your-content"
engine_root = "/absolute/path/to/syndirelay"
ui_locale = "auto"

[publishing.site]
origin = "https://your-site.example"
```

On Windows, use TOML literal strings such as `content_root = 'C:\sites\my-content'`. `ui_locale` defaults to `auto` and also accepts `en` or `zh-CN`; `BLOGCTL_LANG` overrides the CLI language.

Set `SYNDIRELAY_CONTENT_ROOT` and `SYNDIRELAY_HOME` explicitly for CLI repository selection, as shown below. In copyright auto mode, an explicit SyndiRelay copyright policy takes precedence over the content repository's `src/content/site/copyright.json`. Manage platform credentials, image storage, and other optional settings through the extension settings; additional fields are defined in [the configuration types](bridge/config.go) and [publishing settings](publishing/config.go).

## Optional Features

### Browser extension and remote drafts

Use a native SyndiRelay executable on the same operating system as your browser. Windows does not require WSL.

1. Run `./syndirelay install` (PowerShell: `.\syndirelay.exe install`) to register the Native Messaging host.
2. Open `edge://extensions` or `chrome://extensions`, enable developer mode, and load the checkout's `extension/` directory.
3. Open the extension and confirm its Bridge connection. It listens on `127.0.0.1:32146`; if needed, start it manually with `./syndirelay --bridge`.
4. Configure the content workspace and sign in to the intended platform in that browser. Preview the selected article, then choose an available draft or publishing action.
5. Check the task result and open the returned platform URL to verify the actual draft or article.

Keep the executable at its registered path; rerun `install` if you move it. If the extension cannot connect, verify that it uses the same OS installation and reload it after registration. Platform login and an available adapter operation are required for remote writes.

### Reuse an Inkerio content repository

A repository created from [Inkerio Content Template](https://github.com/ThinkerQAQ/inkerio-content-template) can serve both your website and SyndiRelay. No second content repository is needed. CLI sync expects `src/content/articles/` and `src/data/content-manifest.json`.

After configuring `publishing.site.origin`, select your checkout and content paths, then preview an existing article. Replace the sample paths and `your-article-slug` with your own values.

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

Dry run compiles a preview without changing publishing state. Review the output before creating or updating remote content.

### MCP clients and ChatGPT

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

For ChatGPT, additionally install the official [tunnel-client](https://github.com/openai/tunnel-client/releases/latest) for your OS and configure an authorized Tunnel runtime credential using `CONTROL_PLANE_API_KEY` or `SYNDIRELAY_RUNTIME_KEY_REF` (`env:NAME` or `file:ABSOLUTE_PATH`). On Linux/macOS, credential files must have mode `0600`. Existing authorized credentials can be reused.

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

Add that Tunnel in ChatGPT and call `list_platforms`. A healthy Tunnel alone does not verify the ChatGPT connection. Keep the Bridge local.

[Reverfolio](https://github.com/ThinkerQAQ/reverfolio) impact tracking is a separate, optional integration and is disabled by default.

## Development

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

## License

Original source code is licensed under [MIT](LICENSE). Third-party components retain their own licenses; see [third-party notices](THIRD_PARTY_NOTICES.md) and [extension notices](extension/THIRD_PARTY_NOTICES.md). Public installation packages remain pending dependency-license and native-installation review.

The source license does not grant rights to user articles, example media, or third-party materials. Publish only content you may use, and follow each platform's terms and authorization requirements; some adapters use undocumented interfaces.

Keep account credentials, Cookies, local configuration, and publishing state out of Git and distribution packages. Remote publishing sends the selected content to the selected platform.

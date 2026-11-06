# SyndiRelay

[简体中文](README_ZH.md)

- [1. Introduction](#1-introduction)
- [2. Quick Start](#2-quick-start)
- [3. Development](#3-development)
- [4. License](#4-license)

## 1. Introduction

I started SyndiRelay because distributing the same Markdown article to several platforms meant repeatedly copying its text, fixing formatting, handling images, and creating drafts one by one.

Each platform has its own Markdown, HTML, image, and editor requirements. SyndiRelay keeps the original article in a local workspace, generates platform-specific output, and uses a browser extension to operate accounts you have already signed in to. Preview first, then create a draft. **Compiling an article does not publish it remotely.**

The compiler supports 11 formats: DEV, Medium, Zhihu, Juejin, Toutiao, CSDN, CNBlogs, Hashnode, 51CTO, OSChina, and SegmentFault. Draft creation and updates depend on the adapter and website. Format support does not imply full remote-write support.

SyndiRelay provides native installers for Windows, Linux, and macOS, a browser extension, a CLI, and MCP tools. OpenAI Secure MCP Tunnel connects the local MCP server to ChatGPT on the web without exposing the Bridge publicly. Local MCP clients can also use stdio directly. Ordinary Markdown does not require [Inkerio](https://github.com/ThinkerQAQ/inkerio), while existing Inkerio content repositories can be reused.

## 2. Quick Start

Install SyndiRelay, connect the browser extension, and preview an article. Configure a Tunnel only if you also want to use the tools from ChatGPT. **Installers are still under review; no official Release has been published.** Until then, use a local test installer or run from source as described in section 3.

### 2.1. Installation

#### 2.1.1. Graphical Interface

**Windows:** Run `SyndiRelay-Setup-*-windows-amd64.exe` and complete the wizard. From the Start Menu, open **SyndiRelay**. It registers the Native Messaging host and opens the extension directory and browser extensions page.

**macOS:** Open the corresponding installer. After installation, choose **Open SyndiRelay** to open the extension directory.

**Linux:** Run the `.run` installer from a graphical desktop. If blocked, enable execution in its file properties. Choose **Open SyndiRelay** when installation finishes.

In Edge (`edge://extensions`) or Chrome (`chrome://extensions`), enable developer mode, select **Load unpacked**, and choose the installed `extension/` directory. Browser authorization remains a manual step; the installer does not bypass it.

#### 2.1.2. Command Line

Qt Installer Framework also supports unattended installation. From the download directory, on Windows CMD:

~~~cmd
.\downloaded-installer.exe --accept-licenses --default-answer --confirm-command install
~~~

On a Linux desktop:

~~~bash
chmod +x ./SyndiRelay-Setup-*-linux-amd64.run
QT_QPA_PLATFORM=offscreen ./SyndiRelay-Setup-*-linux-amd64.run --accept-licenses --default-answer --confirm-command install
~~~

Unattended installation still requires manual browser extension approval. Source builds and developer commands are in section 3.

### 2.2. Configuration

#### 2.2.1. Graphical Interface

Open SyndiRelay from the browser toolbar. In **Settings**, choose your content repository, site address, platform credentials, and optional image storage. The local Bridge only listens on `127.0.0.1:32146`, and the Native Messaging host starts it when needed.

If you use [Inkerio Content Template](https://github.com/ThinkerQAQ/inkerio-content-template), select the existing content repository; there is no need for a second copy. Remote draft actions require you to be signed in to the target website in the same browser.

#### 2.2.2. Configuration File

For CLI automation, edit the private `syndirelay.toml` file. On Windows it lives at `Data/syndirelay.toml` below the installation root; Linux and macOS use the system user configuration directory. Set `SYNDIRELAY_DATA_DIR` to an absolute path to override this directory.

Minimal setup for an existing content repository:

~~~toml
content_root = "/absolute/path/to/your-content"
engine_root = "/absolute/path/to/syndirelay"

[publishing.site]
origin = "https://your-site.example"
~~~

Windows paths can be TOML single-quoted strings. Standalone `compile` needs no configuration file. Source CLI workflows use `SYNDIRELAY_HOME` for the checkout and `SYNDIRELAY_CONTENT_ROOT` for the content repository. Other fields are defined in [Bridge configuration](bridge/config.go) and [publishing configuration](publishing/config.go).

### 2.3. Verify an Article

In the extension, select an article and target platform. Preview its output before choosing **Create Draft** if the adapter supports it. Open the returned draft link to inspect the title, body, and images.

You can also test local compilation without any account or browser. From the [source checkout](#3-development):

~~~bash
printf '# Hello SyndiRelay\n\nMy first article.\n' > example.md
./syndirelay compile --file ./example.md --platform devto
~~~

Windows CMD:

~~~cmd
echo # Hello SyndiRelay> example.md
.\syndirelay.exe compile --file .\example.md --platform devto
~~~

The JSON output should include `platform: "devto"`, title `Hello SyndiRelay`, and a nonempty `contentHash`. This is local compilation and **does not create or publish an article remotely**.

To preview an existing Inkerio article without writing remotely:

~~~bash
export SYNDIRELAY_HOME="$PWD"
export SYNDIRELAY_CONTENT_ROOT="/absolute/path/to/your-content"
./syndirelay sync --article your-article-slug --platforms devto --dry-run
~~~

### 2.4. Connect ChatGPT

Create a Tunnel ID in [OpenAI Tunnel settings](https://platform.openai.com/settings/organization/tunnels) and a restricted Runtime API Key in [API Keys](https://platform.openai.com/settings/organization/api-keys).

In the extension, open **Settings → Runtime Environment → MCP Connection**. The native MCP setup is fresh: it never reads or migrates old Tunnel profiles, keys or script settings. Re-enter the connection details; articles and publishing history are unaffected:

1. Under **Environment Check**, inspect the Tunnel Client status and executable path. If missing, click **Download Tunnel Client** to download and SHA-256 verify the official release into SyndiRelay's own `bin/` beneath its data directory. Alternatively, edit the absolute executable path and click **Save Path**. **Reset to Default** restores the SyndiRelay-specific location. No global `PATH` or `~/.local/bin` fallback is used.
2. Under **Local Tunnel Service**, enter the Tunnel ID and Runtime API Key, then choose **Save**. Saving does not download a client or start the service.
3. Select **Start Service** and wait for **Ready**. Use **Restart Service** after changing saved configuration or the executable path; **Stop Service** remains available while the process runs, even if health checks fail.
4. **Start Tunnel at login** controls whether the local service starts at system login. **Connect Tunnel automatically** independently controls whether that service establishes a Tunnel on start. Restart the service after changing automatic connection. While running, **Connect Tunnel** and **Disconnect Tunnel** control the Tunnel without stopping the local service.
5. Under **ChatGPT Plugin**, open [ChatGPT Plugins](https://chatgpt.com/plugins), register and enable the plugin with the same Tunnel ID, and follow [OpenAI's custom MCP connection guide](https://help.openai.com/en/articles/12584461-connecting-custom-mcp-servers-in-chatgpt). Call the read-only `list_platforms` tool to verify.

Saved Runtime API Keys use a masked placeholder; leave the field blank to reuse the saved key. Tunnel configuration and runtime state reside in `mcp/` under the existing SyndiRelay data directory (`Data/mcp/` for Windows installations); old Tunnel settings are never imported. Tunnel lifecycle management is implemented in Go, without the old PowerShell/Bash service scripts. Tunnel readiness does not prove ChatGPT plugin authorization.

The CLI manages the same runtime with `syndirelay mcp status|start|stop|restart|connect|disconnect`. `syndirelay mcp configure` reads JSON with `tunnel_id` and `runtime_api_key` from stdin, never from process arguments.

**Other local MCP clients** do not need a Tunnel. Configure the installed `bin/syndirelay` (`bin/syndirelay.exe` on Windows) as the executable and `mcp-stdio` as the argument, then call `list_platforms`.

### 2.5. Troubleshooting

If the extension cannot connect, check that the browser and native program run on the **same operating system**, the extension is loaded, and the Bridge is reachable. In a source checkout, use `./syndirelay install` to register Native Messaging or `./syndirelay --bridge` to debug the local Bridge.

If an installed preview fails, verify the paths and bundled renderer. Mermaid rendering uses an existing Edge/Chrome browser and does not download Chromium. For source development, run `npm ci` in the source checkout and verify the content path. If the Tunnel is not ready, check the ID, key, network, and service status. If the Tunnel is ready but ChatGPT cannot call tools, ensure the plugin is enabled for the current conversation.

Do not post credentials, cookies, private settings, or unredacted logs in public Issues.

## 3. Development

Installer users do not need this section. Source development needs Git, Go (see [go.mod](go.mod)), Node.js 22+, and npm:

~~~bash
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
go test ./...
npm run test:renderers
npm run test:extension
go build -o syndirelay ./cmd
~~~

On Windows, build with `go build -o syndirelay.exe ./cmd`; Windows browser integration does not require WSL. Run `./syndirelay install` (Windows: `.\syndirelay.exe install`) to register Native Messaging, then load `extension/` from the checkout.

Changes go through PR Review and Squash Merge. [CI](.github/workflows/ci.yml) tests Go, renderers, the extension, and installer preparation on Windows, Linux, and macOS. Real browser sign-in, ChatGPT authorization, and remote publishing require separate integration checks; CI never silently publishes articles.

## 4. License

Original source is licensed under [MIT](LICENSE). Dependencies retain their own licenses; see [third-party notices](THIRD_PARTY_NOTICES.md) and [extension notices](extension/THIRD_PARTY_NOTICES.md). This license does not grant rights to third-party websites, user articles, or photographs.

Distribution packages must not include application `Data`, platform credentials, cookies, Tunnel keys, private logs, or publishing records. Creating a remote draft sends the selected content to the platform and requires an authorized account.

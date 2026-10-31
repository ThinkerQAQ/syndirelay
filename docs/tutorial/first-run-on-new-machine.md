# Set up a BlogCTL workspace from source

This walkthrough starts with an engine checkout, runs the sample site, then connects BlogCTL to a browser. You can complete installation without a publishing account. For prebuilt executables, use [installation](../quick-start.md).

## 1. Get the source and check tools

Install Git, Node.js 22.12+, npm and Go 1.27.1 or a compatible newer toolchain.

```bash
git clone https://github.com/ThinkerQAQ/ThinkerQAQ.github.io.git
cd ThinkerQAQ.github.io
git --version
go version
node --version
npm --version
go run ./tools/blogctl/cmd doctor
npm ci
npm run dev:quick
```

Open [localhost:4321](http://localhost:4321). The sample content is fetched from `inkerio-content-template` beside the engine. In a second terminal, run `npm run check` from the engine root. Stop the development server with **Ctrl+C**.

## 2. Build and register BlogCTL

Bash on Linux/macOS:

```bash
go run ./tools/blogctl/cmd/maint build
.build/out/blogctl install
```

Windows PowerShell, from a Windows checkout with Go installed:

```powershell
go build -o blogctl-windows-amd64.exe ./tools/blogctl/cmd
& .\blogctl-windows-amd64.exe install
```

Use the executable for the OS running the browser. A Linux build in WSL is not a Windows Native Messaging executable. For Edge on Windows, build/download a Windows executable and use Windows-accessible engine/content paths. The [installation guide](../quick-start.md) covers stable install locations and upgrades.

## 3. Load the matching browser Extension

Open the browser's extensions page, enable developer mode and load `tools/blogctl/extension/` from this checkout. Open BlogCTL, confirm the Bridge is connected, then choose **Open workspace**.

Visit `http://127.0.0.1:32145/v1/health` or query it with `curl`/PowerShell. Confirm the response and the Extension indicator; HTTP alone does not verify the browser session relay.

## 4. Select content and language

In **Settings**, fill in Engine Root and Content Root using paths accessible to the executable. For a writing workspace, clone your own content repository as described in the [Inkerio tutorial](../../../../docs/tutorial/first-site.md). A demo checkout can be selected directly for practice.

Choose **Interface & language → Follow browser / Simplified Chinese / English**. This sets `ui_locale`; each publishing platform has its own article-language setting. Save, reopen Settings and confirm both paths and language.

## 5. Inspect and create an article

In **Detect**, choose a signed-in platform and inspect drafts/published articles. On **Create**, select a local article and platforms. Click **Detect** when you want association results; creation does not launch it automatically, and detection is not a prerequisite for the create button.

For an existing article use **Update**, inspect its link and select its remote ID. For a new article, choose **Create draft** only when you intend to write to that account. Inspect **Tasks** and the returned editor link. Follow [first draft](first-publish.md) for a local compile check and the complete write/readback sequence.

## Interface examples

These screenshots were captured on 2026-10-10 and illustrate navigation. Current button labels and manual detection behavior are described above; the screenshots predate later publishing changes.

![Detect page](../assets/01-detect-en.webp)

![Create page](../assets/02-create-en.webp)

![Tasks page](../assets/03-tasks-en.webp)

## If the setup fails

| Symptom | Check |
| --- | --- |
| Browser cannot find the native host | Correct OS binary, permanent path, and `install` registration |
| Console opens but actions fail | Extension enabled and connected in that browser profile |
| No local articles | Content Root and article status/language |
| WSL command and Windows Console show different drafts | Both processes must use the same accessible `BLOGCTL_DATA_DIR` |
| Task timed out after allocating a draft | Open its saved ID before a new create attempt |

See [Bridge troubleshooting](../how-to/debug-bridge.md) and [configuration paths](../reference/configuration.md).

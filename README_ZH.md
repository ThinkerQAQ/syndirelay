# SyndiRelay

[English](README.md)

- [1. Introduction](#1-introduction)
- [2. Quick Start](#2-quick-start)
- [3. Configuration](#3-configuration)
- [4. Optional Features](#4-optional-features)
- [5. Development](#5-development)
- [6. License](#6-license)

## 1. Introduction

SyndiRelay 将 Markdown 转换为不同平台的文章格式，帮助作者从本地工作区分发内容。它提供 CLI、带本地 Bridge 的浏览器扩展，以及用于读取文章、预览结果和管理发布任务的 MCP 服务。

编译器支持 11 种平台格式：`51cto`、`cnblogs`、`csdn`、`devto`、`hashnode`、`juejin`、`medium`、`oschina`、`segmentfault`、`toutiao`、`zhihu`。远程操作取决于对应适配器与账号状态；支持格式转换不代表支持该平台的所有发布操作。

SyndiRelay 可以直接处理普通 Markdown 文件。使用 [Inkerio](https://github.com/ThinkerQAQ/inkerio) 的作者，也可以复用已有内容仓库；博客引擎不是必需依赖。

## 2. Quick Start

将一篇 Markdown 编译为适用于 DEV 的内容，无需账号、浏览器扩展或配置文件。

### 2.1. 准备运行环境

- Windows、macOS 或 Linux。
- Git、Go（版本要求见 [go.mod](go.mod)）、Node.js **22+** 和 npm。
- 能够下载源码与依赖的网络环境。

### 2.2. 获取源码并构建

当前从源码构建。

```sh
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
```

Linux / macOS：

```sh
go build -o syndirelay ./cmd
```

Windows PowerShell：

```powershell
go build -o syndirelay.exe ./cmd
```

### 2.3. 编译第一篇文章

在刚才的 `syndirelay` 仓库目录，新建示例文章并编译为 DEV 格式。

Linux / macOS：

```sh
printf '# Hello SyndiRelay\n\nMy first article.\n' > example.md
./syndirelay compile --file ./example.md --platform devto
```

Windows PowerShell：

```powershell
Set-Content -Path example.md -Encoding utf8 -Value "# Hello SyndiRelay`n`nMy first article."
.\syndirelay.exe compile --file .\example.md --platform devto
```

### 2.4. 检查编译结果

命令应正常退出，并输出包含以下内容的 JSON：

- `"platform": "devto"`
- `"title": "Hello SyndiRelay"`
- `markdown` 中包含 `My first article.`
- `contentHash` 非空。

`compile` 只生成本地结果。JSON 中的 `published` 是源内容状态，不是远程发布结果。

### 2.5. 常见问题

- **找不到 Node 模块或渲染脚本：** 执行 `npm ci` 后，在仓库目录重试。在其他目录运行时，将 `SYNDIRELAY_HOME` 设为源码仓库的绝对路径。
- **不支持的平台：** 使用简介中列出的平台 ID。
- **输入无效：** 使用非空、UTF-8 编码且不超过 5 MiB 的 Markdown 文件；如有 YAML frontmatter，须保证格式有效。
- **图表需要资源地址：** 添加 `--asset-base-url https://your-cdn.example/`，为生成资源指定地址；编译本身不会上传资源。
- **缺少依赖：** 执行 `./syndirelay doctor`（PowerShell：`.\syndirelay.exe doctor`），检查 Node.js、npm 和 Git。

如果仍无法解决，请[提交 Issue](https://github.com/ThinkerQAQ/syndirelay/issues)，附上系统、执行命令和脱敏后的错误信息。

## 3. Configuration

单文件 `compile` 仅需 `--file` 和 `--platform`。可选参数包括 `--language en|zh-CN`、`--canonical-url`、`--asset-base-url`，两个 URL 参数均要求 HTTPS。执行 `./syndirelay compile --help` 查看完整参数。

Bridge 使用 `blogctl.toml` 保存设置：

- Windows：可执行文件旁的 `Data\blogctl.toml`。
- Linux：`$XDG_CONFIG_HOME/SyndiRelay/blogctl.toml`，默认是 `~/.config/SyndiRelay/blogctl.toml`。
- macOS：`~/Library/Application Support/SyndiRelay/blogctl.toml`。
- 如需覆盖数据目录，将 `SYNDIRELAY_DATA_DIR` 设为**绝对目录路径**。

使用内容仓库时，在上述位置创建或编辑 `blogctl.toml`，填写自己的绝对路径与站点地址；父目录不存在时先创建：

```toml
content_root = "/absolute/path/to/your-content"
engine_root = "/absolute/path/to/syndirelay"
ui_locale = "auto"

[publishing.site]
origin = "https://your-site.example"
```

Windows 路径可使用 TOML 字面量字符串，例如 `content_root = 'C:\sites\my-content'`。`ui_locale` 默认是 `auto`，也支持 `en`、`zh-CN`；`BLOGCTL_LANG` 可覆盖 CLI 界面语言。

CLI 选择仓库时，请按下文显式设置 `SYNDIRELAY_CONTENT_ROOT` 和 `SYNDIRELAY_HOME`。版权自动模式下，SyndiRelay 中显式配置的版权策略优先于内容仓库的 `src/content/site/copyright.json`。平台凭据、图片存储等可选设置可通过扩展设置页管理；更多字段见[配置类型](bridge/config.go)和[发布配置](publishing/config.go)。

## 4. Optional Features

### 4.1. Case1: 分发 Inkerio 内容仓库中的文章

通过 [Inkerio Content Template](https://github.com/ThinkerQAQ/inkerio-content-template) 创建的仓库，可以同时用于建站和 SyndiRelay 分发，无需再建第二个内容仓库。CLI 同步要求存在 `src/content/articles/` 和 `src/data/content-manifest.json`。

先完成第 2 节构建，并按第 3 节配置 `publishing.site.origin`。回到 SyndiRelay 源码仓库目录，指定内容仓库路径，预览一篇已有文章。将示例路径及 `your-article-slug` 替换为自己的值。

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

命令应正常退出，并输出所选文章在 `devto` 平台的编译结果。`--dry-run` 不修改发布状态。预览通过后，如需创建远程草稿，继续下一节通过浏览器扩展操作。

### 4.2. Case2: 通过浏览器扩展创建草稿

使用与 Edge 或 Chrome 浏览器位于同一操作系统的原生程序；如果已有内容仓库，可以跳过上一节的 Inkerio 专用预览。先完成 [2.2](#22-获取源码并构建)，并保持终端位于 SyndiRelay 源码仓库。Windows 不需要 WSL。

1. 执行 `./syndirelay install`（PowerShell：`.\syndirelay.exe install`）注册 Native Messaging Host。
2. 看到 `Native Messaging Host registered` 后，打开 `edge://extensions` 或 `chrome://extensions`，启用开发者模式，点击“加载解压缩的扩展”，选择仓库内的 `extension/` 目录。
3. 从浏览器工具栏的扩展菜单打开 SyndiRelay，确认 Bridge 已连接。默认地址为 `127.0.0.1:32146`；如需手动启动，执行 `./syndirelay --bridge`（PowerShell：`.\syndirelay.exe --bridge`），保持该终端运行。
4. 按第 3 节将 `content_root` 指向已有的受支持内容仓库，将 `engine_root` 指向 SyndiRelay 源码目录，并配置站点地址。若使用 Inkerio Content Template，仓库需包含 `src/content/articles/` 和 `src/data/content-manifest.json`。
5. 在同一浏览器登录目标平台，返回扩展选择文章和平台，先预览，再执行该平台支持的创建草稿操作。
6. 查看任务结果，打开返回的草稿链接，核对标题与正文。

注册后请保持可执行文件路径不变；移动后重新执行 `install`。如扩展无法连接，检查是否使用同一操作系统的安装，并在注册后重新加载扩展。远程写入要求平台已登录，且适配器支持所选操作。

### 4.3. Case3: 连接 MCP 客户端

先完成 [2.2](#22-获取源码并构建)。添加服务后重启或重新连接客户端，再调用工具。

对于支持 STDIO MCP 的客户端，使用可执行文件的绝对路径添加服务：

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

Windows 客户端使用原生程序，例如 `"command": "C:\\tools\\syndirelay\\syndirelay.exe"`。调用 `list_platforms`，参数为 `{}`，确认收到真实工具响应。文章工具还需要配置内容工作区；依赖浏览器的发布操作需要安装扩展。

### 4.4. Case4: 连接 ChatGPT

安装并打开 SyndiRelay 的浏览器扩展，在 **设置 → MCP 连接** 中完成以下操作：

1. 打开 [OpenAI Tunnels](https://platform.openai.com/settings/organization/tunnels) 创建 Tunnel，复制完整的 Tunnel ID。
2. 打开 [OpenAI API Keys](https://platform.openai.com/api-keys)，创建或复用具备 **Tunnels Read + Use** 权限的受限 Runtime API Key。该密钥仅供本机 Tunnel 使用，不需要每次新建。
3. 在 **1. 本地 Tunnel 服务** 输入 Tunnel ID 和 Runtime API Key，点击「保存配置」。官方 Tunnel Client 缺失时将自动安装并校验。保存过程不自动启动服务，也不修改开机自启设置。
4. 保存成功后点击「启动服务」。已启动时可停止或重启。顶部状态显示本机 Tunnel 是否就绪。
5. 在 **2. ChatGPT 插件** 点击链接打开 [ChatGPT 插件设置](https://chatgpt.com/settings/plugins-settings)，创建或启用 SyndiRelay 连接，然后调用 `list_platforms` 验证。

已经保存的 Tunnel ID 会显示在输入框中；Runtime API Key 以星号表示，输入框留空表示复用已保存的密钥。更换 Tunnel ID 或 Runtime API Key 后需再次保存，再重启服务使配置生效。密钥不写入仓库、日志或命令行参数。Windows 使用 DPAPI，Linux/macOS 将凭据存放在受限权限的用户配置目录。

**注意：** Tunnel 健康检查通过，只表示本机已连接控制平面，不等于 ChatGPT 插件已获得授权。标准 STDIO MCP 客户端不需要 Tunnel，参见 4.3 节。

原有的 `connect-chatgpt-mcp.ps1` / `connect-chatgpt-mcp.sh` 命令仍保留，可供手动连接使用。

[Reverfolio](https://github.com/ThinkerQAQ/reverfolio) 影响力统计为独立的可选集成，默认关闭。

## 5. Development

准备上述开发依赖，在仓库目录执行：

```sh
npm ci
go test ./...
npm run test:renderers
npm run test:extension
go build -o .build/syndirelay ./cmd
```

Linux/macOS Tunnel 脚本检查：`bash scripts/connect-chatgpt-mcp.test.sh`。Windows PowerShell：`.\scripts\connect-chatgpt-mcp.windows.test.ps1`。

[GitHub Actions](.github/workflows/ci.yml) 在 Linux、macOS 和 Windows 执行 Go 测试、渲染器与扩展测试、原生构建及 Tunnel 脚本检查。脚本检查使用替身，不连接真实 Tunnel。CI 不验证浏览器安装或真实平台账号发布，这些需要在对应系统手动验收。

## 6. License

原创源码采用 [MIT](LICENSE)。第三方组件保留各自许可证，详见[第三方声明](THIRD_PARTY_NOTICES.md)与[扩展声明](extension/THIRD_PARTY_NOTICES.md)。公开安装包仍待完成依赖许可证与原生安装验收。

源码许可不包含用户文章、示例媒体或第三方素材的使用权。请只发布有权使用的内容，并遵守目标平台的条款与授权要求；部分适配器使用未公开接口。

账号凭据、Cookie、本地配置及发布状态不得提交到 Git 或打入发行包。远程发布会将选定内容发送至目标平台。

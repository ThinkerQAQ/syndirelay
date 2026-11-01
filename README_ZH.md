# SyndiRelay

[English](README.md)

## 项目简介

SyndiRelay 将 Markdown 转换为不同平台的文章格式，帮助作者从本地工作区分发内容。它提供 CLI、带本地 Bridge 的浏览器扩展，以及用于读取文章、预览结果和管理发布任务的 MCP 服务。

编译器支持 11 种平台格式：`51cto`、`cnblogs`、`csdn`、`devto`、`hashnode`、`juejin`、`medium`、`oschina`、`segmentfault`、`toutiao`、`zhihu`。远程操作取决于对应适配器与账号状态；支持格式转换不代表支持该平台的所有发布操作。

SyndiRelay 可以直接处理普通 Markdown 文件。使用 [Inkerio](https://github.com/ThinkerQAQ/inkerio) 的作者，也可以复用已有内容仓库；博客引擎不是必需依赖。

## 快速开始

将一篇 Markdown 编译为适用于 DEV 的内容，无需账号、浏览器扩展或配置文件。

### 环境要求

- Windows、macOS 或 Linux。
- Git、Go **1.24.1+**（模块指定工具链 1.24.5）、Node.js **22+** 和 npm。
- 能够下载源码与依赖的网络环境。

### 安装

目前没有已发布的安装包，请从源码构建。

```sh
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
```

Linux / macOS：

```sh
go build -o syndirelay ./cmd
printf '# Hello SyndiRelay\n\nMy first article.\n' > example.md
./syndirelay compile --file ./example.md --platform devto
```

Windows PowerShell：

```powershell
go build -o syndirelay.exe ./cmd
Set-Content -Path example.md -Encoding utf8 -Value "# Hello SyndiRelay`n`nMy first article."
.\syndirelay.exe compile --file .\example.md --platform devto
```

请在仓库目录执行以上命令，便于编译器找到 Node 渲染脚本及已安装的依赖。

### 验收

命令应正常退出，并输出包含以下内容的 JSON：

- `"platform": "devto"`
- `"title": "Hello SyndiRelay"`
- `markdown` 中包含 `My first article.`
- `contentHash` 非空。

至此已完成一次本地转换。该命令不会创建远程草稿或发布文章。JSON 中的 `published` 表示源内容状态，不代表已在远程平台发布成功。

### 常见问题

- **找不到 Node 模块或渲染脚本：** 执行 `npm ci` 后，在仓库目录重试。在其他目录运行时，将 `SYNDIRELAY_HOME` 设为源码仓库的绝对路径。
- **不支持的平台：** 使用简介中列出的平台 ID。
- **输入无效：** 使用非空、UTF-8 编码且不超过 5 MiB 的 Markdown 文件；如有 YAML frontmatter，须保证格式有效。
- **图表需要资源地址：** 添加 `--asset-base-url https://your-cdn.example/`，为生成资源指定地址；编译本身不会上传资源。
- **缺少依赖：** 执行 `./syndirelay doctor`（PowerShell：`.\syndirelay.exe doctor`），检查 Node.js、npm 和 Git。

## 配置说明

单文件 `compile` 仅需 `--file` 和 `--platform`。可选参数包括 `--language en|zh-CN`、`--canonical-url`、`--asset-base-url`，两个 URL 参数均要求 HTTPS。执行 `./syndirelay compile --help` 查看完整参数。

Bridge 使用 `blogctl.toml` 保存设置：

- Windows：可执行文件旁的 `Data\blogctl.toml`。
- Linux：`$XDG_CONFIG_HOME/SyndiRelay/blogctl.toml`，默认是 `~/.config/SyndiRelay/blogctl.toml`。
- macOS：`~/Library/Application Support/SyndiRelay/blogctl.toml`。
- 如需覆盖数据目录，将 `SYNDIRELAY_DATA_DIR` 设为**绝对目录路径**。

使用内容仓库时，配置自己的路径与站点信息：

```toml
content_root = "/absolute/path/to/your-content"
engine_root = "/absolute/path/to/syndirelay"
ui_locale = "auto"

[publishing.site]
origin = "https://your-site.example"
```

Windows 路径可使用 TOML 字面量字符串，例如 `content_root = 'C:\sites\my-content'`。`ui_locale` 默认是 `auto`，也支持 `en`、`zh-CN`；`BLOGCTL_LANG` 可覆盖 CLI 界面语言。

CLI 选择仓库时，请按下文显式设置 `SYNDIRELAY_CONTENT_ROOT` 和 `SYNDIRELAY_HOME`。版权自动模式下，SyndiRelay 中显式配置的版权策略优先于内容仓库的 `src/content/site/copyright.json`。平台凭据、图片存储等可选设置可通过扩展设置页管理；更多字段见[配置类型](bridge/config.go)和[发布配置](publishing/config.go)。

## 可选功能

### 浏览器扩展与远程草稿

在浏览器所在的操作系统运行原生 SyndiRelay。Windows 不需要 WSL。

1. 执行 `./syndirelay install`（PowerShell：`.\syndirelay.exe install`）注册 Native Messaging Host。
2. 打开 `edge://extensions` 或 `chrome://extensions`，启用开发者模式，加载仓库内的 `extension/` 目录。
3. 打开扩展，确认 Bridge 已连接。默认地址为 `127.0.0.1:32146`；如需手动启动，执行 `./syndirelay --bridge`。
4. 配置内容工作区，在该浏览器登录目标平台。先预览选定文章，再选择可用的草稿或发布操作。
5. 查看任务结果，打开返回的平台链接，确认实际草稿或文章内容。

注册后请保持可执行文件路径不变；移动后重新执行 `install`。如扩展无法连接，检查是否使用同一操作系统的安装，并在注册后重新加载扩展。远程写入要求平台已登录，且适配器支持所选操作。

### 复用 Inkerio 内容仓库

通过 [Inkerio Content Template](https://github.com/ThinkerQAQ/inkerio-content-template) 创建的仓库，可以同时用于建站和 SyndiRelay 分发，无需再建第二个内容仓库。CLI 同步要求存在 `src/content/articles/` 和 `src/data/content-manifest.json`。

配置好 `publishing.site.origin` 后，指定源码与内容仓库路径，预览一篇已有文章。将示例路径及 `your-article-slug` 替换为自己的值。

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

Dry Run 编译预览且不修改发布状态。检查输出后，再执行远程创建或更新。

### MCP 客户端与 ChatGPT

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

连接 ChatGPT 时，额外安装对应系统的官方 [tunnel-client](https://github.com/openai/tunnel-client/releases/latest)，通过 `CONTROL_PLANE_API_KEY` 或 `SYNDIRELAY_RUNTIME_KEY_REF`（`env:NAME` 或 `file:ABSOLUTE_PATH`）配置已授权的 Tunnel 运行凭据。Linux/macOS 的凭据文件权限须为 `0600`。已有授权凭据可以复用。

Linux / macOS，在仓库目录执行：

```sh
export SYNDIRELAY_MCP_COMMAND="$PWD/syndirelay mcp-stdio"
./scripts/connect-chatgpt-mcp.sh tunnel_YOUR_TUNNEL_ID
./scripts/connect-chatgpt-mcp.sh tunnel_YOUR_TUNNEL_ID --status
```

Windows PowerShell：

```powershell
.\scripts\connect-chatgpt-mcp.ps1 -TunnelId tunnel_YOUR_TUNNEL_ID
.\scripts\connect-chatgpt-mcp.ps1 -TunnelId tunnel_YOUR_TUNNEL_ID -Status
```

在 ChatGPT 添加该 Tunnel，并调用 `list_platforms`。仅 Tunnel 状态正常，还不能证明 ChatGPT 已连通。Bridge 保持本地监听。

[Reverfolio](https://github.com/ThinkerQAQ/reverfolio) 影响力统计为独立的可选集成，默认关闭。

## 开发与验证

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

## 许可证与合规

原创源码采用 [MIT](LICENSE)。第三方组件保留各自许可证，详见[第三方声明](THIRD_PARTY_NOTICES.md)与[扩展声明](extension/THIRD_PARTY_NOTICES.md)。公开安装包仍待完成依赖许可证与原生安装验收。

源码许可不包含用户文章、示例媒体或第三方素材的使用权。请只发布有权使用的内容，并遵守目标平台的条款与授权要求；部分适配器使用未公开接口。

账号凭据、Cookie、本地配置及发布状态不得提交到 Git 或打入发行包。远程发布会将选定内容发送至目标平台。

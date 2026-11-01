# SyndiRelay

[English](README.md) · [MIT 许可证](LICENSE) · [第三方许可声明](THIRD_PARTY_NOTICES.md)

**SyndiRelay 是独立的 Markdown 编译与多平台内容分发工具。** 它由 BlogCTL 拆分而来，不再要求安装 Inkerio/Astro 引擎，也不依赖 ThinkerQAQ 的个人网站。主要提供单文件 Markdown 编译、可选的内容仓库分发、本地浏览器 Bridge/Extension、STDIO MCP，以及可选的 Reverfolio 影响力统计对接。

**公开状态：SyndiRelay 源码以 MIT 许可证公开。** 二进制与浏览器扩展 Release 尚未完成验收；历史 BlogCTL Release 已退役。源码公开和安装包发布是两项独立的决策，公开安装包仍需完成第三方依赖许可证、真实平台授权及原生安装验收。

## 功能与边界

| 能力 | 必要条件 | 边界 |
| --- | --- | --- |
| 将 Markdown 编译为 11 种平台格式 | 本地文件、Go、Node.js | 仅本地生成预览，不发送网络写入 |
| 预览内容仓库分发 | 指定内容目录、个人站点及版权策略 | `--dry-run` 不修改远程文章或本地发布状态 |
| 创建、更新、发布远程文章 | 平台授权、操作支持、必要时连接本地浏览器 | 用户明确选择操作及目标，不能假定所有平台都支持 |
| Bridge、Web Console、浏览器扩展 | 与浏览器系统匹配的本机可执行文件 | Bridge 仅监听 `127.0.0.1:32146`，Cookie 和密钥保留本机 |
| STDIO MCP | 本地可执行文件及 Bridge | 只暴露命名工具，不开放整个 Bridge 到公网 |
| 作品传播统计 | 另行连接 Reverfolio | 默认关闭，独立保存数据 |
| 搜索通知与 AI Search | 可选的站点 CI 与服务商凭据 | 服务端执行，不要求安装网站引擎 |

11 种**文章编译格式**：`51cto`、`cnblogs`、`csdn`、`devto`、`hashnode`、`juejin`、`medium`、`oschina`、`segmentfault`、`toutiao`、`zhihu`。**支持编译某个平台，不代表获得其官方 API 授权或已验证远程发布。** 对非公开接口和浏览器登录态自动化必须另行核对平台规则与使用权限。

## 本地编译快速开始

准备 Go **1.24+**、Node.js **22+**、npm 和 Git。

```sh
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
go build -o syndirelay ./cmd
printf '# 示例文章\n\n这是我自己的原创内容。\n' > example.md
./syndirelay compile --file ./example.md --platform devto
```

此流程不会登录、上传图片、创建草稿或发布。带图表的分发可额外指定自己的 `--asset-base-url https://your-cdn.example/`。从内容仓库分发时，还要显式配置自己的站点 Canonical 域名和版权政策；默认不使用软件作者的站点或作品署名。

Windows PowerShell 使用 `go build -o syndirelay.exe ./cmd`，自行创建 `.md` 文件，再执行 `./syndirelay.exe compile --file ./example.md --platform devto`。

## 仓库边界

- `compiler/`、`publisher/`、`publishing/`、`platform/`、`renderers/`：编译、平台适配与发布前校验。
- `bridge/`、`extension/`、`ui/`、`cmd/`：本地 Bridge、浏览器扩展、Web UI 和 MCP。
- `search/`、`aisearch/` 与旧 `site` 命令：仅为旧部署和站点 CI 保留的兼容功能；正式静态网站由 **Inkerio** 构建。
- `Data/`、`SYNDIRELAY_DATA_DIR`：本机私有数据库、授权状态和发布绑定，禁止写入 Git 或安装包。
- Reverfolio：可选独立作品影响力统计，不默认上传文章正文或凭据。

[Inkerio](https://github.com/ThinkerQAQ/inkerio) 负责通用静态博客引擎，[inkerio-content-template](https://github.com/ThinkerQAQ/inkerio-content-template) 负责公开样例，[ThinkerQAQ.github.io](https://github.com/ThinkerQAQ/ThinkerQAQ.github.io) 只负责个人网站配置与部署。SyndiRelay 的单文件编译不依赖上述仓库。

ChatGPT 的 **Windows 原生 PowerShell / Linux / macOS Secure MCP Tunnel 安装步骤**（见下方）现已区分 **Windows 原生 PowerShell**、Linux 和 macOS 三套流程。**Windows 不要求 WSL/Ubuntu**；已获授权的 Tunnel 运行凭据可以复用，不需要为 SyndiRelay 新申请一次性密钥。MCP、Tunnel 和 ChatGPT 插件连通性须分别验收，禁止将凭据写入 Git。

## 可选的 Bridge、浏览器扩展与 ChatGPT MCP

SyndiRelay 的 Bridge 应当运行在**正在登录浏览器的同一个操作系统**上。Windows 使用 PowerShell 编译 `go build -o syndirelay.exe ./cmd`，**不要求 WSL/Ubuntu**。运行 `./syndirelay install`（Windows：`.\syndirelay.exe install`）注册 Native Messaging，在 Edge/Chrome 扩展管理页面加载仓库内的 `extension/` 文件夹。Bridge 默认监听 `127.0.0.1:32146`，不是旧 BlogCTL 的 32145。Cookie、凭据及 `Data/` 保存在本机，不向公网开放 Bridge。

本地 MCP 使用 STDIO 启动：

```sh
./syndirelay mcp-stdio
# Windows：.\syndirelay.exe mcp-stdio
```

要连接 ChatGPT，下载与当前系统匹配的 [tunnel-client 官方版本](https://github.com/openai/tunnel-client/releases/latest)，复用已有的、受授权的私有 Tunnel 运行凭据，**不必为每个应用创建 One-Time API Key**。Tunnel ID 只是标识符，不是密钥。

```powershell
# Windows PowerShell
.\scripts\connect-chatgpt-mcp.ps1 -TunnelId tunnel_YOUR_TUNNEL_ID
.\scripts\connect-chatgpt-mcp.ps1 -TunnelId tunnel_YOUR_TUNNEL_ID -Status
```

```sh
# Linux/macOS：在本机启动对应原生客户端
./scripts/connect-chatgpt-mcp.sh tunnel_YOUR_TUNNEL_ID
./scripts/connect-chatgpt-mcp.sh tunnel_YOUR_TUNNEL_ID --status
```

之后在 ChatGPT 里添加自定义 MCP Tunnel，先调用只读工具。**本机 MCP 启动、Tunnel 健康、ChatGPT 插件调用成功是三次独立验收**，不可直接把整个 Bridge HTTP 服务暴露到公网，也不可公开 Cookie、令牌或授权头。

复用博客内容仓库时，在本地私有 `Data/blogctl.toml` 配置 `content_root` 与 `[publishing.site].origin`。在版权自动模式下，SyndiRelay TOML 中显式配置的版权内容优先于仓库内的 `src/content/site/copyright.json`。远程创建、更新或发布之前，应先预览 / Dry Run，并确认平台授权和目标账号。

## 可选：复用 Inkerio 的内容仓库

如果通过 [Inkerio Content Template](https://github.com/ThinkerQAQ/inkerio-content-template) 创建了博客，**这就是现成的内容仓库**。只需将 SyndiRelay 的本地工作区 `content_root` 指向这个仓库，并在 `[publishing.site].origin` 配置自己的站点地址；无需再创建第二个 `blog-content`，也不需要让 SyndiRelay 依赖 Inkerio 引擎。

例如，同一个 `alice.github.io` 仓库既可以由 Inkerio 构建为 GitHub Pages 网站，也可以在用户明确授权后由 SyndiRelay 读取并分发。域名与版权始终由用户管理：`mode = "auto"` 时优先使用 SyndiRelay 本地显式配置的版权声明，未配置时才读取内容仓库的 `src/content/site/copyright.json`。建站流程统一见 [Inkerio README](https://github.com/ThinkerQAQ/inkerio)。

## 开发与测试

```sh
npm ci
go test ./...
npm run test:renderers
npm run test:extension
```

GitHub CI 已覆盖 **Windows、macOS、Linux** 的测试和原生编译；这不等于真实平台账号发布、浏览器安装、三系统整包运行或依赖许可证验收全部通过。公开二进制及扩展之前仍需完成第三方依赖许可证、平台授权、浏览器安装与三系统实机验收；CI 编译通过不等于发行验收通过。

**版权边界：** SyndiRelay 原创源码采用 MIT，不改变第三方依赖许可证，也不授权他人转载用户文章、图片或访问第三方服务。项目与所支持的内容平台没有官方从属或背书关系。

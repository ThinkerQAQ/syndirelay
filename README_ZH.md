# SyndiRelay

[English](README.md) · [中文文档](docs/zh-CN/index.md) · [快速开始](docs/zh-CN/quick-start.md) · [MIT 许可证](LICENSE) · [第三方许可声明](THIRD_PARTY_NOTICES.md)

**SyndiRelay 是独立的 Markdown 编译与多平台内容分发工具。** 它由 BlogCTL 拆分而来，不再要求安装 Inkerio/Astro 引擎，也不依赖 ThinkerQAQ 的个人网站。主要提供单文件 Markdown 编译、可选的内容仓库分发、本地浏览器 Bridge/Extension、STDIO MCP，以及可选的 Reverfolio 影响力统计对接。

**公开状态：SyndiRelay 源码以 MIT 许可证公开。** 二进制与浏览器扩展 Release 尚未完成验收；历史 BlogCTL Release 已退役。源码公开和安装包发布是两项独立的决策，详见开源及发布审核文档。

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
- [Reverfolio](docs/zh-CN/reverfolio.md)：可选独立作品影响力统计，不默认上传文章正文或凭据。

[Inkerio](https://github.com/ThinkerQAQ/inkerio) 负责通用静态博客引擎，[inkerio-content-template](https://github.com/ThinkerQAQ/inkerio-content-template) 负责公开样例，[ThinkerQAQ.github.io](https://github.com/ThinkerQAQ/ThinkerQAQ.github.io) 只负责个人网站配置与部署。SyndiRelay 的单文件编译不依赖上述仓库。

ChatGPT 的[专用 Tunnel 安装说明](docs/MCP.md#connect-to-chatgpt-through-a-dedicated-secure-mcp-tunnel)现已区分 **Windows 原生 PowerShell**、Linux 和 macOS 三套流程。**Windows 不要求 WSL/Ubuntu**；已获授权的 Tunnel 运行凭据可以复用，不需要为 SyndiRelay 新申请一次性密钥。MCP、Tunnel 和 ChatGPT 插件连通性须分别验收，禁止将凭据写入 Git。

## 开发与测试

```sh
npm ci
go test ./...
npm run test:renderers
npm run test:extension
```

GitHub CI 已覆盖 **Windows、macOS、Linux** 的测试和原生编译；这不等于真实平台账号发布、浏览器安装、三系统整包运行或依赖许可证验收全部通过。公开二进制/扩展仍需完成[发布许可、平台授权和安装验收](docs/compliance/PUBLICATION_READINESS.md)。

**版权边界：** SyndiRelay 原创源码采用 MIT，不改变第三方依赖许可证，也不授权他人转载用户文章、图片或访问第三方服务。项目与所支持的内容平台没有官方从属或背书关系。

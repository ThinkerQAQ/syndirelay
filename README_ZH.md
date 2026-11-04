# SyndiRelay

[English](README.md)

- [1. Introduction](#1-introduction)
- [2. Quick Start](#2-quick-start)
- [3. Development](#3-development)
- [4. License](#4-license)

## 1. Introduction

我开发 SyndiRelay，是因为一篇 Markdown 文章要分发到多个平台时，总得反复复制正文、调整格式、处理图片，再逐个平台创建草稿。

不同平台对 Markdown、HTML、图片以及草稿编辑的要求不同。SyndiRelay 保留本地原始文章，分别生成平台需要的内容格式，并通过浏览器扩展操作已登录的账号。先预览，再创建草稿；**编译文章本身不会发布到远程平台**。

目前支持 DEV、Medium、知乎、掘金、头条、CSDN、博客园、Hashnode、51CTO、开源中国和 SegmentFault 共 11 种输出格式。具体能否创建或更新草稿，取决于对应适配器和平台。支持编译格式不等于支持全部发布操作。

项目提供 Windows、Linux、macOS 原生安装器、浏览器扩展、命令行与 MCP 工具。使用 OpenAI Secure MCP Tunnel，可以把本机 MCP 接入网页版 ChatGPT，无需将 Bridge 暴露到公网。本地 MCP 客户端也可以直接使用 STDIO。普通 Markdown 不依赖 [Inkerio](https://github.com/ThinkerQAQ/inkerio)，已有 Inkerio 内容仓库则可以直接复用。

## 2. Quick Start

先安装 SyndiRelay，再连接扩展并预览文章。需要在 ChatGPT 中调用工具时，再配置 Tunnel。**当前安装包仍在验收，尚未公开发布 Release**。正式发布前，可使用本地测试安装包，或按第 3 节从源码运行。

### 2.1. 安装

#### 2.1.1. 图形界面

**Windows：**运行 `SyndiRelay-Setup-*-windows-amd64.exe`，按向导完成安装。从开始菜单打开 **SyndiRelay**，程序会注册 Native Messaging Host，并打开扩展目录和浏览器扩展管理页面。

**macOS：**打开对应平台的安装包，完成安装后选择 **Open SyndiRelay**，打开扩展目录。

**Linux：**在图形桌面中运行 `.run` 安装器。如果没有执行权限，先在文件属性中允许作为程序运行。安装完成时选择 **Open SyndiRelay**。

在 Edge 的 `edge://extensions` 或 Chrome 的 `chrome://extensions` 中启用开发者模式，点击**加载解压缩的扩展**，选择安装目录下的 `extension/`。浏览器仍需手动确认扩展授权；安装器不能绕过这一步。

#### 2.1.2. 命令行

安装器也支持 Qt Installer Framework 的无人值守参数。Windows PowerShell 在下载目录运行：

~~~powershell
$installer = (Get-ChildItem .\SyndiRelay-Setup-*-windows-amd64.exe | Select-Object -First 1).FullName
Start-Process -FilePath $installer -ArgumentList '--accept-licenses --default-answer --confirm-command install' -Wait
~~~

Linux 图形桌面：

~~~bash
chmod +x ./SyndiRelay-Setup-*-linux-amd64.run
QT_QPA_PLATFORM=offscreen ./SyndiRelay-Setup-*-linux-amd64.run --accept-licenses --default-answer --confirm-command install
~~~

无人值守安装后仍需手动加载浏览器扩展。源码构建与开发命令见第 3 节。

### 2.2. 配置

#### 2.2.1. 图形界面

从浏览器工具栏打开 SyndiRelay，在**设置**中选择内容仓库、站点地址、平台凭据和可选的图片存储。Bridge 仅监听本机 `127.0.0.1:32146`，Native Messaging Host 会在需要时启动它。

如果使用 [Inkerio Content Template](https://github.com/ThinkerQAQ/inkerio-content-template)，选择已有内容仓库即可，无需复制第二份。创建远程草稿前，需要在同一个浏览器中登录目标平台。

#### 2.2.2. 配置文件

如果要通过命令行自动化，可以编辑私有 `syndirelay.toml`。Windows 位于安装目录的 `Data/syndirelay.toml`；Linux/macOS 使用系统用户配置目录。可通过绝对路径环境变量 `SYNDIRELAY_DATA_DIR` 覆盖数据目录。

已有内容仓库的最小配置：

~~~toml
content_root = "/absolute/path/to/your-content"
engine_root = "/absolute/path/to/syndirelay"

[publishing.site]
origin = "https://your-site.example"
~~~

Windows 路径可使用 TOML 单引号字符串。独立的 `compile` 命令不需要配置文件。源码 CLI 使用 `SYNDIRELAY_HOME` 指向源码目录、`SYNDIRELAY_CONTENT_ROOT` 指向内容仓库。完整字段见 [Bridge 配置](bridge/config.go)和[发布配置](publishing/config.go)。

### 2.3. 验证文章

在扩展中选择文章和平台，先预览生成内容。如果适配器支持创建草稿，再执行**创建草稿**，打开任务返回的链接，核对标题、正文和图片。

也可以不登录任何账号，直接在[源码目录](#3-development)测试本地编译：

~~~bash
printf '# Hello SyndiRelay\n\nMy first article.\n' > example.md
./syndirelay compile --file ./example.md --platform devto
~~~

Windows PowerShell：

~~~powershell
Set-Content -Path example.md -Encoding utf8 -Value "# Hello SyndiRelay`n`nMy first article."
.\syndirelay.exe compile --file .\example.md --platform devto
~~~

命令返回 JSON，应包含 `platform: "devto"`、标题 `Hello SyndiRelay` 和非空的 `contentHash`。这只是本地编译，**不会创建或发布远程文章**。

已有 Inkerio 内容仓库可以先执行只读预览：

~~~bash
export SYNDIRELAY_HOME="$PWD"
export SYNDIRELAY_CONTENT_ROOT="/absolute/path/to/your-content"
./syndirelay sync --article your-article-slug --platforms devto --dry-run
~~~

### 2.4. 连接 ChatGPT

先从 [OpenAI Tunnel 设置](https://platform.openai.com/settings/organization/tunnels)创建 Tunnel ID，在 [API Keys](https://platform.openai.com/settings/organization/api-keys)中创建受限 Runtime API Key。

在浏览器扩展中打开**设置 → 运行环境 → MCP 连接**。新版 MCP 使用独立配置，不读取、迁移旧 Tunnel ID、Key 或脚本配置；重新填写即可，文章和发布记录不受影响：

1. 在**本地 Tunnel 服务**中填写 Tunnel ID 和 Runtime API Key，点击**保存配置**。系统会校验官方 Tunnel Client，但不会启动服务。
2. 点击**启动服务**，等待状态显示 **Ready**。修改配置后使用**重启服务**；即使 Tunnel 进程运行但健康检查失败，也可以停止。
3. **开机自启 Tunnel** 控制本地服务是否随系统登录启动；**Tunnel 自动连接** 决定服务启动后是否立即建立 Tunnel。两项独立设置，修改自动连接后重启服务生效。服务运行期间也可手动点击**连接 Tunnel**或**断开 Tunnel**，不会停止本地服务。
4. 在**ChatGPT 插件**模块打开 [ChatGPT Plugins](https://chatgpt.com/plugins)，用相同的 Tunnel ID 注册并启用插件。详细流程见 [OpenAI 自定义 MCP 连接指南](https://help.openai.com/en/articles/12584461-connecting-custom-mcp-servers-in-chatgpt)，然后调用只读工具 `list_platforms` 验证。

已保存的 Runtime API Key 显示星号占位，留空即可复用。Tunnel 配置与运行状态统一保存在 SyndiRelay 数据目录的 `mcp/` 下（Windows 安装版为 `Data/mcp/`）；不读取旧 Tunnel 配置。Tunnel 生命周期由 Go 负责，不再经由 PowerShell/Bash 服务脚本。Tunnel 已就绪不代表 ChatGPT 插件已授权。

命令行可使用 `syndirelay mcp status|start|stop|restart|connect|disconnect` 管理同一服务；`syndirelay mcp configure` 从标准输入读取 JSON（`tunnel_id` 和 `runtime_api_key`），不通过命令参数传密钥。

**其他本地 MCP 客户端**不需要 Tunnel。把安装目录的 `bin/syndirelay`（Windows 为 `bin/syndirelay.exe`）作为程序，参数设置为 `mcp-stdio`，再调用 `list_platforms` 验证。

### 2.5. 故障排查

扩展无法连接时，检查浏览器和程序是否在**同一个操作系统**、扩展是否已加载、Bridge 是否可访问。源码模式可执行 `./syndirelay install` 注册 Native Messaging，使用 `./syndirelay --bridge` 排查本地服务。

文章预览失败时，在源码目录执行 `npm ci` 并检查内容路径。Tunnel 无法连接时检查 ID、密钥、网络和服务状态。Tunnel 已就绪但 ChatGPT 无法调用时，检查插件是否已在当前会话启用。

不要在 Issue 中公开配置、凭据、Cookie 或未脱敏日志。

## 3. Development

如果只使用安装包，不需要本节。源码开发需要 Git、Go（版本见 [go.mod](go.mod)）、Node.js 22+ 和 npm：

~~~bash
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
go test ./...
npm run test:renderers
npm run test:extension
go build -o syndirelay ./cmd
~~~

Windows 改用 `go build -o syndirelay.exe ./cmd`，原生浏览器集成不需要 WSL。运行 `./syndirelay install`（Windows：`.\syndirelay.exe install`）注册 Native Messaging，再加载源码 `extension/`。

代码通过 PR Review 和 Squash Merge 合入。[CI](.github/workflows/ci.yml) 在 Windows、Linux、macOS 验证 Go、渲染器、扩展和安装器准备流程。真实浏览器登录、ChatGPT 授权及远程发布需要单独验收，CI 不会自动发布文章。

## 4. License

原创源码采用 [MIT](LICENSE)。第三方依赖保留其许可证，见[第三方声明](THIRD_PARTY_NOTICES.md)与[扩展声明](extension/THIRD_PARTY_NOTICES.md)。源码许可不包含第三方网站、用户文章和摄影图片的使用权。

打包时不得包含应用 `Data` 目录、平台凭据、Cookie、Tunnel Key、私人日志和发布记录。远程创建草稿会向目标平台发送所选内容，必须使用已授权账号。

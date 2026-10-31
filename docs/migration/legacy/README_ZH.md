# BlogCTL

> **平台合规说明：** 浏览器已登录不等于获得非公开接口自动发布授权。Medium 已停止发放新的 API 集成 Token，且限制脚本化和非公开接口访问；无平台明确授权时，请通过 Medium 官方网页导入或手动发布。去除 TLS 指纹模拟并不等于所有平台发布接口已经合规。

[English](README.md) · [中文文档](docs/zh-CN/index.md) · [Releases](https://github.com/ThinkerQAQ/ThinkerQAQ.github.io/releases/latest)

BlogCTL 是 Inkerio 配套的 Go 应用，负责内容装配、面向发布平台的文章编译、搜索引擎提交，以及供浏览器扩展和工作台使用的本地 Bridge。

扩展访问当前浏览器的登录会话，Bridge 执行本地任务。各平台的能力不同：创建草稿、修改草稿、更新已发布文章需要分别判断。

## 首次使用

在引擎仓库根目录执行：

```bash
go run ./tools/blogctl/cmd help
go run ./tools/blogctl/cmd doctor
```

桌面安装时，优先从 Releases 下载匹配系统与架构的 **ZIP 完整包**（可执行文件与 MIT LICENSE）及扩展 ZIP，并按 `SHA256SUMS` 校验；旧用户仍可下载单独的可执行文件，解压到固定目录，运行程序的 `install` 命令，然后在 Edge 或 Chrome 加载解压后的扩展。Windows 示例：

```powershell
& C:\software\Coding\blogctl\blogctl-windows-amd64.exe install
```

升级时保留相邻的 `Data/` 目录。其他系统的数据位置见[快速开始](docs/zh-CN/quick-start.md)，界面截图见[新机器安装教程](docs/zh-CN/tutorial/first-run-on-new-machine.md)。

## 文章分发

在工作台选择本地文章和平台。已有文章检测是可选操作，只在主动点击时执行；检测命中的平台会从批量创建中排除，检测失败不会阻止创建。创建任务失败后，先检查远端是否已有草稿，再决定是否重试。

更新必须指定远端文章或草稿 ID。Medium 已发布文章的修改会保存在编辑器中，需要手动发布。完整操作和重试规则见[工作流](docs/zh-CN/guide/workflows.md)。

通过 Buffer 发布短内容使用独立的[审核与发布流程](docs/zh-CN/guide/social-publishing.md)。

| 任务 | 中文 | English |
| --- | --- | --- |
| 安装与连接 | [快速开始](docs/zh-CN/quick-start.md) | [Quick Start](docs/quick-start.md) |
| 创建第一篇草稿 | [首次发布](docs/zh-CN/tutorial/first-publish.md) | [Tutorial](docs/tutorial/first-publish.md) |
| 配置路径和存储 | [配置参考](docs/zh-CN/reference/configuration.md) | [Configuration](docs/reference/configuration.md) |
| 排查工作台连接 | [Bridge 排查](docs/zh-CN/how-to/debug-bridge.md) | [Troubleshooting](docs/how-to/debug-bridge.md) |
| 查询 CLI 参数 | [命令行](docs/zh-CN/reference/cli.md) | [CLI](docs/reference/cli.md) |

## 开发

在引擎仓库根目录执行：

```bash
go run ./tools/blogctl/cmd/maint verify
go run ./tools/blogctl/cmd/maint build
go run ./tools/blogctl/cmd/maint package
```

构建产物位于 `.build/out/`。打包生成六个平台可执行文件、扩展 ZIP 和校验和。修改代码前请阅读[根目录规则](../../AGENTS.md)和 [BlogCTL 规则](AGENTS.md)。

[MIT License](../../LICENSE)

## MCP 集成

使用现有 Go Bridge 和浏览器扩展为 ChatGPT 提供文章查询、发布与任务查询。运行 `blogctl -mcp-stdio`，详见 [MCP 接入文档](docs/MCP.md)。不需要额外 Skill 或 SQLite。

MCP 新增 `upload_image_to_r2`：读取授权目录中的本地图片，自动纠正方向、清理 EXIF 等元数据后使用已有 R2 配置上传，返回带处理版本的 Markdown 地址。授权目录在本机 TOML 顶层 `image_upload_roots` 配置，详见 [MCP 图片上传](docs/MCP.md#private-image-upload-to-r2)。

## 合法使用与第三方平台声明

BlogCTL 是用于内容管理与多平台分发的开源工具，与所支持的第三方平台不存在官方关联。

使用者应确保对所操作的账号及发布内容拥有合法权限，并自行遵守适用法律及第三方平台的服务条款。不得将本项目用于未经授权的访问、侵犯他人权益或其他违法活动。

本项目依据 [MIT License](../../LICENSE) 开源。开源许可不代表获得任何第三方平台的官方授权。软件按现状提供，相关免责声明以 LICENSE 文件为准。

新增依赖许可打包检查目前被 fhttp v0.6.9 的授权缺口阻断，详见[第三方声明](../../THIRD_PARTY_NOTICES.md)。v0.1.193 已附项目 MIT 文件，但尚未通过这项新增传递依赖审查。

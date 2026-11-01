# 在新机器从源码安装 SyndiRelay

SyndiRelay 不依赖 Inkerio。**当前仓库仍为 Private，需要已经授权的 GitHub 账号才能克隆。** 公开安装包尚未完成许可证和安全验收。建议先只做本地 Markdown 编译，再按需配置浏览器与授权发布流程。

## 1. 环境与源码

安装 Git、Go 1.24+、Node.js 22+ 和 npm，在目标操作系统中执行：

```sh
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
go test ./...
go build -o syndirelay ./cmd
printf '# 我自己的文章\n\nSyndiRelay 示例。\n' > example.md
./syndirelay compile --file ./example.md --platform devto
```

Windows PowerShell 中用 `go build -o syndirelay.exe ./cmd` 编译，通过编辑器创建 `example.md`，再执行 `.\syndirelay.exe compile --file .\example.md --platform devto`。上述命令不会连接账号、创建草稿或发布文章。

渲染器和扩展测试执行 `npm run test:renderers && npm run test:extension`。

## 2. 可选的浏览器集成

选择**不同于原 BlogCTL** 的长期目录，放入当前系统的程序、`extension/`、`renderers/`、Node 运行依赖及自己的 `Data/`。通过 `SYNDIRELAY_HOME` 配置程序根目录；`SYNDIRELAY_DATA_DIR` 必须是本机私有绝对路径。不要将 Cookies、数据库或旧账号绑定上传 GitHub。

执行 `./syndirelay install`（Windows 使用 `.\syndirelay.exe install`），然后在 Edge/Chrome 扩展管理器中加载本仓库的 `extension/`。Native Messaging Host 为 `com.thinkerqaq.syndirelay`，本机 Bridge 监听 `127.0.0.1:32146`。这与旧 BlogCTL 的扩展、端口 `32145` 和安装目录不同。

不要再使用 `ThinkerQAQ.github.io/tools/blogctl/` 或旧 Release 的安装路径。Windows 浏览器需要 Windows Native Host，WSL 中编译的 Linux 程序不能直接获取 Windows 浏览器登录态。

## 3. 可选的内容仓库与远端发布

需要从内容仓库选择文章时，设置包含 `src/content/articles/` 的内容根目录，并显式填写自己的 `[publishing.site]` Canonical、站点名称和版权策略。详见[工作区配置](../how-to/configure-workspace.md)。单文件编译不需要内容仓库或博客引擎。

任何远端创建、更新、发布都需确认自己的平台账号拥有相应权限，选择对应草稿或文章 ID，操作超时时先检查任务和远端记录，防止重复发布。详细流程见[首篇草稿](first-publish.md)。

## 4. 可选的 MCP

运行 `./syndirelay mcp-stdio`，仅向 MCP 客户端连接受限的 STDIO 接口；不要将整个 Bridge HTTP 服务暴露到公网。浏览器扩展与 Bridge 应运行在同一个操作系统，详见 [MCP 文档](../../MCP.md)。

## 排查

| 情况 | 检查 |
| --- | --- |
| 无法克隆 | 仓库仍为 Private，需要 GitHub 访问权限 |
| 编译图表失败 | Node 渲染依赖是否安装、素材 URL 是否明确配置 |
| 找不到 Native Host | 程序系统、安装路径、`install` 是否匹配 |
| 扩展无法连接 | 当前版本扩展/Host、端口 32146、浏览器授权 |
| 没有文章 | 内容根目录、状态和语言 |
| 远端任务超时 | 优先核对已保存的任务和文章 ID，不直接重复创建 |

不要将 Cookie、API Key 或 `Data/` 作为排查附件公开。

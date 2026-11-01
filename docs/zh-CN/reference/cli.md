# SyndiRelay 命令行参考

独立程序名为 `syndirelay`（Windows 为 `syndirelay.exe`），源码入口是当前仓库的 `./cmd`。执行 `syndirelay help` 可查看当前版本。**不要再调用旧网站仓库的 `go run ./tools/blogctl/cmd`。**

| 命令 | 功能 | 副作用 |
| --- | --- | --- |
| `syndirelay help` | 显示命令用法 | 只读 |
| `syndirelay doctor` | 检查 Node/npm/Git 与运行系统 | 只读 |
| `syndirelay compile --file FILE --platform ID` | 将普通 Markdown 编译为指定平台格式 | 本地输出，不发布 |
| `syndirelay sync --article SLUG --platforms ID,... --dry-run` | 预览内容仓库分发 | 不写远程，也不修改本地发布状态 |
| `syndirelay sync --article SLUG --platforms ID,...` | 执行明确要求的平台发布流程 | 可能实际写入，必须有授权 |
| `syndirelay mcp-stdio` | 启动本机 STDIO MCP | 根据授权执行本地 Bridge 工具 |
| `syndirelay install` / `uninstall` | 注册或移除浏览器 Native Host | 修改本机系统注册信息 |
| `syndirelay social ...` | 社交内容管理与可选 Buffer 发布 | 按子命令决定，`publish` 可能远端写入 |

平台 ID 表示编译适配器，不等于官方发布 API 授权。单文件 `compile` 不需要 Inkerio/Astro。`--canonical-url` 和 `--asset-base-url` 应使用自己的站点／素材地址；不会默认使用 ThinkerQAQ 域名。

## 本地路径

- `SYNDIRELAY_HOME`：当前程序、渲染器及可选扩展的安装目录。
- `SYNDIRELAY_DATA_DIR`：本地私有配置、任务和发布记录目录，必须为绝对路径；应与旧 BlogCTL 数据隔离。
- `BLOG_CONTENT_ROOT` / 显式 `--content-root`：内容仓库。旧 `BLOGCTL_DATA_DIR`、`BLOGCTL_ENGINE_ROOT` 仅作为兼容旧版本的配置名，不建议新安装依赖。

## 旧站点命令

部分 `site`、`build`、`dev`、`preview`、`diagrams`、`search` 和 `ai-search` 分发仍存在，为历史网站 CI 兼容保留。**构建静态博客应该使用独立 [Inkerio](https://github.com/ThinkerQAQ/inkerio) 引擎**，执行 `node scripts/assemble-content.mjs` 和 `npm run build`。站外分发与站点构建是两条不同流程。

更多内容见 [MCP](../../MCP.md)、[工作区配置](../how-to/configure-workspace.md)、[公开发布验收](../../compliance/PUBLICATION_READINESS.md)。

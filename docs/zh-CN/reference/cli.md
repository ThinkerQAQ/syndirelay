# BlogCTL 命令

`blogctl help` 显示当前安装程序的命令；分发入口见 [`cmd/main.go`](../../../cmd/main.go)。CLI 与界面提供的流程并不完全相同。

## 工作目录

| 命令 | 运行位置 |
| --- | --- |
| `help`、`install`、`uninstall`、`-mcp-stdio` | 不要求引擎仓库（MCP 复用本机 Bridge） |
| `social ...` | 使用本地配置与指定内容，见 [Buffer 指南](../guide/social-publishing.md) |
| `sync`、`toutiao` | 内容仓库，或通过环境变量显式定位工作区 |
| `doctor`、`dev`、`preview`、`build`、`check`、`test`、`site`、`search`、`ai-search`、`diagrams` | Inkerio 引擎目录内 |

`sync` 优先使用 `BLOG_CONTENT_ROOT`、`BLOGCTL_ENGINE_ROOT`。未设置时，从当前内容目录寻找同级 `ThinkerQAQ.github.io`；从引擎目录可找到同级 `blog-content`。工作台保存的路径不能替代这套 CLI 定位规则。`BLOGCTL_DATA_DIR` 单独决定配置与运行数据目录，必须是绝对路径。

## 站点与维护命令

| 命令 | 用途 |
| --- | --- |
| `blogctl doctor` | 检查 Node/npm/Git 和系统 |
| `blogctl dev` | 执行 npm 示例开发流程，会重新装配公开模板 |
| `blogctl preview` | 预览已有 `dist/` |
| `blogctl check` / `test` | 执行 npm 检查或完整测试流程 |
| `blogctl build` | 构建当前已装配内容 |
| `blogctl site assemble --content-root PATH` | 从指定仓库替换生成内容 |
| `blogctl site build [--content-root PATH]` | 按需装配，构建站点、URL 清单和 Pagefind |
| `blogctl diagrams [plantuml\|drawio]` | 全部渲染或执行支持的单独子命令 |
| `blogctl search build\|inventory\|submit\|audit\|notify` | 构建/读取清单和执行搜索服务操作 |
| `blogctl ai-search prepare\|sync\|verify` | 准备、同步、验证可选 AI Search 内容 |

索引提交要求明确 Provider，IndexNow/百度还需用 `--all` 或 `--urls-file` 选择 URL。`search submit` 没有 dry-run 模式，会产生真实请求。提交前核对配置及 [`cmd/search.go`](../../../cmd/search.go) 的参数。`inventory --json` 读取本地构建输出，不提交 URL。

工程维护命令 `go run ./tools/blogctl/cmd/maint verify|build|package` 在引擎根目录运行。`verify` 安装 Node 依赖并运行 BlogCTL 检查；`package` 生成二进制、扩展 ZIP、校验和，不发布 Release。详见[引擎命令](../../../../../docs/zh-CN/reference/commands.md)。

## 文章 sync

```bash
blogctl sync --article sample-article --platforms devto --dry-run
```

| 参数 | 说明 |
| --- | --- |
| `--article SLUG` | 原文 slug，可重复指定多篇 |
| `--all` | 全部已发布文章，与 `--article` 互斥 |
| `--platforms p1,p2` | 必填的平台 ID 列表 |
| `--dry-run` | 本地编译，不写远端文章、不上传发布素材 |
| `--changed` | 在 sync 流程中只处理变化内容 |
| `--draft` | 旧 sync 草稿选项，不用于选择远端目标 |

Dry-run 不需要浏览器连接，但可能安装 Node 依赖、在本地保存工作区路径。它返回编译结果，不承诺导出正文预览文件。真实 `sync` 走 Bridge 的 `draft` 流程；界面中显式创建/更新的目标选择不是 CLI `sync` 参数。指定远端更新目标时使用工作台。

平台 ID 见 [`platform/capabilities.go`](../../../platform/capabilities.go)。平台注册不代表所有操作开放：今日头条支持账号检测、草稿和显式发布；开源中国和 Hashnode 不开放显式发布。

## 社交与实验命令

`blogctl social channels|source|create|list|show|stage|reconcile|publish` 管理本地短文和 Buffer 操作，[指南](../guide/social-publishing.md)说明输入及版本检查。

`toutiao probe --har FILE` 是开发诊断，不作为日常发布入口。HAR 可能包含账号凭据，应保存在仓库外；日常使用已支持的工作台流程。

## MCP / ChatGPT 集成

`blogctl -mcp-stdio` 启动仅提供 STDIO 的 MCP 服务，直接调用已有 Bridge，不重复实现发布逻辑。Windows Edge 登录态需要使用同一 Windows 实例的 BlogCTL；WSL 的 Bridge 不会自动共享 Windows Cookie。参见 [MCP 接入](../../MCP.md)。

# BlogCTL 配置

通过扩展或工作台「设置」编辑 `blogctl.toml`。手工修改前先停止 Bridge，避免与界面保存互相覆盖。

## 数据目录

| 系统 | 默认位置 |
| --- | --- |
| Windows | 程序旁的 `Data/` |
| Linux | 通常为 `~/.config/BlogCTL/`，遵循 `os.UserConfigDir()` |
| macOS | 通常为 `~/Library/Application Support/BlogCTL/` |
| 覆盖 | 启动前设置绝对路径 `BLOGCTL_DATA_DIR` |

`jobs.json`、`publications.json`、`distribution/`、日志和 `social/` 属于运行数据，可能包含私有内容。迁移前备份整个目录；设置环境变量不会自动搬文件。

Windows 与 WSL 默认目录和路径语法不同。共享社交草稿时，两个进程应指向同一 Data，分别使用 Windows 路径和 `/mnt/...` 路径。保存的工作区路径若是 Windows 格式，WSL social 命令应显式传入 Linux `--content-root`。

## 常用 TOML 字段

| 键 | 用途 |
| --- | --- |
| `engine_root`、`content_root` | 程序可访问的仓库根目录 |
| `ui_locale` | 默认 `auto`，可选 `zh-CN`、`en`，只控制界面 |
| `log_level`、`log_directory` | 默认 `info`，可选日志目录 |
| `proxy_enabled`、`proxy_host`、`proxy_port` | HTTP 代理开关、地址和端口 |
| `tool_paths` | 可执行工具的路径映射 |
| `distribution_root`、`publication_bindings_path` | 可选运行产物和历史记录目录 |
| `devto_api_key` | DEV.to 凭据 |
| `indexnow_endpoint`、`indexnow_key`、`indexnow_key_location` | IndexNow 接口和站点密钥 |
| `baidu_token` | 百度提交凭据 |
| `google_search_console_service_json` | Google Service Account JSON 设置 |

部分配置示例，通过设置保存可保留初始化默认值：

```toml
engine_root = "/home/user/blog/ThinkerQAQ.github.io"
content_root = "/home/user/blog/blog-content"
ui_locale = "zh-CN"
log_level = "info"
proxy_enabled = false

[publishing.platforms.devto]
language = "en"
changed_only = false
```

Windows 路径可用 TOML 字面字符串避免反斜杠转义，如 `engine_root = 'C:\code\ThinkerQAQ.github.io'`。

## 要索引的站点

在「设置 → 搜索引擎 → 索引站点与 URL」填写：

```toml
[search]
site_origin = "https://example.com"
url_list_url = "https://example.com/sitemap-all.txt"
sitemap_url = "https://example.com/sitemap-index.xml"
fingerprint_url = "https://example.com/sitemap-inventory.json"
google_property = "sc-domain:example.com"
```

全部换成自己已经部署的站点，资源 URL 必须属于该站点且能访问。`fingerprint_url = ""` 关闭指纹获取；此时增量保守地将已有 URL 视为变化。

缺少必填字段、未知搜索键、跨域 URL 和不匹配的 Google Property 会被拒绝。CLI `--site-url` 必须与 `google_property` 一致。缺少 `[search]` 的旧配置会迁移为此前默认站点，提交自己的站点前应检查；旧 `baidu_site` 在迁移时删除。

索引任务运行时禁止修改站点，排队任务保存其站点身份。域名迁移会重置当前清单，不会生成旧域名的错误删除请求。此配置不会部署网站或修改 DNS。

## 文章分发

| 配置段 | 字段及默认值 |
| --- | --- |
| `[publishing.platforms.PLATFORM]` | `language`、`changed_only` |
| `.footer` | `enabled`、`template`，占位符 `{site}`、`{url}`、`{title}` |
| `.canonical` | `mode`：`native`、`footer` 或 `none` |
| `.tracking` | `enabled`、`source`、`medium`、`campaign` |
| `[publishing.compiler.mermaid]` | `format`（`png`）、`width`（1200）、`scale`（2） |
| `[publishing.assets]` | `store`（`r2`） |
| `[publishing.assets.r2]` | `bucket`、`public_base_url`、`account_id`、`endpoint`、`access_key_id`、`secret_access_key` |

点号行嵌套在平台下，如 `[publishing.platforms.devto.footer]`。DEV.to、Medium、Hashnode 默认英文和原生 canonical，其余平台使用注册的默认语言与页脚模式。文章语言用于选择已有译文，不会自动翻译正文。

发布素材前配置自己的 R2 Bucket 和凭据。内置的作者公开素材地址不代表拥有对应 Bucket 的写权限。

**域名限制：** 分发编译器仍使用 [compiler/markdown.go](../../../compiler/markdown.go) 中的 `SiteOrigin` 常量生成 canonical 和根路径链接。修改 Astro `site` 或 `[search]` 不会改变它。Fork 用于自己的站外分发前，需要调整并验证这些链接。

## Buffer

```toml
[social.buffer]
api_key = "YOUR_PERSONAL_BUFFER_API_KEY"
```

Bridge API 会遮蔽已保存 Key；配置为空时使用 `BUFFER_API_KEY`。Bash 启动变量不一定传递到 Windows 或非交互 Bridge。完整操作见 [Buffer 发布](../guide/social-publishing.md)。

实现见 [Bridge 配置](../../../bridge/config.go)、[分发配置](../../../publishing/config.go)。真实配置、凭据和 Data 文件不要提交到 Git。

## MCP 与安全隧道

MCP 不新增 TOML 读写权限开关，也不改变发布配置。请在安装并登录浏览器扩展的 Windows 本机运行 `blogctl -mcp-stdio`。Tunnel ID、密钥引用及启动命令存放在独立的本地私有配置中，不能提交 Git。具体见 [MCP 接入](../../MCP.md)。

### 本地图片上传目录

`upload_image_to_r2` 仅允许读取 `content_root` 或本地 TOML 顶层配置的 `image_upload_roots = ['C:\Pictures']`，不要授权整块系统磁盘。白名单不会通过 Bridge 配置接口返回。需要已配置 `engine_root`、Node.js 和 Inkerio 的 Sharp 依赖；R2 继续复用 `[publishing.assets.r2]`，不在 MCP 暴露密钥。

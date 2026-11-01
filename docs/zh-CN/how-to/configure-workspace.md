# 配置 SyndiRelay 工作区

SyndiRelay 的**单文件 Markdown 编译不需要博客引擎或内容仓库**。只有从内容仓库筛选文章、使用浏览器工作台时，才需要配置内容根目录。

| 选项 | 作用 |
| --- | --- |
| `SYNDIRELAY_HOME` | 本地程序、Node 渲染依赖、Extension 和可选图表脚本所在目录 |
| `SYNDIRELAY_DATA_DIR` | 本地私有任务、日志、配置与发布关联记录目录，要求绝对路径 |
| Content Root | 包含 `src/content/articles/` 的内容仓库根目录 |
| Canonical／站点域名 | 自己的 HTTPS 地址，内容仓库分发时必须明确配置 |
| Engine Root（旧界面） | 仅为历史兼容保留；单独使用 SyndiRelay 不需要 Inkerio |

在浏览器扩展或 Console 的 **Settings → Content and paths** 中配置内容根目录，保存并重新打开确认。**Create/Update** 用于明确选择本地文章和远端目标，**Detect** 用于有权限的账号内容检索。单文件 `compile --file` 不需要检测或登录。

Windows Edge/Chrome 应使用 Windows 的 Native Host。WSL 和 Windows 的 Bridge 是不同进程，不会自动共享 Cookie。SyndiRelay 本地端口为 **32146**，旧 BlogCTL 使用 32145。

从 BlogCTL 迁移时先在私有路径备份旧 `Data/`，再遵循 [Windows 并行安装](../../how-to/windows-side-by-side.md) 保留两个工具的独立状态；不能让两个程序并发写入同一数据库。

站点作者及版权配置见[身份与版权](../reference/copyright-and-identity.md)，底层 TOML 字段见[配置参考](../reference/configuration.md)。

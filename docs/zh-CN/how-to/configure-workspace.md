# 设置工作区

在已连接的扩展或工作台打开「设置 → 内容与路径」。

| 字段 | 选择目录 |
| --- | --- |
| Engine Root | 包含 `package.json`、`astro.config.mjs` 和 `tools/blogctl/` 的根目录 |
| Content Root | 包含 `src/content/articles/` 的内容仓库根目录 |

不要选择 `src/content/` 或程序安装目录。保存后离开设置，再打开确认。到「创建/更新」检查本地文章；「检测」页列的是远端账号内容。

## Windows 与 WSL

Windows Bridge 使用 `C:\code\blog-content` 等 Windows 路径，WSL Linux 程序使用 Linux 路径。Windows 浏览器需要 Windows Native Messaging 程序，源码在 WSL 编辑并不改变这个要求。

CLI `sync` 通过当前目录或 `BLOG_CONTENT_ROOT`、`BLOGCTL_ENGINE_ROOT` 定位仓库，界面保存路径不能覆盖这些规则。详见 [CLI](../reference/cli.md)。

## 迁移或共享 Data

启动前设置绝对路径 `BLOGCTL_DATA_DIR`。停止原 Bridge，备份、复制已有 Data，从新位置启动，再检查设置与任务。环境变量不会搬文件，也不会转换工作区路径。

WSL 社交命令共享 Windows 安装时，使用对应的 `/mnt/c/.../Data`；保存路径为 Windows 格式时，显式传入 Linux `--content-root`。避免两个运行环境并发覆盖同一配置中的不兼容路径。

默认目录和字段见[配置参考](../reference/configuration.md)。

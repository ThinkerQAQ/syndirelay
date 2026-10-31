# 切换 BlogCTL 界面语言

浏览器扩展和 Web Console 使用同一套界面与翻译资源。

进入**设置 → 界面与语言**，选择 **跟随浏览器 / 简体中文 / English**。Auto 跟随浏览器语言（英文浏览器显示英文，其余默认简体中文）。保存成功后，Bridge 将偏好写入 `blogctl.toml` 的 `ui_locale`；再次启动或在另一种工作台入口打开时会读取同一设置。保存需要连接 Bridge。

| 配置 | 效果 |
| --- | --- |
| `auto` | 跟随浏览器语言 |
| `zh-CN` | 简体中文 |
| `en` | English |

CLI 可通过 `BLOGCTL_LANG=zh-CN` 或 `BLOGCTL_LANG=en` 临时选择语言；命令名、参数名、错误码和机器可读输出保持原样。

**界面语言与文章语言完全独立**。`ui_locale` 只控制按钮、提示、状态等文案；`publishing.platforms.<id>.language` 决定发往对应平台的文章语言。

实现使用本地打包的 i18next 及 Chrome Manifest 原生国际化。原始技术日志和用户文章内容不会自动翻译。

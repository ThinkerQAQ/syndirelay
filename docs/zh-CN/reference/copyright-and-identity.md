# 站点身份、版权声明与配置优先级

**SyndiRelay 不默认认领用户文章版权。** 新安装没有预置开发者域名、作者名称、版权声明或 R2 公共桶地址。代码仓库的 MIT License 只适用于 SyndiRelay 软件源码，不代表用户文章的许可协议。

在本地 `blogctl.toml` 或 **设置 → 平台配置 → 内容来源与版权** 中填写：

```toml
[publishing.site]
origin = "https://your-site.example"
name_zh = "我的站点"
name_en = "My Blog"
articles_zh = "/articles/"
articles_en = "/en/articles/"
legacy_origins = []

[publishing.copyright]
mode = "auto"

[publishing.copyright.zh]
syndication = "© 作者。本文原创文字采用作者自行指定的许可协议。"
source = "原文"

[publishing.copyright.en]
syndication = "© The Author. Original text is licensed as stated by its owner."
source = "Original article"
```

默认 `mode = "auto"`：**SyndiRelay 显式配置 → 内容仓库 `src/content/site/copyright.json` → 不附加版权声明**。`config` 强制使用工具自身文本（需中英文均填写）；`content` 强制读取内容仓库（不存在则报错）；`none` 禁用版权声明。配置文件存在但格式无效会报错，不静默回退到软件作者的旧声明。

版权声明与原文归属 Footer 分离：即使没有版权文案，只要平台 Footer 启用，也会保留原文链接。可按平台关闭 Footer 以同时关闭来源段落。

同步内容库必须明确设置 `publishing.site.origin`；普通 `syndirelay compile --file` 不依赖内容仓库，且不会自动写入开发者版权。域名和 `{site}` 名称来源于用户配置，`{url}` 可按平台附加 UTM，Canonical 始终不带跟踪参数。素材存储默认 `none`，需要图片/图表上传时请自行配置 R2，不使用软件开发者的地址。

迁移旧 BlogCTL 时，先备份 `Data/`、保留旧浏览器扩展和发布绑定，再填写你自己的站点与版权规则。配置变更**不会自动覆盖已经创建的 11 篇草稿或任何已发布文章**；后续更新须明确发起。

旧搜索/AI Search 兼容模块仍在，首次安装采用保留域名 `example.invalid` 作为无效占位符，使用索引工具前必须设置真实授权站点。

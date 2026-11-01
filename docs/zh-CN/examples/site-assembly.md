# 示例：独立构建 Inkerio 静态网站

SyndiRelay 不负责构建 Astro 网站。需要博客引擎时，分别获取公开的 Inkerio 与公开内容模板：

```sh
git clone https://github.com/ThinkerQAQ/inkerio.git
git clone https://github.com/ThinkerQAQ/inkerio-content-template.git
cd inkerio
npm ci
node scripts/assemble-content.mjs --content-root ../inkerio-content-template
npm run build
```

引擎输出 `dist/`，由用户自己的部署仓库和静态平台部署。详见 [Inkerio 首站教程](https://github.com/ThinkerQAQ/inkerio/blob/main/docs/zh-CN/tutorial/first-site.md)和[站点配置](https://github.com/ThinkerQAQ/inkerio/blob/main/docs/zh-CN/reference/site-features.md)。

要向外部平台分发文章，应另外运行 SyndiRelay 并配置自己的 Canonical、版权政策和平台账户。Inkerio 仓库不再包含 `tools/blogctl` 目录。

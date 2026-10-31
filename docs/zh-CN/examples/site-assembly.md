# 装配独立内容仓库

在引擎根目录中，确保同级目录存在 `blog-content/`：

```bash
go run ./tools/blogctl/cmd site assemble --content-root ../blog-content
npx --no-install astro dev --port 4321
```

原文保留在内容仓库，引擎仅生成构建输入。

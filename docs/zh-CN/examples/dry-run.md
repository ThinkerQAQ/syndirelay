# 只编译，不发布

公开模板包含 `sample-article` 及其英文译文。在引擎旁的内容仓库执行：

```bash
blogctl sync --article sample-article --platforms devto,medium --dry-run
```

使用已构建或安装并加入 PATH 的 `blogctl`。目录布局不同，可先用绝对路径设置 `BLOG_CONTENT_ROOT` 和 `BLOGCTL_ENGINE_ROOT`。

命令在本地校验文章和平台格式，可能安装 Node 依赖、更新本地工作区配置；不会调用远端文章创建/更新/发布，也不上传发布素材。它不要求浏览器会话或正在运行的 Bridge。成功退出表示编译检查通过，不代表生成了 HTML 预览文件或平台已经接受内容。

分发语言由 `publishing.platforms.PLATFORM.language` 决定，选择英文需要对应英文源文件。参数使用原文 slug，不要猜测翻译文件路径。远端写入参照[显式工作台流程](../tutorial/first-publish.md)。

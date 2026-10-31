# 内容与发布流水线

BlogCTL 把同一份 Markdown 原文编译成不同平台接受的格式，文章的权威来源始终是 `blog-content`。

```text
blog-content Markdown
        ↓
Go Compiler（正文、元信息、Canonical）
        ↓
图片与图表处理
        ↓
Platform Adapter（平台格式、上传协议）
        ↓
创建草稿 / 更新指定文章
        ↓
明确发布（平台支持时）
```

编译器负责内容语义和通用资源处理；平台 Adapter 负责具体 API、草稿 ID 和平台限制。这使得新平台可以复用编译、图片和任务能力。

例如，为 DEV.to 生成文章时，先用 `--dry-run` 检查内容和链接；创建或更新时再走已配置账号的 Bridge。更新必须指定真实远端目标，不能只靠标题推断。

实现入口在 [Compiler](../../../compiler/) 和 [Publishing Config](../../../publishing/config.go)；具体操作见[工作流](../guide/workflows.md)。

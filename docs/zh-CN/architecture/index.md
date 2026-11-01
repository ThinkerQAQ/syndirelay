# SyndiRelay 架构

SyndiRelay 从 BlogCTL 拆分成**独立的内容编译与发布产品**，与 Inkerio 静态引擎及个人私有内容仓库解耦。

```text
普通 Markdown / 内容适配器
  -> compiler/ + renderers/（本地生成不同平台格式）
  -> publisher/ + publishing/ + platform/（显式创建/更新/发布）
  -> bridge/（本机鉴权任务服务）
     -> extension/（可选浏览器登录态）
     -> cmd/mcp.go（可选 STDIO MCP）
  -> 可选 Reverfolio 持久队列（只传传播元数据）
```

Bridge 仅监听 `127.0.0.1:32146`，不要将整个 Bridge 开放至公网。`Data/` 仍保存在本地；新扩展和 Native Host 与历史 BlogCTL 分开管理。`search/` 与 `aisearch/` 仅为网站 CI 的可选集成保留，正常网站构建由 Inkerio 负责。

参阅[内容与发布](../concepts/content-and-publishing.md)、[Bridge 与数据](../concepts/bridge-and-state.md)、[操作安全](../concepts/operations.md)、[开源审核](../../compliance/PUBLICATION_READINESS.md)。

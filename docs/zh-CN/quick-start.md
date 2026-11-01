# SyndiRelay 快速开始

SyndiRelay 是独立的 Markdown 编译与内容分发工具，不依赖 Inkerio 或 Astro。软件的 MIT 协议不等于使用者文章的版权协议。

**源码已经公开并采用 MIT 许可**；可执行文件和浏览器扩展仍待单独完成分发许可、真实机器与平台授权验收，暂不提供正式安装包。旧 BlogCTL 下载不属于新版 SyndiRelay。

## 从源码安装

需要 Go 1.24+、Node.js 22+ 和 Git，公开源码可直接克隆。

```sh
git clone https://github.com/ThinkerQAQ/syndirelay.git
cd syndirelay
npm ci
go build -o syndirelay ./cmd
printf '# 我的文章\n\n这是原创示例。\n' > hello.md
./syndirelay compile --file ./hello.md --platform devto
```

上述命令**仅在本地编译并生成 JSON**，不会创建草稿、上传图片或公开发布。涉及图表图片时，请明确配置自己的 `--asset-base-url`。

通过 `content_root` 接入博客内容库时，请显式配置 `[publishing.site]` 站点域名、名称及版权说明。详见 [站点身份与版权](reference/copyright-and-identity.md)、[配置参考](reference/configuration.md)。没有域名时会阻止内容库发布，而不是自动指向软件作者的博客。

如需通过平台账户操作，先注册新程序的 Native Messaging Host (`./syndirelay install`)，再在 Edge/Chrome 加载 `extension/`。单文件编译不需要浏览器扩展；MCP stdio 使用 `./syndirelay mcp-stdio`，通过本地 Bridge 使用授权平台能力。

原 BlogCTL 的 Windows 数据迁移详见 [并行安装说明](../how-to/windows-side-by-side.md)，务必保留旧 `Data/`、浏览器扩展及历史发布绑定。`--dry-run` 不执行任何远端写入。

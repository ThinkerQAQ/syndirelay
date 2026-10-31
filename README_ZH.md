# SyndiRelay

[English](README.md) · [迁移来源记录](docs/migration/SOURCE_COMMIT.txt) · [MIT License](LICENSE)

**SyndiRelay 是独立的跨平台 Markdown 内容编译与分发工具**，由 BlogCTL 拆分。完整保留原有 11 个平台的编译规则、草稿与发布适配器、图片处理、Bridge、浏览器扩展和 MCP 源码，但不再需要安装 Inkerio/Astro 引擎。

**状态：私有迁移与兼容性审核阶段，尚未公开发布。** 只有编译和本地工具能力完成验证后，才进一步评估各平台接口授权、跨平台打包及旧配置迁移。

## 处理任意 Markdown（只编译，不发布）

需要 Go 1.24+ 和 Node.js 22+，**无需 Inkerio**：

```bash
npm ci
go build -o syndirelay ./cmd
./syndirelay compile --file ./hello.md --platform devto
./syndirelay compile --file ./hello.md --platform medium --asset-base-url https://your-cdn.example/
```

输入可以是普通 `# 标题` 加正文的 Markdown，也可包含 YAML Frontmatter。`--canonical-url` 可选，**不会默认写入 ThinkerQAQ 的域名、作者和版权页脚**。图片与图表如需生成分发素材，必须提供明确的 CDN 地址；本命令不会上传、登录或发布。

完整安装包除 Go 可执行文件外，还须包含 `renderers/`、`scripts/plantuml/`、`package.json`、`package-lock.json`；安装 Node 依赖后，通过 `SYNDIRELAY_HOME` 指定目录。

## 不丢失平台编译能力

迁移时，原 BlogCTL 共 356 个跟踪文件完成 SHA-256 比对；11 个平台的同一份测试文章编译 JSON 与旧版**逐字节一致**。包含 Medium 专用格式、文章表格、Canonical、链接、版权页脚和内容哈希。私有用户数据不会迁入新仓库。

原 `blog-content` 适配器保留，以保证旧业务兼容。SyndiRelay 的新入口独立解析普通 Markdown；平台发布前仍需用户授权。站点构建及 AI Search 等旧兼容模块暂时保留，确认生产部署改用独立接口后再删除。

## 合规与安全

拆仓不能消除非公开平台接口、Cookie 自动化、图片版权等风险。当前私有仓库仅用于迁移与审计，不对外发布新的浏览器扩展或安装包。历史 `tls-client`/`fhttp` 许可问题已从当前依赖中清除，但不追溯影响旧 Release。公开前需完成逐平台合规审核、依赖许可收集及 Windows/macOS/Linux 验收。

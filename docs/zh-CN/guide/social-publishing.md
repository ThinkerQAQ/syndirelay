# 通过 Buffer 发布短内容

BlogCTL 的社交短内容独立于长文分发。用户或外部 AI 编写英文文案，BlogCTL 校验内容源、追加带跟踪参数的链接，并记录内容版本。工作台提供预览与编辑，再由显式操作创建 Buffer 私有草稿和公开发布。

BlogCTL 本身不调用 LLM 生成摘要，也不通过 LinkedIn 浏览器 Cookie 发帖。X、Bluesky 需要先在 Buffer 连接对应频道。

## 配置

在本地 `blogctl.toml` 中设置：

```toml
[social.buffer]
api_key = "YOUR_PERSONAL_BUFFER_API_KEY"
```

也可以在「设置 → 社交平台 → Buffer API」填写。Bridge 返回值会遮蔽 Key。配置为空时可使用 `BUFFER_API_KEY`，但交互式 Bash 的环境变量不一定传入 Windows 或非交互 Bridge。

Windows 与 WSL 操作同一批草稿时，必须指向同一 Data 目录。Windows 默认是程序旁的 `Data/`，Linux 通常是 `~/.config/BlogCTL/`。在 WSL 设置绝对 `BLOGCTL_DATA_DIR`，指向挂载的 Windows Data；保存的内容路径若是 Windows 路径，CLI 需传 Linux 可访问的 `--content-root`。

不要提交 API Key、真实 `blogctl.toml` 或 `Data/social/*.json`。

## 草稿输入

输入只包含内容源引用和实际文案，不自行指定远端发布 ID、凭据或编造 canonical URL：

```json
{
  "id": "mutex-memory-guarantees",
  "source": {
    "kind": "article",
    "slug": "concurrency-series-03-mutex",
    "language": "en"
  },
  "posts": {
    "linkedin": {
      "text": "A mutex is more than mutual exclusion. ..."
    }
  }
}
```

将 slug 换成自己内容仓库中存在的英文内容。`source.kind` 支持 `article`、`series`、`project`。文案不要自行附加文章 URL，渲染器会生成 canonical 和跟踪链接。

图片帖可添加公开 HTTPS `mediaUrl`；纯文字帖省略该字段。LinkedIn 链接卡片使用 `linkCard: true`，不能同时设置 `mediaUrl`。

## 创建并审核

使用与工作台相同的 Data 目录：

```bash
blogctl social channels
blogctl social source --kind article --slug concurrency-series-03-mutex \
  --content-root /path/to/blog-content
blogctl social create --input /tmp/mutex-social.json \
  --content-root /path/to/blog-content
blogctl social list
blogctl social show --id mutex-memory-guarantees
```

在工作台 Social 标签检查、编辑并保存，或用 `show` 读取当前 `revision`。检查实际文案、链接和媒体后，用当前版本创建私有 Buffer 草稿：

```bash
blogctl social stage --id mutex-memory-guarantees --platform linkedin \
  --revision REVIEWED_REVISION
```

把 `REVIEWED_REVISION` 换成审核过的精确版本。点击工作台的 **Stage Buffer draft** 本身就是确认该版本，无需额外的批准步骤。此时只是 Buffer 私有草稿，LinkedIn 尚未公开。

## 公开发布

确认要公开后，再执行：

```bash
blogctl social publish --id mutex-memory-guarantees --platform linkedin \
  --revision REVIEWED_REVISION
```

`publish` 只推进已经暂存并校验的远端草稿，不会失败后重新创建。修改文案会使此前暂存的版本失效，不能悄悄覆盖远端草稿。

目前没有后台定时发布界面，也不会在结果不明时自动重发。

## 排查结果不明

超时或 API 异常会记录未决状态及错误。使用工作台 **Check publishing status**，或：

```bash
blogctl social reconcile --id mutex-memory-guarantees --platform linkedin \
  --revision REVIEWED_REVISION
```

此操作对 Buffer 只读，校验远端 Post ID、频道 ID 和完整渲染内容后，把观察结果保存到本地。

如果本地为 `publishing`，远端仍是私有草稿，失败尝试后至少等待两分钟再检查。确认回到 `draft` 后，用户可以明确决定是否再次提交同一草稿。检查状态不会自动发布。

配置位置见[配置参考](../reference/configuration.md)，命令入口见 [CLI](../reference/cli.md)。

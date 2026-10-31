"use strict";

(function (root) {
  // Availability is defined by each platform capability and browser session.
  const HIDDEN_PLATFORMS = new Set();

  function isVisiblePlatform(id) {
    return !HIDDEN_PLATFORMS.has(id);
  }

  function visiblePlatforms(platforms = []) {
    return platforms.filter((platform) => isVisiblePlatform(platform.id));
  }

  function visiblePlatformIDs(ids = []) {
    return ids.filter(isVisiblePlatform);
  }

  function visiblePublicationRecords(records = []) {
    return records.filter((record) => isVisiblePlatform(record.platform));
  }

  function platformCapabilities(platform) {
    return platform?.capabilities ?? {};
  }

  function deliveryToolAvailability(platform, tools = []) {
    const capabilities = platformCapabilities(platform);
    if (Object.keys(capabilities).length === 0) {
      return { available: false, reason: "平台能力未知" };
    }
    const needsAPIKey = capabilities.apiKey === true;
    if (!needsAPIKey) return { available: true, reason: "" };
    const tool = tools.find((item) => item.name === "devto-api");
    if (!tool) return { available: false, reason: "DEV.to API 状态未知" };
    if (tool.health?.ok) return { available: true, reason: "" };
    return { available: false, reason: tool.health?.summary || "DEV.to API Key 未配置" };
  }

  function platformAvailability(article, platform, publishingProfile = {}) {
    const capabilities = platformCapabilities(platform);
    if (Object.keys(capabilities).length === 0) {
      return { available: false, reason: "平台能力未知" };
    }
    const publisherManagedAuth = capabilities.browserSession === true || capabilities.apiKey === true;
    if (!publisherManagedAuth) {
      if (platform.known === false) return { available: false, reason: "登录状态检测失败" };
      if (!platform.loggedIn) return { available: false, reason: "未登录" };
    }
    if (!article) return { available: true, reason: "" };
    const language = publishingProfile.language || "zh-CN";
    if (language === "en" && !article.englishMirror) {
      return { available: false, reason: "缺少英文版本" };
    }
    return { available: true, reason: "" };
  }

  function statusPlatform(status, id) {
    return (status?.platforms ?? []).find((item) => item.id === id);
  }

  function canUpdatePublished(slug, platforms, bridgeRunning, status) {
    if (!slug || !bridgeRunning || platforms.length !== 1) return false;
    const platform = statusPlatform(status, platforms[0]);
    return platform?.capabilities?.publishedUpdate === true;
  }

  function canConfirmPublish(job, status) {
    const platforms = job?.platforms ?? [];
    return job?.operation === "draft"
      && job?.state === "completed"
      && platforms.length > 0
      && platforms.every((platform) => statusPlatform(status, platform)?.capabilities?.explicitPublish === true)
      && platforms.every((platform) => job?.results?.[platform]?.state === "completed");
  }

  const resultLabels = {
    completed: "完成",
    created: "已创建",
    updated: "已更新",
    skipped: "无变化",
    "dry-run": "Dry Run 完成",
    "draft-created": "草稿已创建",
    "published-edit-saved": "已保存待发布修改",
    published: "已发布",
    "waiting-for-session": "等待 Session",
    "rate-limit-retry": "等待限流重试",
  };

  function statePresentation(state, result = "") {
    if (state === "queued") return { kind: "unknown", label: "排队" };
    if (state === "running") return { kind: "checking", label: "运行中" };
    if (state === "waiting") return { kind: "checking", label: resultLabels[result] || "等待" };
    if (state === "completed") return { kind: "ok", label: resultLabels[result] || "完成" };
    if (state === "failed") return { kind: "error", label: "失败" };
    return { kind: "unknown", label: state || "未知" };
  }

  function platformRows(job, status) {
    return (job?.platforms ?? []).map((platform) => {
      const result = job?.results?.[platform] || {
        state: "unknown",
        error: "任务缺少平台结果",
      };
      const presentation = statePresentation(result.state, result.result);
      const platformStatus = statusPlatform(status, platform);
      return {
        id: platform,
        label: platformStatus?.label || platform,
        capabilities: platformStatus?.capabilities ?? {},
        state: result.state || "unknown",
        result: result.result || "",
        url: result.url || "",
        error: result.error || "",
        message: result.message || "",
        kind: presentation.kind,
        statusLabel: presentation.label,
      };
    });
  }

  // Remote-association cards distinguish creator drafts from public posts.
  // A platform inventory may return a public/preview URL for a draft (notably
  // DEV.to and older CNBlogs records), so a draft must resolve to its editor.
  function articleMatchLink(platformID, item) {
    const source = String(item?.url || "").trim();
    let parsed;
    try {
      parsed = new URL(source);
      if (parsed.protocol !== "https:" || parsed.username || parsed.password || parsed.port) parsed = null;
    } catch {
      parsed = null;
    }
    const hosts = {
      cnblogs: ["i.cnblogs.com", "www.cnblogs.com"],
      juejin: ["juejin.cn"],
      csdn: ["editor.csdn.net", "blog.csdn.net"],
      segmentfault: ["segmentfault.com"],
      zhihu: ["zhuanlan.zhihu.com"],
      "51cto": ["blog.51cto.com"],
      oschina: ["my.oschina.net"],
      toutiao: ["mp.toutiao.com", "www.toutiao.com"],
      devto: ["dev.to"],
      hashnode: ["hashnode.com"],
      medium: ["medium.com"],
    };
    // Public Hashnode URLs are on each publication's *.hashnode.dev host,
    // while the editor lives on hashnode.com. Never allow lookalike domains.
    const hashnodePublicHost = platformID === "hashnode" && item?.published === true &&
      /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.hashnode\.dev$/u.test(parsed?.hostname || "");
    if (parsed && !hosts[platformID]?.includes(parsed.hostname) && !hashnodePublicHost) parsed = null;
    if (item?.published === true) {
      return parsed ? { label: "查看文章", url: parsed.href } : null;
    }

    const id = String(item?.id ?? "").trim();
    const numeric = /^[0-9]+$/.test(id);
    const safeID = /^[A-Za-z0-9_-]+$/.test(id);
    let editor = "";
    switch (platformID) {
      case "cnblogs":
        if (numeric) editor = `https://i.cnblogs.com/posts/edit;postId=${id}`;
        break;
      case "juejin":
        if (numeric) editor = `https://juejin.cn/editor/drafts/${id}`;
        break;
      case "csdn":
        if (numeric) editor = `https://editor.csdn.net/md?articleId=${id}`;
        break;
      case "segmentfault":
        if (numeric) editor = `https://segmentfault.com/write?draftId=${id}`;
        break;
      case "zhihu":
        if (numeric) editor = `https://zhuanlan.zhihu.com/p/${id}/edit`;
        break;
      case "51cto":
        // The creator list can supply a more specific editor URL.
        if (parsed?.pathname.startsWith("/blogger/")) editor = parsed.href;
        else if (numeric) editor = `https://blog.51cto.com/blogger/draft/${id}`;
        break;
      case "oschina": {
        // OSChina uses a numeric creator ID, which is available in its
        // inventory editor link but cannot be inferred from a username.
        const creatorID = parsed?.pathname.match(/^\/u\/([0-9]+)\/blog\//)?.[1];
        if (creatorID && numeric) {
          editor = `https://my.oschina.net/u/${creatorID}/blog/ai-write/draft/${id}`;
        }
        break;
      }
      case "toutiao":
        if (numeric) editor = `https://mp.toutiao.com/profile_v4/graphic/publish?pgc_id=${id}`;
        break;
      case "devto": {
        // Forem edits by username + full slug, not /dashboard/edit/:id.
        // Draft slugs can include a temp-slug suffix; retain it verbatim.
        const articlePath = parsed?.pathname.match(/^\/([A-Za-z0-9_-]+)\/([A-Za-z0-9_-]+)(?:\/edit)?\/?$/);
        if (articlePath && !["dashboard", "settings", "api"].includes(articlePath[1])) {
          editor = `https://dev.to/${articlePath[1]}/${articlePath[2]}/edit`;
        }
        break;
      }
      case "hashnode":
        if (/^[a-f0-9]{24}$/iu.test(id)) editor = `https://hashnode.com/draft/${id}`;
        break;
      case "medium":
        if (safeID) editor = `https://medium.com/p/${encodeURIComponent(id)}/edit`;
        break;
    }
    return editor ? { label: "编辑草稿", url: editor } : null;
  }

  // Task events retain every remote write, unlike a per-platform summary
  // (which has only one URL and can be cleared by a terminal state event).
  // Reuse the same verified editor/public URL policy as Detection/Update.
  function taskArtifactLinks(job, platformID) {
    const completedKinds = new Set([
      "draft-created", "draft-updated", "published", "published-updated", "published-edit-saved",
    ]);
    const byRemote = new Map();
    for (const event of job?.events ?? []) {
      if (event?.platform !== platformID || !completedKinds.has(event.result)) continue;
      const isPublished = ["published", "published-updated", "published-edit-saved"].includes(event.result);
      const target = { id: String(event.targetId || ""), url: event.url || "", published: isPublished };
      const link = articleMatchLink(platformID, target);
      if (!link) continue;
      if (event.result === "published-edit-saved") link.label = "检查待发布修改";
      const identity = target.id || link.url;
      byRemote.set(identity, {
        ...link, id: target.id, state: isPublished ? "published" : "draft",
      });
    }
    // Earlier tasks might only have a per-platform URL.
    if (!byRemote.size) {
      const result = job?.results?.[platformID];
      if (result?.url && result.state === "completed") {
        const published = ["published", "published-updated", "published-edit-saved"].includes(result.result) ||
          job.operation === "publish";
        const link = articleMatchLink(platformID, {
          id: String(result.targetId || ""), url: result.url, published,
        });
        if (link) byRemote.set(link.url, { ...link, id: "", state: published ? "published" : "draft" });
      }
    }
    return [...byRemote.values()];
  }

  root.BlogCTLSyncModel = {
    deliveryToolAvailability,
    isVisiblePlatform,
    visiblePlatforms,
    visiblePlatformIDs,
    visiblePublicationRecords,
    articleMatchLink,
    taskArtifactLinks,
    platformRows,
    statePresentation,
    platformAvailability,
    canUpdatePublished,
    canConfirmPublish,
  };
})(globalThis);

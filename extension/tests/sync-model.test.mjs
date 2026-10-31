import assert from "node:assert/strict";
import test from "node:test";
import { toError } from "../errors.js";

await import("../popup/sync-model.js");
const model = globalThis.BlogCTLSyncModel;

test("maps structured sync results to platform rows", () => {
  const rows = model.platformRows({
    state: "running",
    platforms: ["devto", "medium"],
    results: {
      devto: { state: "completed", result: "updated", url: "https://dev.to/example" },
      medium: { state: "waiting", result: "waiting-for-session", message: "Waiting for Medium browser session." },
    },
  }, {
    platforms: [
      { id: "devto", label: "DEV.to", capabilities: { apiKey: true } },
      { id: "medium", label: "Medium", capabilities: { browserSession: true, draftCreate: true } },
    ],
  });

  assert.deepEqual(rows[0], {
    id: "devto",
    label: "DEV.to",
    capabilities: { apiKey: true },
    state: "completed",
    result: "updated",
    url: "https://dev.to/example",
    error: "",
    message: "",
    kind: "ok",
    statusLabel: "已更新",
  });
  assert.equal(rows[1].label, "Medium");
  assert.equal(rows[1].kind, "checking");
  assert.equal(rows[1].statusLabel, "等待 Session");
  assert.match(rows[1].message, /Waiting for Medium/u);
});

test("surfaces missing per-platform task results", () => {
  const [row] = model.platformRows({
    state: "failed",
    error: "network failed",
    platforms: ["cnblogs"],
  }, { platforms: [{ id: "cnblogs", label: "博客园" }] });

  assert.equal(row.label, "博客园");
  assert.equal(row.state, "unknown");
  assert.equal(row.kind, "unknown");
  assert.equal(row.statusLabel, "unknown");
  assert.equal(row.error, "任务缺少平台结果");
});

test("gates source availability by configured content language", () => {
  const loggedIn = { id: "medium", known: true, loggedIn: true, capabilities: { browserSession: true } };
  const withoutMirror = { slug: "only-cn", englishMirror: false };
  assert.deepEqual(
    model.platformAvailability(withoutMirror, loggedIn, { language: "en" }),
    { available: false, reason: "缺少英文版本" },
  );
  assert.deepEqual(
    model.platformAvailability(withoutMirror, loggedIn, { language: "zh-CN" }),
    { available: true, reason: "" },
  );

  const chinesePlatform = { id: "cnblogs", known: true, loggedIn: true, capabilities: { browserSession: true } };
  assert.deepEqual(
    model.platformAvailability(withoutMirror, chinesePlatform, { language: "en" }),
    { available: false, reason: "缺少英文版本" },
  );

  const withMirror = { slug: "with-en", englishMirror: true };
  assert.deepEqual(
    model.platformAvailability(withMirror, loggedIn, { language: "en" }),
    { available: true, reason: "" },
  );
});


test("native platforms defer authoritative login checks to the publisher", () => {
  const article = { slug: "with-en", englishMirror: true };
  assert.deepEqual(
    model.platformAvailability(article, { id: "csdn", known: true, loggedIn: false, capabilities: { browserSession: true } }),
    { available: true, reason: "" },
  );
  assert.deepEqual(
    model.platformAvailability(article, { id: "juejin", known: false, loggedIn: false, capabilities: { browserSession: true } }),
    { available: true, reason: "" },
  );
  assert.deepEqual(
    model.platformAvailability(article, { id: "medium", known: true, loggedIn: false, capabilities: { browserSession: true } }),
    { available: true, reason: "" },
  );
});

test("native publishing follows backend platform capabilities", () => {
  for (const id of ["cnblogs", "juejin", "csdn", "segmentfault", "zhihu", "51cto", "oschina", "toutiao"]) {
    assert.deepEqual(
      model.deliveryToolAvailability({ id, capabilities: { browserSession: true } }, []),
      { available: true, reason: "" },
    );
  }
  assert.deepEqual(
    model.deliveryToolAvailability({ id: "unknown", capabilities: {} }, []),
    { available: false, reason: "平台能力未知" },
  );
});

test("requires backend capabilities even when no article is selected", () => {
  assert.deepEqual(
    model.platformAvailability(undefined, { id: "devto", capabilities: { apiKey: true } }),
    { available: true, reason: "" },
  );
  assert.deepEqual(
    model.platformAvailability(undefined, { id: "devto", capabilities: {} }),
    { available: false, reason: "平台能力未知" },
  );
});

test("maps structured bridge error payloads to an error with code and details", () => {
  const error = toError({ code: "sync_job_not_found", message: "sync job not found", details: { id: "abc" } }, 404);
  assert.equal(error.message, "sync job not found");
  assert.equal(error.code, "sync_job_not_found");
  assert.equal(error.status, 404);
  assert.deepEqual(error.details, { id: "abc" });
});

test("falls back to a plain HTTP error when no structured payload exists", () => {
  const error = toError({}, 500);
  assert.equal(error.message, "bridge HTTP 500");
  assert.equal(error.code, "");
});


test("uses backend capabilities for published-update actions", () => {
  const status = {
    platforms: [
      { id: "cnblogs", capabilities: { publishedUpdate: true } },
      { id: "toutiao", capabilities: { publishedUpdate: false } },
      { id: "juejin", capabilities: { publishedUpdate: false } },
    ],
  };
  assert.equal(model.canUpdatePublished("example", ["cnblogs"], true, status), true);
  assert.equal(model.canUpdatePublished("example", ["toutiao"], true, status), false);
  assert.equal(model.canUpdatePublished("", ["cnblogs"], true, status), false);
  assert.equal(model.canUpdatePublished("example", [], true, status), false);
  assert.equal(model.canUpdatePublished("example", ["juejin"], true, status), false);
  assert.equal(model.canUpdatePublished("example", ["cnblogs", "juejin"], true, status), false);
  assert.equal(model.canUpdatePublished("example", ["cnblogs"], false, status), false);
  assert.equal(model.canUpdatePublished("example", ["cnblogs"], true, { platforms: [{ id: "cnblogs", capabilities: {} }] }), false);
});

test("uses backend API-key capability for delivery tool requirements", () => {
  const devto = { id: "devto", capabilities: { apiKey: true } };
  assert.deepEqual(
    model.deliveryToolAvailability(devto, [{ name: "devto-api", health: { ok: true } }]),
    { available: true, reason: "" },
  );
  assert.deepEqual(
    model.deliveryToolAvailability(devto, [{ name: "devto-api", health: { ok: false, summary: "未配置" } }]),
    { available: false, reason: "未配置" },
  );
});


test("uses explicit-publish capability for task confirmation", () => {
  const status = {
    platforms: [
      { id: "cnblogs", capabilities: { explicitPublish: true } },
      { id: "juejin", capabilities: { explicitPublish: true } },
      { id: "medium", capabilities: { explicitPublish: false } },
    ],
  };
  const completed = {
    operation: "draft",
    state: "completed",
    platforms: ["cnblogs", "juejin"],
    results: {
      cnblogs: { state: "completed" },
      juejin: { state: "completed" },
    },
  };
  assert.equal(model.canConfirmPublish(completed, status), true);
  assert.equal(model.canConfirmPublish({ ...completed, platforms: ["medium"], results: { medium: { state: "completed" } } }, status), false);
  assert.equal(model.canConfirmPublish({ ...completed, results: { cnblogs: { state: "failed" }, juejin: { state: "completed" } } }, status), false);
});

test("remote-association draft platforms resolve to an editor instead of a public preview", () => {
  const cases = [
    ["cnblogs", "23247130", "https://www.cnblogs.com/ThinkerQAQ/p/23247130",
      "https://i.cnblogs.com/posts/edit;postId=23247130"],
    ["juejin", "7694502718519017508", "https://juejin.cn/post/7694502718519017508",
      "https://juejin.cn/editor/drafts/7694502718519017508"],
    ["csdn", "167467490", "https://blog.csdn.net/ThinkerQAQ/article/details/167467490",
      "https://editor.csdn.net/md?articleId=167467490"],
    ["segmentfault", "123456", "https://segmentfault.com/a/123456",
      "https://segmentfault.com/write?draftId=123456"],
    ["zhihu", "2092192670206768438", "https://zhuanlan.zhihu.com/p/2092192670206768438",
      "https://zhuanlan.zhihu.com/p/2092192670206768438/edit"],
    ["51cto", "3223421", "https://blog.51cto.com/ThinkerQAQ/3223421",
      "https://blog.51cto.com/blogger/draft/3223421"],
    ["oschina", "3328466", "https://my.oschina.net/u/2360403/blog/3328466",
      "https://my.oschina.net/u/2360403/blog/ai-write/draft/3328466"],
    ["toutiao", "72599220101", "https://www.toutiao.com/article/72599220101/",
      "https://mp.toutiao.com/profile_v4/graphic/publish?pgc_id=72599220101"],
    ["devto", "4826123", "https://dev.to/thinkerqaq/temp-slug-7479138",
      "https://dev.to/thinkerqaq/temp-slug-7479138/edit"],
    ["hashnode", "6ad381a9ccbf8101ffb43761", "https://hashnode.com/draft/6ad381a9ccbf8101ffb43761",
      "https://hashnode.com/draft/6ad381a9ccbf8101ffb43761"],
    ["medium", "6e2fff4d49cd", "https://medium.com/@thinkerqaq/temp-6e2fff4d49cd",
      "https://medium.com/p/6e2fff4d49cd/edit"],
  ];
  for (const [platform, id, preview, editor] of cases) {
    assert.deepEqual(
      model.articleMatchLink(platform, { id, url: preview, published: false }),
      { label: "编辑草稿", url: editor },
      `draft of ${platform} should link to its editor`,
    );
  }
});

test("published articles keep their original public links across all supported platforms", () => {
  const published = [
    ["cnblogs", "https://www.cnblogs.com/ThinkerQAQ/p/23247130"],
    ["juejin", "https://juejin.cn/post/7694502718519017508"],
    ["csdn", "https://blog.csdn.net/ThinkerQAQ/article/details/167467490"],
    ["segmentfault", "https://segmentfault.com/a/123456"],
    ["zhihu", "https://zhuanlan.zhihu.com/p/2092192670206768438"],
    ["51cto", "https://blog.51cto.com/ThinkerQAQ/3223421"],
    ["oschina", "https://my.oschina.net/u/2360403/blog/19763618"],
    ["toutiao", "https://www.toutiao.com/article/72599220101/"],
    ["devto", "https://dev.to/thinkerqaq/published"],
    ["hashnode", "https://thinkerqaq.hashnode.dev/concurrency-programming-0-the-problem-space-and-scope"],
    ["medium", "https://medium.com/@thinkerqaq/published-example"],
  ];
  for (const [platform, url] of published) {
    assert.deepEqual(model.articleMatchLink(platform, { published: true, url, id: "123" }),
      { label: "查看文章", url },
      `published post on ${platform} must keep its public URL`);
  }
});

test("draft link resolution is fail-closed for bad IDs or untrusted domains", () => {
  assert.equal(model.articleMatchLink("oschina", {
    id: "3328466", published: false,
    url: "https://my.oschina.net/shengkunz/blog/write/draft/3328466",
  }), null, "OSChina cannot derive numeric creator ID from a username");
  assert.equal(model.articleMatchLink("devto", { id: "../../secret", published: false }), null);
  assert.equal(model.articleMatchLink("cnblogs", { id: "abc", published: false }), null);
  assert.equal(model.articleMatchLink("hashnode", { id: "../../secret", published: false }), null);
  assert.equal(model.articleMatchLink("hashnode", { id: "invalid-id", published: false }), null);
  assert.equal(model.articleMatchLink("hashnode", { id: "6ad381a9ccbf8101ffb43761", url: "https://hashnode.com.evil.example/draft/6ad381a9ccbf8101ffb43761", published: true }), null);
  assert.equal(model.articleMatchLink("hashnode", { id: "6ad381a9ccbf8101ffb43761", url: "https://thinkerqaq.hashnode.dev.evil.example/example", published: true }), null);
  assert.equal(model.articleMatchLink("hashnode", { id: "6ad381a9ccbf8101ffb43761", url: "http://thinkerqaq.hashnode.dev/example", published: true }), null);
  assert.equal(model.articleMatchLink("devto", { id: "123", url: "javascript:alert(1)", published: true }), null);
  assert.equal(model.articleMatchLink("medium", { id: "abc", url: "https://medium.com.evil.example/p/abc", published: true }), null);
  assert.equal(model.articleMatchLink("juejin", { id: "123", url: "http://juejin.cn/post/123", published: true }), null);
});

test("51CTO draft keeps an authenticated creator-provided editor route", () => {
  assert.deepEqual(model.articleMatchLink("51cto", {
    id: "3223421", url: "https://blog.51cto.com/blogger/edit/3223421", published: false,
  }), { label: "编辑草稿", url: "https://blog.51cto.com/blogger/edit/3223421" });
});

test("Toutiao is visible without modifying capabilities", () => {
  const list = [
    { id: "cnblogs", capabilities: { draftCreate: true } },
    { id: "medium", capabilities: { draftCreate: true, remoteList: true } },
    { id: "juejin", capabilities: { draftCreate: true } },
    { id: "toutiao", capabilities: { draftCreate: true, remoteList: true } },
    { id: "devto", capabilities: { draftCreate: true } },
  ];
  assert.deepEqual(model.visiblePlatforms(list).map((p) => p.id), ["cnblogs", "medium", "juejin", "toutiao", "devto"]);
  assert.equal(list[1].capabilities.draftCreate, true, "Medium capabilities are retained");
  assert.equal(list[3].capabilities.draftCreate, true, "Toutiao capabilities are retained");
  assert.deepEqual(model.visiblePlatformIDs(["medium", "juejin", "toutiao", "cnblogs", "medium"]),
    ["medium", "juejin", "toutiao", "cnblogs", "medium"], "retain restored platform selections");
  assert.equal(model.isVisiblePlatform("medium"), true);
  assert.equal(model.isVisiblePlatform("toutiao"), true);
  assert.equal(model.isVisiblePlatform("oschina"), true);
});

test("restored published history retains existing identities", () => {
  const records = [
    { article: "article", platform: "medium", remoteId: "abc" },
    { article: "article", platform: "toutiao", remoteId: "123" },
    { article: "article", platform: "cnblogs", remoteId: "456" },
  ];
  const active = model.visiblePublicationRecords(records);
  assert.deepEqual(active.map((x) => x.platform), ["medium", "toutiao", "cnblogs"]);
  assert.equal(records.length, 3, "local data and historical records are preserved");
  assert.equal(records[0].remoteId, "abc");
  assert.equal(records[1].remoteId, "123");
});

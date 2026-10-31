import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("article search uses a native datalist rather than a custom ARIA dropdown", async () => {
  const html = await readFile(new URL("../popup/popup.html", import.meta.url), "utf8");
  const code = await readFile(new URL("../popup/drafts.js", import.meta.url), "utf8");
  assert.match(html, /id="draftArticlePicker"[^>]*list="draftArticleOptions"/u);
  assert.match(html, /<datalist id="draftArticleOptions"><\/datalist>/u);
  assert.match(code, /function populateArticleOptions\(\)/u);
  assert.match(code, /articlePicker\.addEventListener\("input"/u);
  assert.doesNotMatch(code, /function articlePickerOpen\(/u);
});

async function creationUI(extraPlatforms = []) {
  const { runInNewContext } = await import("node:vm");
  class Element {
    children = []; handlers = {}; dataset = {}; disabled = false; value = "";
    classList = { toggle() {} };
    append(...items) { this.children.push(...items); }
    replaceChildren(...items) { this.children = items; }
    setAttribute() {}
    addEventListener(name, fn) { this.handlers[name] = fn; }
    click() { if (!this.disabled) return this.handlers.click?.(); }
  }
  const elements = new Map();
  const listeners = new Map();
  const document = {
    getElementById(id) { if (!elements.has(id)) elements.set(id, new Element()); return elements.get(id); },
    createElement() { return new Element(); },
    addEventListener(name, fn) { listeners.set(name, [...(listeners.get(name) || []), fn]); },
    dispatchEvent(event) { for (const fn of listeners.get(event.type) || []) fn(event); },
  };
  const calls = [];
  let resolveMatch, rejectMatch;
  const platforms = [...["devto", "cnblogs"].map(id => ({ id, capabilities: { draftCreate: true, remoteList: true, explicitPublish: true } })), ...extraPlatforms];
  const context = {
    document, localStorage: { getItem: () => "one", setItem() {} },
    CustomEvent: class { constructor(type, options) { this.type = type; this.detail = options?.detail; } },
    setTimeout: () => 1, clearTimeout() {}, window: { confirm: () => true },
    BlogCTLSyncState: { loadPlatforms: () => ["devto"], hasPlatformPreference: () => true, savePlatforms() {} },
    BlogCTLSyncModel: {
      visiblePlatformIDs: ids => ids, visiblePlatforms: items => items,
      isVisiblePlatform: () => true, platformAvailability: () => ({ available: true }),
      deliveryToolAvailability: () => ({ available: true }), taskArtifactLinks: () => [],
      articleMatchLink: () => null,
    },
    BlogCTLPopup: {
      setMessage() {}, setStatus() {}, errorMessage: e => e.message,
      refreshBridgeIndicator: async () => {},
      async send(type, payload) {
        calls.push({ type, payload });
        if (type === "blogctl.article.match") return new Promise((resolve, reject) => { resolveMatch = resolve; rejectMatch = reject; });
        if (type === "blogctl.articles") return { articles: [{ slug: "one", title: "One" }, { slug: "two", title: "Two" }] };
        if (type === "blogctl.status") return { status: { bridge: { running: true }, platforms } };
        if (type === "blogctl.job.start" || type === "blogctl.job.get") return { job: { id: "job", state: "completed", operation: "create", article: "one", events: [] } };
        return {};
      },
    },
  };
  for (const file of ["sync.js", "drafts.js"]) runInNewContext(await readFile(new URL("../popup/" + file, import.meta.url), "utf8"), context);
  context.BlogCTLSync.init(); context.BlogCTLDrafts.init();
  context.BlogCTLSync.activate(); context.BlogCTLDrafts.setMode("create"); context.BlogCTLDrafts.activate();
  const flush = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };
  await flush();
  const button = id => document.getElementById(id);
  const findText = (node, text) => node.textContent === text ? node : node.children.map(child => findText(child, text)).find(Boolean);
  return { context, calls, button, flush, findText,
    finish: items => resolveMatch({ match: { items } }), fail: () => rejectMatch(new Error("offline")) };
}

test("creation opens, refreshes and changes article without automatic detection", async () => {
  const ui = await creationUI();
  assert.equal(ui.button("refreshArticleMatches").textContent, "检测");
  assert.equal(ui.button("saveDrafts").disabled, false);
  assert.equal(ui.findText(ui.button("draftPlatforms"), "正在检查远端是否已有对应文章…"), undefined);
  await ui.context.BlogCTLDrafts.refresh();
  ui.button("draftArticlePicker").value = "two";
  ui.button("draftArticlePicker").handlers.input();
  assert.equal(ui.calls.filter(call => call.type === "blogctl.article.match").length, 0);
  await ui.button("saveDrafts").click(); await ui.flush();
  assert.equal(ui.calls.find(call => call.type === "blogctl.job.start").payload.request.article, "two");
  assert.equal(ui.calls.filter(call => call.type === "blogctl.article.match").length, 0);
});

test("manual detection checks selected platforms and does not disable creation", async () => {
  const ui = await creationUI();
  ui.button("refreshArticleMatches").click();
  assert.deepEqual(ui.calls.filter(call => call.type === "blogctl.article.match").map(call => call.payload.platform), ["devto"]);
  assert.equal(ui.button("saveDrafts").disabled, false);
  assert.equal(ui.findText(ui.button("draftPlatforms"), "创建草稿").disabled, false);
  await ui.button("saveDrafts").click();
  assert.ok(ui.calls.some(call => call.type === "blogctl.job.start"));
  ui.finish([]); await ui.flush();
});

test("failed detection still permits per-platform creation", async () => {
  const ui = await creationUI();
  ui.button("refreshArticleMatches").click(); ui.fail(); await ui.flush();
  assert.equal(ui.button("saveDrafts").disabled, false);
  await ui.findText(ui.button("draftPlatforms"), "创建草稿").click();
  assert.ok(ui.calls.some(call => call.type === "blogctl.job.start"));
});

test("detected existing articles remain excluded and stale results cannot affect another article", async () => {
  const ui = await creationUI();
  ui.button("refreshArticleMatches").click(); ui.finish([{ id: "remote", title: "One" }]); await ui.flush();
  assert.equal(ui.button("saveDrafts").disabled, true);
  assert.equal(ui.findText(ui.button("draftPlatforms").children[0], "创建草稿"), undefined);
  ui.button("refreshArticleMatches").click();
  ui.button("draftArticlePicker").value = "two"; ui.button("draftArticlePicker").handlers.input();
  ui.finish([{ id: "old-result" }]); await ui.flush();
  assert.equal(ui.button("saveDrafts").disabled, false);
});


test("actual update submit accepts Medium published revisions and keeps publishing manual", async () => {
 const ui = await creationUI([{id:"medium",capabilities:{draftCreate:true,remoteList:true,publishedDraftEdit:true}}]);
 ui.context.BlogCTLDrafts.setMode("update");
 await ui.context.BlogCTLDrafts.updateTargets([{platform:"medium",id:"fb20c187d357",state:"published"}]);
 const call = ui.calls.find(x=>x.type==="blogctl.job.start");
 assert.ok(call, "Update was blocked before Bridge submission");
 assert.equal(call.payload.request.operation,"update");
 assert.equal(call.payload.request.targets[0].id,"fb20c187d357");
 assert.equal(call.payload.request.publishAfter,false);
 const before = ui.calls.filter(x=>x.type==="blogctl.job.start").length;
 await ui.context.BlogCTLDrafts.updateTargets([{platform:"medium",id:"fb20c187d357",state:"published"}],true);
 assert.equal(ui.calls.filter(x=>x.type==="blogctl.job.start").length,before);
});


test("published Medium targets without staged-edit capability still fail closed", async () => {
 const ui = await creationUI([{id:"medium",capabilities:{draftCreate:true,remoteList:true}}]);
 ui.context.BlogCTLDrafts.setMode("update");
 await ui.context.BlogCTLDrafts.updateTargets([{platform:"medium",id:"fb20c187d357",state:"published"}]);
 assert.equal(ui.calls.some(x=>x.type==="blogctl.job.start"),false);
});

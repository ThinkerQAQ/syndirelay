import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const contentScriptPath = new URL("../google-indexing-content.js", import.meta.url);
const backgroundPath = new URL("../background.js", import.meta.url);

class FakeElement {
  constructor(tagName = "div", { text = "", attrs = {}, onClick = null } = {}) {
    this.tagName = String(tagName || "div").toUpperCase();
    this.textContent = text;
    this.attrs = new Map(Object.entries(attrs));
    this.onClick = onClick;
    this.parent = null;
  }

  getAttribute(name) {
    return this.attrs.has(name) ? this.attrs.get(name) : null;
  }

  hasAttribute(name) {
    return this.attrs.has(name);
  }

  getBoundingClientRect() {
    return { width: 240, height: 32 };
  }

  click() {
    this.onClick?.();
  }

  focus() {}

  dispatchEvent() {
    return true;
  }

  closest(selector) {
    if (!this.parent) return null;
    return matchesSelector(this.parent, selector) ? this.parent : this.parent.closest?.(selector) || null;
  }
}

class FakeInputElement extends FakeElement {
  constructor(options = {}) {
    super("input", options);
    this.value = "";
  }

  select() {}
}

class FakeTextAreaElement extends FakeInputElement {
  constructor(options = {}) {
    super(options);
    this.tagName = "TEXTAREA";
  }
}

function matchesSelector(node, selector) {
  if (selector.includes(",")) return selector.split(",").some((item) => matchesSelector(node, item.trim()));
  if (selector === "button") return node.tagName === "BUTTON";
  if (selector === 'input:not([type="hidden"])') {
    return node.tagName === "INPUT" && node.getAttribute("type") !== "hidden";
  }
  if (selector === "textarea") return node.tagName === "TEXTAREA";
  if (selector === '[contenteditable="true"]') return node.getAttribute("contenteditable") === "true";
  if (selector === "a") return node.tagName === "A";
  if (selector === "[aria-label]") return node.getAttribute("aria-label") !== null;
  if (selector === '[aria-modal="true"]') return node.getAttribute("aria-modal") === "true";
  if (selector === "[title]") return node.getAttribute("title") !== null;
  if (selector === "[tabindex]") return node.getAttribute("tabindex") !== null;
  if (selector === "[jsaction]") return node.getAttribute("jsaction") !== null;
  const role = selector.match(/^\[role="([^"]+)"\]$/u)?.[1];
  if (role) return node.getAttribute("role") === role;
  const tabindex = selector.match(/^\[tabindex="([^"]+)"\]$/u)?.[1];
  if (tabindex) return node.getAttribute("tabindex") === tabindex;
  return false;
}

async function createHarness({ bodyText = "", nodes = [], pathname = "/search-console/inspect", requestTimeoutMs = 180000 } = {}) {
  const source = await readFile(contentScriptPath, "utf8");
  let listener = null;
  const body = { innerText: bodyText };
  const document = {
    body,
    documentElement: body,
    title: "Google Search Console",
    querySelectorAll(selector) {
      return nodes.filter((node) => matchesSelector(node, selector));
    },
    execCommand() {
      return false;
    },
  };
  const chrome = {
    runtime: {
      onMessage: {
        addListener(fn) {
          listener = fn;
        },
      },
      sendMessage() {
        return Promise.resolve({ ok: true });
      },
    },
  };
  class FakeEvent {
    constructor(type, init = {}) {
      this.type = type;
      Object.assign(this, init);
    }
  }
  const context = vm.createContext({
    chrome,
    console,
    document,
    Element: FakeElement,
    HTMLInputElement: FakeInputElement,
    HTMLTextAreaElement: FakeTextAreaElement,
    Event: FakeEvent,
    InputEvent: FakeEvent,
    KeyboardEvent: FakeEvent,
    getComputedStyle() {
      return { display: "block", visibility: "visible", opacity: "1" };
    },
    location: {
      hostname: "search.google.com",
      pathname,
      href: `https://search.google.com${pathname}`,
    },
    setTimeout,
    clearTimeout,
    URL,
    __BLOGCTL_GSC_REQUEST_TIMEOUT_MS__: requestTimeoutMs,
  });
  vm.runInContext(source, context, { filename: "google-indexing-content.js" });
  assert.equal(typeof listener, "function", "content script should register its runtime listener");

  return {
    body,
    nodes,
    async send(message) {
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error("content-script response timed out")), 5000);
        const sendResponse = (value) => {
          clearTimeout(timer);
          resolve(value);
        };
        const handled = listener(message, {}, sendResponse);
        assert.equal(handled, true);
      });
    },
  };
}

test("GSC probe accepts an already-open Chinese inspection result page without a textbox", async () => {
  const url = "https://thinkerqaq.github.io/about/";
  const requestButton = new FakeElement("button", { text: "请求编入索引" });
  const harness = await createHarness({
    bodyText: `${url} 网址尚未收录到 Google 请求编入索引`,
    nodes: [requestButton],
  });

  const result = await harness.send({ type: "blogctl.google.index.probe" });
  assert.equal(result.ok, true);
  assert.equal(result.ready, true);
  assert.equal(result.inspectionInput, false);
  assert.equal(result.inspectionState, "not_indexed");
  assert.equal(result.hasRequestButton, true);
});

test("GSC ignores generic retry text outside an active error surface", async () => {
  const url = "https://thinkerqaq.github.io/notes/test/";
  const requestButton = new FakeElement("button", { text: "请求编入索引" });
  const harness = await createHarness({
    bodyText: `${url} 网址尚未收录到 Google 请求编入索引 帮助：出现问题时请稍后重试`,
    nodes: [requestButton],
  });

  const result = await harness.send({ type: "blogctl.google.index.probe" });
  assert.equal(result.ok, true);
  assert.equal(result.inspectionState, "not_indexed");
});

test("GSC still recognizes an error inside a visible alert surface", async () => {
  const url = "https://thinkerqaq.github.io/notes/test/";
  const requestButton = new FakeElement("button", { text: "请求编入索引" });
  const alert = new FakeElement("div", { text: "出现错误，请稍后重试", attrs: { role: "alert" } });
  const harness = await createHarness({
    bodyText: `${url} 网址尚未收录到 Google 请求编入索引 出现错误，请稍后重试`,
    nodes: [requestButton, alert],
  });

  const result = await harness.send({ type: "blogctl.google.index.probe" });
  assert.equal(result.ok, false);
  assert.equal(result.ready, false);
  assert.equal(result.inspectionState, "failed");
});

test("GSC ignores stale quota text outside an active feedback surface", async () => {
  const url = "https://thinkerqaq.github.io/notes/quota-stale/";
  const requestButton = new FakeElement("button", { text: "请求编入索引" });
  const harness = await createHarness({
    bodyText: `${url} 网址尚未收录到 Google 请求编入索引 已超出配额`,
    nodes: [requestButton],
  });

  const result = await harness.send({ type: "blogctl.google.index.probe" });
  assert.equal(result.ok, true);
  assert.equal(result.inspectionState, "not_indexed");
});

test("Request Indexing does not reuse stale quota text from before the click", async () => {
  const url = "https://thinkerqaq.github.io/notes/quota-stale-request/";
  let requested = 0;
  let closed = 0;
  const harness = await createHarness({
    bodyText: `${url} 网址尚未收录到 Google 请求编入索引 已超出配额`,
  });
  const dialog = new FakeElement("div", {
    text: "已提交编入索引请求 关闭",
    attrs: { role: "dialog", "aria-modal": "true" },
  });
  const closeButton = new FakeElement("button", {
    text: "关闭",
    onClick() {
      closed += 1;
      harness.body.innerText = "Google Search Console 已超出配额";
      harness.nodes.splice(harness.nodes.indexOf(dialog), 1);
      harness.nodes.splice(harness.nodes.indexOf(closeButton), 1);
    },
  });
  closeButton.parent = dialog;
  const requestButton = new FakeElement("button", {
    text: "请求编入索引",
    onClick() {
      requested += 1;
      harness.body.innerText = `${url} 已超出配额 已提交编入索引请求 关闭`;
      harness.nodes.push(dialog, closeButton);
    },
  });
  harness.nodes.push(requestButton);

  const result = await harness.send({ type: "blogctl.google.index.request", url });
  assert.equal(requested, 1);
  assert.equal(closed, 1);
  assert.equal(result.ok, true);
  assert.equal(result.action, "requested_indexing");
});

test("Request Indexing still recognizes a newly displayed quota alert", async () => {
  const url = "https://thinkerqaq.github.io/notes/quota-current/";
  const harness = await createHarness({
    bodyText: `${url} 网址尚未收录到 Google 请求编入索引`,
  });
  const requestButton = new FakeElement("button", {
    text: "请求编入索引",
    onClick() {
      harness.body.innerText = `${url} 已超出配额`;
      harness.nodes.push(new FakeElement("div", {
        text: "已超出配额",
        attrs: { role: "alert" },
      }));
    },
  });
  harness.nodes.push(requestButton);

  const result = await harness.send({ type: "blogctl.google.index.request", url });
  assert.equal(result.ok, false);
  assert.equal(result.action, "quota_blocked");
  assert.equal(result.stage, "request_result");
});

test("Request Indexing closes the success dialog so the next URL can continue", async () => {
  const url = "https://thinkerqaq.github.io/about/";
  let requested = 0;
  let closed = 0;
  const harness = await createHarness({
    bodyText: `${url} 网址尚未收录到 Google 请求编入索引`,
  });
  const dialog = new FakeElement("div", {
    text: "已提交编入索引请求 关闭",
    attrs: { role: "dialog", "aria-modal": "true" },
  });
  const closeButton = new FakeElement("button", {
    text: "关闭",
    onClick() {
      closed += 1;
      harness.body.innerText = "Google Search Console";
      harness.nodes.splice(harness.nodes.indexOf(dialog), 1);
      harness.nodes.splice(harness.nodes.indexOf(closeButton), 1);
      harness.nodes.push(new FakeInputElement({
        attrs: { "aria-label": "检查网址", role: "searchbox" },
      }));
    },
  });
  closeButton.parent = dialog;
  const requestButton = new FakeElement("button", {
    text: "请求编入索引",
    onClick() {
      requested += 1;
      harness.body.innerText = `${url} 已提交编入索引请求 关闭`;
      harness.nodes.push(dialog, closeButton);
    },
  });
  harness.nodes.push(requestButton);

  const result = await harness.send({ type: "blogctl.google.index.request", url });
  assert.equal(requested, 1);
  assert.equal(closed, 1);
  assert.equal(result.ok, true);
  assert.equal(result.action, "requested_indexing");
  assert.equal(result.stage, "request_result");

  const probe = await harness.send({ type: "blogctl.google.index.probe" });
  assert.equal(probe.ready, true);
  assert.equal(probe.inspectionInput, true);
});

test("Request Indexing reports cleanup fallback when the Google success modal cannot be closed", async () => {
  const url = "https://thinkerqaq.github.io/articles/";
  const harness = await createHarness({
    bodyText: `${url} 网址尚未收录到 Google 请求编入索引`,
  });
  const requestButton = new FakeElement("button", {
    text: "请求编入索引",
    onClick() {
      harness.body.innerText = `${url} 已提交编入索引请求 关闭`;
      harness.nodes.push(new FakeElement("div", {
        text: "已提交编入索引请求",
        attrs: { role: "dialog", "aria-modal": "true" },
      }));
      // Deliberately do not expose a clickable close control. The background
      // worker must force-reset GSC after recording this successful request.
    },
  });
  harness.nodes.push(requestButton);

  const result = await harness.send({ type: "blogctl.google.index.request", url });
  assert.equal(result.ok, true);
  assert.equal(result.action, "requested_indexing");
  assert.equal(result.cleanup.closed, false);
  assert.equal(result.cleanup.dialogPresent, true);
});
test("GSC overview probe activates the custom 检查网址 control and discovers the real input", async () => {
  let activated = 0;
  const harness = await createHarness({
    bodyText: "Google Search Console 概述 网址已在 Google 上",
    pathname: "/search-console",
  });
  const trigger = new FakeElement("button", {
    text: "检查网址",
    onClick() {
      activated += 1;
      harness.nodes.push(new FakeInputElement({
        attrs: { "aria-label": "检查网址", role: "searchbox" },
      }));
    },
  });
  harness.nodes.push(trigger);

  const result = await harness.send({ type: "blogctl.google.index.probe" });
  assert.equal(result.ok, true);
  assert.equal(result.ready, true);
  assert.equal(result.inspectionInput, true);
  assert.equal(activated, 1, "overview text must not be mistaken for an actionable inspection result");
});

test("Request Indexing cancels and retries a stuck live URL test", async () => {
  const url = "https://thinkerqaq.github.io/articles/tags/example/";
  let cancelled = 0;
  const harness = await createHarness({
    bodyText: `${url} 网址尚未收录到 Google 请求编入索引`,
    requestTimeoutMs: 1000,
  });
  const dialog = new FakeElement("div", {
    text: "正在测试实际网址是否可编入索引 取消",
    attrs: { role: "dialog", "aria-modal": "true" },
  });
  const cancelButton = new FakeElement("button", {
    text: "取消",
    onClick() {
      cancelled += 1;
      harness.body.innerText = `${url} 网址尚未收录到 Google 请求编入索引`;
      harness.nodes.splice(harness.nodes.indexOf(dialog), 1);
      harness.nodes.splice(harness.nodes.indexOf(cancelButton), 1);
    },
  });
  cancelButton.parent = dialog;
  const requestButton = new FakeElement("button", {
    text: "请求编入索引",
    onClick() {
      harness.body.innerText = `${url} 正在测试实际网址是否可编入索引 取消`;
      harness.nodes.push(dialog, cancelButton);
    },
  });
  harness.nodes.push(requestButton);

  const result = await harness.send({ type: "blogctl.google.index.request", url });
  assert.equal(cancelled, 1);
  assert.equal(result.ok, false);
  assert.equal(result.action, "ui_changed");
  assert.equal(result.stage, "request_processing_timeout");
  assert.equal(result.recovery.cancelled, true);
});

test("GSC inspection matches the target URL from the inspection control value", async () => {
  const url = "https://thinkerqaq.github.io/articles/tags/%E4%B8%AA%E4%BA%BA%E5%8D%9A%E5%AE%A2/";
  const input = new FakeInputElement({ attrs: { "aria-label": "检查网址", role: "searchbox" } });
  input.value = decodeURIComponent(url);
  const harness = await createHarness({
    bodyText: "网址尚未收录到 Google 请求编入索引",
    nodes: [input],
  });

  const result = await harness.send({ type: "blogctl.google.index.inspect", url });
  assert.equal(result.ok, true);
  assert.equal(result.action, "not_indexed");
  assert.equal(result.stage, "inspection_result_reused");
});

test("historical success text outside a dialog does not block the next inspection", async () => {
  const url = "https://thinkerqaq.github.io/articles/next/";
  const harness = await createHarness({
    bodyText: `${url} 网址尚未收录到 Google 已提交编入索引请求`,
    nodes: [new FakeInputElement({ attrs: { "aria-label": "检查网址", role: "searchbox" } })],
  });

  const result = await harness.send({ type: "blogctl.google.index.inspect", url });
  assert.equal(result.ok, true);
  assert.equal(result.action, "not_indexed");
});

test("Request Indexing start/resume preflight GSC before moving the bridge queue to running", async () => {
  const source = await readFile(backgroundPath, "utf8");
  for (const marker of [
    'case "blogctl.index.google.request.start":',
    'case "blogctl.index.google.request.resume":',
    'case "blogctl.job.resume":',
  ]) {
    const start = source.indexOf(marker);
    assert.notEqual(start, -1, `missing ${marker}`);
    const block = source.slice(start, start + 1800);
    const preflight = block.indexOf("ensureGoogleSearchConsoleReady");
    const mutation = Math.min(
      ...["/request-queue/start", "/request-queue/resume", "/resume"].map((needle) => {
        const index = block.indexOf(needle);
        return index < 0 ? Number.POSITIVE_INFINITY : index;
      }),
    );
    assert.ok(preflight >= 0, `${marker} should preflight GSC`);
    assert.ok(preflight < mutation, `${marker} should preflight before queue/job resume`);
  }
  assert.match(source, /request-queue\/pause/u);
  assert.match(source, /gsc queue pump failed/u);
  assert.match(source, /const maxPrepareAttempts = 3/u);
  assert.match(source, /gsc transient ui miss; retrying same url/u);
  assert.match(source, /request_processing_timeout/u);
  assert.match(source, /action === "failed" && stage === "inspection_result"/u);
  assert.match(source, /gsc request succeeded; forcing clean page for next url/u);
  assert.match(source, /result: "processing"/u);
  assert.match(source, /gsc queue item started/u);
  assert.match(source, /gsc queue item finished/u);
  assert.match(source, /cleanup\.dialogPresent \|\| cleanup\.closed === false/u);
  assert.match(source, /await delay\(1200\);/u);
  assert.doesNotMatch(source, /requested_indexing.*includes\(action\)/u);
});

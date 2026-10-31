import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const extension = path.resolve(here, "..");
const popup = path.join(extension, "popup");
const load = (name) => fs.readFileSync(path.join(popup, name), "utf8");

function runtime(language = "en-US") {
  const handlers = new Map();
  const document = {
    documentElement: { lang: "" },
    body: { nodeType: 1, ownerDocument: null, querySelectorAll: () => [], closest: () => null, hasAttribute: () => false },
    addEventListener(name, handler) { handlers.set(name, handler); },
    getElementById: () => null,
    createTreeWalker: () => ({ nextNode: () => false }),
  };
  document.body.ownerDocument = document;
  const root = {
    navigator: { language },
    document, NodeFilter: { SHOW_TEXT: 4 },
    CustomEvent: class { constructor(name, properties) { this.type = name; this.detail = properties.detail; } },
    dispatchEvent() {},
    addEventListener() {},
    BlogCTLTransport: { send: async (_, payload) => ({ uiLocale: payload?.uiLocale ?? "auto" }) },
  };
  root.globalThis = root;
  vm.createContext(root);
  for (const name of ["i18next-vendor.js", "i18n-messages.js", "i18n.js"]) {
    vm.runInContext(load(name), root, { filename: name });
  }
  return root;
}

test("i18next is locally bundled with source-language fallback", async () => {
  const context = runtime();
  assert.equal(context.BlogCTLI18n.systemLocale(), "en");
  await context.BlogCTLI18n.setLocale("en");
  assert.equal(context.BlogCTLI18n.t("保存"), "Save");
  assert.equal(context.BlogCTLI18n.t("Bridge 已连接"), "Bridge connected");
  assert.equal(context.BlogCTLI18n.t("匹配 3 个设置分类；打开分类后可直接编辑原有配置。"),
    "Found 3 settings categories. Open a category to edit its settings.");
  assert.equal(context.BlogCTLI18n.t("My Chinese-language article 中国文化"), "My Chinese-language article 中国文化");
  await context.BlogCTLI18n.setLocale("zh-CN");
  assert.equal(context.BlogCTLI18n.t("保存"), "保存");
  const chineseSystem = runtime("zh-HK");
  assert.equal(chineseSystem.BlogCTLI18n.systemLocale(), "zh-CN");
});

test("all static Chinese UI labels have an English catalog entry", () => {
  const context = runtime();
  const html = load("popup.html");
  const labels = new Set();
  for (const match of html.matchAll(/>([^<>]*[\u3400-\u9fff][^<>]*)</gu)) {
    const label = match[1].trim();
    if (label) labels.add(label);
  }
  for (const match of html.matchAll(/(?:title|placeholder|aria-label|data-tooltip)="([^"]*[\u3400-\u9fff][^"]*)"/gu)) {
    labels.add(match[1]);
  }
  const missing = [...labels].filter((label) => !Object.hasOwn(context.BlogCTLEnMessages, label));
  assert.deepEqual(missing, [], "missing localized static labels");
});

test("Manifest uses the standard Chrome locale contract", () => {
  const manifest = JSON.parse(fs.readFileSync(path.join(extension, "manifest.json"), "utf8"));
  assert.equal(manifest.default_locale, "en");
  for (const locale of ["en", "zh_CN"]) {
    const resources = JSON.parse(fs.readFileSync(path.join(extension, "_locales", locale, "messages.json"), "utf8"));
    for (const name of ["extensionName", "extensionDescription", "openPanel"]) {
      assert.ok(resources[name]?.message);
    }
  }
  assert.equal(manifest.name, "__MSG_extensionName__");
  assert.equal(manifest.action.default_title, "__MSG_openPanel__");
});

test("Bridge and browser Console load the same translation assets", () => {
  const html = load("popup.html");
  const scripts = [...html.matchAll(/<script src="([^"]+)"/gu)].map((match) => match[1]);
  assert.deepEqual(scripts.slice(0, 4), [
    "i18next-vendor.js", "i18n-messages.js", "i18n.js", "transport.js",
  ]);
  const embedded = fs.readFileSync(path.join(extension, "assets.go"), "utf8");
  assert.match(embedded, /popup\/\*\.js/u);
});


test("interface language is a Settings category, not a header control", () => {
  const html = load("popup.html");
  const settings = load("settings-navigation.js");
  const ui = load("i18n.js");
  assert.match(html, /data-panel="environment"[\s\S]*id="environmentInterfaceSettings"[\s\S]*id="blogctlUiLocale"/u);
  assert.equal([...html.matchAll(/id="blogctlUiLocale"/gu)].length, 1);
  for (const locale of ["auto", "zh-CN", "en"]) {
    assert.match(html, new RegExp(`<option value="${locale}">`));
  }
  assert.match(settings, /id: "interface"[\s\S]*selectors: \["#environmentInterfaceSettings"\]/u);
  assert.match(ui, /document\.getElementById\("blogctlUiLocale"\)/u);
  assert.doesNotMatch(ui, /querySelector\("\.header-actions"\)/u);
});

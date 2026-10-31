"use strict";

// Shared localized presentation for the Extension and Web Console. All strings
// come from bundled catalogs; original user content and technical logs remain
// unchanged. The DOM binding also covers feature modules that render on demand.
(function (root) {
  const SUPPORTED = new Set(["auto", "zh-CN", "en"]);
  const catalog = root.BlogCTLEnMessages;
  if (!root.i18next || !catalog) throw new Error("BlogCTL translation assets are missing");

  const engine = root.i18next.createInstance();
  engine.init({
    initImmediate: false, showSupportNotice: false, lng: "zh-CN", fallbackLng: "zh-CN",
    keySeparator: false, nsSeparator: false,
    interpolation: { escapeValue: false },
    resources: { "zh-CN": { translation: {} }, en: { translation: catalog } },
  });

  let preference = "auto";
  let effective = "zh-CN";
  let observer;
  const originalText = new WeakMap();
  const originalAttributes = new WeakMap();
  const attributes = ["title", "placeholder", "aria-label", "data-tooltip"];
  const avoid = "script,style,pre,code,textarea,.cm-editor,.log-viewer,.log-entry,[data-i18n-ignore]";
  const patterns = [
    [/^将在 (\d+) 个平台创建新草稿(并发布)?，确认？$/, (_, count, publishing) =>
      `Create drafts on ${count} platforms${publishing ? " and publish them" : ""}?`],
    [/^将覆盖 (\d+) 篇所选远端文章(并发布草稿)?，确认？$/, (_, count, publishing) =>
      `Overwrite ${count} selected remote articles${publishing ? " and publish drafts" : ""}?`],
    [/^平台原生图片上传仍可使用；共享 R2 缺少：(.+)。平台原生上传失败时将无法使用 R2 兜底。$/, (_, fields) =>
      `Native image uploads work; shared R2 is missing: ${fields}. Fallback is unavailable if native upload fails.`],
    [/^已用系统编辑器打开 (.+)。$/, (_, path) => `Opened ${path} in the system editor.`],
    [/^IndexNow (.+)任务已创建。$/, (_, label) => `IndexNow ${label} task created.`],
    [/^Baidu Search Resource (.+)任务已创建。$/, (_, label) => `Baidu Search Resource ${label} task created.`],
    [/^Google URL Inspection 任务已创建：offset (\d+), limit (\d+)。$/, (_, offset, limit) =>
      `Google URL Inspection task created: offset ${offset}, limit ${limit}.`],
    [/^· 读取中 (\d+) 个平台$/, (_, count) => ` · Loading ${count} platforms`],
    [/^· 失败 (\d+) 个平台$/, (_, count) => ` · ${count} platforms failed`],
    [/^内容语言: (.+)$/, (_, name) => `Content language: ${name}`],
    [/^已选择：(.*) · ID (.+)$/, (_, state, id) => `Selected: ${state} · ID ${id}`],
    [/^确定发布「(.+)」在 (.+) 的草稿（ID (.+)）？\n\n将直接调用该平台的发布接口，不再创建草稿，也不进入更新页。$/,
      (_, article, platform, id) =>
        `Publish draft "${article}" to ${platform} (ID ${id})?\n\nThis calls the platform publish endpoint directly without creating or updating the draft.`],
    [/^进度 (\d+) \/ (\d+)(.*)$/, (_, current, total, unit) => `Progress ${current} / ${total}${unit}`],
    [/^正在重试任务 (.+)…$/, (_, id) => `Retrying task ${id}…`],
    [/^正在(重试|暂停|继续|取消)任务 (.+)…$/, (_, action, id) =>
      `${({ 重试: "Retrying", 暂停: "Pausing", 继续: "Resuming", 取消: "Canceling" })[action]} task ${id}…`],
    [/^任务已(重试|暂停|继续|取消)。$/, (_, action) =>
      `Task ${({ 重试: "retried", 暂停: "paused", 继续: "resumed", 取消: "canceled" })[action]}.`],
    [/^(.+) · (\d+) 个平台$/, (_, article, count) => `${article} · ${count} platforms`],
    [/^开始 (.+)$/, (_, started) => `Started ${started}`],
    [/^结束 (.+)$/, (_, finished) => `Finished ${finished}`],
    [/^匹配 (\d+) 个设置分类；打开分类后可直接编辑原有配置。$/, (_, count) =>
      `Found ${count} settings categories. Open a category to edit its settings.`],
    [/^已清理 (\d+) 个任务。$/, (_, count) => `Cleared ${count} tasks.`],
    [/^共 (\d+) 篇远端文章 · 当前显示 (\d+) 篇$/, (_, count, visible) =>
      `${count} remote articles · ${visible} shown`],
    [/^(\d+) \/ (\d+) 可用$/, (_, available, total) => `${available} of ${total} available`],
    [/^版本 v(.+)$/, (_, version) => `Version v${version}`],
    [/^已选择：(.*)$/, (_, value) => `Selected: ${value}`],
    [/^任务读取失败：(.*)$/, (_, value) => `Failed to load tasks: ${value}`],
    [/^检测失败：(.*)$/, (_, value) => `Detection failed: ${value}`],
    [/^远端检查失败：(.*)$/, (_, value) => `Remote check failed: ${value}`],
    [/^正在保存 (.+)…$/, (_, value) => `Saving ${value}…`],
    [/^(.+) 已保存并重新检测。$/, (_, value) => `${value} saved and rechecked.`],
    [/^正在执行 (.+)：(.+)…$/, (_, tool, action) => `Running ${tool}: ${action}…`],
    [/^(.+) 平台配置已保存。$/, (_, value) => `${value} platform settings saved.`],
    [/^已加入任务 (.+)$/, (_, id) => `Task ${id} queued`],
  ];

  function systemLocale() {
    const language = String(root.navigator?.languages?.[0] || root.navigator?.language || "zh-CN");
    return language.toLowerCase().startsWith("en") ? "en" : "zh-CN";
  }
  function resolvedLocale(value) {
    return value === "auto" ? systemLocale() : value;
  }
  function t(source) {
    if (effective !== "en" || typeof source !== "string") return source;
    const match = /^(\s*)([\s\S]*?)(\s*)$/.exec(source);
    if (!match) return source;
    const [, prefix, phrase, suffix] = match;
    if (!phrase) return source;
    if (Object.hasOwn(catalog, phrase)) return prefix + engine.t(phrase) + suffix;
    for (const [pattern, format] of patterns) {
      const parts = pattern.exec(phrase);
      if (parts) return prefix + format(...parts) + suffix;
    }
    return source;
  }
  function shouldIgnore(element) {
    return Boolean(element?.closest?.(avoid));
  }
  function translateText(node) {
    if (!node || node.nodeType !== 3 || shouldIgnore(node.parentElement)) return;
    const current = node.nodeValue || "";
    const cached = originalText.get(node);
    const source = cached && cached.translated === current ? cached.source : current;
    const translated = t(source);
    originalText.set(node, { source, translated });
    if (translated !== current) node.nodeValue = translated;
  }
  function translateAttributes(element) {
    if (shouldIgnore(element)) return;
    let cached = originalAttributes.get(element);
    if (!cached) { cached = new Map(); originalAttributes.set(element, cached); }
    for (const name of attributes) {
      if (!element.hasAttribute(name)) continue;
      const current = element.getAttribute(name) ?? "";
      const prior = cached.get(name);
      const source = prior && prior.translated === current ? prior.source : current;
      const translated = t(source);
      cached.set(name, { source, translated });
      if (translated !== current) element.setAttribute(name, translated);
    }
  }
  function localizeNode(node) {
    if (!node) return;
    if (node.nodeType === 3) return translateText(node);
    if (node.nodeType !== 1 || shouldIgnore(node)) return;
    translateAttributes(node);
    const iter = node.ownerDocument.createTreeWalker(node, NodeFilter.SHOW_TEXT);
    while (iter.nextNode()) translateText(iter.currentNode);
    for (const element of node.querySelectorAll("*")) translateAttributes(element);
  }
  function applyLocale() {
    const locale = resolvedLocale(preference);
    effective = locale;
    engine.changeLanguage(locale);
    document.documentElement.lang = locale;
    localizeNode(document.body);
    const select = document.getElementById("blogctlUiLocale");
    if (select) select.value = preference;
    root.dispatchEvent(new CustomEvent("blogctl:locale-changed", { detail: { locale, preference } }));
  }
  async function saveLocale(value) {
    if (!SUPPORTED.has(value)) throw new Error("invalid UI locale");
    const previous = preference;
    preference = value;
    applyLocale();
    try {
      await root.BlogCTLTransport.send("blogctl.locale.set", { uiLocale: value });
    } catch (error) {
      preference = previous;
      applyLocale();
      throw error;
    }
  }
  async function reloadLocale() {
    try {
      const response = await root.BlogCTLTransport.send("blogctl.locale.get");
      if (SUPPORTED.has(response.uiLocale) && response.uiLocale !== preference) {
        preference = response.uiLocale;
        applyLocale();
      }
    } catch {
      // Keep the browser-language fallback while the Bridge is unavailable.
    }
  }
  function initialize() {
    if (observer) return;
    const select = document.getElementById("blogctlUiLocale");
    const message = document.getElementById("uiLocaleMessage");
    if (!select || !message) throw new Error("BlogCTL interface language setting is missing");
    select.addEventListener("change", async () => {
      select.disabled = true;
      message.textContent = "";
      message.className = "message";
      try {
        await saveLocale(select.value);
        message.textContent = "界面语言已保存。";
        message.className = "message ok";
      } catch (error) {
        message.textContent = "保存失败，请检查 Bridge 连接。";
        message.className = "message error";
        console.warn("[BlogCTL i18n] language preference was not saved:", error);
      } finally {
        select.disabled = false;
      }
    });
    applyLocale();
    observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        if (mutation.type === "characterData") translateText(mutation.target);
        else if (mutation.type === "attributes") translateAttributes(mutation.target);
        else for (const node of mutation.addedNodes) localizeNode(node);
      }
    });
    observer.observe(document.body, {
      subtree: true, childList: true, characterData: true, attributes: true,
      attributeFilter: attributes,
    });
    reloadLocale();
    root.addEventListener("focus", reloadLocale);
  }
  root.BlogCTLI18n = Object.freeze({
    t, systemLocale, get locale() { return effective; },
    get preference() { return preference; }, setLocale: saveLocale, reloadLocale,
    formatDate: (value, options) => new Intl.DateTimeFormat(effective, options).format(new Date(value)),
    formatNumber: (value) => new Intl.NumberFormat(effective).format(value),
    initialize,
  });
  document.addEventListener("DOMContentLoaded", initialize);
})(globalThis);

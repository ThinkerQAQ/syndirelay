"use strict";

(() => {
  if (globalThis.__BLOGCTL_GOOGLE_INDEXING_CONTENT__) return;
  globalThis.__BLOGCTL_GOOGLE_INDEXING_CONTENT__ = true;

  const TEXT = {
    inspect: [
      "inspect any url",
      "inspect url",
      "url inspection",
      "检查网址",
      "检查 url",
      "网址检查",
    ],
    indexed: [
      "url is on google",
      "网址在 google 上",
      "网址已在 google 上",
      "网址已收录到 google",
      "网页已收录到 google",
      "已编入索引",
    ],
    notIndexed: [
      "url is not on google",
      "网址不在 google 上",
      "网址未在 google 上",
      "网址尚未收录到 google",
      "网页尚未收录到 google",
      "此网页未编入索引",
    ],
    request: [
      "request indexing",
      "请求编入索引",
      "申请编入索引",
    ],
    close: [
      "close",
      "关闭",
    ],
    cancel: [
      "cancel",
      "取消",
    ],
    testing: [
      "testing if live url can be indexed",
      "testing live url",
      "正在测试实际网址是否可编入索引",
      "正在测试实际网址",
    ],
    requested: [
      "indexing requested",
      "已请求编入索引",
      "已申请编入索引",
      "已提交编入索引请求",
    ],
    quota: [
      "quota exceeded",
      "daily quota",
      "已超出配额",
      "配额已用完",
      "每日配额",
    ],
    rateLimited: [
      "too many requests",
      "请求过多",
    ],
    failed: [
      "something went wrong",
      "an error occurred",
      "something went wrong. please try again",
      "出现错误",
      "发生错误",
      "请稍后重试",
    ],
  };

  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

  function normalizedText(value) {
    return String(value || "").replace(/\s+/gu, " ").trim().toLowerCase();
  }

  function bodyText() {
    return normalizedText(document.body?.innerText || document.documentElement?.innerText || "");
  }

  function includesAny(text, candidates) {
    return candidates.some((candidate) => text.includes(candidate));
  }

  function visible(node) {
    if (!(node instanceof Element)) return false;
    const style = getComputedStyle(node);
    const rect = node.getBoundingClientRect();
    return style.display !== "none" &&
      style.visibility !== "hidden" &&
      Number(style.opacity || 1) !== 0 &&
      rect.width > 0 &&
      rect.height > 0;
  }

  function textMatches(node, candidates) {
    const text = normalizedText([
      node?.textContent,
      node?.getAttribute?.("aria-label"),
      node?.getAttribute?.("title"),
    ].filter(Boolean).join(" "));
    return candidates.some((candidate) => text.includes(candidate));
  }

  function emit(level, message, fields = {}) {
    try {
      chrome.runtime.sendMessage({
        type: "blogctl.google.index.event",
        level,
        message,
        fields: {
          pagePath: location.pathname,
          ...fields,
        },
      }).catch(() => {});
    } catch {}
  }

  function inspectionControls() {
    const nodes = [
      ...document.querySelectorAll('input:not([type="hidden"])'),
      ...document.querySelectorAll("textarea"),
      ...document.querySelectorAll('[role="textbox"]'),
      ...document.querySelectorAll('[role="combobox"]'),
      ...document.querySelectorAll('[role="searchbox"]'),
      ...document.querySelectorAll('[contenteditable="true"]'),
    ];
    return [...new Set(nodes)].filter(visible);
  }

  function findInspectionInput() {
    const ranked = inspectionControls().map((input) => {
      const haystack = normalizedText([
        input.getAttribute?.("aria-label"),
        input.getAttribute?.("placeholder"),
        input.getAttribute?.("title"),
        input.getAttribute?.("data-tooltip"),
        input.getAttribute?.("role"),
        input.getAttribute?.("name"),
      ].filter(Boolean).join(" "));
      let score = 0;
      if (haystack.includes("inspect")) score += 7;
      if (haystack.includes("url")) score += 5;
      if (haystack.includes("网址")) score += 5;
      if (haystack.includes("search console")) score += 2;
      if (input.getAttribute?.("role") === "combobox") score += 1;
      if (input.getAttribute?.("role") === "textbox") score += 1;
      return { input, score };
    }).sort((a, b) => b.score - a.score);

    if (ranked[0]?.score > 0) return ranked[0].input;
    return ranked[0]?.input || null;
  }

  function inspectionTriggerCandidates() {
    const nodes = [
      ...document.querySelectorAll("button"),
      ...document.querySelectorAll('[role="button"]'),
      ...document.querySelectorAll('[role="search"]'),
      ...document.querySelectorAll('[tabindex="0"]'),
      ...document.querySelectorAll("[aria-label]"),
      ...document.querySelectorAll("[title]"),
    ];
    return [...new Set(nodes)].filter(visible);
  }

  function findInspectionTrigger() {
    const ranked = inspectionTriggerCandidates()
      .filter((node) => !textMatches(node, TEXT.request))
      .map((node) => {
        const haystack = normalizedText([
          node?.textContent,
          node?.getAttribute?.("aria-label"),
          node?.getAttribute?.("placeholder"),
          node?.getAttribute?.("title"),
          node?.getAttribute?.("data-tooltip"),
        ].filter(Boolean).join(" "));
        let score = 0;
        if (haystack.includes("inspect")) score += 7;
        if (haystack.includes("url")) score += 5;
        if (haystack.includes("检查")) score += 7;
        if (haystack.includes("网址")) score += 5;
        if (haystack.includes("search console")) score += 1;
        return { node, score };
      })
      .filter((entry) => entry.score >= 7)
      .sort((a, b) => b.score - a.score);
    return ranked[0]?.node || null;
  }

  async function ensureInspectionInput(timeoutMs = 30000) {
    let input = findInspectionInput();
    if (input) return input;

    const startedAt = Date.now();
    const trigger = await waitFor(findInspectionTrigger, Math.min(5000, timeoutMs), 250);
    if (trigger) {
      try {
        trigger.click();
        trigger.focus?.();
        emit("info", "gsc inspection trigger activated", pageDiagnostic());
      } catch (error) {
        emit("warn", "gsc inspection trigger activation failed", { error: error?.message || String(error) });
      }
      await sleep(200);
    }

    const remaining = Math.max(0, timeoutMs - (Date.now() - startedAt));
    if (remaining === 0) return findInspectionInput();
    return waitFor(findInspectionInput, remaining, 300);
  }

  function setInputValue(input, value) {
    input.click();
    input.focus();

    if (input instanceof HTMLInputElement || input instanceof HTMLTextAreaElement) {
      if (typeof input.select === "function") input.select();
      let inserted = false;
      try {
        inserted = document.execCommand("insertText", false, value) === true;
      } catch {}
      if (!inserted || input.value !== value) {
        const prototype = input instanceof HTMLTextAreaElement
          ? HTMLTextAreaElement.prototype
          : HTMLInputElement.prototype;
        const descriptor = Object.getOwnPropertyDescriptor(prototype, "value");
        descriptor?.set?.call(input, value);
        input.dispatchEvent(new InputEvent("input", {
          bubbles: true,
          inputType: "insertText",
          data: value,
        }));
      }
    } else {
      if (typeof input.focus === "function") input.focus();
      input.textContent = value;
      input.dispatchEvent(new InputEvent("input", {
        bubbles: true,
        inputType: "insertText",
        data: value,
      }));
    }
    input.dispatchEvent(new Event("change", { bubbles: true }));
  }

  function pressEnter(input) {
    for (const type of ["keydown", "keypress", "keyup"]) {
      input.dispatchEvent(new KeyboardEvent(type, {
        key: "Enter",
        code: "Enter",
        keyCode: 13,
        which: 13,
        bubbles: true,
      }));
    }
  }

  async function waitFor(check, timeoutMs, intervalMs = 500) {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      const value = check();
      if (value) return value;
      await sleep(intervalMs);
    }
    return null;
  }

  function urlVariants(url) {
    const values = new Set([normalizedText(url)]);
    try {
      values.add(normalizedText(decodeURIComponent(url)));
    } catch {}
    try {
      const parsed = new URL(url);
      values.add(normalizedText(parsed.href));
      values.add(normalizedText(decodeURIComponent(parsed.href)));
    } catch {}
    return [...values].filter(Boolean);
  }

  function pageMentionsURL(url) {
    const controls = inspectionControls().flatMap((node) => [
      node?.value,
      node?.textContent,
      node?.getAttribute?.("aria-label"),
      node?.getAttribute?.("title"),
    ]).filter(Boolean).join(" ");
    const text = normalizedText([
      bodyText(),
      controls,
      location.href || "",
    ].join(" "));
    return urlVariants(url).some((candidate) => text.includes(candidate));
  }

  function feedbackSurfaceText() {
    const selectors = [
      '[role="alert"]',
      '[role="dialog"]',
      '[role="status"]',
      '[aria-live="assertive"]',
      '[aria-live="polite"]',
    ];
    const nodes = selectors.flatMap((selector) => [...document.querySelectorAll(selector)]);
    return normalizedText(
      [...new Set(nodes)]
        .filter(visible)
        .map((node) => node?.innerText || node?.textContent || "")
        .join(" "),
    );
  }

  function hasVisibleFailureSignal() {
    return includesAny(feedbackSurfaceText(), TEXT.failed);
  }

  function signalAdded(afterText, beforeText, candidates) {
    const after = normalizedText(afterText);
    const before = normalizedText(beforeText);
    return includesAny(after, candidates) && !includesAny(before, candidates);
  }

  function detectPageState(url = "") {
    const text = bodyText();
    const feedback = feedbackSurfaceText();
    if (includesAny(feedback, TEXT.rateLimited)) return { kind: "rate_limited" };
    if (includesAny(feedback, TEXT.quota)) return { kind: "quota_blocked" };
    if (hasVisibleFailureSignal()) return { kind: "failed" };

    const state = includesAny(text, TEXT.notIndexed)
      ? { kind: "not_indexed" }
      : includesAny(text, TEXT.indexed)
        ? { kind: "already_indexed" }
        : null;
    if (!state) return null;

    // Never accept a stale result left on the page for the previous URL.
    if (url && !pageMentionsURL(url)) return null;
    return state;
  }

  function findRequestIndexingButton() {
    const nodes = [
      ...document.querySelectorAll("button"),
      ...document.querySelectorAll('[role="button"]'),
    ];
    return nodes.find((node) =>
      visible(node) &&
      textMatches(node, TEXT.request) &&
      !node.hasAttribute("disabled") &&
      node.getAttribute("aria-disabled") !== "true"
    ) || null;
  }

  function findSuccessDialogState(beforeText = "", beforeFeedbackText = "") {
    const text = bodyText();
    const feedback = feedbackSurfaceText();
    if (
      signalAdded(feedback, beforeFeedbackText, TEXT.rateLimited) ||
      signalAdded(text, beforeText, TEXT.rateLimited)
    ) return "rate_limited";
    if (
      signalAdded(feedback, beforeFeedbackText, TEXT.quota) ||
      signalAdded(text, beforeText, TEXT.quota)
    ) return "quota_blocked";
    if (
      signalAdded(feedback, beforeFeedbackText, TEXT.failed) ||
      signalAdded(text, beforeText, TEXT.failed)
    ) return "failed";
    if (
      signalAdded(feedback, beforeFeedbackText, TEXT.requested) ||
      signalAdded(text, beforeText, TEXT.requested)
    ) return "requested_indexing";
    return "";
  }

  function requestProcessingDialog() {
    const dialogs = [
      ...document.querySelectorAll('[role="dialog"]'),
      ...document.querySelectorAll('[aria-modal="true"]'),
    ];
    return [...new Set(dialogs)].find((dialog) =>
      visible(dialog) && includesAny(normalizedText(dialog.textContent), TEXT.testing)
    ) || null;
  }

  function findRequestProcessingCancelButton(dialog) {
    if (!dialog) return null;
    const nodes = [
      ...document.querySelectorAll("button"),
      ...document.querySelectorAll('[role="button"]'),
      ...document.querySelectorAll("a"),
      ...document.querySelectorAll("[tabindex]"),
      ...document.querySelectorAll("[jsaction]"),
    ];
    return [...new Set(nodes)].find((node) => {
      if (!visible(node) || !textMatches(node, TEXT.cancel)) return false;
      try {
        return node.closest?.('[role="dialog"], [aria-modal="true"]') === dialog;
      } catch {
        return false;
      }
    }) || null;
  }

  async function waitForRequestResult(beforeText, beforeFeedbackText, timeoutMs) {
    const startedAt = Date.now();
    let nextHeartbeatAt = startedAt + 30000;
    while (Date.now() - startedAt < timeoutMs) {
      const result = findSuccessDialogState(beforeText, beforeFeedbackText);
      if (result) return result;
      const now = Date.now();
      if (now >= nextHeartbeatAt) {
        emit("info", "gsc live test still running", {
          elapsedMs: now - startedAt,
          processingDialog: Boolean(requestProcessingDialog()),
        });
        nextHeartbeatAt = now + 30000;
      }
      await sleep(500);
    }
    return "";
  }

  async function cancelTimedOutRequestProcessing() {
    const dialog = requestProcessingDialog();
    if (!dialog) return { processingDialog: false, cancelFound: false, cancelled: false };
    const cancelButton = findRequestProcessingCancelButton(dialog);
    if (!cancelButton) return { processingDialog: true, cancelFound: false, cancelled: false };
    try {
      cancelButton.click();
    } catch (error) {
      emit("warn", "gsc timed-out live test cancel failed", { error: error?.message || String(error) });
      return { processingDialog: true, cancelFound: true, cancelled: false };
    }
    const cancelled = Boolean(await waitFor(() => !requestProcessingDialog(), 3000, 100));
    return { processingDialog: true, cancelFound: true, cancelled };
  }

  function findRequestResultCloseButton() {
    const nodes = [
      ...document.querySelectorAll("button"),
      ...document.querySelectorAll('[role="button"]'),
      ...document.querySelectorAll("a"),
      ...document.querySelectorAll("[tabindex]"),
      ...document.querySelectorAll("[jsaction]"),
    ];
    const candidates = [...new Set(nodes)].filter((node) => {
      if (!visible(node)) return false;
      const text = normalizedText([
        node?.textContent,
        node?.getAttribute?.("aria-label"),
        node?.getAttribute?.("title"),
      ].filter(Boolean).join(" "));
      return TEXT.close.some((candidate) => text === candidate);
    });
    if (!candidates.length) return null;

    const dialogCandidate = candidates.find((node) => {
      try {
        return Boolean(node.closest?.('[role="dialog"], [aria-modal="true"]'));
      } catch {
        return false;
      }
    });
    return dialogCandidate || candidates[0];
  }

  function requestResultDialogPresent() {
    const dialogs = [
      ...document.querySelectorAll('[role="dialog"]'),
      ...document.querySelectorAll('[aria-modal="true"]'),
    ];
    return [...new Set(dialogs)].some((dialog) =>
      visible(dialog) && includesAny(normalizedText(dialog.textContent), TEXT.requested)
    );
  }

  async function dismissRequestResultDialog() {
    const before = requestResultDialogPresent();
    if (!before) {
      return { found: false, closed: true, dialogPresent: false };
    }

    const closeButton = findRequestResultCloseButton();
    if (!closeButton) {
      return { found: false, closed: false, dialogPresent: true };
    }

    try {
      closeButton.click();
    } catch (error) {
      emit("warn", "gsc request result dialog close click failed", {
        error: error?.message || String(error),
      });
      return { found: true, closed: false, dialogPresent: true };
    }

    const closed = Boolean(await waitFor(() => !requestResultDialogPresent(), 2000, 100));
    emit(
      closed ? "info" : "warn",
      closed ? "gsc request result dialog closed" : "gsc request result dialog still open",
      pageDiagnostic(),
    );
    return {
      found: true,
      closed,
      dialogPresent: !closed,
    };
  }

  function pageDiagnostic() {
    const inspectionState = detectPageState("")?.kind || "";
    return {
      hostname: location.hostname,
      pathname: location.pathname,
      title: document.title || "",
      controls: inspectionControls().length,
      hasInspectionTrigger: Boolean(findInspectionTrigger()),
      hasRequestButton: Boolean(findRequestIndexingButton()),
      inspectionState,
    };
  }

  async function probe() {
    emit("debug", "gsc probe started", pageDiagnostic());
    let input = findInspectionInput();
    let diagnostic = pageDiagnostic();
    const inspectionPath = /\/search-console\/inspect(?:\/|$)/u.test(location.pathname);
    const inspectionReady = (value) =>
      !["quota_blocked", "rate_limited", "failed"].includes(String(value?.inspectionState || "")) &&
      Boolean(input || value?.hasRequestButton || (inspectionPath && value?.inspectionState));
    let ready = inspectionReady(diagnostic);
    // A visible terminal error is already an inspection result. Waiting for
    // a new textbox obscures the actual error and can time out the caller.
    if (!ready && !["quota_blocked", "rate_limited", "failed"].includes(diagnostic.inspectionState)) {
      input = await ensureInspectionInput(15000);
      diagnostic = pageDiagnostic();
      ready = inspectionReady(diagnostic);
    }
    emit(
      ready ? "info" : "warn",
      ready ? "gsc inspection surface ready" : "gsc inspection surface missing",
      diagnostic,
    );
    return {
      ok: ready,
      ready,
      inspectionInput: Boolean(input),
      ...diagnostic,
    };
  }

  async function inspectURL(url) {
    emit("info", "gsc ui inspection started", { url, ...pageDiagnostic() });

    // A successful Request Indexing call leaves a modal over the GSC SPA.
    // Close any stale result modal before trying the next URL; otherwise the
    // inspection trigger/input can be present visually but not actionable.
    const staleDialogCleanup = await dismissRequestResultDialog();
    if (staleDialogCleanup.dialogPresent) {
      emit("warn", "gsc stale request result dialog blocks next inspection", {
        url,
        ...staleDialogCleanup,
      });
      return {
        ok: false,
        action: "ui_changed",
        stage: "request_dialog",
        url,
        error: "Google Search Console Request Indexing result dialog is still open",
        cleanup: staleDialogCleanup,
      };
    }

    const existingState = detectPageState(url);
    if (existingState) {
      if (existingState.kind === "rate_limited") {
        emit("warn", "gsc existing inspection page is rate limited", { url });
        return { ok: false, action: "rate_limited", stage: "inspection_result_reused", url, error: "Google Search Console returned too many requests" };
      }
      if (existingState.kind === "quota_blocked") {
        emit("warn", "gsc existing inspection page is quota blocked", { url });
        return { ok: false, action: "quota_blocked", stage: "inspection_result_reused", url, error: "Google Search Console quota was exhausted" };
      }
      if (existingState.kind === "failed") {
        emit("error", "gsc existing inspection page contains an error", { url });
        return { ok: false, action: "failed", stage: "inspection_result_reused", url, error: "Google Search Console returned an inspection error" };
      }
      emit("info", "gsc existing inspection result reused", { url, action: existingState.kind });
      return { ok: true, action: existingState.kind, stage: "inspection_result_reused", url };
    }

    const input = await ensureInspectionInput(30000);
    if (!input) {
      const diagnostic = pageDiagnostic();
      emit("error", "gsc inspection control not found", { url, ...diagnostic });
      return {
        ok: false,
        action: "ui_changed",
        stage: "inspection_control",
        url,
        error: "Google Search Console URL inspection input was not found",
        diagnostic,
      };
    }

    const beforeText = bodyText();
    setInputValue(input, url);
    await sleep(150);
    pressEnter(input);
    emit("info", "gsc inspection submitted", { url });

    const state = await waitFor(() => detectPageState(url), 90000, 500);
    if (!state) {
      const diagnostic = pageDiagnostic();
      emit("error", "gsc inspection result timed out", { url, ...diagnostic });
      return {
        ok: false,
        action: "timeout",
        stage: "inspection_result",
        url,
        error: "Google Search Console inspection result did not arrive for the requested URL",
        diagnostic,
      };
    }

    if (state.kind === "rate_limited") {
      emit("warn", "gsc inspection rate limited", { url });
      return { ok: false, action: "rate_limited", stage: "inspection_result", url, error: "Google Search Console returned too many requests" };
    }
    if (state.kind === "quota_blocked") {
      emit("warn", "gsc inspection quota blocked", { url });
      return { ok: false, action: "quota_blocked", stage: "inspection_result", url, error: "Google Search Console quota was exhausted" };
    }
    if (state.kind === "failed") {
      emit("error", "gsc inspection returned error", { url });
      return { ok: false, action: "failed", stage: "inspection_result", url, error: "Google Search Console returned an inspection error" };
    }

    // A matching target URL plus a concrete indexed/not-indexed state is the
    // acknowledgement that the GSC UI has finished inspecting this URL.
    emit("info", "gsc inspection result received", { url, action: state.kind, bodyChanged: beforeText !== bodyText() });
    return { ok: true, action: state.kind, stage: "inspection_result", url };
  }

  async function requestIndexing(url) {
    const inspected = await inspectURL(url);
    if (!inspected.ok) return inspected;

    if (inspected.action === "already_indexed") {
      emit("info", "gsc request indexing skipped because URL is indexed", { url });
      return inspected;
    }
    if (inspected.action !== "not_indexed") {
      emit("error", "gsc inspection returned unexpected state", { url, action: inspected.action });
      return {
        ok: false,
        action: "failed",
        stage: "inspection_result",
        url,
        error: `Unexpected Google Search Console inspection state: ${inspected.action}`,
      };
    }

    const button = await waitFor(findRequestIndexingButton, 30000, 500);
    if (!button) {
      const diagnostic = pageDiagnostic();
      emit("error", "gsc request indexing button not found", { url, ...diagnostic });
      return {
        ok: false,
        action: "ui_changed",
        stage: "request_button",
        url,
        error: "Google Search Console confirmed the URL is not indexed, but Request indexing button was not found",
        diagnostic,
      };
    }

    const beforeText = bodyText();
    const beforeFeedbackText = feedbackSurfaceText();
    button.click();
    emit("info", "gsc request indexing clicked", { url });

    const requestTimeoutMs = Math.max(1000, Number(globalThis.__BLOGCTL_GSC_REQUEST_TIMEOUT_MS__) || 180000);
    const result = await waitForRequestResult(beforeText, beforeFeedbackText, requestTimeoutMs);
    if (!result) {
      const diagnostic = pageDiagnostic();
      const recovery = await cancelTimedOutRequestProcessing();
      emit("error", "gsc request indexing result timed out", { url, requestTimeoutMs, ...recovery, ...diagnostic });
      if (recovery.processingDialog) {
        return {
          ok: false,
          action: "ui_changed",
          stage: "request_processing_timeout",
          url,
          error: "Google Search Console live URL test remained in progress and was reset",
          diagnostic,
          recovery,
        };
      }
      return {
        ok: false,
        action: "timeout",
        stage: "request_result",
        url,
        error: "Google Request Indexing did not return a result",
        diagnostic,
      };
    }
    if (result === "quota_blocked") {
      emit("warn", "gsc request indexing quota blocked", { url });
      return { ok: false, action: result, stage: "request_result", url, error: "Google Request Indexing daily quota was exhausted" };
    }
    if (result === "rate_limited") {
      emit("warn", "gsc request indexing rate limited", { url });
      return { ok: false, action: result, stage: "request_result", url, error: "Google Search Console rate limited Request Indexing" };
    }
    if (result === "failed") {
      emit("error", "gsc request indexing returned error", { url });
      return { ok: false, action: "failed", stage: "request_result", url, error: "Google Search Console returned a Request Indexing error" };
    }

    emit("info", "gsc request indexing confirmed", { url });
    const cleanup = await dismissRequestResultDialog();
    emit(
      cleanup.dialogPresent ? "warn" : "info",
      cleanup.dialogPresent
        ? "gsc request indexing succeeded but dialog cleanup needs fallback"
        : "gsc request indexing ready for next url",
      {
        url,
        ...cleanup,
        ...pageDiagnostic(),
      },
    );
    return {
      ok: true,
      action: "requested_indexing",
      stage: "request_result",
      url,
      cleanup,
    };
  }

  chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
    if (!message || typeof message !== "object") return false;
    let task = null;
    if (message.type === "blogctl.google.index.probe") task = probe();
    if (message.type === "blogctl.google.index.inspect") task = inspectURL(String(message.url || "").trim());
    if (message.type === "blogctl.google.index.request") task = requestIndexing(String(message.url || "").trim());
    if (!task) return false;
    Promise.resolve(task)
      .then(sendResponse)
      .catch((error) => {
        emit("error", "gsc automation exception", { error: error?.message || String(error) });
        sendResponse({ ok: false, action: "failed", error: error?.message || String(error) });
      });
    return true;
  });
})();

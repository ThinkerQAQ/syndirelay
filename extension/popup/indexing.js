"use strict";

(function (root) {
  const state = {
    initialized: false,
    active: false,
    index: {},
    gsc: { known: false, loggedIn: false, error: "" },
    busy: new Set(),
    pollTimer: null,
    inventoryRepairSignature: "",
    renderDeferred: false,
  };

  let elements = {};
  const deferredProviders = [
    { key: "so360", prefix: "index360" },
    { key: "sogou", prefix: "indexSogou" },
  ];

  function el(id) { return document.getElementById(id); }

  function setBusy(name, busy) {
    if (busy) state.busy.add(name);
    else state.busy.delete(name);
    render();
  }

  function operationKind(value) {
    switch (String(value || "idle")) {
      case "completed": return "ok";
      case "running":
      case "queued": return "checking";
      case "failed":
      case "quota_blocked": return "error";
      case "paused": return "unknown";
      case "unsupported": return "unknown";
      default: return "disabled";
    }
  }

  function operationLabel(value) {
    switch (String(value || "idle")) {
      case "completed": return "完成";
      case "running": return "运行中";
      case "queued": return "排队中";
      case "failed": return "失败";
      case "paused": return "已暂停";
      case "quota_blocked": return "配额已用尽";
      case "unsupported": return "未接入";
      default: return "未运行";
    }
  }

  function formatDate(value) {
    if (!value) return "-";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return String(value);
    return date.toLocaleString();
  }

  function inspectionStats(results) {
    const list = Array.isArray(results) ? results : [];
    let indexed = 0;
    let neverCrawled = 0;
    let withoutSitemap = 0;
    for (const result of list) {
      if (String(result?.verdict || "").toUpperCase() === "PASS") indexed += 1;
      if (!result?.lastCrawlTime) neverCrawled += 1;
      if (!Array.isArray(result?.sitemap) || result.sitemap.length === 0) withoutSitemap += 1;
    }
    return {
      checked: list.length,
      indexed,
      notIndexed: Math.max(0, list.length - indexed),
      neverCrawled,
      withoutSitemap,
    };
  }

  function inspectionRequestCandidate(result) {
    const url = String(result?.url || "").trim();
    if (!url || String(result?.verdict || "").toUpperCase() === "PASS") return false;
    if (String(result?.robotsTxtState || "").toUpperCase() === "DISALLOWED") return false;
    const indexingState = String(result?.indexingState || "").toUpperCase();
    return !["BLOCKED_BY_META_TAG", "BLOCKED_BY_HTTP_HEADER", "BLOCKED_BY_ROBOTS_TXT"].includes(indexingState);
  }

  function requestOverview(results, queue) {
    const inspectionResults = Array.isArray(results) ? results : [];
    const queueItems = Array.isArray(queue?.items) ? queue.items : [];
    const queueByURL = new Map(
      queueItems.map((item) => [String(item?.url || "").trim(), item])
    );
    const candidateURLs = new Set();
    const indexedURLs = new Set();

    for (const result of inspectionResults) {
      const url = String(result?.url || "").trim();
      if (!url) continue;
      if (String(result?.verdict || "").toUpperCase() === "PASS") {
        indexedURLs.add(url);
        continue;
      }
      if (inspectionRequestCandidate(result)) candidateURLs.add(url);
    }

    let pending = 0;
    for (const url of candidateURLs) {
      const status = String(queueByURL.get(url)?.status || "queued");
      if (!["requested", "indexed", "failed", "quota_blocked"].includes(status)) pending += 1;
    }

    for (const item of queueItems) {
      if (String(item?.status || "") === "indexed") {
        const url = String(item?.url || "").trim();
        if (url) indexedURLs.add(url);
      }
    }

    const qStats = queueStats(queue);
    return {
      candidates: candidateURLs.size,
      requested: qStats.requested,
      indexedSkipped: indexedURLs.size,
      pending,
      failed: qStats.failed,
      queueTotal: qStats.total,
    };
  }

  function queueStats(queue) {
    const items = Array.isArray(queue?.items) ? queue.items : [];
    const counts = { total: items.length, requested: 0, indexed: 0, queued: 0, failed: 0 };
    for (const item of items) {
      switch (item?.status) {
        case "requested": counts.requested += 1; break;
        case "indexed": counts.indexed += 1; break;
        case "failed":
        case "quota_blocked": counts.failed += 1; break;
        default: counts.queued += 1; break;
      }
    }
    return counts;
  }

  function setText(target, value) {
    const text = String(value);
    if (target && target.textContent !== text) target.textContent = text;
  }

  function setLink(target, value) {
    if (!target) return;
    setText(target, value || "配置不可用");
    if (value && target.getAttribute("href") !== value) target.href = value;
    else if (!value && target.hasAttribute("href")) target.removeAttribute("href");
  }

  function setStatus(target, kind, text, detail = "") {
    BlogCTLPopup.setStatus(target, kind, text, detail);
  }

  function render() {
    // A native select owns a browser popup outside the document. Keep its
    // anchor and surrounding layout stable until the user leaves the control.
    // Polling and push events still update state, so blur paints the latest data.
    if (document.activeElement === el("indexEngineFilter")) {
      if (!state.renderDeferred) console.debug("[BlogCTL indexing] render deferred while engine selector is focused");
      state.renderDeferred = true;
      return;
    }
    state.renderDeferred = false;
    const index = state.index || {};
    const inventory = index.inventory || {};
    const indexNow = index.indexNow || {};
    const baidu = index.baidu || {};
    const baiduReady = Boolean(index.baiduConfigured);
    const google = index.google || {};
    const sitemaps = google.sitemaps || {};
    const inspection = google.inspection || {};
    const queue = google.requestQueue || {};
    const iStats = inspectionStats(inspection.results);
    const qStats = queueStats(queue);
    const requestStats = requestOverview(inspection.results, queue);
    const requestCandidateCount = requestStats.candidates;
    const requestPendingCount = requestStats.pending + requestStats.failed;

    const site = index.site || {};
    const consoles = index.consoleLinks || {};
    setLink(elements.source, site.urlListURL);
    setLink(elements.sitemap, site.sitemapURL);
    setLink(elements.bingConsole, consoles.indexnow);
    setLink(elements.baiduConsole, consoles.baidu);
    setLink(elements.googleConsole, consoles.google);
    setLink(elements.so360Console, consoles.so360);
    setLink(elements.sogouConsole, consoles.sogou);
    for (const provider of deferredProviders) {
      const operation = index[provider.key] || { state: "unsupported" };
      const card = elements[provider.key];
      setStatus(card.status, operationKind(operation.state), operationLabel(operation.state), operation.error || "");
      setText(card.error, operation.error || "");
      setText(card.finishedAt, formatDate(operation.finishedAt));
      setText(card.mode, operation.mode === "full" ? "全量" : operation.mode === "incremental" ? "增量" : "-");
      for (const field of ["count", "newCount", "changedCount", "deletedCount", "unchangedCount"]) {
        setText(card[field], operation.mode && (operation.state !== "unsupported" || field === "count") ? Number(operation[field] || 0) : "-");
      }
      setText(card.http, operation.httpStatus || (operation.mode ? "未请求" : "-"));
      card.incremental.disabled = !(Number(inventory.total || 0) > 0) || state.busy.has(provider.key);
      card.full.disabled = card.incremental.disabled;
    }
    setText(elements.inventoryTotal, Number(inventory.total || 0) || "-");
    const fingerprintCoverage = Number(inventory.fingerprintCoverage || 0);
    const inventoryTotal = Number(inventory.total || 0);
    setText(
      elements.inventoryFingerprintCoverage,
      inventoryTotal ? `${fingerprintCoverage} / ${inventoryTotal}` : "-"
    );
    setText(elements.inventoryFetchedAt, formatDate(inventory.fetchedAt));

    setStatus(elements.indexNowStatus, operationKind(indexNow.state), operationLabel(indexNow.state), indexNow.error || "");
    setText(elements.indexNowFinishedAt, formatDate(indexNow.finishedAt));
    setText(elements.indexNowMode, indexNow.mode === "full" ? "全量" : indexNow.mode === "incremental" ? "增量" : "-");
    setText(elements.indexNowCount, Number.isFinite(Number(indexNow.count)) ? Number(indexNow.count) : "-");
    setText(elements.indexNowNewCount, Number.isFinite(Number(indexNow.newCount)) ? Number(indexNow.newCount) : "-");
    setText(elements.indexNowChangedCount, Number.isFinite(Number(indexNow.changedCount)) ? Number(indexNow.changedCount) : "-");
    setText(elements.indexNowDeletedCount, Number.isFinite(Number(indexNow.deletedCount)) ? Number(indexNow.deletedCount) : "-");
    setText(elements.indexNowUnchangedCount, Number.isFinite(Number(indexNow.unchangedCount)) ? Number(indexNow.unchangedCount) : "-");
    setText(elements.indexNowHTTP, indexNow.httpStatus || (indexNow.state === "completed" && Number(indexNow.count || 0) === 0 ? "未请求" : "-"));

    if (baiduReady) {
      setStatus(elements.baiduStatus, operationKind(baidu.state), operationLabel(baidu.state), baidu.error || "");
    } else {
      setStatus(elements.baiduStatus, "disabled", "未配置", "请先在「环境与配置」中填写 Baidu Push Token。");
    }
    setText(elements.baiduFinishedAt, formatDate(baidu.finishedAt));
    setText(elements.baiduMode, baidu.mode === "full" ? "全量" : baidu.mode === "incremental" ? "增量" : "-");
    setText(elements.baiduCount, Number.isFinite(Number(baidu.count)) ? Number(baidu.count) : "-");
    setText(elements.baiduNewCount, Number.isFinite(Number(baidu.newCount)) ? Number(baidu.newCount) : "-");
    setText(elements.baiduChangedCount, Number.isFinite(Number(baidu.changedCount)) ? Number(baidu.changedCount) : "-");
    setText(elements.baiduDeletedCount, Number.isFinite(Number(baidu.deletedCount)) ? Number(baidu.deletedCount) : "-");
    setText(elements.baiduUnchangedCount, Number.isFinite(Number(baidu.unchangedCount)) ? Number(baidu.unchangedCount) : "-");
    setText(elements.baiduHTTP, baidu.httpStatus || (baidu.state === "completed" && Number(baidu.count || 0) === 0 ? "未请求" : "-"));
    setText(elements.baiduError, baidu.error || "");

    setStatus(
      elements.googleCredentials,
      google.credentialsConfigured ? "ok" : "error",
      google.credentialsConfigured
        ? "Service Account 已配置"
        : google.credentialsError
          ? "Service Account 配置无效"
          : "Service Account 未配置",
      google.credentialsError || "",
    );
    setStatus(elements.googleSitemapStatus, operationKind(sitemaps.state), operationLabel(sitemaps.state), sitemaps.error || "");
    setText(elements.googleSitemapFinishedAt, formatDate(sitemaps.finishedAt));
    setText(elements.googleSitemapCount, Number(sitemaps.count || 0) || "-");
    setText(elements.googleSitemapError, sitemaps.error || "");

    setStatus(elements.googleInspectionStatus, operationKind(inspection.state), operationLabel(inspection.state), inspection.error || "");
    setText(elements.inspectionInspected, iStats.checked);
    setText(elements.inspectionIndexed, iStats.indexed);
    setText(elements.inspectionNotIndexed, iStats.notIndexed);
    setText(elements.inspectionNeverCrawled, iStats.neverCrawled);
    setText(elements.inspectionWithoutSitemap, iStats.withoutSitemap);
    setText(elements.inspectionRemaining, Number.isFinite(Number(inspection.remaining)) ? Number(inspection.remaining) : "-");

    const queueState = String(queue.state || "idle");
    if (queueState === "idle" && requestPendingCount > 0) {
      setStatus(elements.googleRequestStatus, "unknown", "待启动");
    } else if (queueState === "completed" && requestPendingCount > 0) {
      setStatus(elements.googleRequestStatus, "unknown", "有新候选");
    } else if (queueState === "running" && (!state.gsc.known || !state.gsc.ready)) {
      setStatus(elements.googleRequestStatus, "checking", "正在恢复 GSC", state.gsc.error || "");
    } else {
      setStatus(elements.googleRequestStatus, operationKind(queueState), operationLabel(queueState), queue.lastError || "");
    }

    if (state.gsc.known) {
      setStatus(
        elements.googleGSCStatus,
        state.gsc.ready ? "ok" : "error",
        state.gsc.ready ? "GSC 已就绪" : state.gsc.loggedIn ? "GSC 页面未就绪" : "GSC 未就绪",
        state.gsc.error || "",
      );
    } else {
      setStatus(elements.googleGSCStatus, "disabled", queueState === "running" ? "正在检测" : "未检测");
    }

    setText(elements.requestTotal, requestStats.candidates);
    setText(elements.requestRequested, requestStats.requested);
    setText(elements.requestIndexed, requestStats.indexedSkipped);
    setText(elements.requestQueued, requestStats.pending);
    setText(elements.requestFailed, requestStats.failed);
    setText(elements.requestPosition, requestStats.queueTotal ? `${Math.min(Number(queue.currentIndex || 0) + 1, requestStats.queueTotal)} / ${requestStats.queueTotal}` : "-");
    setText(elements.requestError, queue.lastError || "");

    const inventoryReady = Number(inventory.total || 0) > 0;
    const inspectionReady = iStats.checked > 0;
    elements.refreshInventory.disabled = state.busy.has("inventory");
    elements.indexNowSubmitIncremental.disabled = !inventoryReady || state.busy.has("indexnow");
    elements.indexNowSubmitFull.disabled = !inventoryReady || state.busy.has("indexnow");
    elements.baiduSubmitIncremental.disabled = !inventoryReady || !baiduReady || state.busy.has("baidu");
    elements.baiduSubmitFull.disabled = !inventoryReady || !baiduReady || state.busy.has("baidu");
    const sitemapState = String(sitemaps.state || "idle");
    elements.googleSitemaps.disabled = !google.credentialsConfigured || ["queued", "running"].includes(sitemapState) || state.busy.has("sitemaps");
    const inspectionComplete = inventoryReady && iStats.checked >= Number(inventory.total || 0);
    const inspectionState = String(inspection.state || "idle");
    elements.googleInspect.disabled = !inventoryReady || !google.credentialsConfigured || inspectionComplete || ["queued", "running"].includes(inspectionState) || state.busy.has("inspect");
    elements.googleInspect.textContent = inspectionComplete
      ? "Inspection 已完成"
      : inspectionState === "quota_blocked"
        ? "重试检查"
        : "检查下一批";

    elements.googleRequestStart.disabled = !inspectionReady || requestPendingCount === 0 || ["running", "paused", "quota_blocked"].includes(queueState) || state.busy.has("request");
    elements.googleRequestPause.disabled = queueState !== "running" || state.busy.has("request");
    elements.googleRequestResume.disabled = !["paused", "quota_blocked"].includes(queueState) || state.busy.has("request");
    elements.googleOpen.disabled = state.busy.has("gsc");
  }

  async function run(name, message, successMessage) {
    setBusy(name, true);
    BlogCTLPopup.setMessage(elements.message);
    try {
      const response = await BlogCTLPopup.send(message.type, message.payload || {});
      if (response.index) state.index = response.index;
      if (response.google) state.gsc = response.google;
      render();
      if (response.result?.supported === false) {
        BlogCTLPopup.setMessage(elements.message, response.result.message, "error");
      } else if (successMessage) {
        const jobSuffix = response.job?.id ? ` · 已加入任务 ${response.job.id}` : "";
        BlogCTLPopup.setMessage(elements.message, successMessage + jobSuffix, "ok");
      }
      return response;
    } catch (error) {
      BlogCTLPopup.setMessage(elements.message, BlogCTLPopup.errorMessage(error), "error");
      throw error;
    } finally {
      setBusy(name, false);
    }
  }

  async function refresh() {
    if (!state.active) return;
    const selectorFocused = () => document.activeElement === el("indexEngineFilter");
    if (!selectorFocused()) BlogCTLPopup.setMessage(elements.message);
    try {
      const response = await BlogCTLPopup.send("blogctl.index.get");
      state.index = response.index || {};
      if (response.google) state.gsc = response.google;

      const inventory = state.index?.inventory || {};
      const inspection = state.index?.google?.inspection || {};
      const total = Number(inventory.total || 0);
      const checked = Array.isArray(inspection.results) ? inspection.results.length : Number(inspection.inspected || 0);
      const repairSignature = `${total}:${checked}:${String(inventory.fetchedAt || "")}`;
      if (total > 0 && checked > total && state.inventoryRepairSignature !== repairSignature) {
        state.inventoryRepairSignature = repairSignature;
        const repaired = await BlogCTLPopup.send("blogctl.index.inventory.refresh");
        if (repaired.index) state.index = repaired.index;
      }
      render();
      if (!selectorFocused()) BlogCTLPopup.refreshBridgeIndicator().catch(() => {});
    } catch (error) {
      if (!selectorFocused()) {
        BlogCTLPopup.setMessage(elements.message, BlogCTLPopup.errorMessage(error), "error");
        BlogCTLPopup.refreshBridgeIndicator().catch(() => {});
      } else {
        console.warn("[BlogCTL indexing] refresh failed while engine selector is focused", BlogCTLPopup.errorMessage(error));
      }
    }
  }

  async function refreshInventory() {
    await run("inventory", { type: "blogctl.index.inventory.refresh" }, "URL Inventory 已刷新。");
  }

  async function submitIndexNow(mode) {
    const label = mode === "full" ? "全量" : "增量";
    await run("indexnow", {
      type: "blogctl.index.indexnow.submit",
      payload: { mode },
    }, `IndexNow ${label}任务已创建。`);
  }

  async function submitBaidu(mode) {
    const label = mode === "full" ? "全量" : "增量";
    await run("baidu", {
      type: "blogctl.index.baidu.submit",
      payload: { mode },
    }, `Baidu Search Resource ${label}任务已创建。`);
  }

  async function submitGoogleSitemaps() {
    await run("sitemaps", { type: "blogctl.index.google.sitemaps" }, "Google Sitemap 任务已创建。");
  }

  async function submitDeferredProvider(provider, mode) {
    await run(provider, { type: `blogctl.index.${provider}.submit`, payload: { mode } });
  }

  async function inspectGoogle() {
    const refreshed = await BlogCTLPopup.send("blogctl.index.inventory.refresh");
    if (refreshed.index) state.index = refreshed.index;
    render();

    const inspection = state.index?.google?.inspection || {};
    const inventory = state.index?.inventory || {};
    const total = Number(inventory.total || 0);
    const checked = Array.isArray(inspection.results) ? inspection.results.length : Number(inspection.inspected || 0);
    if (total > 0 && checked >= total) {
      BlogCTLPopup.setMessage(elements.message, "Google URL Inspection 已覆盖全部 URL。", "ok");
      return;
    }
    const offset = Number.isInteger(inspection.nextOffset)
      ? inspection.nextOffset
      : checked;

    const remaining = Math.max(0, total - offset);
    const limit = Math.min(2000, remaining || 2000);
    await run("inspect", {
      type: "blogctl.index.google.inspect",
      payload: { offset, limit },
    }, `Google URL Inspection 任务已创建：offset ${offset}, limit ${limit}。`);
  }

  async function openGSC() {
    await run("gsc", { type: "blogctl.index.google.open" }, "Google Search Console 已打开并检测。");
  }

  async function startGoogleRequestQueue() {
    await run("request", { type: "blogctl.index.google.request.start" }, "Request Indexing 队列已启动。");
  }

  async function pauseGoogleRequestQueue() {
    await run("request", { type: "blogctl.index.google.request.pause" }, "Request Indexing 队列已暂停。");
  }

  async function resumeGoogleRequestQueue() {
    await run("request", { type: "blogctl.index.google.request.resume" }, "Request Indexing 队列已继续。");
  }

  function onRuntimeMessage(message) {
    if (!state.active) return;
    if (message?.type === "blogctl.index.gsc") {
      state.gsc = message.google || state.gsc;
      render();
      return;
    }
    if (message?.type !== "blogctl.index.progress") return;
    state.index = message.index || {};
    if (message.google) state.gsc = message.google;
    render();
  }

  function init() {
    if (state.initialized) return;
    elements = {
      source: el("indexSource"),
      sitemap: el("indexSitemap"),
      bingConsole: el("indexBingConsole"),
      baiduConsole: el("indexBaiduConsole"),
      googleConsole: el("indexGoogleConsole"),
      so360Console: el("index360Console"),
      sogouConsole: el("indexSogouConsole"),
      inventoryTotal: el("indexInventoryTotal"),
      inventoryFingerprintCoverage: el("indexInventoryFingerprintCoverage"),
      inventoryFetchedAt: el("indexInventoryFetchedAt"),
      refreshInventory: el("indexRefreshInventory"),
      indexNowStatus: el("indexNowStatus"),
      indexNowFinishedAt: el("indexNowFinishedAt"),
      indexNowMode: el("indexNowMode"),
      indexNowCount: el("indexNowCount"),
      indexNowNewCount: el("indexNowNewCount"),
      indexNowChangedCount: el("indexNowChangedCount"),
      indexNowDeletedCount: el("indexNowDeletedCount"),
      indexNowUnchangedCount: el("indexNowUnchangedCount"),
      indexNowHTTP: el("indexNowHTTP"),
      indexNowSubmitIncremental: el("indexNowSubmitIncremental"),
      indexNowSubmitFull: el("indexNowSubmitFull"),
      baiduStatus: el("indexBaiduStatus"),
      baiduError: el("indexBaiduError"),
      baiduFinishedAt: el("indexBaiduFinishedAt"),
      baiduMode: el("indexBaiduMode"),
      baiduCount: el("indexBaiduCount"),
      baiduNewCount: el("indexBaiduNewCount"),
      baiduChangedCount: el("indexBaiduChangedCount"),
      baiduDeletedCount: el("indexBaiduDeletedCount"),
      baiduUnchangedCount: el("indexBaiduUnchangedCount"),
      baiduHTTP: el("indexBaiduHTTP"),
      baiduSubmitIncremental: el("indexBaiduSubmitIncremental"),
      baiduSubmitFull: el("indexBaiduSubmitFull"),
      googleCredentials: el("indexGoogleCredentials"),
      googleSitemapStatus: el("indexGoogleSitemapStatus"),
      googleSitemapError: el("indexGoogleSitemapError"),
      googleSitemapFinishedAt: el("indexGoogleSitemapFinishedAt"),
      googleSitemapCount: el("indexGoogleSitemapCount"),
      googleSitemaps: el("indexGoogleSitemaps"),
      googleInspectionStatus: el("indexGoogleInspectionStatus"),
      inspectionInspected: el("indexInspectionInspected"),
      inspectionIndexed: el("indexInspectionIndexed"),
      inspectionNotIndexed: el("indexInspectionNotIndexed"),
      inspectionNeverCrawled: el("indexInspectionNeverCrawled"),
      inspectionWithoutSitemap: el("indexInspectionWithoutSitemap"),
      inspectionRemaining: el("indexInspectionRemaining"),
      googleInspect: el("indexGoogleInspect"),
      googleRequestStatus: el("indexGoogleRequestStatus"),
      googleGSCStatus: el("indexGoogleGSCStatus"),
      requestTotal: el("indexRequestTotal"),
      requestRequested: el("indexRequestRequested"),
      requestIndexed: el("indexRequestIndexed"),
      requestQueued: el("indexRequestQueued"),
      requestFailed: el("indexRequestFailed"),
      requestPosition: el("indexRequestPosition"),
      requestError: el("indexRequestError"),
      googleOpen: el("indexGoogleOpen"),
      googleRequestStart: el("indexGoogleRequestStart"),
      googleRequestPause: el("indexGoogleRequestPause"),
      googleRequestResume: el("indexGoogleRequestResume"),
      message: el("indexingMessage"),
    };

    for (const provider of deferredProviders) {
      const prefix = provider.prefix;
      const card = elements[provider.key] = {
        status: el(prefix + "Status"), error: el(prefix + "Error"), finishedAt: el(prefix + "FinishedAt"),
        mode: el(prefix + "Mode"), count: el(prefix + "Count"), newCount: el(prefix + "NewCount"),
        changedCount: el(prefix + "ChangedCount"), deletedCount: el(prefix + "DeletedCount"),
        unchangedCount: el(prefix + "UnchangedCount"), http: el(prefix + "HTTP"),
        incremental: el(prefix + "SubmitIncremental"), full: el(prefix + "SubmitFull"),
      };
      card.incremental.addEventListener("click", () => submitDeferredProvider(provider.key, "incremental"));
      card.full.addEventListener("click", () => submitDeferredProvider(provider.key, "full"));
    }

    // Search engines share one selector and one presentation
    // contract. Filtering is purely visual; background jobs keep running.
    const engineFilter = el("indexEngineFilter");
    const engineCards = [...document.querySelectorAll(
      '[data-panel="indexing"] [data-index-engine]'
    )];
    const knownEngines = new Set(["all", ...engineCards.map((card) => card.dataset.indexEngine)]);
    const stored = localStorage.getItem("blogctl.indexEngine");
    engineFilter.value = knownEngines.has(stored) ? stored : "indexnow";
    const applyEngineFilter = () => {
      const active = engineFilter.value;
      for (const card of engineCards) {
        card.hidden = active !== "all" && card.dataset.indexEngine !== active;
      }
      localStorage.setItem("blogctl.indexEngine", active);
    };
    engineFilter.addEventListener("change", applyEngineFilter);
    engineFilter.addEventListener("blur", () => {
      if (!state.active) return;
      if (state.renderDeferred) {
        console.debug("[BlogCTL indexing] applying deferred render after engine selector blur");
        render();
      }
      refresh();
    });
    applyEngineFilter();

    elements.refreshInventory.addEventListener("click", refreshInventory);
    elements.indexNowSubmitIncremental.addEventListener("click", () => submitIndexNow("incremental"));
    elements.indexNowSubmitFull.addEventListener("click", () => submitIndexNow("full"));
    elements.baiduSubmitIncremental.addEventListener("click", () => submitBaidu("incremental"));
    elements.baiduSubmitFull.addEventListener("click", () => submitBaidu("full"));
    elements.googleSitemaps.addEventListener("click", submitGoogleSitemaps);
    elements.googleInspect.addEventListener("click", inspectGoogle);
    elements.googleOpen.addEventListener("click", openGSC);
    elements.googleRequestStart.addEventListener("click", startGoogleRequestQueue);
    elements.googleRequestPause.addEventListener("click", pauseGoogleRequestQueue);
    elements.googleRequestResume.addEventListener("click", resumeGoogleRequestQueue);
    // Extension listens for push events; Web Console has the same Feature
    // refresh() polling and must not require privileged chrome APIs in page.
    if (globalThis.chrome?.runtime?.onMessage) {
      chrome.runtime.onMessage.addListener(onRuntimeMessage);
    }
    state.initialized = true;
    render();
  }

  function activate() {
    state.active = true;
    refresh();
    if (!state.pollTimer) state.pollTimer = setInterval(() => refresh(), 2500);
  }

  function deactivate() {
    state.active = false;
    if (state.pollTimer) {
      clearInterval(state.pollTimer);
      state.pollTimer = null;
    }
  }

  root.BlogCTLIndexing = { init, activate, deactivate, refresh };
})(globalThis);

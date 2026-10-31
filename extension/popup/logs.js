"use strict";

const BlogCTLLogs = (() => {
  const state = {
    initialized: false,
    active: false,
    entries: [],
    path: "",
    configuredLevel: "",
    pollTimer: null,
    pointerSelecting: false,
  };

  let status, pathValue, output, logViewer, queryInput, levelFilter, autoRefresh, selectAllButton, copyButton, refreshButton, clearButton, message;

  function formatAttribute(value) {
    if (typeof value === "string") return value;
    try {
      return JSON.stringify(value);
    } catch {
      return String(value);
    }
  }

  function rawEntryParts(entry) {
    const raw = String(entry?.raw || "");
    const match = raw.match(/^(\[[^\]]+\])\s+(DEBUG|INFO|WARN|ERROR)\s*(.*)$/u);
    if (!match) return { time: "", level: "", body: raw };
    return { time: match[1], level: match[2], body: match[3] };
  }

  function entryLevel(entry) {
    const explicit = String(entry?.level || "").trim().toUpperCase();
    return explicit || rawEntryParts(entry).level;
  }

  function formatEntry(entry) {
    if (entry?.raw) return String(entry.raw);
    const time = entry?.time ? BlogCTLPopup.formatTime(entry.time) : "--:--:--";
    const level = String(entry?.level || "INFO").toUpperCase().padEnd(5, " ");
    const text = String(entry?.message || "");
    const attributes = Object.entries(entry?.attributes || {})
      .map(([key, value]) => `${key}=${formatAttribute(value)}`)
      .join(" ");
    return [`[${time}]`, level, text, attributes].filter(Boolean).join(" ");
  }

  function filteredEntries() {
    const selected = String(levelFilter?.value || "").toUpperCase();
    const query = String(queryInput?.value || "").trim().toLowerCase();
    return state.entries.filter((entry) => {
      if (selected && entryLevel(entry) !== selected) return false;
      if (query && !formatEntry(entry).toLowerCase().includes(query)) return false;
      return true;
    });
  }

  function selectedLogText() {
    return logViewer?.selectedText() || "";
  }

  function selectionLocksViewer() {
    return state.pointerSelecting || selectedLogText() !== "";
  }

  function renderOutput(entries) {
    const text = entries.length ? entries.map(formatEntry).join("\n") : "暂无日志";
    logViewer.setText(text, autoRefresh.checked);
  }

  function render({ preserveSelection = false } = {}) {
    const entries = filteredEntries();
    if (!(preserveSelection && selectionLocksViewer())) {
      renderOutput(entries);
    }
    pathValue.textContent = state.path || "-";
    BlogCTLPopup.setStatus(
      status,
      "ok",
      `${String(state.configuredLevel || "info").toUpperCase()} · ${entries.length}${entries.length !== state.entries.length ? ` / ${state.entries.length}` : ""} 条`,
      state.path || "",
    );
  }

  async function refresh({ quiet = false } = {}) {
    if (!state.active && quiet) return;
    if (!quiet) refreshButton.disabled = true;
    try {
      const response = await BlogCTLPopup.send("blogctl.logs", { limit: 1000 });
      state.entries = Array.isArray(response.entries) ? response.entries : [];
      state.path = String(response.path || "");
      state.configuredLevel = String(response.level || "info");
      render({ preserveSelection: quiet });
      if (!quiet) BlogCTLPopup.setMessage(message, "日志已刷新。", "ok");
    } catch (error) {
      BlogCTLPopup.setStatus(status, "error", "读取失败", BlogCTLPopup.errorMessage(error));
      if (!quiet) BlogCTLPopup.setMessage(message, BlogCTLPopup.errorMessage(error), "error");
    } finally {
      refreshButton.disabled = false;
    }
  }

  function stopPolling() {
    if (state.pollTimer) {
      clearInterval(state.pollTimer);
      state.pollTimer = null;
    }
  }

  function syncPolling() {
    stopPolling();
    if (!state.active || !autoRefresh.checked) return;
    state.pollTimer = setInterval(() => {
      refresh({ quiet: true });
    }, 1500);
  }

  function selectAllLogs() {
    if (!logViewer || logViewer.getText() === "暂无日志") return;
    if (autoRefresh.checked) {
      autoRefresh.checked = false;
      syncPolling();
    }
    logViewer.selectAll();
    BlogCTLPopup.setMessage(message, "已选中当前日志；自动刷新已暂停。", "ok");
  }

  async function writeClipboard(text) {
    if (globalThis.navigator?.clipboard?.writeText) {
      await globalThis.navigator.clipboard.writeText(text);
      return;
    }
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.append(textarea);
    textarea.select();
    const copied = document.execCommand("copy");
    textarea.remove();
    if (!copied) throw new Error("浏览器拒绝复制日志");
  }

  async function copyLogs() {
    const selected = selectedLogText();
    const current = logViewer?.getText() || "";
    const text = selected || (current === "暂无日志" ? "" : current);
    if (!text) {
      BlogCTLPopup.setMessage(message, "没有可复制的日志。", "error");
      return;
    }
    copyButton.disabled = true;
    try {
      await writeClipboard(text);
      BlogCTLPopup.setMessage(message, selected ? "已复制选中的日志。" : "已复制当前可见日志。", "ok");
    } catch (error) {
      BlogCTLPopup.setMessage(message, BlogCTLPopup.errorMessage(error), "error");
    } finally {
      copyButton.disabled = false;
    }
  }

  async function clearLogs() {
    clearButton.disabled = true;
    BlogCTLPopup.setMessage(message, "正在清空日志…");
    try {
      await BlogCTLPopup.send("blogctl.logs.clear");
      await refresh({ quiet: true });
      BlogCTLPopup.setMessage(message, "日志已清空。", "ok");
    } catch (error) {
      BlogCTLPopup.setMessage(message, BlogCTLPopup.errorMessage(error), "error");
    } finally {
      clearButton.disabled = false;
    }
  }

  function init() {
    if (state.initialized) return;
    status = document.getElementById("logsStatus");
    pathValue = document.getElementById("logPath");
    output = document.getElementById("logOutput");
    logViewer = BlogCTLLogEditor.create(output);
    queryInput = document.getElementById("logQuery");
    levelFilter = document.getElementById("logLevelFilter");
    autoRefresh = document.getElementById("logAutoRefresh");
    selectAllButton = document.getElementById("selectAllLogs");
    copyButton = document.getElementById("copyLogs");
    refreshButton = document.getElementById("refreshLogs");
    clearButton = document.getElementById("clearLogs");
    message = document.getElementById("logsMessage");

    refreshButton.addEventListener("click", () => refresh());
    clearButton.addEventListener("click", clearLogs);
    selectAllButton.addEventListener("click", selectAllLogs);
    document.getElementById("findInLogs").addEventListener("click", () => logViewer.find());
    copyButton.addEventListener("mousedown", (event) => event.preventDefault());
    copyButton.addEventListener("click", copyLogs);
    queryInput.addEventListener("input", () => render());
    levelFilter.addEventListener("change", () => render());
    autoRefresh.addEventListener("change", syncPolling);
    output.addEventListener("pointerdown", () => {
      state.pointerSelecting = true;
    });
    globalThis.addEventListener("pointerup", () => {
      state.pointerSelecting = false;
    });
    globalThis.addEventListener("pointercancel", () => {
      state.pointerSelecting = false;
    });
    state.initialized = true;
  }

  function activate() {
    state.active = true;
    refresh({ quiet: true });
    syncPolling();
  }

  function deactivate() {
    state.active = false;
    stopPolling();
    state.pointerSelecting = false;
  }

  return { init, activate, deactivate, refresh };
})();

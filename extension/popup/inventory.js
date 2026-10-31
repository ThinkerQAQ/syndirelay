"use strict";

(function (root) {
  // Account-scoped, read-only remote inventory. Binding belongs to Update.
  const state = {
    active: false,
    initialized: false,
    platforms: [],
    records: {},
    loading: new Set(),
  };
  let container, message, platformSelect, statusSelect, summary, refreshButton;

  const platforms = () => BlogCTLSyncModel.visiblePlatforms(state.platforms);
  const selectedPlatforms = () => platforms().filter((platform) =>
    !platformSelect.value || platform.id === platformSelect.value);
  const matchesStatus = (item) =>
    !statusSelect.value ||
    (statusSelect.value === "published" ? Boolean(item.published) : !item.published);

  function render() {
    container.replaceChildren();
    let visibleCount = 0;
    let totalCount = 0;
    let errors = 0;
    const visible = selectedPlatforms();

    for (const platform of visible) {
      const record = state.records[platform.id];
      const busy = state.loading.has(platform.id);
      const items = record?.items ?? [];
      const filtered = items.filter(matchesStatus);
      visibleCount += filtered.length;
      totalCount += items.length;
      if (record?.error) errors++;

      const card = document.createElement("section");
      card.className = "platform-choice-card remote-inventory-card";

      const heading = document.createElement("div");
      heading.className = "job-platform-main";
      const label = document.createElement("strong");
      label.textContent = platform.label || platform.id;
      // Top-level status filtering and the single Refresh control already
      // provide these actions; keep platform headings free of duplicates.
      heading.append(label);
      card.append(heading);

      if (record?.error) {
        const error = document.createElement("p");
        error.className = "error-text";
        error.textContent = record.error;
        card.append(error);
      }
      if (record?.partial) {
        const hint = document.createElement("p");
        hint.className = "card-hint";
        hint.textContent = record.partial;
        card.append(hint);
      }

      // Direct rows rather than a nested draft/published accordion. The
      // state selector above determines which items are shown.
      for (const item of filtered) {
        const row = document.createElement("div");
        row.className = "article-match-row";
        const identity = document.createElement("div");
        identity.className = "inventory-article-content";
        const title = document.createElement("strong");
        title.className = "inventory-article-title";
        if (item.title) title.setAttribute("data-i18n-ignore", "");
        title.textContent = item.title || "(无标题)";
        const metadata = document.createElement("div");
        metadata.className = "inventory-article-meta";
        const status = document.createElement("span");
        status.className = item.published
          ? "inventory-article-state is-published"
          : "inventory-article-state is-draft";
        status.textContent = item.published ? "已发布" : "草稿";
        const remoteID = document.createElement("span");
        remoteID.className = "inventory-article-id";
        remoteID.textContent = `ID ${item.id}`;
        metadata.append(status, remoteID);
        identity.append(title, metadata);
        row.append(identity);

        const target = BlogCTLSyncModel.articleMatchLink(platform.id, item);
        if (target) {
          const link = document.createElement("a");
          link.className = "inventory-article-action";
          link.textContent = target.label;
          link.href = target.url;
          link.target = "_blank";
          link.rel = "noopener noreferrer";
          row.append(link);
        }
        card.append(row);
      }

      if (record && !record.error && !busy && !filtered.length) {
        const empty = document.createElement("p");
        empty.className = "card-hint";
        empty.textContent = items.length ? "当前状态没有文章。" : "当前读取范围内没有远端文章。";
        card.append(empty);
      }
      container.append(card);
    }

    const waiting = visible.filter((platform) =>
      state.loading.has(platform.id) || !state.records[platform.id]).length;
    summary.textContent = `共 ${totalCount} 篇远端文章 · 当前显示 ${visibleCount} 篇`
      + (waiting ? ` · 读取中 ${waiting} 个平台` : "")
      + (errors ? ` · 失败 ${errors} 个平台` : "");
    refreshButton.disabled = state.loading.size > 0;
  }

  async function loadPlatform(platformID) {
    if (!state.active || state.loading.has(platformID)) return;
    state.loading.add(platformID);
    render();
    try {
      const result = await BlogCTLPopup.send("blogctl.remote.inventory", { platform: platformID });
      state.records[platformID] = { items: result.items ?? [], partial: result.partial || "" };
    } catch (error) {
      state.records[platformID] = { items: [], error: BlogCTLPopup.errorMessage(error) };
    } finally {
      state.loading.delete(platformID);
      if (state.active) render();
    }
  }

  async function refresh() {
    if (!state.active) return;
    BlogCTLPopup.setMessage(message);
    try {
      const response = await BlogCTLPopup.send("blogctl.status");
      if (!state.active) return;
      state.platforms = response.status?.platforms ?? [];
      const previous = platformSelect.value;
      platformSelect.replaceChildren();
      const all = document.createElement("option");
      all.value = "";
      all.textContent = "全部平台";
      platformSelect.append(all);
      for (const platform of platforms()) {
        const option = document.createElement("option");
        option.value = platform.id;
        option.textContent = platform.label || platform.id;
        platformSelect.append(option);
      }
      platformSelect.value = platforms().some((platform) => platform.id === previous) ? previous : "";
      render();
      // Independent requests: one platform's failure does not block others.
      await Promise.all(selectedPlatforms().map((platform) => loadPlatform(platform.id)));
    } catch (error) {
      BlogCTLPopup.setMessage(message, BlogCTLPopup.errorMessage(error), "error");
    }
  }

  function init() {
    if (state.initialized) return;
    container = document.getElementById("inventoryPlatforms");
    platformSelect = document.getElementById("inventoryPlatform");
    statusSelect = document.getElementById("inventoryStatus");
    summary = document.getElementById("inventorySummary");
    refreshButton = document.getElementById("refreshInventory");
    message = document.getElementById("inventoryMessage");
    platformSelect.addEventListener("change", () => {
      render();
      for (const platform of selectedPlatforms()) {
        if (!state.records[platform.id]) loadPlatform(platform.id);
      }
    });
    statusSelect.addEventListener("change", render);
    refreshButton.addEventListener("click", refresh);
    state.initialized = true;
  }

  function activate() { state.active = true; refresh(); }
  function deactivate() { state.active = false; }

  root.BlogCTLInventory = { init, activate, deactivate, refresh };
})(globalThis);

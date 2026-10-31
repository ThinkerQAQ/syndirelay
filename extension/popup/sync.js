"use strict";

// Update-target discovery. No PublicationBinding mutations: selected remote IDs
// are forwarded directly to an explicit, durable task snapshot.
(function (root) {
  const state = {
    initialized: false, active: false, article: "", matches: {},
    selected: new Map(), pending: new Set(), generation: 0,
  };
  let detectButton, message;

  function selectedTargets() {
    return [...state.selected.values()].map((item) => ({ ...item }));
  }

  function refreshToolbar() {
    if (!detectButton || root.BlogCTLDrafts?.currentMode?.()==="create") return;
    detectButton.disabled = !state.article || state.pending.size > 0 ||
      root.BlogCTLDrafts?.isJobRunning?.() ||
      !(root.BlogCTLDrafts?.selectedPlatformIDs?.().length);
  }

  function render() {
    refreshToolbar();
    document.dispatchEvent(new CustomEvent("blogctl:association-results-changed"));
  }

  function clearMatches() {
    state.matches = {};
    state.selected.clear();
    ++state.generation;
    state.pending.clear();
    render();
  }

  // The same bounded discovery cadence as Create (3 workers).
  // A generation belongs to the selected local article, not the individual
  // platform. A per-platform refresh never cancels another platform's result.
  async function refreshArticleMatches(
    platformIDs = root.BlogCTLDrafts?.selectedPlatformIDs?.() ?? [],
    { force = true } = {}
  ) {
    if (!state.active || !state.article) return;
    const article = state.article;
    const generation = state.generation;
    const platforms = BlogCTLSyncModel.visiblePlatformIDs([...new Set(platformIDs)])
      .filter((id) => !state.pending.has(id) && (force || !state.matches[id]));
    if (!platforms.length) return;
    for (const platform of platforms) {
      state.matches[platform] = { text: "正在检测远端关联…", items: [] };
      if (force) {
        for (const key of state.selected.keys()) {
          if (key.startsWith(platform + ":")) state.selected.delete(key);
        }
      }
      state.pending.add(platform);
    }
    render();
    const workerCount = Math.min(3, platforms.length);
    await Promise.all(Array.from({ length: workerCount }, async (_, worker) => {
      for (let index = worker; index < platforms.length; index += workerCount) {
        const platform = platforms[index];
        let match;
        try {
          const data = await BlogCTLPopup.send("blogctl.article.match", { article, platform });
          match = data.match || { text: "未找到相关远端文章", items: [] };
        } catch (error) {
          match = {
            text: "检测失败：" + BlogCTLPopup.errorMessage(error),
            items: [], failed: true,
          };
        }
        // A change of article/mode invalidates every old response. Never
        // resurrect stale post IDs into a newly selected article's UI.
        if (!state.active || generation !== state.generation || article !== state.article) return;
        state.matches[platform] = match;
        state.pending.delete(platform);
        render();
      }
    }));
  }

  function ensureMatches(platformIDs = root.BlogCTLDrafts?.selectedPlatformIDs?.() ?? []) {
    if (!state.active || !state.article) return;
    return refreshArticleMatches(platformIDs, { force: false });
  }

  function availableForUpdate(platform, item) {
    if (!item || !String(item.id || "").trim() || item.localOnly) return false;
    if (item.published) {
      // Medium edits an existing published story as a saved, pending revision.
      // Its Save and publish button stays exclusively in the user's hands.
      if (platform.id === "medium") return platform.capabilities?.publishedDraftEdit === true;
      if (platform.id === "toutiao" && !String(item.updatedAt || "").trim()) return false;
      // Other published article update contracts remain independently guarded.
      return platform.capabilities?.publishedUpdate === true &&
        ["cnblogs", "devto", "toutiao"].includes(platform.id);
    }
    return platform.capabilities?.draftCreate === true;
  }

  // Emphasize title fragments shared with the selected local article.
  // Use DOM text nodes and <mark>, never HTML received from a platform.
  function appendHighlightedTitle(container, remoteTitle) {
    const text = String(remoteTitle || "(无标题)");
    const localTitle = String(root.BlogCTLDrafts?.selectedArticleTitle?.() || "");
    const tokens = [...new Set(
      localTitle.split(/[^\p{L}\p{N}]+/u).filter((part) => [...part].length >= 2)
    )].sort((a, b) => b.length - a.length);
    if (!tokens.length) {
      container.textContent = text;
      return;
    }
    const folded = text.toLocaleLowerCase();
    let cursor = 0;
    while (cursor < text.length) {
      let matchEnd = -1;
      for (const token of tokens) {
        const from = folded.indexOf(token.toLocaleLowerCase(), cursor);
        if (from === cursor && (matchEnd < cursor || token.length > matchEnd - cursor)) {
          matchEnd = cursor + token.length;
        }
      }
      if (matchEnd > cursor) {
        const mark = document.createElement("mark");
        mark.textContent = text.slice(cursor, matchEnd);
        container.append(mark);
        cursor = matchEnd;
      } else {
        const next = tokens.map((token) => folded.indexOf(token.toLocaleLowerCase(), cursor + 1))
          .filter((position) => position >= 0);
        const end = next.length ? Math.min(...next) : text.length;
        container.append(document.createTextNode(text.slice(cursor, end)));
        cursor = end;
      }
    }
  }

  function appendPlatformMatches(platform, container) {
    if (!state.article || !state.matches[platform.id]) return;
    const match = state.matches[platform.id];
    const section = document.createElement("div");
    section.className = "article-match";
    const summary = document.createElement("p");
    summary.className = "article-match-summary";
    summary.textContent = match.text || "检测完成";
    section.append(summary);

    for (const item of match.items || []) {
      if (!item?.id) continue;
      const row = document.createElement("div");
      row.className = "article-match-row";
      const target = {
        platform: platform.id, id: String(item.id),
        state: item.published ? "published" : "draft",
        url: String(item.url || ""),
        updatedAt: String(item.updatedAt || item.remoteUpdatedAt || ""),
      };
      const key = platform.id + ":" + target.id;
      const check = document.createElement("label");
      check.className = "article-match-choice";
      const box = document.createElement("input");
      box.type = "checkbox";
      box.dataset.platform = platform.id;
      box.dataset.postId = target.id;
      box.checked = state.selected.has(key);
      row.classList.toggle("is-selected", box.checked);
      box.disabled = !availableForUpdate(platform, item) ||
        root.BlogCTLDrafts?.isJobRunning?.();
      box.title = box.disabled ? "此状态暂不支持安全更新" : "选择此文章作为更新目标";
      box.addEventListener("change", () => {
        if (box.checked) state.selected.set(key, target);
        else state.selected.delete(key);
        row.classList.toggle("is-selected", box.checked);
        root.BlogCTLDrafts?.selectionChanged?.();
      });
      const identity = document.createElement("span");
      identity.className = "article-match-identity";
      const title = document.createElement("span");
      title.className = "article-match-choice-text";
      appendHighlightedTitle(title, item.title);
      const metadata = document.createElement("span");
      metadata.className = "article-match-meta";
      const stateLabel = document.createElement("span");
      stateLabel.className = item.published ? "article-match-state is-published" : "article-match-state is-draft";
      stateLabel.textContent = item.published ? "已发布" : "草稿";
      const idLabel = document.createElement("span");
      idLabel.textContent = "ID " + item.id;
      metadata.append(stateLabel, idLabel);
      identity.append(title, metadata);
      check.append(box, identity);
      row.append(check);
      const actions = document.createElement("div");
      actions.className = "article-match-actions";
      const link = BlogCTLSyncModel.articleMatchLink(platform.id, item);
      if (link) {
        const anchor = document.createElement("a");
        anchor.href = link.url;
        anchor.textContent = link.label;
        anchor.rel = "noopener noreferrer";
        anchor.target = "_blank";
        actions.append(anchor);
      }
      if (availableForUpdate(platform, item)) {
        const action = document.createElement("button");
        action.type = "button";
        action.className = "article-match-quick-update";
        action.textContent = "更新此文章";
        action.disabled = root.BlogCTLDrafts?.isJobRunning?.();
        action.addEventListener("click", () =>
          root.BlogCTLDrafts?.updateTargets?.([target], false));
        actions.append(action);
      }
      row.append(actions);
      section.append(row);
    }

    // Optional manual lookup remains for older remote records outside the list limit.
    // The ID is still verified by the server-side adapter when updating.
    const manual = document.createElement("details");
    const head = document.createElement("summary");
    head.textContent = "候选中没有？按远端 ID 指定目标";
    const idInput = document.createElement("input");
    idInput.placeholder = "远端文章或草稿 ID";
    const select = document.createElement("select");
    for (const value of ["draft", "published"]) {
      const option = document.createElement("option");
      option.value = value;
      option.textContent = value === "draft" ? "草稿" : "已发布";
      select.append(option);
    }
    const add = document.createElement("button");
    add.type = "button";
    add.className = "secondary compact";
    add.textContent = "添加目标";
    add.addEventListener("click", () => {
      const id = idInput.value.trim();
      if (!/^[A-Za-z0-9_-]{1,100}$/.test(id)) {
        BlogCTLPopup.setMessage(message, "远端 ID 格式不正确", "error"); return;
      }
      const target = { platform: platform.id, id, state: select.value, url: "" };
      if (!availableForUpdate(platform, { id, published: select.value === "published" })) {
        BlogCTLPopup.setMessage(message, "此平台不支持所选状态的安全更新", "error"); return;
      }
      state.selected.set(platform.id + ":" + id, target);
      render();
      root.BlogCTLDrafts?.selectionChanged?.();
    });
    manual.append(head, idInput, select, add);
    const additional = [...state.selected.values()].filter((item) =>
      item.platform === platform.id && !(match.items || []).some((row) => String(row.id) === item.id));
    for (const target of additional) {
      const row = document.createElement("div");
      row.className = "article-match-row";
      const caption = document.createElement("span");
      caption.textContent = `已选择：${target.state === "draft" ? "草稿" : "已发布"} · ID ${target.id}`;
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "secondary compact";
      remove.textContent = "移除目标";
      remove.addEventListener("click", () => {
        state.selected.delete(target.platform + ":" + target.id);
        render();
        root.BlogCTLDrafts?.selectionChanged?.();
      });
      row.append(caption, remove);
      section.append(row);
    }
    section.append(manual);
    container.append(section);
  }

  function init() {
    if (state.initialized) return;
    detectButton = document.getElementById("refreshArticleMatches");
    message = document.getElementById("syncMessage");
    detectButton.addEventListener("click", () => {
      if(root.BlogCTLDrafts?.currentMode?.()==="create"){
        root.BlogCTLDrafts.refreshCreateMatches();
      } else {
        refreshArticleMatches();
      }
    });
    document.addEventListener("blogctl:update-platform-selection", refreshToolbar);
    document.addEventListener("blogctl:detect-association", (event) => {
      if (!state.active || state.article !== event.detail?.article) return;
      refreshArticleMatches([event.detail?.platform]);
    });
    document.addEventListener("blogctl:article-selected", (event) => {
      const next = String(event.detail?.article || "");
      if (next === state.article) return;
      state.article = next;
      clearMatches();
    });
    state.initialized = true;
  }

  function activate() {
    state.active = true;
    const article = localStorage.getItem("blogctl.selectedArticle") || "";
    if (article !== state.article) {
      state.article = article;
      clearMatches();
    }
    refreshToolbar();
  }

  function deactivate() {
    state.active = false;
    // A pending response from a hidden Update panel is obsolete.
    ++state.generation;
    state.pending.clear();
  }
  function refresh() { refreshToolbar(); }
  root.BlogCTLSync = {
    init, activate, deactivate, refresh, refreshArticleMatches, ensureMatches,
    appendPlatformMatches, selectedTargets, clearMatches,
    isBindingBusy: () => state.pending.size > 0,
  };
})(globalThis);

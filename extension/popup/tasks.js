"use strict";

(function (root) {
  const state = {
    initialized: false,
    active: false,
    jobs: [],
    status: null,
    ui: BlogCTLTaskUIState.create(localStorage),
    lastRenderedSnapshot: "",
    renderDeferredForSelection: false,
    pollTimer: null,
    focusJobID: "",
  };

  let list, refreshButton, clearButton, message;

  function stateLabel(job) {
    if (job?.kind !== "search") return BlogCTLSyncModel.statePresentation(job.state);
    switch (String(job.state || "")) {
      case "completed": return { kind: "ok", label: "完成" };
      case "running": return { kind: "checking", label: "运行中" };
      case "queued": return { kind: "unknown", label: "排队中" };
      case "paused": return { kind: "unknown", label: "已暂停" };
      case "failed": return { kind: "error", label: "失败" };
      default: return { kind: "disabled", label: job.state || "未知" };
    }
  }

  function platformRows(job) {
    return BlogCTLSyncModel.platformRows(job, state.status);
  }

  async function publishExistingDraft(job, platform, artifact, button) {
    if (!job.article || !artifact.id || artifact.state !== "draft") return;
    if (!root.confirm(
      `确定发布「${job.article}」在 ${platform.label || platform.id} 的草稿（ID ${artifact.id}）？\n\n将直接调用该平台的发布接口，不再创建草稿，也不进入更新页。`
    )) return;
    button.disabled = true;
    BlogCTLPopup.setMessage(message, "正在启动草稿发布任务…");
    try {
      const response = await BlogCTLPopup.send("blogctl.job.start", {
        request: {
          article: job.article,
          platforms: [platform.id],
          operation: "publish-draft",
          targets: [{
            platform: platform.id,
            id: artifact.id,
            url: artifact.url,
            state: "draft",
          }],
          dryRun: false,
        },
      });
      if (!response.job?.id) throw new Error("发布任务没有返回任务 ID");
      state.ui.setJobExpanded(response.job.id, true);
      BlogCTLPopup.setMessage(message, "已创建发布任务；请在任务页查看发布结果。", "ok");
      await refresh();
      focusJob(response.job.id);
    } catch (error) {
      BlogCTLPopup.setMessage(message,
        "发布任务启动失败：" + BlogCTLPopup.errorMessage(error), "error");
      button.disabled = false;
    }
  }

  function renderPlatformResults(job, card) {
    const rows = platformRows(job);
    if (!rows.length) return;

    const container = document.createElement("div");
    container.className = "job-platform-results";
    for (const row of rows) {
      const item = document.createElement("div");
      item.className = "job-platform-result";

      const main = document.createElement("div");
      main.className = "job-platform-main";
      const name = document.createElement("strong");
      name.textContent = row.label;
      const status = document.createElement("span");
      BlogCTLPopup.setStatus(status, row.kind, row.statusLabel);
      main.append(name, status);
      item.append(main);

      const detailText = row.error || row.message;
      if (detailText) {
        const detail = document.createElement("small");
        detail.className = row.error ? "job-platform-message error-text" : "job-platform-message";
        detail.textContent = detailText;
        item.append(detail);
      }

      const artifacts = BlogCTLSyncModel.taskArtifactLinks(job, row.id);
      for (const artifact of artifacts) {
        const actions = document.createElement("div");
        actions.className = "task-artifact-actions";
        const link = document.createElement("a");
        link.className = "task-publication-link";
        link.href = artifact.url;
        link.target = "_blank";
        link.rel = "noreferrer noopener";
        link.textContent = artifact.label;
        if (artifact.id) link.title = "远端 ID " + artifact.id;
        actions.append(link);
        // Publish an already-created remote draft directly; editing is a
        // separate link. This shares the same backend PublishDraft capability
        // used by Create-and-Publish and Update-and-Publish.
        if (artifact.state === "draft" && artifact.id && job.article &&
            job.kind !== "search" && !job.dryRun &&
            BlogCTLSyncModel.isVisiblePlatform(row.id) &&
            row.capabilities?.explicitPublish === true) {
          const publish = document.createElement("button");
          publish.type = "button";
          publish.className = "task-publication-link";
          publish.textContent = "发布草稿";
          publish.title = "直接发布这个远端草稿，不需要先更新";
          publish.addEventListener("click", () =>
            publishExistingDraft(job, row, artifact, publish));
          actions.append(publish);
        }
        item.append(actions);
      }

      container.append(item);
    }
    card.append(container);
  }

  function renderSearchTask(job, card) {
    const progress = job.progress || {};
    const current = Number(progress.current || 0);
    const total = Number(progress.total || 0);
    const unit = String(progress.unit || "");
    const block = document.createElement("div");
    block.className = "job-platform-results";

    const row = document.createElement("div");
    row.className = "job-platform-result";
    const main = document.createElement("div");
    main.className = "job-platform-main";
    const name = document.createElement("strong");
    name.textContent = total > 0 ? `进度 ${current} / ${total}${unit ? " " + unit : ""}` : "进度";
    const status = document.createElement("span");
    const presentation = stateLabel(job);
    BlogCTLPopup.setStatus(status, presentation.kind, presentation.label);
    main.append(name, status);
    row.append(main);

    if (progress.message) {
      const message = document.createElement("small");
      message.className = "job-platform-message";
      message.textContent = progress.message;
      row.append(message);
    }
    block.append(row);

    const detailEntries = Object.entries(job.detail || {}).filter(([, value]) =>
      value !== "" && value !== null && value !== undefined);
    if (detailEntries.length) {
      const detail = document.createElement("div");
      detail.className = "job-meta";
      detail.textContent = detailEntries.map(([key, value]) => `${key}: ${value}`).join(" · ");
      block.append(detail);
    }
    card.append(block);
  }

  function renderDebugOutput(job, card) {
    if (!job.output) return;
    const details = document.createElement("details");
    details.className = "job-debug";
    BlogCTLTaskUIState.bindDetails(details, state.ui, "log", job.id);
    const summary = document.createElement("summary");
    summary.textContent = "详细日志";
    const output = document.createElement("pre");
    output.className = "job-output";
    output.textContent = job.output;
    details.append(summary, output);
    card.append(details);
  }

  function actionButton(label, className, handler) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = className;
    button.textContent = label;
    button.addEventListener("click", handler);
    return button;
  }

  async function retryJob(job, button) {
    button.disabled = true;
    BlogCTLPopup.setMessage(message, `正在重试任务 ${job.id}…`);
    try {
      const response = await BlogCTLPopup.send("blogctl.job.retry", { id: job.id });
      if (response.job?.id) state.ui.setJobExpanded(response.job.id, true);
      BlogCTLPopup.setMessage(message, "任务已重置并重新执行。", "ok");
      await refresh();
    } catch (error) {
      BlogCTLPopup.setMessage(message, BlogCTLPopup.errorMessage(error), "error");
      button.disabled = false;
    }
  }

  async function controlJob(job, action, button) {
    button.disabled = true;
    const labels = { pause: "暂停", resume: "继续", retry: "重试" };
    BlogCTLPopup.setMessage(message, `正在${labels[action] || action}任务 ${job.id}…`);
    try {
      const response = await BlogCTLPopup.send(`blogctl.job.${action}`, { id: job.id });
      if (response.job?.id) state.ui.setJobExpanded(response.job.id, true);
      BlogCTLPopup.setMessage(message, `任务已${labels[action] || action}。`, "ok");
      await refresh();
    } catch (error) {
      BlogCTLPopup.setMessage(message, BlogCTLPopup.errorMessage(error), "error");
      button.disabled = false;
    }
  }

  function renderActions(job, card) {
    if (job.kind === "search") {
      const actions = document.createElement("div");
      actions.className = "task-actions";
      if (job.canRetry) {
        let retry;
        retry = actionButton("重试", "secondary compact", () => controlJob(job, "retry", retry));
        actions.append(retry);
      }
      if (job.canPause) {
        let pause;
        pause = actionButton("暂停", "secondary compact", () => controlJob(job, "pause", pause));
        actions.append(pause);
      }
      if (job.canResume) {
        let resume;
        resume = actionButton("继续", "secondary compact", () => controlJob(job, "resume", resume));
        actions.append(resume);
      }
      if (actions.childElementCount) card.append(actions);
      return;
    }
    if (job.state !== "failed" || !job.canRetry) return;
    const actions = document.createElement("div");
    actions.className = "task-actions";
    let retry;
    retry = actionButton("重试", "secondary compact", () => retryJob(job, retry));
    actions.append(retry);
    card.append(actions);
  }

  function selectionTouchesNode(node, selection) {
    if (!node || !selection || selection.isCollapsed || selection.rangeCount === 0) return false;
    return node.contains(selection.anchorNode) || node.contains(selection.focusNode);
  }

  function renderJobs(snapshot) {
    const selection = typeof root.getSelection === "function" ? root.getSelection() : null;
    if (selectionTouchesNode(list, selection)) {
      state.renderDeferredForSelection = true;
      return false;
    }
    state.renderDeferredForSelection = false;

    BlogCTLTaskUIState.captureDetails(
      list.querySelectorAll("details.job-item[data-job-id]"),
      state.ui,
    );
    const validIDs = new Set(state.jobs.map((job) => job.id));
    state.ui.prune(validIDs);
    list.replaceChildren();
    clearButton.disabled = !state.jobs.some((job) => job.state !== "running");
    let focusedCard = null;

    if (!state.jobs.length) {
      list.innerHTML = '<div class="platform-loading">暂无任务</div>';
      state.lastRenderedSnapshot = snapshot;
      return true;
    }

    for (const job of state.jobs) {
      const card = document.createElement("details");
      card.className = "job-item";
      card.dataset.jobId = job.id;
      if (state.focusJobID === job.id) {
        state.ui.setJobExpanded(job.id, true);
        state.ui.setLogExpanded(job.id, true);
        card.classList.add("job-item-highlight");
        focusedCard = card;
      }
      BlogCTLTaskUIState.bindDetails(card, state.ui, "job", job.id);

      const summary = document.createElement("summary");
      const title = document.createElement("span");
      title.textContent = job.kind === "search"
        ? (job.title || job.type || "索引任务")
        : `${job.article} · ${(job.platforms ?? []).length} 个平台`;
      const status = document.createElement("strong");
      const presentation = stateLabel(job);
      BlogCTLPopup.setStatus(status, presentation.kind, presentation.label);
      summary.append(title, status);
      card.append(summary);

      const meta = document.createElement("div");
      meta.className = "job-meta";
      const started = BlogCTLPopup.formatTime(job.startedAt || job.createdAt);
      const finished = BlogCTLPopup.formatTime(job.finishedAt);
      meta.textContent = [
        started ? `开始 ${started}` : "",
        finished ? `结束 ${finished}` : "",
        job.kind === "search"
          ? (job.type || "索引")
          : job.operation === "publish" ? "发布"
            : job.operation === "create" ? "创建"
            : job.operation === "publish-draft" ? "发布草稿"
            : ["update", "update-published"].includes(job.operation) ? "更新"
            : job.operation ? "保存" : "",
        job.id ? `ID ${job.id}` : "",
      ].filter(Boolean).join(" · ");
      card.append(meta);

      if (job.kind === "search") renderSearchTask(job, card);
      else renderPlatformResults(job, card);
      if (job.error) {
        const error = document.createElement("pre");
        error.className = "job-output error-output";
        error.textContent = job.error;
        card.append(error);
      }
      renderDebugOutput(job, card);
      renderActions(job, card);
      list.append(card);
    }

    if (focusedCard) {
      const focusedID = state.focusJobID;
      state.focusJobID = "";
      requestAnimationFrame(() => {
        focusedCard.scrollIntoView({ behavior: "smooth", block: "center" });
        focusedCard.focus?.({ preventScroll: true });
        setTimeout(() => focusedCard.classList.remove("job-item-highlight"), 2200);
      });
      state.ui.setJobExpanded(focusedID, true);
      state.ui.setLogExpanded(focusedID, true);
    }

    state.lastRenderedSnapshot = snapshot;
    return true;
  }

  async function refresh(includeStatus = true) {
    if (!state.active) return;
    try {
      const jobsPromise = BlogCTLPopup.send("blogctl.jobs");
      const statusPromise = includeStatus || !state.status
        ? BlogCTLPopup.send("blogctl.status")
        : Promise.resolve(null);
      const [jobsResponse, statusResponse] = await Promise.all([jobsPromise, statusPromise]);
      state.jobs = jobsResponse.jobs ?? [];
      if (statusResponse) state.status = statusResponse.status;
      const snapshot = BlogCTLTaskUIState.renderSnapshot(state.jobs, state.status);
      if (!BlogCTLTaskUIState.shouldRender(state.lastRenderedSnapshot, snapshot, state.renderDeferredForSelection)) return;
      renderJobs(snapshot);
    } catch (error) {
      BlogCTLPopup.setMessage(message, `任务读取失败：${BlogCTLPopup.errorMessage(error)}`, "error");
    }
  }

  async function clearFinished() {
    clearButton.disabled = true;
    BlogCTLPopup.setMessage(message, "正在清理已结束任务…");
    try {
      const response = await BlogCTLPopup.send("blogctl.jobs.clear");
      state.jobs = response.jobs ?? [];
      state.ui.prune(new Set(state.jobs.map((job) => job.id)));
      const snapshot = BlogCTLTaskUIState.renderSnapshot(state.jobs, state.status);
      renderJobs(snapshot);
      BlogCTLPopup.setMessage(message, `已清理 ${response.removed || 0} 个任务。`, "ok");
    } catch (error) {
      BlogCTLPopup.setMessage(message, BlogCTLPopup.errorMessage(error), "error");
      clearButton.disabled = false;
    }
  }

  function init() {
    if (state.initialized) return;
    list = document.getElementById("taskList");
    refreshButton = document.getElementById("refreshTasks");
    clearButton = document.getElementById("clearFinishedTasks");
    message = document.getElementById("tasksMessage");
    refreshButton.addEventListener("click", () => refresh(true));
    clearButton.addEventListener("click", clearFinished);
    state.initialized = true;
  }

  function focusJob(id) {
    const jobID = String(id || "").trim();
    if (!jobID) return;
    state.focusJobID = jobID;
    state.ui.setJobExpanded(jobID, true);
    state.ui.setLogExpanded(jobID, true);
    if (state.active) refresh(true);
  }

  function activate() {
    state.active = true;
    refresh(true);
    if (!state.pollTimer) state.pollTimer = setInterval(() => refresh(false), 2000);
  }

  function deactivate() {
    state.active = false;
    if (state.pollTimer) {
      clearInterval(state.pollTimer);
      state.pollTimer = null;
    }
  }

  root.BlogCTLTasks = { init, activate, deactivate, refresh, focusJob };
})(globalThis);

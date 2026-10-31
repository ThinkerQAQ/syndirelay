"use strict";

const modules = {
  binding: BlogCTLInventory,
  drafts: BlogCTLDrafts,
  creation: BlogCTLDrafts,
  tasks: BlogCTLTasks,
  logs: BlogCTLLogs,
  social: BlogCTLSocial,
  indexing: BlogCTLIndexing,
  environment: BlogCTLEnvironment,
};

const ACTIVE_TAB_KEY = "blogctl.activeTab";
let activeTab = "binding";

function activateTab(name) {
  if (!modules[name]) return;

  document.querySelectorAll("[data-tab]").forEach((tab) => {
    const active = tab.dataset.tab === name;
    tab.classList.toggle("active", active);
    tab.setAttribute("aria-selected", active ? "true" : "false");
    tab.tabIndex = active ? 0 : -1;
  });

  document.querySelectorAll("[data-panel]").forEach((panel) => {
    panel.classList.toggle("active", panel.dataset.panel === name);
  });

  modules[activeTab]?.deactivate();
  if (activeTab === "drafts") BlogCTLSync.deactivate();
  if (name === "drafts" || name === "creation") {
    const workspace = document.getElementById("sharedDraftWorkspace");
    document.querySelector(`[data-panel="${name}"]`).append(workspace);
    BlogCTLDrafts.setMode(name === "creation" ? "create" : "update");
  }
  activeTab = name;
  localStorage.setItem(ACTIVE_TAB_KEY, name);
  modules[name].activate();
  if (name === "drafts") BlogCTLSync.activate();
}

async function refreshActiveTab() {
  BlogCTLPopup.refreshBridgeIndicator().catch(() => {});
  const module = modules[activeTab];
  if (typeof module?.refresh === "function") {
    await module.refresh();
    return;
  }
  module?.deactivate();
  module?.activate();
}

document.addEventListener("DOMContentLoaded", () => {
  Object.values(modules).forEach((module) => module.init());
  BlogCTLSettingsNavigation.init();
  BlogCTLSync.init();

  document.querySelectorAll("[data-tab]").forEach((tab) => {
    tab.addEventListener("click", () => activateTab(tab.dataset.tab));
  });

  document.getElementById("refresh").addEventListener("click", refreshActiveTab);
  if (BlogCTLTransport.host === "web") {
    document.getElementById("openWorkspace").hidden = true;
  }
  document.getElementById("openWorkspace").addEventListener("click", () => {
    const url = "http://127.0.0.1:32145/console/";
    if (BlogCTLTransport.host === "web") return;
    if (globalThis.chrome?.tabs?.create) chrome.tabs.create({ url });
    else window.open(url, "_blank", "noopener");
  });
  document.addEventListener("blogctl:navigate-task", (event) => {
    const jobId = String(event.detail?.jobId || "").trim();
    if (!jobId) return;
    BlogCTLTasks.focusJob(jobId);
    activateTab("tasks");
  });
  BlogCTLPopup.refreshBridgeIndicator().catch(() => {});
  const savedTab = localStorage.getItem(ACTIVE_TAB_KEY);
  const normalizedTab = savedTab === "sync"
    ? "binding"
    : savedTab === "publishing"
      ? "environment"
      : savedTab === "publications" ? "drafts" : savedTab;
  activateTab(modules[normalizedTab] ? normalizedTab : "binding");
});

window.addEventListener("unload", () => {
  Object.values(modules).forEach((module) => module.deactivate());
  BlogCTLSync.deactivate();
});

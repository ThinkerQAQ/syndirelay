"use strict";

(function (root) {
  function parseStringSet(raw) {
    if (!raw) return new Set();
    try {
      const value = JSON.parse(raw);
      return Array.isArray(value)
        ? new Set(value.filter((item) => typeof item === "string" && item))
        : new Set();
    } catch {
      return new Set();
    }
  }

  function create(storage, prefix = "blogctl.tasks") {
    const jobKey = `${prefix}.expandedJobs`;
    const logKey = `${prefix}.expandedLogs`;
    const expandedJobs = parseStringSet(storage?.getItem?.(jobKey));
    const expandedLogs = parseStringSet(storage?.getItem?.(logKey));

    function persist(key, values) {
      storage?.setItem?.(key, JSON.stringify([...values]));
    }

    function setExpanded(values, key, id, expanded) {
      if (!id) return;
      if (expanded) values.add(id);
      else values.delete(id);
      persist(key, values);
    }

    function prune(validIDs) {
      const valid = validIDs instanceof Set ? validIDs : new Set(validIDs || []);
      let jobsChanged = false;
      let logsChanged = false;
      for (const id of [...expandedJobs]) {
        if (!valid.has(id)) {
          expandedJobs.delete(id);
          jobsChanged = true;
        }
      }
      for (const id of [...expandedLogs]) {
        if (!valid.has(id)) {
          expandedLogs.delete(id);
          logsChanged = true;
        }
      }
      if (jobsChanged) persist(jobKey, expandedJobs);
      if (logsChanged) persist(logKey, expandedLogs);
    }

    return {
      isJobExpanded: (id) => expandedJobs.has(id),
      isLogExpanded: (id) => expandedLogs.has(id),
      setJobExpanded: (id, expanded) => setExpanded(expandedJobs, jobKey, id, expanded),
      setLogExpanded: (id, expanded) => setExpanded(expandedLogs, logKey, id, expanded),
      prune,
      snapshot() {
        return {
          expandedJobs: [...expandedJobs],
          expandedLogs: [...expandedLogs],
        };
      },
    };
  }

  function bindDetails(details, state, kind, id) {
    const isExpanded = kind === "log" ? state.isLogExpanded : state.isJobExpanded;
    const setExpanded = kind === "log" ? state.setLogExpanded : state.setJobExpanded;
    details.open = Boolean(isExpanded(id));
    details.addEventListener("toggle", () => setExpanded(id, Boolean(details.open)));
  }

  function captureDetails(cards, state) {
    for (const card of cards || []) {
      const id = card?.dataset?.jobId;
      if (!id) continue;
      state.setJobExpanded(id, Boolean(card.open));
      const log = card.querySelector?.("details.job-debug");
      if (log) state.setLogExpanded(id, Boolean(log.open));
    }
  }

  function shouldRender(lastRenderedSnapshot, nextSnapshot, renderDeferred = false) {
    return renderDeferred || lastRenderedSnapshot !== nextSnapshot;
  }

  function renderSnapshot(jobs, status) {
    const platforms = (status?.platforms ?? []).map((platform) => ({
      id: platform.id,
      label: platform.label,
    }));
    return JSON.stringify({ jobs: jobs ?? [], platforms });
  }

  root.BlogCTLTaskUIState = {
    create,
    parseStringSet,
    bindDetails,
    captureDetails,
    shouldRender,
    renderSnapshot,
  };
})(globalThis);

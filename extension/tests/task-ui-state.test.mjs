import fs from "node:fs/promises";
import assert from "node:assert/strict";
import test from "node:test";

await import("../popup/task-ui-state.js");
const model = globalThis.BlogCTLTaskUIState;

function fakeStorage(initial = {}) {
  const data = new Map(Object.entries(initial));
  return {
    getItem(key) { return data.has(key) ? data.get(key) : null; },
    setItem(key, value) { data.set(key, String(value)); },
    dump() { return Object.fromEntries(data); },
  };
}

test("task expansion and log expansion persist independently", () => {
  const storage = fakeStorage();
  const state = model.create(storage, "test.tasks");

  state.setJobExpanded("job-1", true);
  state.setLogExpanded("job-1", true);
  state.setJobExpanded("job-2", true);
  state.setLogExpanded("job-2", false);

  const reloaded = model.create(storage, "test.tasks");
  assert.equal(reloaded.isJobExpanded("job-1"), true);
  assert.equal(reloaded.isLogExpanded("job-1"), true);
  assert.equal(reloaded.isJobExpanded("job-2"), true);
  assert.equal(reloaded.isLogExpanded("job-2"), false);
});

test("prune removes expansion state only for jobs that no longer exist", () => {
  const storage = fakeStorage();
  const state = model.create(storage, "test.tasks");
  state.setJobExpanded("job-1", true);
  state.setLogExpanded("job-1", true);
  state.setJobExpanded("job-2", true);
  state.setLogExpanded("job-2", true);

  state.prune(new Set(["job-2"]));

  assert.equal(state.isJobExpanded("job-1"), false);
  assert.equal(state.isLogExpanded("job-1"), false);
  assert.equal(state.isJobExpanded("job-2"), true);
  assert.equal(state.isLogExpanded("job-2"), true);
});

test("render snapshot changes only when rendered task data or platform labels change", () => {
  const jobs = [{ id: "job-1", state: "running", output: "" }];
  const status = {
    platforms: [{ id: "juejin", label: "掘金", loggedIn: true }],
    sessions: { medium: { expiresInSeconds: 600 } },
  };

  const first = model.renderSnapshot(jobs, status);
  const sessionOnlyChange = model.renderSnapshot(jobs, {
    ...status,
    sessions: { medium: { expiresInSeconds: 599 } },
  });
  assert.equal(sessionOnlyChange, first);

  const updatedJob = model.renderSnapshot([{ ...jobs[0], state: "failed" }], status);
  assert.notEqual(updatedJob, first);

  const renamedPlatform = model.renderSnapshot(jobs, {
    ...status,
    platforms: [{ id: "juejin", label: "Juejin" }],
  });
  assert.notEqual(renamedPlatform, first);
});



test("captureDetails records the live DOM state before replacement", () => {
  const storage = fakeStorage();
  const state = model.create(storage, "test.tasks");
  const log = { open: true };
  const card = {
    open: true,
    dataset: { jobId: "job-1" },
    querySelector(selector) {
      assert.equal(selector, "details.job-debug");
      return log;
    },
  };

  model.captureDetails([card], state);

  const reloaded = model.create(storage, "test.tasks");
  assert.equal(reloaded.isJobExpanded("job-1"), true);
  assert.equal(reloaded.isLogExpanded("job-1"), true);
});

test("unchanged task snapshots do not request a DOM replacement", () => {
  const snapshot = model.renderSnapshot([{ id: "job-1", state: "running" }], {
    platforms: [{ id: "juejin", label: "掘金" }],
  });
  assert.equal(model.shouldRender(snapshot, snapshot, false), false);
  assert.equal(model.shouldRender(snapshot, snapshot, true), true);
  assert.equal(model.shouldRender(snapshot, snapshot + "changed", false), true);
});

test("invalid persisted state cannot force phantom expansion", () => {
  const storage = fakeStorage({
    "test.tasks.expandedJobs": "{broken",
    "test.tasks.expandedLogs": JSON.stringify([null, "", 42, "job-1"]),
  });
  const state = model.create(storage, "test.tasks");
  assert.equal(state.isJobExpanded("job-1"), false);
  assert.equal(state.isLogExpanded("job-1"), true);
});


test("bindDetails restores persisted state and records later toggles", () => {
  const storage = fakeStorage();
  const state = model.create(storage, "test.tasks");
  state.setJobExpanded("job-1", true);

  const listeners = new Map();
  const details = {
    open: false,
    addEventListener(name, handler) { listeners.set(name, handler); },
  };
  model.bindDetails(details, state, "job", "job-1");
  assert.equal(details.open, true);

  details.open = false;
  listeners.get("toggle")();
  const reloaded = model.create(storage, "test.tasks");
  assert.equal(reloaded.isJobExpanded("job-1"), false);
});

test("popup loads task state model before the task renderer", async () => {
  const html = await fs.readFile(new URL("../popup/popup.html", import.meta.url), "utf8");
  const stateIndex = html.indexOf('src="task-ui-state.js"');
  const tasksIndex = html.indexOf('src="tasks.js"');
  assert.ok(stateIndex >= 0, "task-ui-state.js is missing from popup");
  assert.ok(tasksIndex >= 0, "tasks.js is missing from popup");
  assert.ok(stateIndex < tasksIndex, "task state model must load before tasks.js");
});

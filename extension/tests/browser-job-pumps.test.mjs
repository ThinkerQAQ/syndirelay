import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { activeBrowserJobPumps } from "../browser-job-pumps.js";

test("read-only job metadata wakes only active browser transports", () => {
  assert.deepEqual(activeBrowserJobPumps(null), { toutiao: false, medium: false });
  assert.deepEqual(activeBrowserJobPumps([{ state: "completed", platforms: ["toutiao", "medium"] }, { state: "failed", platforms: ["toutiao"] }]), { toutiao: false, medium: false });
  assert.deepEqual(activeBrowserJobPumps([{ state: "running", platforms: ["toutiao"] }]), { toutiao: true, medium: false });
  assert.deepEqual(activeBrowserJobPumps([{ state: "queued", platforms: ["medium"] }, { state: "waiting", platforms: ["toutiao"] }]), { toutiao: true, medium: true });
  assert.deepEqual(activeBrowserJobPumps([{ state: "running", platforms: ["devto", "hashnode"] }]), { toutiao: false, medium: false });
  assert.deepEqual(activeBrowserJobPumps([null, {}, { state: "running", platforms: "toutiao" }]), { toutiao: false, medium: false });
});

test("MCP alarm actually checks the Bridge job list and wakes pumps without requiring opening tasks UI", async () => {
  const source = await readFile(new URL("../background.js", import.meta.url), "utf8");
  assert.match(source, /if \(alarm\.name === MCP_SESSION_ALARM\) void pollMCPSessionRefresh\(\)/u);
  const poll=source.match(/async function pollMCPSessionRefresh\(\) \{([\s\S]*?)\r?\n\}\r?\nchrome\.alarms\?/u)?.[1] ?? "";
  assert.match(poll, /fetchJSON\("\/v1\/jobs"\)/u);
  // Git on windows-latest checks out CRLF; test the same source there too.
  const crlf = source.replace(/\r?\n/g, "\r\n");
  const windowsPoll=crlf.match(/async function pollMCPSessionRefresh\(\) \{([\s\S]*?)\r?\n\}\r?\nchrome\.alarms\?/u)?.[1] ?? "";
  assert.match(windowsPoll, /fetchJSON\("\/v1\/jobs"\)/u);
  assert.match(poll, /wakeBrowserPumpsForJobs\(response\?\.jobs \?\? \[\]\)/u);
  assert.doesNotMatch(poll, /chrome\.tabs\.(create|update)|active:\s*true|window\.focus/u);
  const wake=source.match(/function wakeBrowserPumpsForJobs\(jobs\) \{([\s\S]*?)\n\}/u)?.[1] ?? "";
  assert.match(wake, /if \(active\.toutiao\) beginToutiaoRequestPump\(\)/u);
  assert.match(wake, /if \(active\.medium\) beginMediumUploadPump\(\)/u);
  assert.doesNotMatch(wake, /POST|\/v1\/sync\/jobs|publishAfter/u);
});

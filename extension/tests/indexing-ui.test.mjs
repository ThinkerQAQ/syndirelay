import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const cssPath = new URL("../popup/popup.css", import.meta.url);
const indexingPath = new URL("../popup/indexing.js", import.meta.url);

test("indexing buttons use black for enabled and gray for disabled", async () => {
  const [css, indexing] = await Promise.all([
    readFile(cssPath, "utf8"),
    readFile(indexingPath, "utf8"),
  ]);

  assert.match(css, /\.panel\[data-panel="indexing"\] button:not\(:disabled\).*background: #111827.*color: #fff/u);
  assert.match(css, /\.panel\[data-panel="indexing"\] button:disabled.*background: #e5e7eb.*color: #9ca3af/u);
  assert.doesNotMatch(indexing, /\["queued", "running", "quota_blocked"\]\.includes\(inspectionState\)/u);
  assert.match(indexing, /\["queued", "running"\]\.includes\(inspectionState\)/u);
  assert.match(indexing, /inspectionState === "quota_blocked"\s*\?\s*"重试检查"/u);
  assert.match(indexing, /\["paused", "quota_blocked"\]\.includes\(queueState\)/u);
});

test("task card buttons use black for enabled and gray for disabled", async () => {
  const css = await readFile(cssPath, "utf8");

  assert.match(css, /\.task-actions button:not\(:disabled\).*background: #111827.*color: #fff/u);
  assert.match(css, /\.task-actions button:disabled.*background: #e5e7eb.*color: #9ca3af/u);
});

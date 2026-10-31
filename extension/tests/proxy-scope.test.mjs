import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const manifestPath = new URL("../manifest.json", import.meta.url);
const backgroundPath = new URL("../background.js", import.meta.url);

test("BlogCTL proxy stays scoped to BlogCTL components", async () => {
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  const background = await readFile(backgroundPath, "utf8");

  assert.equal((manifest.permissions || []).includes("proxy"), false);
  assert.doesNotMatch(background, /chrome\.proxy/u);
  assert.doesNotMatch(background, /pac_script|fixed_servers/u);
});

test("extension edits environment configuration through the Bridge tools API", async () => {
  const background = await readFile(backgroundPath, "utf8");
  assert.match(background, /fetchJSON\("\/v1\/tools"\)/u);
  assert.match(background, /\/v1\/tools\/\$\{encodeURIComponent\(name\)\}/u);
  assert.doesNotMatch(background, /chrome\.proxy/u);
});

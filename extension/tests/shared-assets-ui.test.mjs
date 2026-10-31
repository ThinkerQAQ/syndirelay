import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const htmlPath = new URL("../popup/popup.html", import.meta.url);
const assetsPath = new URL("../popup/assets.js", import.meta.url);
const publishingPath = new URL("../popup/publishing.js", import.meta.url);
const environmentPath = new URL("../popup/environment.js", import.meta.url);

test("shared R2 configuration is outside platform-specific configuration", async () => {
  const html = await readFile(htmlPath, "utf8");

  const assetsCard = html.indexOf('id="assetConfigEnvironmentCard"');
  const platformCard = html.indexOf('id="platformConfigEnvironmentCard"');
  const r2Bucket = html.indexOf('id="r2Bucket"');
  const platformSelect = html.indexOf('id="publishingPlatform"');

  assert.ok(assetsCard >= 0);
  assert.ok(platformCard > assetsCard);
  assert.ok(r2Bucket > assetsCard && r2Bucket < platformCard);
  for (const id of ["r2AccessKeyId", "r2SecretAccessKey", "r2AccountId", "r2Endpoint"]) {
    const field = html.indexOf(`id="${id}"`);
    assert.ok(field > assetsCard && field < platformCard, `${id} must live in shared assets`);
  }
  assert.ok(platformSelect > platformCard);
  assert.match(html, /<script src="assets\.js"><\/script>[\s\S]*<script src="publishing\.js"><\/script>/u);
});

test("shared asset save and platform save are independent", async () => {
  const [assets, publishing] = await Promise.all([
    readFile(assetsPath, "utf8"),
    readFile(publishingPath, "utf8"),
  ]);

  assert.match(
    assets,
    /BlogCTLPopup\.send\("blogctl\.publishing\.save", \{\s*compiler: runtime\.compiler,\s*assets: runtime\.assets,\s*\}\)/u,
  );
  assert.doesNotMatch(assets, /platforms:\s*\[/u);

  assert.match(
    publishing,
    /BlogCTLPopup\.send\("blogctl\.publishing\.save", \{\s*platforms: \[current\],\s*\}\)/u,
  );
  assert.doesNotMatch(publishing, /compiler:\s*runtime\.compiler/u);
  assert.doesNotMatch(publishing, /assets:\s*runtime\.assets/u);
});

test("environment owns expansion lifecycle for the shared asset card", async () => {
  const environment = await readFile(environmentPath, "utf8");

  assert.match(environment, /BlogCTLAssets\.init\(\)/u);
  assert.match(environment, /trackExpansion\(assetConfigCard, "shared-assets"\)/u);
  assert.match(environment, /BlogCTLAssets\.activate\(\)/u);
  assert.match(environment, /BlogCTLAssets\.deactivate\(\)/u);
});

test("environment hides a health path when the configured path already renders it", async () => {
  const environment = await readFile(environmentPath, "utf8");
  assert.match(environment, /const configuredPaths = new Set/u);
  assert.match(environment, /!configuredPaths\.has\(String\(tool\.health\.path\)\.trim\(\)\)/u);
});

test("environment configuration cards expose edit and requirement affordances", async () => {
  const [html, environment] = await Promise.all([
    readFile(htmlPath, "utf8"),
    readFile(environmentPath, "utf8"),
  ]);

  assert.match(html, /id="editConfigFile"[^>]*>✎ 编辑</u);
  assert.match(environment, /BlogCTLPopup\.send\("blogctl\.config\.edit"\)/u);

  for (const [card, requirement] of [
    ["assetConfigEnvironmentCard", "optional"],
    ["platformConfigEnvironmentCard", "required"],
    ["socialConfigEnvironmentCard", "optional"],
    ["searchEngineEnvironmentCard", "optional"],
  ]) {
    const start = html.indexOf(`id="${card}"`);
    assert.ok(start >= 0, `${card} missing`);
    const fragment = html.slice(start, start + 700);
    assert.match(fragment, new RegExp(`tool-requirement ${requirement}`));
  }
});

test("R2 credentials are stored through BlogCTL config instead of environment variables", async () => {
  const assets = await readFile(assetsPath, "utf8");
  assert.match(assets, /accessKeyId: r2AccessKeyId\.value\.trim\(\)/u);
  assert.match(assets, /secretAccessKey: r2SecretAccessKey\.value\.trim\(\)/u);
  assert.doesNotMatch(assets, /R2_ACCESS_KEY_ID|R2_SECRET_ACCESS_KEY|R2_ACCOUNT_ID/u);
});


test("environment renders blogctl.toml as a dedicated config file surface", async () => {
  const [environment, html] = await Promise.all([
    readFile(environmentPath, "utf8"),
    readFile(htmlPath, "utf8"),
  ]);
  assert.match(html, /id="environmentConfigPath"/u);
  assert.match(html, />blogctl\.toml</u);
  assert.match(environment, /bridgeTool\?\.health\?\.path \|\| "未找到 blogctl\.toml"/u);
  assert.doesNotMatch(environment, /tool\.name === "bridge" \? `配置/u);
});

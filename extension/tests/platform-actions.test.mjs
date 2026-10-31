import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = async (path) => readFile(new URL("../" + path, import.meta.url), "utf8");

test("navigation includes Social publishing between Update and Index", async () => {
  const html = await source("popup/popup.html");
  const tabs = [...html.matchAll(/data-tab="([^"]+)"/gu)].map((item) => item[1]);
  assert.deepEqual(tabs, ["binding","creation","drafts","social","indexing","tasks","logs","environment"]);
  assert.doesNotMatch(html, /data-panel="publications"/u);
  assert.doesNotMatch(html, /id="publishSelected"/u);
  assert.match(html, /id="publishAfterSave"/u);
  assert.match(html, /id="sharedDraftWorkspace"/u);
});

test("Extension and Web Console use the same Feature DOM and transport contract", async () => {
  const [html, popup, transport, relay] = await Promise.all([
    source("popup/popup.html"), source("popup/popup.js"),
    source("popup/transport.js"), source("console-relay.js"),
  ]);
  assert.match(popup, /document\.getElementById\("sharedDraftWorkspace"\)/u);
  assert.match(popup, /BlogCTLDrafts\.setMode/u);
  assert.match(html, /src="transport\.js"/u);
  assert.match(transport, /blogctl:console:request:v1/u);
  assert.match(transport, /http:\/\/127\.0\.0\.1:32145/u);
  assert.match(relay, /location\.pathname\.startsWith\("\/console\/"\)/u);
  assert.doesNotMatch(relay, /x-thinkerqaq-token|Cookie:/u);
});

test("the same Settings Catalog and controls mount in both UI hosts", async () => {
  const [settings, popup, html] = await Promise.all([
    source("popup/settings-navigation.js"), source("popup/popup.js"), source("popup/popup.html"),
  ]);
  assert.match(settings, /CATEGORY_META/u);
  assert.match(settings, /#platformConfigEnvironmentCard/u);
  assert.match(settings, /#socialConfigEnvironmentCard/u);
  assert.match(settings, /#assetConfigEnvironmentCard/u);
  assert.match(settings, /#searchEngineEnvironmentCard/u);
  assert.match(settings, /section\.append\(element\)/u);
  assert.match(popup, /BlogCTLSettingsNavigation\.init\(\)/u);
  assert.match(html, /id="environmentRuntimeTools"/u);
});

test("Update sends selected remote IDs directly and never silently creates", async () => {
  const [drafts, sync, html] = await Promise.all([
    source("popup/drafts.js"), source("popup/sync.js"), source("popup/popup.html"),
  ]);
  assert.match(sync, /selectedTargets\(\)/u);
  assert.match(drafts, /operation:state\.mode/u);
  assert.match(drafts, /targets:state\.mode==="create"\?\[\]:targets/u);
  assert.match(drafts, /请选择先检测|请先检测并勾选至少一篇远端文章/u);
  assert.doesNotMatch(drafts, /usePlatformChangedOnly|operation:"draft"/u);
  assert.doesNotMatch(html, /id="bindSelectedMatches"|id="unbindSelectedMatches"/u);
  assert.match(sync, /textContent = "更新此文章"/u);
});

test("Creation is a separate explicit task and exposes create or create-and-publish", async () => {
  const drafts = await source("popup/drafts.js");
  assert.match(drafts, /state\.mode==="create"\?"创建草稿":"更新所选"/u);
  assert.match(drafts, /"创建并发布":"更新并发布"/u);
  assert.match(drafts, /if\(state\.mode==="update"\)validateTargets\(targets\)/u);
  assert.match(drafts, /!window\.confirm/u);
});

test("Medium and Toutiao are visible with their backend capability controls", async () => {
  const [background, model] = await Promise.all([source("background.js"),source("popup/sync-model.js")]);
  assert.match(model, /const HIDDEN_PLATFORMS = new Set\(\)/u);
  assert.match(background, /platform === "toutiao"/u);
  assert.match(background, /case "blogctl\.job\.start":/u);
  assert.doesNotMatch(background, /cto51PublishInBrowser|ensureToutiaoEditorTab/u);
});

test("Medium published targets can be updated as saved editor revisions without an automatic publish action", async () => {
  const [sync, drafts] = await Promise.all([source("popup/sync.js"), source("popup/drafts.js")]);
  assert.match(sync, /platform\.id === "medium"/u);
  assert.match(sync, /platform\.capabilities\?\.publishedDraftEdit === true/u);
  assert.match(drafts, /candidateTargets\(\)\.every\(\(target\) => target\.state==="draft"\)/u);
  assert.match(drafts, /publishButton\.disabled=!canSubmit \|\| !canPublishAfter\(\)/u);
});

test("Medium remote inventory is enabled through the existing read-only endpoint", async () => {
  const [background, inventory, model] = await Promise.all([
    source("background.js"), source("popup/inventory.js"), source("popup/sync-model.js"),
  ]);
  const handler = background.match(/case "blogctl\.remote\.inventory": \{([\s\S]*?)case "blogctl\.article\.match":/u)?.[1];
  assert.ok(handler, "remote inventory handler missing");
  assert.match(handler, /"medium"/u, "Medium must be allowed in inventory requests");
  assert.match(handler, /syncPlatformSession\(platform\)/u);
  assert.match(handler, /\/v1\/remote\/inventory/u);
  assert.match(inventory, /blogctl\.remote\.inventory/u);
  assert.match(model, /medium: \["medium\.com"\]/u);
  assert.match(model, /case "medium":/u);
});

test("Hashnode detection must query the remote draft and published inventory, not only local bindings", async () => {
  const [background, drafts] = await Promise.all([source("background.js"), source("popup/drafts.js")]);
  assert.match(background, /\/v1\/hashnode\/articles\/list\?article=/u);
  assert.match(background, /\/v1\/remote\/inventory/u);
  assert.doesNotMatch(background, /仅检查了本地关联，未穷尽查询 Hashnode/u);
  assert.match(drafts, /existing\[0\]\.bound===true/u);
});

test("Medium upload is delegated to the browser and never sends session credentials through Go", async () => {
  const [background, pageUpload, manifest] = await Promise.all([
    source("background.js"), source("medium-upload.js"), source("manifest.json"),
  ]);
  assert.match(background, /world: "MAIN"/u);
  assert.match(background, /\/v1\/medium\/browser-upload\/next/u);
  assert.match(background, /\/v1\/medium\/browser-upload\/result/u);
  assert.match(pageUpload, /credentials: "same-origin"/u);
  assert.ok(pageUpload.includes("/_/upload?source=6"));
  assert.match(pageUpload, /location\.origin !== "https:\/\/medium\.com"/u);
  assert.ok(JSON.parse(manifest).permissions.includes("alarms"));
});

test("Medium job polling and retries wake browser uploads", async () => {
  const background = await source("background.js");
  assert.match(background, /case "blogctl.job.get"/u);
  assert.match(background, /beginMediumUploadPump\(\)/u);
  assert.match(background, /medium browser upload pump failed/u);
});

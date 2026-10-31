import assert from "node:assert/strict";
import {readFile} from "node:fs/promises";
import test from "node:test";

const html=await readFile(new URL("../popup/popup.html",import.meta.url),"utf8");
const app=await readFile(new URL("../popup/social.js",import.meta.url),"utf8");
const bg=await readFile(new URL("../background.js",import.meta.url),"utf8");
const env=await readFile(new URL("../popup/environment.js",import.meta.url),"utf8");

test("social panel shares side panel and web console UI",()=>{
 assert.match(html,/data-tab="social"/u);
 assert.match(html,/data-panel="social"/u);
 assert.match(html,/<script src="social\.js"><\/script>/u);
 for(const id of ["socialDraftPicker","socialPlatform","socialText","socialStage","socialPublish"]) {
  assert.match(html,new RegExp(`id="${id}"`));
 }
 assert.doesNotMatch(html,/id="socialApprove"/u);
 assert.match(app,/root\.BlogCTLSocial=api/u);
 assert.doesNotMatch(app,/innerHTML/u);
});
test("staging confirms review; publication retains a separate confirmation",()=>{
 assert.match(app,/action==="publish"/u);
 assert.match(app,/confirm\("/u);
 assert.match(app,/revision:draft\.revision/u);
 assert.match(app,/action==="stage"/u);
 assert.match(bg,/blogctl\.social\.stage/u);
 assert.match(bg,/blogctl\.social\.publish/u);
});
test("Buffer secret is displayed through existing masked config UI",()=>{
 assert.match(env,/socialPlatformAccess/u);
 assert.match(html,/id="socialPlatformAccessConfig"/u);
});

test("Social settings is a standalone category, not duplicated under article distribution",async()=>{
 const nav=await readFile(new URL("../popup/settings-navigation.js",import.meta.url),"utf8");
 const html=await readFile(new URL("../popup/popup.html",import.meta.url),"utf8");
 const start=html.indexOf('id="socialConfigEnvironmentCard"');
 const end=html.indexOf('id="searchEngineEnvironmentCard"');
 assert.ok(start>0 && end>start);
 const card=html.slice(start,end);
 assert.match(nav,/id: "social", title: "社交平台"[^\n]*#socialConfigEnvironmentCard/u);
 assert.match(card,/id="socialPlatformAccessConfig"/u);
 assert.match(card,/id="socialChannelsRefresh"/u);
 assert.match(card,/id="socialChannelsList"/u);
 const longForm=html.slice(html.indexOf('id="platformConfigEnvironmentCard"'),start);
 assert.doesNotMatch(longForm,/id="socialPlatformAccessConfig"/u);
 assert.match(bg,/case "blogctl\.social\.channels":/u);
 assert.match(env,/state\.tools\.filter\(\(tool\) => tool\.kind === "social"\)/u);
 assert.match(env,/BlogCTLPopup\.send\("blogctl\.social\.channels"\)/u);
});

test("failed publication exposes read-only reconcile before an explicit retry",()=>{
  assert.match(html,/id="socialReconcile"/u);
  assert.match(app,/action==="reconcile"/u);
  assert.match(app,/remote\?\.error/u);
  assert.match(bg,/case "blogctl\.social\.reconcile":/u);
});

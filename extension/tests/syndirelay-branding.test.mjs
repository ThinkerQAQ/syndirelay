import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const read = (name) => fs.readFileSync(path.join(root, name), "utf8");

test("runtime tool card uses SyndiRelay identity in both languages", () => {
  const background = read("extension/background.js");
  const catalog = read("extension/popup/i18n-messages.js");
  const descriptors = read("bridge/control.go");
  assert.match(background, /displayName: "SyndiRelay Extension"/u);
  assert.match(background, /版本必须与 SyndiRelay Bridge 一致/u);
  assert.doesNotMatch(background, /displayName: "BlogCTL Extension"/u);
  assert.match(descriptors, /DisplayName: "SyndiRelay Bridge"/u);
  assert.doesNotMatch(descriptors, /Description: "[^"]*BlogCTL/u);
  assert.doesNotMatch(catalog, /"[^"]*BlogCTL (?:Bridge|Extension|Network Proxy)[^"]*"/u);
});

test("settings show canonical config name without changing legacy command protocol", () => {
  const html = read("extension/popup/popup.html");
  const environment = read("extension/popup/environment.js");
  const messages = read("extension/popup/i18n-messages.js");
  assert.match(html, />syndirelay\.toml</u);
  assert.doesNotMatch(html, />blogctl\.toml</u);
  assert.match(environment, /未找到 syndirelay\.toml/u);
  assert.match(messages, /"正在读取 syndirelay\.toml…"/u);
  // Keep stable internal message identifiers for old extension and console hosts.
  assert.match(environment, /"blogctl\.config\.edit"/u);
});

test("standalone renderer paths no longer refer to nested tools/blogctl", () => {
  for (const name of ["assets/assets.go", "compiler/compile.go", "app/sync.go", "bridge/image_upload.go"]) {
    const source = read(name);
    assert.doesNotMatch(source, /"tools", "blogctl", "renderers"/u, name);
    assert.match(source, /"renderers", "node"/u, name);
  }
});

test("UI rebrand retains module exports used by the existing popup runtime", () => {
  const popup = read("extension/popup/popup.js");
  const indexing = read("extension/popup/indexing.js");
  const settings = read("extension/popup/settings-navigation.js");
  assert.match(popup, /indexing: BlogCTLIndexing/u);
  assert.match(indexing, /root\.BlogCTLIndexing =/u);
  assert.match(settings, /root\.BlogCTLMCPConnection\?\.refresh\(\)/u);
  assert.doesNotMatch(indexing, /root\.SyndiRelayIndexing =/u);
});

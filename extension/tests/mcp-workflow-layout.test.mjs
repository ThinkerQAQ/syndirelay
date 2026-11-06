import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const dir = path.dirname(fileURLToPath(import.meta.url));
const read = name => fs.readFileSync(path.join(dir, '..', 'popup', name), 'utf8');
const html = read('popup.html');
const css = read('popup.css');
const js = read('mcp-connection.js');

test('MCP connection follows environment, Tunnel, then ChatGPT workflow', () => {
  const section = html.match(/<section id="mcpConnectionSettings"[\s\S]+?<\/section>\s*(?=<div id="environmentRuntimeTools")/)?.[0];
  assert.ok(section, 'MCP connection section is present');
  const environment = section.indexOf('mcpEnvironmentTitle');
  const local = section.indexOf('mcpLocalTitle');
  const chatgpt = section.indexOf('mcpPluginTitle');
  assert.ok(environment > 0 && local > environment && chatgpt > local, 'environment precedes local Tunnel and ChatGPT');
  assert.match(section, /id="mcpClientDownload"/);
  assert.match(section, /id="mcpClientPath"/);
  assert.match(section, /id="mcpClientPathSave"/);
  assert.match(section, /id="mcpClientPathReset"/);
  assert.match(section, /<span class="mcp-step" aria-hidden="true">03<\/span>/);
  assert.match(section, /mcp-workflow-header/);
  assert.match(section, /mcp-status-chip/);
  assert.match(section, /mcp-section-heading/);
  assert.match(section, /mcp-step/);
  assert.match(section, /mcp-flow-body/);
  assert.match(section, /mcp-plugin-link/);
  assert.match(section, /aria-live="polite"/);
});
test('MCP form uses compact independent controls and a left-aligned startup preference', () => {
  assert.match(html, /class="mcp-preference"/);
  assert.match(html, /class="mcp-button-row"/);
  assert.match(html, /id="mcpKeyReveal"/);
  assert.match(html, /id="mcpRuntimeKey"[^>]*type="password"/);
  assert.match(html, /id="mcpAutoStart"[^>]*type="checkbox"/);
  assert.match(css, /#mcpConnectionSettings \.mcp-preference input\[type="checkbox"\]/);
  assert.match(css, /#mcpConnectionSettings \.mcp-button-row button/);
  assert.match(css, /#mcpConnectionSettings \.mcp-flow-body/);
  assert.doesNotMatch(css, /mcp-setup-actions button \{ flex: 1 1/);
});
test('MCP status and plugin text follow actual runtime state', () => {
  assert.match(js, /mcpConnectionStatusText/);
  assert.match(js, /mcpPluginDescription/);
  assert.match(js, /mcpKeyReveal/);
  assert.match(js, /data-kind/);
  assert.match(css, /mcp-status-chip\[data-kind="ready"\]/);
});

test('MCP connection appears immediately below runtime environment in Settings', () => {
  const nav = fs.readFileSync(path.join(dir, '../popup/settings-navigation.js'), 'utf8');
  const runtime = nav.indexOf('id: "runtime"');
  const mcp = nav.indexOf('id: "mcp"');
  const platforms = nav.indexOf('id: "platforms"');
  assert.ok(runtime >= 0 && runtime < mcp && mcp < platforms);
  assert.match(nav, /id: "mcp", parent: "runtime"/);
  assert.match(nav, /entry\.className = "tool-card settings-runtime-link"/);
  assert.match(nav, /nodes\.get\("runtime"\)\.prepend\(entry\)/);
  assert.doesNotMatch(nav, /tree\.append\(entry\)/);
  assert.match(nav, /selected = meta\?\.parent \|\| ""/);
  assert.match(css, /\.settings-category-pane \.settings-runtime-link \{/);
});

test('Tunnel supervisor controls and automatic connection are independent', () => {
  assert.match(html, /id="mcpAutoStart"/);
  assert.match(html, /本地服务开机自启/);
  assert.match(html, /id="mcpAutoConnect"/);
  assert.match(html, /Tunnel 自动连接/);
  assert.match(html, /id="mcpTunnelToggle"/);
  assert.match(html, /id="mcpSave"[^>]*>保存<\/button>/);
  assert.match(css, /\.mcp-primary \{\s*background: #25364e/);
  assert.match(js, /auto-connect-enable/);
  assert.match(js, /auto-connect-disable/);
  assert.match(js, /action:connecting\?'disconnect':'connect'/);
});

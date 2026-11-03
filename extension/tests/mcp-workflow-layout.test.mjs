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

test('MCP connection follows the two-step browser connection workflow', () => {
  const section = html.match(/<section id="mcpConnectionSettings"[\s\S]+?<\/section>\s*(?=<div id="environmentRuntimeTools")/)?.[0];
  assert.ok(section, 'MCP connection section is present');
  const local = section.indexOf('mcpLocalTitle');
  const chatgpt = section.indexOf('mcpPluginTitle');
  assert.ok(local > 0 && chatgpt > local, 'local Tunnel precedes ChatGPT plugin');
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

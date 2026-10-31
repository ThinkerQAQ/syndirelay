import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("MCP browser session refresh reuses the existing background session function without taking focus", async () => {
  const source = await readFile(new URL("../background.js", import.meta.url), "utf8");
  assert.match(source, /MCP_SESSION_ALARM\s*=\s*"blogctl-mcp-session-refresh"/u);
  assert.match(source, /chrome\.alarms\?\.create\(MCP_SESSION_ALARM, \{ periodInMinutes: 0\.5 \}\)/u);
  assert.match(source, /fetchJSON\("\/v1\/mcp\/session-refresh"\)/u);
  assert.match(source, /await syncPlatformSession\(platform\)/u);
  assert.match(source, /if \(alarm\.name === MCP_SESSION_ALARM\) void pollMCPSessionRefresh\(\)/u);
  const refresh = source.match(/async function pollMCPSessionRefresh\(\) \{([\s\S]*?)\n\}/u)?.[1] ?? "";
  assert.doesNotMatch(refresh, /chrome\.tabs\.(create|update)|active:\s*true|window\.focus/u);
});

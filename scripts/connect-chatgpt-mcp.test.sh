#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
ROOT="$(mktemp -d)"
trap 'rm -rf "$ROOT"' EXIT
mkdir -p "$ROOT/bin" "$ROOT/config"
cat > "$ROOT/bin/tunnel-client" <<'TOOL'
#!/bin/sh
printf '%s\n' "$*" >> "$SYNDIRELAY_TEST_RECORD"
case "$*" in
  *'runtimes status'*) echo '{"alias":"syndirelay","healthy":true,"ready":true}';;
  *) echo '{"status":"connected"}';;
esac
TOOL
chmod +x "$ROOT/bin/tunnel-client"
export SYNDIRELAY_TEST_RECORD="$ROOT/calls"
export TUNNEL_CLIENT_BIN="$ROOT/bin/tunnel-client"
export SYNDIRELAY_TUNNEL_PROFILE_DIR="$ROOT/config"
export SYNDIRELAY_MCP_COMMAND='/private/syndirelay mcp-stdio'
unset CONTROL_PLANE_API_KEY || true
unset SYNDIRELAY_RUNTIME_KEY_REF || true
if "$SCRIPT_DIR/connect-chatgpt-mcp.sh" tunnel_test123 >"$ROOT/out" 2>&1; then echo 'Missing authorization should fail' >&2; exit 1; fi
[[ ! -e "$ROOT/calls" ]]
export CONTROL_PLANE_API_KEY='offline-fixture-not-a-real-secret'
"$SCRIPT_DIR/connect-chatgpt-mcp.sh" tunnel_test123 >"$ROOT/out"
grep -q -- '--alias syndirelay' "$ROOT/calls"
grep -q -- '--tunnel-id tunnel_test123' "$ROOT/calls"
grep -q -- '--runtime-api-key env:CONTROL_PLANE_API_KEY' "$ROOT/calls"
grep -q -- '--mcp-command /private/syndirelay mcp-stdio' "$ROOT/calls"
[[ "$(wc -l < "$ROOT/calls")" == 2 ]]
if "$SCRIPT_DIR/connect-chatgpt-mcp.sh" tunnel_invalid! >"$ROOT/out" 2>&1; then echo 'Invalid tunnel ID was accepted' >&2; exit 1; fi
"$SCRIPT_DIR/connect-chatgpt-mcp.sh" tunnel_test123 --status >"$ROOT/out"
grep -q '"healthy":true' "$ROOT/out"
echo 'SyndiRelay Tunnel setup tests passed'

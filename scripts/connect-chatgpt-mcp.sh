#!/usr/bin/env bash
# Connect a dedicated, local SyndiRelay STDIO MCP process to a user-owned
# OpenAI tunnel. Does not read old BlogCTL profiles or start a public Bridge.
set -euo pipefail

usage() {
  cat <<'HELP'
Usage: scripts/connect-chatgpt-mcp.sh tunnel_<id> [--status]

Preconditions:
  - tunnel-client in PATH (or set TUNNEL_CLIENT_BIN)
  - a separate, authorized OpenAI Tunnel ID
  - An existing authorized runtime credential in CONTROL_PLANE_API_KEY,
    or SYNDIRELAY_RUNTIME_KEY_REF=env:VARIABLE / file:/private/path.
    A new or one-time key is not required for each project.
  - Native Linux/macOS `syndirelay` executable on PATH;
    override SYNDIRELAY_MCP_COMMAND for an explicit native path.
  - Windows users should run connect-chatgpt-mcp.ps1 natively instead.

The script installs/reuses only the alias `syndirelay` and never touches BlogCTL.
It does not create a remote Tunnel or a ChatGPT connector automatically.
HELP
}

[[ $# -ge 1 && $# -le 2 ]] || { usage; exit 2; }
TUNNEL_ID="$1"
[[ "$TUNNEL_ID" =~ ^tunnel_[a-zA-Z0-9]+$ ]] || { echo 'Invalid tunnel ID' >&2; exit 2; }
MODE="${2:-connect}"
[[ "$MODE" == connect || "$MODE" == --status ]] || { usage; exit 2; }
TC="${TUNNEL_CLIENT_BIN:-tunnel-client}"
command -v "$TC" >/dev/null 2>&1 || { echo 'tunnel-client is not installed' >&2; exit 1; }
if [[ "$MODE" == --status ]]; then
  exec "$TC" runtimes status syndirelay --json
fi

PROFILE_DIR="${SYNDIRELAY_TUNNEL_PROFILE_DIR:-$HOME/.config/tunnel-client}"
KEY_ARGS=()
KEY_REF="${SYNDIRELAY_RUNTIME_KEY_REF:-}"
if [[ -z "$KEY_REF" && -n "${CONTROL_PLANE_API_KEY:-}" ]]; then
  KEY_REF='env:CONTROL_PLANE_API_KEY'
fi
if [[ -z "$KEY_REF" ]]; then
  echo 'Provide an existing authorized Tunnel runtime credential through CONTROL_PLANE_API_KEY or SYNDIRELAY_RUNTIME_KEY_REF; no new per-project key is required.' >&2
  exit 1
fi
case "$KEY_REF" in
  env:*)
    KEY_ENV="${KEY_REF#env:}"
    [[ "$KEY_ENV" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || { echo 'Invalid credential environment variable' >&2; exit 2; }
    [[ -n "${!KEY_ENV:-}" ]] || { echo 'Selected runtime credential environment variable is empty' >&2; exit 1; }
    ;;
  file:*)
    KEY_FILE="${KEY_REF#file:}"
    [[ "$KEY_FILE" = /* && -f "$KEY_FILE" && ! -L "$KEY_FILE" && -s "$KEY_FILE" ]] || { echo 'Credential must be a nonempty regular absolute file' >&2; exit 1; }
    KEY_MODE="$(stat -c '%a' "$KEY_FILE" 2>/dev/null || stat -f '%Lp' "$KEY_FILE")"
    [[ "$KEY_MODE" == 600 ]] || { echo 'Runtime credential file must have mode 0600' >&2; exit 1; }
    ;;
  *) echo 'Use env:NAME or file:/absolute/path, not a literal secret' >&2; exit 2;;
esac
KEY_ARGS=(--runtime-api-key "$KEY_REF")

if [[ -n "${SYNDIRELAY_MCP_COMMAND:-}" ]]; then
  MCP_COMMAND="$SYNDIRELAY_MCP_COMMAND"
elif command -v syndirelay >/dev/null 2>&1; then
  MCP_COMMAND="$(command -v syndirelay) mcp-stdio"
else
  echo 'SyndiRelay executable not found; set SYNDIRELAY_MCP_COMMAND' >&2; exit 1
fi
if [[ "$MCP_COMMAND" == *blogctl* || "$MCP_COMMAND" != *syndirelay* ]]; then
  echo 'Refusing a legacy/foreign MCP executable; use SyndiRelay only' >&2; exit 2
fi

mkdir -p "$PROFILE_DIR"
chmod 700 "$PROFILE_DIR"
"$TC" runtimes connect \
  --alias syndirelay \
  --name SyndiRelay \
  --description 'SyndiRelay local MCP: article reading, previews and authorized publication' \
  --tunnel-id "$TUNNEL_ID" \
  "${KEY_ARGS[@]}" \
  --profile syndirelay \
  --profile-dir "$PROFILE_DIR" \
  --mcp-command "$MCP_COMMAND" \
  --json

"$TC" runtimes status syndirelay --json

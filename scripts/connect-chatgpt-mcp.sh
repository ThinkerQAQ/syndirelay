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
  - a private runtime API key file, 0600, at:
      ~/.config/tunnel-client/syndirelay-runtime-key
    or set SYNDIRELAY_RUNTIME_KEY_FILE to an absolute file path.
  - the SyndiRelay native executable on the same OS as the signed-in browser;
    override SYNDIRELAY_MCP_COMMAND if not automatically detected.

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
KEY_FILE="${SYNDIRELAY_RUNTIME_KEY_FILE:-$PROFILE_DIR/syndirelay-runtime-key}"
[[ "$KEY_FILE" = /* ]] || { echo 'Runtime key file must be an absolute path' >&2; exit 2; }
[[ -f "$KEY_FILE" && ! -L "$KEY_FILE" && -s "$KEY_FILE" ]] || {
  echo "Missing private runtime API key file: $KEY_FILE" >&2
  echo 'Create your own runtime key in OpenAI Platform and save it locally; do not paste it into ChatGPT.' >&2
  exit 1
}
# GNU stat on Linux/WSL; BSD stat on macOS.
KEY_MODE="$(stat -c '%a' "$KEY_FILE" 2>/dev/null || stat -f '%Lp' "$KEY_FILE")"
if [[ "$KEY_MODE" != 600 ]]; then
  echo 'Runtime key file must have mode 0600' >&2; exit 1
fi

if [[ -n "${SYNDIRELAY_MCP_COMMAND:-}" ]]; then
  MCP_COMMAND="$SYNDIRELAY_MCP_COMMAND"
elif [[ -x /mnt/c/software/Coding/SyndiRelay/syndirelay-windows-amd64.exe ]]; then
  MCP_COMMAND='/mnt/c/software/Coding/SyndiRelay/syndirelay-windows-amd64.exe mcp-stdio'
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
  --runtime-api-key "file:$KEY_FILE" \
  --profile syndirelay \
  --profile-dir "$PROFILE_DIR" \
  --mcp-command "$MCP_COMMAND" \
  --json

"$TC" runtimes status syndirelay --json

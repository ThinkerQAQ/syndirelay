#!/usr/bin/env bash
set -euo pipefail
umask 077
root="$(cd "$(dirname "$0")/.." && pwd)"
os="$(uname -s)"
app="${SYNDIRELAY_MCP_BINARY:-$root/bin/syndirelay}"
[[ -x "$app" ]] || { echo 'Installed SyndiRelay binary missing' >&2; exit 1; }
client="$(command -v tunnel-client || true)"
[[ -n "$client" ]] || client="$HOME/.local/bin/tunnel-client"
[[ -x "$client" ]] || { echo 'Install official openai/tunnel-client first' >&2; exit 1; }
echo 'Tunnel ID: https://platform.openai.com/settings/organization/tunnels'
echo 'Runtime API Key: https://platform.openai.com/api-keys'
echo 'Select the intended organization; create a restricted key with Tunnels Read + Use.'
id="${SYNDIRELAY_TUNNEL_ID:-}"
[[ -n "$id" ]] || read -r -p 'SyndiRelay Tunnel ID: ' id
[[ "$id" =~ ^tunnel_([a-z0-9]{32}|[a-z0-9]{4}_[a-z0-9]{32})$ ]] || { echo 'Invalid Tunnel ID' >&2; exit 1; }
private="${XDG_CONFIG_HOME:-$HOME/.config}/syndirelay/mcp"
profile_dir="${XDG_CONFIG_HOME:-$HOME/.config}/tunnel-client"
mkdir -p "$private" "$profile_dir"
chmod 700 "$private"
env_file="$private/runtime.env"
profile="$profile_dir/syndirelay.yaml"
old_id=""
if [[ -f "$profile" ]]; then
  old_id="$(sed -nE 's/^[[:space:]]*tunnel_id:[[:space:]]*"?((tunnel_[a-z0-9_]+))"?.*/\1/p' "$profile" | head -n1)"
  [[ "$old_id" =~ ^tunnel_([a-z0-9]{32}|[a-z0-9]{4}_[a-z0-9]{32})$ ]] || { echo 'Invalid saved profile' >&2; exit 1; }
  [[ "${SYNDIRELAY_REPLACE_TUNNEL:-}" == 1 && "${SYNDIRELAY_EXPECTED_TUNNEL_ID:-}" == "$old_id" ]] || {
    echo 'Changing connection credentials requires confirmation with the latest saved ID.' >&2; exit 1;
  }
elif [[ "${SYNDIRELAY_REPLACE_TUNNEL:-}" == 1 ]]; then
  echo 'No existing connection to replace' >&2; exit 1
fi
key=""
if [[ "${SYNDIRELAY_MCP_KEY_STDIN:-}" == 1 ]]; then
  IFS= read -r key
elif [[ "${SYNDIRELAY_MCP_TEST_MODE:-}" == 1 ]]; then
  key="${CONTROL_PLANE_API_KEY:-}"
fi
if [[ -z "$key" && "${SYNDIRELAY_MCP_REUSE_KEY:-}" == 1 && -s "$env_file" ]]; then
  key="$(sed -n 's/^CONTROL_PLANE_API_KEY=//p' "$env_file" | head -n1)"
fi
if [[ -z "$key" && "${SYNDIRELAY_MCP_KEY_STDIN:-}" != 1 ]]; then
  echo 'Enter the key for this connection; the saved key is not reused automatically.'
  read -r -s -p 'Runtime API Key (hidden input): ' key
  printf '\n'
fi
[[ "$key" =~ ^[a-zA-Z0-9_-]{16,}$ ]] || { echo 'Runtime API Key is required' >&2; exit 1; }
staging="$(mktemp -d "$private/pending.XXXXXXXX")"
had_profile=false; had_key=false; committing=false; completed=false
[[ ! -f "$profile" ]] || { cp -p "$profile" "$staging/old.yaml"; had_profile=true; }
[[ ! -f "$env_file" ]] || { cp -p "$env_file" "$staging/old.env"; had_key=true; }
stop_own() {
  if [[ "$os" == Linux ]]; then
    systemctl --user stop syndirelay-tunnel.service >/dev/null 2>&1 || true
  else
    launchctl bootout "gui/$(id -u)/com.syndirelay.mcp" >/dev/null 2>&1 || true
  fi
}
restore_on_exit() {
  local status=$?
  trap - EXIT
  if [[ "$completed" != true && "$committing" == true ]]; then
    if [[ "${SYNDIRELAY_MCP_SAVE_ONLY:-}" != 1 ]]; then stop_own; fi
    if [[ "$had_profile" == true ]]; then cp -p "$staging/old.yaml" "$profile"; else rm -f "$profile"; fi
    if [[ "$had_key" == true ]]; then cp -p "$staging/old.env" "$env_file"; else rm -f "$env_file"; fi
    if [[ "${SYNDIRELAY_MCP_SAVE_ONLY:-}" != 1 && "$had_profile" == true && "$had_key" == true ]]; then
      if [[ "$os" == Linux ]]; then systemctl --user start syndirelay-tunnel.service >/dev/null 2>&1 || true
      else launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.syndirelay.mcp.plist" >/dev/null 2>&1 || true; fi
    fi
    echo 'Previous Tunnel ID and Runtime API Key restored.' >&2
  fi
  rm -rf "$staging"
  unset CONTROL_PLANE_API_KEY key
  return "$status"
}
trap restore_on_exit EXIT
printf 'CONTROL_PLANE_API_KEY=%s\n' "$key" > "$staging/runtime.env"
export CONTROL_PLANE_API_KEY="$key"
unset key
"$client" init --sample sample_mcp_stdio_local --profile-dir "$staging" --profile syndirelay \
  --tunnel-id "$id" --mcp-command "\"$app\" mcp-stdio" --health-listen-addr 127.0.0.1:0
"$client" doctor --config "$staging/syndirelay.yaml"
if [[ "${SYNDIRELAY_MCP_SAVE_ONLY:-}" != 1 ]]; then stop_own; fi
committing=true
cp "$staging/syndirelay.yaml" "$profile"
cp "$staging/runtime.env" "$env_file"
chmod 600 "$profile" "$env_file"
if [[ "${SYNDIRELAY_MCP_SAVE_ONLY:-}" == 1 ]]; then
  printf '%s\n' "$(basename "$staging")" > "$private/configuration-revision"
  printf '%s\n' true > "$private/restart-required"
  completed=true
  echo 'MCP configuration saved; running service and login tasks unchanged.'
  exit 0
fi
rm -f "$private/health.url"
if [[ "$os" == "Linux" ]] && command -v systemctl >/dev/null && systemctl --user is-system-running >/dev/null 2>&1; then
  unit="$HOME/.config/systemd/user/syndirelay-tunnel.service"
  mkdir -p "$(dirname "$unit")"
  cat > "$unit" <<EOF
[Unit]
Description=SyndiRelay MCP Tunnel
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
EnvironmentFile="$env_file"
ExecStart="$client" run --config "$profile" --health.url-file "$private/health.url"
Restart=on-failure
RestartSec=5
UMask=0077
[Install]
WantedBy=default.target
EOF
  systemctl --user daemon-reload
  systemctl --user start syndirelay-tunnel.service
  echo "SyndiRelay MCP tunnel started as a systemd user service."
elif [[ "$os" == "Darwin" ]]; then
  runner="$private/run.sh"
  cat > "$runner" <<EOF
#!/bin/bash
set -a
source "$env_file"
set +a
exec "$client" run --config "$profile" --health.url-file "$private/health.url"
EOF
  chmod 700 "$runner"
  runner_xml="$(printf "%s" "$runner" | sed -e "s/\&/\&amp;/g" -e "s/</\&lt;/g" -e "s/>/\&gt;/g")"
  agent="$HOME/Library/LaunchAgents/com.syndirelay.mcp.plist"
  mkdir -p "$(dirname "$agent")"
  cat > "$agent" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.syndirelay.mcp</string>
<key>ProgramArguments</key><array><string>$runner_xml</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
</dict></plist>
EOF
  launchctl bootout "gui/$(id -u)/com.syndirelay.mcp" >/dev/null 2>&1 || true
  if launchctl bootstrap "gui/$(id -u)" "$agent"; then
    echo "SyndiRelay MCP tunnel started through LaunchAgent."
  else
    echo "No GUI launchctl session. Start manually: $client run --config $profile" >&2
  fi
else
  echo "No supported service manager. Start manually: $client run --config $profile"
fi
healthy=false
health_attempts="${SYNDIRELAY_MCP_HEALTH_ATTEMPTS:-35}"
[[ "$health_attempts" =~ ^[1-9][0-9]?$ ]] || health_attempts=35
for _ in $(seq 1 "$health_attempts"); do
  sleep 1
  if "$client" health --url-file "$private/health.url" --require-control-plane-poll --json >/dev/null 2>&1; then
    healthy=true; break
  fi
done
[[ "$healthy" == true ]] || { echo 'New connection failed live polling verification.' >&2; exit 1; }
printf '%s\n' "$(basename "$staging")" > "$private/configuration-revision"
rm -f "$private/restart-required"
completed=true
echo 'Tunnel ID and Runtime API Key verified. Continue in ChatGPT Plugins.'

#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")" && pwd)"
"$root/bin/syndirelay" install
echo "SyndiRelay Native Messaging Host registered."
echo "Load unpacked extension from $root/extension in Chrome or Edge."
if command -v xdg-open >/dev/null 2>&1; then
  xdg-open "$root/extension" >/dev/null 2>&1 || true
elif command -v open >/dev/null 2>&1; then
  open "$root/extension" || true
fi

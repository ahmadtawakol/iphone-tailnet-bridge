#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
BIN="$ROOT/.build/iphone-tailnet-bridge"
USER_ID="$(id -u)"

if [[ -x "$BIN" ]]; then
    "$BIN" uninstall
elif [[ -x "$HOME/.local/bin/iphone-tailnet-bridge" ]]; then
    "$HOME/.local/bin/iphone-tailnet-bridge" uninstall
fi

/bin/launchctl bootout "gui/$USER_ID/local.iphone-tailnet-bridge" 2>/dev/null || true
rm -f "$HOME/Library/LaunchAgents/local.iphone-tailnet-bridge.plist"

if /usr/bin/awk '$3 == "#" && $4 == "iphone-tailnet-bridge" { found=1 } END { exit !found }' /etc/hosts; then
    echo "Removing the original prototype's tagged /etc/hosts entry requires your macOS password."
    /usr/bin/sudo /bin/bash "$ROOT/hosts.sh" remove
fi

echo "Bridge services removed. Private device profiles were preserved."

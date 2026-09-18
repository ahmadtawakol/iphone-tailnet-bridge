#!/bin/bash
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
LABEL=local.iphone-tailnet-bridge
STATE="$HOME/Library/Application Support/iphone-tailnet-bridge"
UID_VALUE="$(id -u)"
/bin/launchctl bootout "gui/$UID_VALUE/$LABEL" 2>/dev/null || true
if [[ -f "$STATE/lock/pid" ]]; then
    pid="$(cat "$STATE/lock/pid")"
    if [[ "$pid" =~ ^[0-9]+$ ]] && /bin/ps -p "$pid" -o command= | /usr/bin/grep -Fq "$DIR/bridge.sh"; then
        kill -TERM "$pid"
        for ((i=0; i<15; i++)); do
            kill -0 "$pid" 2>/dev/null || break
            sleep 1
        done
        if kill -0 "$pid" 2>/dev/null; then echo 'Bridge has not stopped; inspect it before retrying.' >&2; exit 1; fi
    fi
    rm -f "$STATE/lock/pid"
    rmdir "$STATE/lock" 2>/dev/null || true
fi
rm -f "$HOME/Library/LaunchAgents/$LABEL.plist"
/usr/bin/sudo /bin/bash "$DIR/hosts.sh" remove
echo 'Bridge stopped, Bonjour registrations withdrawn, and hosts mapping removed.'

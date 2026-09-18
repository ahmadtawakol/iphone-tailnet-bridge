#!/bin/bash
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
LABEL=local.iphone-tailnet-bridge
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
UID_VALUE="$(id -u)"
[[ -f "$DIR/bridge.local.conf" ]] || { echo "Create bridge.local.conf from bridge.local.conf.example first." >&2; exit 1; }
mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs/iphone-tailnet-bridge"
/bin/bash "$DIR/bridge.sh" --prepare-hosts
cp "$DIR/$LABEL.plist.in" "$PLIST"
/usr/libexec/PlistBuddy -c "Set :ProgramArguments:1 $DIR/bridge.sh" "$PLIST"
/usr/libexec/PlistBuddy -c "Set :StandardOutPath $HOME/Library/Logs/iphone-tailnet-bridge/output.log" "$PLIST"
/usr/libexec/PlistBuddy -c "Set :StandardErrorPath $HOME/Library/Logs/iphone-tailnet-bridge/error.log" "$PLIST"
/usr/bin/plutil -lint "$PLIST"
/bin/launchctl bootout "gui/$UID_VALUE/$LABEL" 2>/dev/null || true
/bin/launchctl bootstrap "gui/$UID_VALUE" "$PLIST"
echo "Installed $LABEL. Logs: ~/Library/Logs/iphone-tailnet-bridge/"
